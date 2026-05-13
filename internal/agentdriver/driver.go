package agentdriver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
)

// Driver orchestrates one Session through one turn at a time. A Driver is
// safe to reuse across many Drive calls but does NOT support concurrent
// turns on the same Session; the agentcore type system rejects that.
type Driver struct {
	// Provider streams the LLM. Required.
	Provider Provider
	// Tools authorizes and executes tool invocations. Required.
	Tools ToolDispatcher
	// Sink receives every event the driver wants journaled and/or broadcast.
	// Required.
	Sink EventSink
	// Permissions hosts pending permission resolutions. Required only when
	// Tools.Authorize ever returns AuthorisationRequiresPrompt.
	Permissions *PermissionGate

	// IDGen produces unique IDs prefixed by kind ("turn", "part", "perm").
	// Tests inject a deterministic counter; production wires uuid.
	IDGen func(kind string) string
	// Now returns the current time. Tests inject a fixed-clock function.
	Now func() time.Time
}

// Drive runs one full turn against the given session: appends a user
// message, streams the assistant's response, dispatches tools, requests
// permissions when needed, and emits turn.completed (or turn.failed) at
// the end. Returns the updated Session value reconstructed from the events
// the driver emitted; consumers may also load the same state from the
// Store after Drive returns.
//
// Drive is the only mutation entry point. It does not export the
// individual phases of the turn loop because the caller has no need to
// mix them with other state.
func (d *Driver) Drive(ctx context.Context, session agentcore.Session, userText string) (agentcore.Session, error) {
	if err := d.validate(); err != nil {
		return session, err
	}
	return d.DriveTurn(ctx, session, agentcore.TurnID(d.IDGen("turn")), userText)
}

// DriveTurn is Drive with a caller-supplied turn ID. Used by callers that
// need to know the turn ID before the goroutine starts (e.g. to match
// turn.cancel requests against the active turn). The caller is responsible
// for ensuring turnID is unique; passing a duplicate yields an
// ErrTurnAlreadyRunning from agentcore.StartTurn.
func (d *Driver) DriveTurn(ctx context.Context, session agentcore.Session, turnID agentcore.TurnID, userText string) (agentcore.Session, error) {
	if err := d.validate(); err != nil {
		return session, err
	}
	now := d.Now()

	// 1. Open the turn with the user's text as the first part.
	// Validate the in-memory transition BEFORE journaling turn.started so a
	// session that already replays to a Running turn (e.g. after a server
	// restart while the in-memory dispatcher map is empty) rejects the new
	// submit without writing an unreplayable second turn.started event.
	userPart := agentcore.NewTextPart(agentcore.PartID(d.IDGen("part")), now, userText)
	updated, err := agentcore.StartTurn(session, turnID, agentcore.TurnRoleUser, userPart, now)
	if err != nil {
		return session, fmt.Errorf("StartTurn: %w", err)
	}
	if err := d.emitTurnStarted(session.ID, turnID, userPart, now); err != nil {
		return session, err
	}

	// 2. Stream the provider until we see done or error.
	finalSession, err := d.streamTurn(ctx, updated, turnID, userText)
	if err != nil {
		return finalSession, err
	}
	return finalSession, nil
}

// streamTurn consumes the provider channel, emits events, dispatches
// tools, and finishes with turn.completed or turn.failed.
func (d *Driver) streamTurn(ctx context.Context, session agentcore.Session, turnID agentcore.TurnID, userInput string) (agentcore.Session, error) {
	events, err := d.Provider.Generate(ctx, session.Model, session.History, userInput)
	if err != nil {
		return d.failTurn(session, turnID, fmt.Errorf("provider Generate: %w", err))
	}

	var textBuf strings.Builder
	var reasoningBuf strings.Builder
	// The streaming protocol identifies an accumulating part by part_id.
	// Allocate the id ONCE per buffer (first delta), reuse it for every
	// subsequent delta in the same logical part, then reuse it again for
	// the materialized part on flush. Reset to empty after flushing so the
	// next chunk gets a fresh id.
	var textPartID, reasoningPartID agentcore.PartID

	flushText := func(s agentcore.Session) (agentcore.Session, error) {
		if textBuf.Len() == 0 {
			return s, nil
		}
		text := textBuf.String()
		textBuf.Reset()
		partID := textPartID
		textPartID = ""
		return d.emitToolPart(s, turnID, partID, text, agentcore.PartKindText)
	}
	flushReasoning := func(s agentcore.Session) (agentcore.Session, error) {
		if reasoningBuf.Len() == 0 {
			return s, nil
		}
		text := reasoningBuf.String()
		reasoningBuf.Reset()
		partID := reasoningPartID
		reasoningPartID = ""
		return d.emitToolPart(s, turnID, partID, text, agentcore.PartKindReasoning)
	}

	for {
		select {
		case <-ctx.Done():
			return d.failTurn(session, turnID, ctx.Err())
		case ev, open := <-events:
			if !open {
				// A provider following the documented cancellation contract
				// closes its event channel when ctx is canceled. Both
				// ctx.Done() and this !open receive can be ready at the same
				// time; Go's select picks one at random. If the close branch
				// wins we would emit turn.completed for a turn that was
				// actually canceled. Check ctx.Err() first.
				if cerr := ctx.Err(); cerr != nil {
					return d.failTurn(session, turnID, cerr)
				}
				session, err = flushText(session)
				if err != nil {
					return d.failTurn(session, turnID, fmt.Errorf("flush text: %w", err))
				}
				session, err = flushReasoning(session)
				if err != nil {
					return d.failTurn(session, turnID, fmt.Errorf("flush reasoning: %w", err))
				}
				return d.completeTurn(session, turnID)
			}
			switch e := ev.(type) {
			case ProviderTextDelta:
				if textPartID == "" {
					textPartID = agentcore.PartID(d.IDGen("part"))
				}
				textBuf.WriteString(e.Delta)
				d.broadcastTextDelta(session.ID, turnID, textPartID, e.Delta)
			case ProviderReasoningDelta:
				if reasoningPartID == "" {
					reasoningPartID = agentcore.PartID(d.IDGen("part"))
				}
				reasoningBuf.WriteString(e.Delta)
				d.broadcastReasoningDelta(session.ID, turnID, reasoningPartID, e.Delta)
			case ProviderToolCall:
				session, err = flushText(session)
				if err != nil {
					return d.failTurn(session, turnID, fmt.Errorf("flush text: %w", err))
				}
				session, err = flushReasoning(session)
				if err != nil {
					return d.failTurn(session, turnID, fmt.Errorf("flush reasoning: %w", err))
				}
				session, err = d.handleToolCall(ctx, session, turnID, e)
				if err != nil {
					// handleToolCall has already journaled events (tool_use.started,
					// possibly permission.requested/resolved) and any error here
					// leaves the turn in Running state. Route through failTurn so
					// the journal gets a terminal event and replay does not block
					// the next submit. A bare return here would drop the
					// dispatcher's in-memory running entry while the journal still
					// shows Running, requiring manual repair.
					return d.failTurn(session, turnID, fmt.Errorf("tool call: %w", err))
				}
			case ProviderTurnDone:
				// Both ctx.Done() and a buffered ProviderTurnDone can be
				// ready in the same select cycle (e.g. an operator cancels
				// while handleToolCall was running and the provider had
				// already queued its done event). If this branch wins the
				// race we would journal turn.completed for a canceled
				// turn. Mirror the closed-channel guard above.
				if cerr := ctx.Err(); cerr != nil {
					return d.failTurn(session, turnID, cerr)
				}
				session, err = flushText(session)
				if err != nil {
					return d.failTurn(session, turnID, fmt.Errorf("flush text: %w", err))
				}
				session, err = flushReasoning(session)
				if err != nil {
					return d.failTurn(session, turnID, fmt.Errorf("flush reasoning: %w", err))
				}
				return d.completeTurn(session, turnID)
			case ProviderError:
				// Provider contract (provider.go: ProviderError) says the
				// driver flushes pending text/reasoning before failing.
				// Without flushing, partial output that operators already
				// saw over SSE is dropped from the journal — replay and
				// GET /session/{id} reconstruct a Session that disagrees
				// with the live stream.
				session, err = flushText(session)
				if err != nil {
					return d.failTurn(session, turnID, fmt.Errorf("flush text: %w", err))
				}
				session, err = flushReasoning(session)
				if err != nil {
					return d.failTurn(session, turnID, fmt.Errorf("flush reasoning: %w", err))
				}
				return d.failTurn(session, turnID, e.Err)
			default:
				return d.failTurn(session, turnID, fmt.Errorf("unknown provider event %T", ev))
			}
		}
	}
}

func (d *Driver) handleToolCall(ctx context.Context, session agentcore.Session, turnID agentcore.TurnID, call ProviderToolCall) (agentcore.Session, error) {
	now := d.Now()
	startedPart := agentcore.NewToolUsePart(agentcore.PartID(d.IDGen("part")), now, call.CallID, call.Name, call.Args)
	startedEvent := agentproto.PartToolUseStartedEvent{}
	startedEvent.SessionID = session.ID
	startedEvent.At = now
	startedEvent.TurnID = turnID
	startedEvent.PartID = startedPart.ID()
	startedEvent.ToolCallID = call.CallID
	startedEvent.ToolName = call.Name
	startedEvent.Args = call.Args
	if err := d.Sink.Publish(startedEvent); err != nil {
		return session, err
	}
	session, err := agentcore.AppendPart(session, turnID, startedPart, now)
	if err != nil {
		return session, fmt.Errorf("append tool_use: %w", err)
	}

	verdict := d.Tools.Authorize(ctx, call.Name, call.Args)
	switch verdict {
	case AuthorisationGranted:
		// fall through to Run.
	case AuthorisationRequiresPrompt:
		decision, reason, err := d.gatePermission(ctx, session.ID, turnID, call)
		if err != nil {
			return session, err
		}
		// Only the exact "approved" decision authorizes execution. A typo,
		// malformed body, or new-but-unknown value MUST be treated as denial
		// — otherwise a client posting `decision: "approve"` (or any other
		// non-"denied" string) would silently bypass the gate.
		if decision != agentcore.PermissionApproved {
			if reason == "" {
				reason = "permission not approved"
			}
			return d.appendDeniedToolResult(session, turnID, call, reason)
		}
	case AuthorisationDenied:
		return d.appendDeniedToolResult(session, turnID, call, "denied by tool policy")
	}

	content, isError, err := d.Tools.Run(ctx, call.Name, call.Args)
	if err != nil {
		isError = true
		content = err.Error()
	}
	resultNow := d.Now()
	resultPart := agentcore.NewToolResultPart(agentcore.PartID(d.IDGen("part")), resultNow, call.CallID, call.Name, content, isError)
	resultEvent := agentproto.PartToolUseCompletedEvent{}
	resultEvent.SessionID = session.ID
	resultEvent.At = resultNow
	resultEvent.TurnID = turnID
	resultEvent.PartID = resultPart.ID()
	resultEvent.ToolCallID = call.CallID
	resultEvent.ToolName = call.Name
	resultEvent.Content = content
	resultEvent.IsError = isError
	if err := d.Sink.Publish(resultEvent); err != nil {
		return session, err
	}
	session, err = agentcore.AppendPart(session, turnID, resultPart, resultNow)
	if err != nil {
		return session, fmt.Errorf("append tool_result: %w", err)
	}
	return session, nil
}

func (d *Driver) gatePermission(ctx context.Context, sessionID agentcore.SessionID, turnID agentcore.TurnID, call ProviderToolCall) (agentcore.PermissionDecision, string, error) {
	if d.Permissions == nil {
		return "", "", errors.New("agentdriver: tool requires permission but no PermissionGate configured")
	}
	id := agentcore.PermissionID(d.IDGen("perm"))
	now := d.Now()
	// Register the pending entry BEFORE advertising the request. The
	// permission.requested event is broadcast over SSE on Publish; a fast
	// operator could POST /permission/{id} before the gate has an entry,
	// causing Resolve to return ErrUnknownPermission.
	ch := d.Permissions.Open(id)
	requested := agentproto.PermissionRequestedEvent{}
	requested.SessionID = sessionID
	requested.At = now
	requested.TurnID = turnID
	requested.PermissionID = id
	requested.ToolCallID = call.CallID
	requested.ToolName = call.Name
	requested.Args = call.Args
	if err := d.Sink.Publish(requested); err != nil {
		d.Permissions.Discard(id)
		return "", "", err
	}
	decision, reason, err := d.Permissions.Wait(ctx, id, ch)
	if err != nil {
		// Wait removed the in-memory gate entry already, but the durable
		// journal still has permission.requested without a matching
		// permission.resolved. On reload the session would replay with the
		// permission stuck Pending while any late POST /permission/{id}
		// would 404 against the empty gate. Journal a denial so replay
		// and the live gate agree: the permission is decided, just not by
		// the operator. Best effort — the original cancel/error is still
		// the cause we propagate. PermissionPending is rejected by
		// ResolvePermission, so use PermissionDenied.
		canceled := agentproto.PermissionResolvedEvent{}
		canceled.SessionID = sessionID
		canceled.At = d.Now()
		canceled.TurnID = turnID
		canceled.PermissionID = id
		canceled.Decision = agentcore.PermissionDenied
		canceled.Reason = "turn canceled before operator responded: " + err.Error()
		_ = d.Sink.Publish(canceled)
		return "", "", err
	}
	resolved := agentproto.PermissionResolvedEvent{}
	resolved.SessionID = sessionID
	resolved.At = d.Now()
	resolved.TurnID = turnID
	resolved.PermissionID = id
	resolved.Decision = decision
	resolved.Reason = reason
	if err := d.Sink.Publish(resolved); err != nil {
		return "", "", err
	}
	return decision, reason, nil
}

func (d *Driver) appendDeniedToolResult(session agentcore.Session, turnID agentcore.TurnID, call ProviderToolCall, reason string) (agentcore.Session, error) {
	now := d.Now()
	body := "tool denied: " + reason
	resultPart := agentcore.NewToolResultPart(agentcore.PartID(d.IDGen("part")), now, call.CallID, call.Name, body, true)
	resultEvent := agentproto.PartToolUseCompletedEvent{}
	resultEvent.SessionID = session.ID
	resultEvent.At = now
	resultEvent.TurnID = turnID
	resultEvent.PartID = resultPart.ID()
	resultEvent.ToolCallID = call.CallID
	resultEvent.ToolName = call.Name
	resultEvent.Content = body
	resultEvent.IsError = true
	if err := d.Sink.Publish(resultEvent); err != nil {
		return session, err
	}
	return agentcore.AppendPart(session, turnID, resultPart, now)
}

func (d *Driver) emitTurnStarted(sessionID agentcore.SessionID, turnID agentcore.TurnID, firstPart agentcore.Part, now time.Time) error {
	payload, err := agentproto.EncodePart(firstPart)
	if err != nil {
		return fmt.Errorf("encode first part: %w", err)
	}
	ev := agentproto.TurnStartedEvent{}
	ev.SessionID = sessionID
	ev.At = now
	ev.TurnID = turnID
	ev.Role = agentcore.TurnRoleUser
	ev.FirstPart = payload
	return d.Sink.Publish(ev)
}

func (d *Driver) emitToolPart(session agentcore.Session, turnID agentcore.TurnID, partID agentcore.PartID, content string, kind agentcore.PartKind) (agentcore.Session, error) {
	now := d.Now()
	if partID == "" {
		partID = agentcore.PartID(d.IDGen("part"))
	}
	switch kind {
	case agentcore.PartKindText:
		// Journal the finalized text part BEFORE applying it locally so a
		// crash between publish and AppendPart does not lose the assistant
		// response: on reload, replay reconstructs the same Session value
		// Drive would have returned. Deltas remain wire-only; the
		// completed event is the canonical record.
		ev := agentproto.PartTextCompletedEvent{}
		ev.SessionID = session.ID
		ev.At = now
		ev.TurnID = turnID
		ev.PartID = partID
		ev.Text = content
		if err := d.Sink.Publish(ev); err != nil {
			return session, err
		}
		part := agentcore.NewTextPart(partID, now, content)
		return agentcore.AppendPart(session, turnID, part, now)
	case agentcore.PartKindReasoning:
		ev := agentproto.PartReasoningCompletedEvent{}
		ev.SessionID = session.ID
		ev.At = now
		ev.TurnID = turnID
		ev.PartID = partID
		ev.Text = content
		if err := d.Sink.Publish(ev); err != nil {
			return session, err
		}
		part := agentcore.NewReasoningPart(partID, now, content)
		return agentcore.AppendPart(session, turnID, part, now)
	}
	return session, fmt.Errorf("emitToolPart: unsupported kind %s", kind)
}

func (d *Driver) broadcastTextDelta(sessionID agentcore.SessionID, turnID agentcore.TurnID, partID agentcore.PartID, delta string) {
	ev := agentproto.PartTextDeltaEvent{}
	ev.SessionID = sessionID
	ev.At = d.Now()
	ev.TurnID = turnID
	ev.PartID = partID
	ev.Delta = delta
	_ = d.Sink.Publish(ev)
}

func (d *Driver) broadcastReasoningDelta(sessionID agentcore.SessionID, turnID agentcore.TurnID, partID agentcore.PartID, delta string) {
	ev := agentproto.PartReasoningDeltaEvent{}
	ev.SessionID = sessionID
	ev.At = d.Now()
	ev.TurnID = turnID
	ev.PartID = partID
	ev.Delta = delta
	_ = d.Sink.Publish(ev)
}

func (d *Driver) completeTurn(session agentcore.Session, turnID agentcore.TurnID) (agentcore.Session, error) {
	now := d.Now()
	ev := agentproto.TurnCompletedEvent{}
	ev.SessionID = session.ID
	ev.At = now
	ev.TurnID = turnID
	if err := d.Sink.Publish(ev); err != nil {
		return session, err
	}
	return agentcore.CompleteTurn(session, turnID, now)
}

func (d *Driver) failTurn(session agentcore.Session, turnID agentcore.TurnID, cause error) (agentcore.Session, error) {
	now := d.Now()
	verdict := agentcore.VerdictError
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		verdict = agentcore.VerdictCanceled
	}
	ev := agentproto.TurnFailedEvent{}
	ev.SessionID = session.ID
	ev.At = now
	ev.TurnID = turnID
	ev.Verdict = verdict
	ev.Message = cause.Error()
	// If Publish fails (journal write/fsync error, meta update error, etc.)
	// the durable journal still shows the turn Running. The dispatcher's
	// pre-submit HasLiveTurn check then blocks every future submit until
	// the journal is repaired by hand. Surface the publish failure rather
	// than silently overwriting it with the original cause — the cause is
	// still wrapped so callers can errors.Is it.
	perr := d.Sink.Publish(ev)
	failed, ferr := agentcore.FailTurn(session, turnID, verdict, cause.Error(), now)
	if ferr != nil {
		return session, fmt.Errorf("FailTurn (cause=%w): %v", cause, ferr)
	}
	if perr != nil {
		return failed, fmt.Errorf("publish turn.failed (cause=%w): %v", cause, perr)
	}
	return failed, cause
}

func (d *Driver) validate() error {
	switch {
	case d.Provider == nil:
		return errors.New("agentdriver: Provider required")
	case d.Tools == nil:
		return errors.New("agentdriver: Tools required")
	case d.Sink == nil:
		return errors.New("agentdriver: Sink required")
	case d.IDGen == nil:
		return errors.New("agentdriver: IDGen required")
	case d.Now == nil:
		return errors.New("agentdriver: Now required")
	}
	return nil
}
