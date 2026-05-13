package agentstore

import (
	"errors"
	"fmt"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
)

// IsJournalEvent reports whether an AgentEvent participates in persisted
// state. Streaming deltas and observability-only events return false.
//
// The predicate is total over EventKind: every variant is enumerated, so
// adding a new event kind to Layer P forces a compile-time decision here.
func IsJournalEvent(kind agentproto.EventKind) bool {
	switch kind {
	case agentproto.EventSessionCreated,
		agentproto.EventSessionUpdated,
		agentproto.EventSessionArchived,
		agentproto.EventTurnStarted,
		agentproto.EventTurnCompleted,
		agentproto.EventTurnFailed,
		agentproto.EventPartTextCompleted,
		agentproto.EventPartReasoningCompleted,
		agentproto.EventPartToolUseStarted,
		agentproto.EventPartToolUseCompleted,
		agentproto.EventPartFileRef,
		agentproto.EventPartStepBoundary,
		agentproto.EventSubAgentSpawned,
		agentproto.EventSubAgentCompleted,
		agentproto.EventPermissionRequested,
		agentproto.EventPermissionResolved,
		agentproto.EventModelSwitched:
		return true
	case agentproto.EventPartTextDelta,
		agentproto.EventPartReasoningDelta,
		agentproto.EventAuthExpired:
		return false
	}
	return false
}

// Apply maps a Layer-P AgentEvent to its agentcore transition and returns
// the resulting Session. It is total over the journal-event subset; calling
// Apply with a non-journal event is a programmer error and returns
// ErrNotJournalEvent rather than silently no-op'ing.
//
// Apply is pure: no IO, no clocks. Time comes from the event itself.
func Apply(s agentcore.Session, ev agentproto.AgentEvent) (agentcore.Session, error) {
	if !IsJournalEvent(ev.Kind()) {
		return agentcore.Session{}, fmt.Errorf("agentstore: %w: %s", ErrNotJournalEvent, ev.Kind())
	}
	switch e := ev.(type) {
	case agentproto.SessionCreatedEvent:
		return agentcore.NewSession(e.SessionID, e.ProjectID, e.Title, e.Model, e.At), nil
	case agentproto.SessionUpdatedEvent:
		return agentcore.Rename(s, e.Title, e.At), nil
	case agentproto.SessionArchivedEvent:
		// Session archival is a metadata flag external to agentcore. Replay
		// produces the same Session value; the store layer tracks archived
		// state separately. This event is accepted for journal completeness
		// but does not mutate the algebraic Session.
		return s, nil
	case agentproto.TurnStartedEvent:
		firstPart, err := agentproto.DecodePart(e.FirstPart)
		if err != nil {
			return agentcore.Session{}, fmt.Errorf("apply turn.started: %w", err)
		}
		if e.Role == agentcore.TurnRoleSubAgent {
			return agentcore.StartSubAgentTurn(s, e.TurnID, e.SubAgent, firstPart, e.At)
		}
		return agentcore.StartTurn(s, e.TurnID, e.Role, firstPart, e.At)
	case agentproto.TurnCompletedEvent:
		return agentcore.CompleteTurn(s, e.TurnID, e.At)
	case agentproto.TurnFailedEvent:
		return agentcore.FailTurn(s, e.TurnID, e.Verdict, e.Message, e.At)
	case agentproto.PartTextCompletedEvent:
		return agentcore.AppendPart(s, e.TurnID, agentcore.NewTextPart(e.PartID, e.At, e.Text), e.At)
	case agentproto.PartReasoningCompletedEvent:
		return agentcore.AppendPart(s, e.TurnID, agentcore.NewReasoningPart(e.PartID, e.At, e.Text), e.At)
	case agentproto.PartToolUseStartedEvent:
		return agentcore.AppendPart(s, e.TurnID, agentcore.NewToolUsePart(e.PartID, e.At, e.ToolCallID, e.ToolName, e.Args), e.At)
	case agentproto.PartToolUseCompletedEvent:
		return agentcore.AppendPart(s, e.TurnID, agentcore.NewToolResultPart(e.PartID, e.At, e.ToolCallID, e.ToolName, e.Content, e.IsError), e.At)
	case agentproto.PartFileRefEvent:
		return agentcore.AppendPart(s, e.TurnID, agentcore.NewFileRefPart(e.PartID, e.At, e.Path, e.MIMEType, e.Bytes), e.At)
	case agentproto.PartStepBoundaryEvent:
		return agentcore.AppendPart(s, e.TurnID, agentcore.NewStepBoundaryPart(e.PartID, e.At, e.Label), e.At)
	case agentproto.SubAgentSpawnedEvent:
		link := agentcore.SubAgentLink{
			ID:            e.SubAgentID,
			ParentSession: e.SessionID,
			ParentTurn:    e.TurnID,
			ChildSession:  e.ChildSession,
			Prompt:        e.Prompt,
		}
		return agentcore.AttachSubAgent(s, link, e.At)
	case agentproto.SubAgentCompletedEvent:
		return agentcore.ResolveSubAgent(s, e.SubAgentID, e.Verdict, e.At)
	case agentproto.PermissionRequestedEvent:
		perm := agentcore.Permission{
			ID:         e.PermissionID,
			TurnID:     e.TurnID,
			ToolCallID: e.ToolCallID,
			ToolName:   e.ToolName,
			Args:       e.Args,
		}
		return agentcore.RequestPermission(s, perm, e.At)
	case agentproto.PermissionResolvedEvent:
		return agentcore.ResolvePermission(s, e.PermissionID, e.Decision, e.Reason, e.At)
	case agentproto.ModelSwitchedEvent:
		return agentcore.SwitchModel(s, e.Model, e.At)
	}
	return agentcore.Session{}, fmt.Errorf("agentstore: unhandled journal event kind %q", ev.Kind())
}

// ErrNotJournalEvent is returned by Apply when a non-state event reaches it.
var ErrNotJournalEvent = errors.New("not a journal event")
