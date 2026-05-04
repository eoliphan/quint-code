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

	turnID := agentcore.TurnID(d.IDGen("turn"))
	now := d.Now()

	// 1. Open the turn with the user's text as the first part.
	userPart := agentcore.NewTextPart(agentcore.PartID(d.IDGen("part")), now, userText)
	if err := d.emitTurnStarted(session.ID, turnID, userPart, now); err != nil {
		return session, err
	}
	updated, err := agentcore.StartTurn(session, turnID, agentcore.TurnRoleUser, userPart, now)
	if err != nil {
		return session, fmt.Errorf("StartTurn: %w", err)
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

	flushText := func(s agentcore.Session) (agentcore.Session, error) {
		if textBuf.Len() == 0 {
			return s, nil
		}
		text := textBuf.String()
		textBuf.Reset()
		return d.emitToolPart(s, turnID, text, agentcore.PartKindText)
	}
	flushReasoning := func(s agentcore.Session) (agentcore.Session, error) {
		if reasoningBuf.Len() == 0 {
			return s, nil
		}
		text := reasoningBuf.String()
		reasoningBuf.Reset()
		return d.emitToolPart(s, turnID, text, agentcore.PartKindReasoning)
	}

	for {
		select {
		case <-ctx.Done():
			return d.failTurn(session, turnID, ctx.Err())
		case ev, open := <-events:
			if !open {
				session, err = flushText(session)
				if err != nil {
					return session, err
				}
				session, err = flushReasoning(session)
				if err != nil {
					return session, err
				}
				return d.completeTurn(session, turnID)
			}
			switch e := ev.(type) {
			case ProviderTextDelta:
				textBuf.WriteString(e.Delta)
				d.broadcastTextDelta(session.ID, turnID, e.Delta)
			case ProviderReasoningDelta:
				reasoningBuf.WriteString(e.Delta)
				d.broadcastReasoningDelta(session.ID, turnID, e.Delta)
			case ProviderToolCall:
				session, err = flushText(session)
				if err != nil {
					return session, err
				}
				session, err = flushReasoning(session)
				if err != nil {
					return session, err
				}
				session, err = d.handleToolCall(ctx, session, turnID, e)
				if err != nil {
					return session, err
				}
			case ProviderTurnDone:
				session, err = flushText(session)
				if err != nil {
					return session, err
				}
				session, err = flushReasoning(session)
				if err != nil {
					return session, err
				}
				return d.completeTurn(session, turnID)
			case ProviderError:
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
		if decision == agentcore.PermissionDenied {
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
	requested := agentproto.PermissionRequestedEvent{}
	requested.SessionID = sessionID
	requested.At = now
	requested.TurnID = turnID
	requested.PermissionID = id
	requested.ToolCallID = call.CallID
	requested.ToolName = call.Name
	requested.Args = call.Args
	if err := d.Sink.Publish(requested); err != nil {
		return "", "", err
	}
	ch := d.Permissions.Open(id)
	decision, reason, err := d.Permissions.Wait(ctx, id, ch)
	if err != nil {
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

func (d *Driver) emitToolPart(session agentcore.Session, turnID agentcore.TurnID, content string, kind agentcore.PartKind) (agentcore.Session, error) {
	now := d.Now()
	partID := agentcore.PartID(d.IDGen("part"))
	switch kind {
	case agentcore.PartKindText:
		// Text deltas are wire-only; the materialized TextPart is journaled
		// via a synthetic event. We use the same EventPartToolUseCompleted
		// shape for journaling text/reasoning would be misleading — the
		// agentstore.Apply path actually has no event type for "text part
		// finalized" right now. For M2b, we journal text via a private
		// helper that updates the Session in memory; persistence of finalized
		// text parts ships in M2c when we add the dedicated event variant.
		// For now, append to the local Session value only, do NOT publish
		// a journal event. The TUI sees text via the deltas.
		part := agentcore.NewTextPart(partID, now, content)
		return agentcore.AppendPart(session, turnID, part, now)
	case agentcore.PartKindReasoning:
		part := agentcore.NewReasoningPart(partID, now, content)
		return agentcore.AppendPart(session, turnID, part, now)
	}
	return session, fmt.Errorf("emitToolPart: unsupported kind %s", kind)
}

func (d *Driver) broadcastTextDelta(sessionID agentcore.SessionID, turnID agentcore.TurnID, delta string) {
	ev := agentproto.PartTextDeltaEvent{}
	ev.SessionID = sessionID
	ev.At = d.Now()
	ev.TurnID = turnID
	ev.PartID = agentcore.PartID(d.IDGen("part-delta"))
	ev.Delta = delta
	_ = d.Sink.Publish(ev)
}

func (d *Driver) broadcastReasoningDelta(sessionID agentcore.SessionID, turnID agentcore.TurnID, delta string) {
	ev := agentproto.PartReasoningDeltaEvent{}
	ev.SessionID = sessionID
	ev.At = d.Now()
	ev.TurnID = turnID
	ev.PartID = agentcore.PartID(d.IDGen("part-delta"))
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
	_ = d.Sink.Publish(ev)
	failed, ferr := agentcore.FailTurn(session, turnID, verdict, cause.Error(), now)
	if ferr != nil {
		return session, fmt.Errorf("FailTurn (cause=%w): %v", cause, ferr)
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
