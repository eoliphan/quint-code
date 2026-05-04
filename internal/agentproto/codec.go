package agentproto

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Envelope is the on-wire shape every AgentEvent and RPCCommand uses:
// a kind discriminator plus a body. Kept as one envelope for both
// directions so the TS SDK has exactly one decoder to write.
type Envelope struct {
	Kind string          `json:"kind"`
	Body json.RawMessage `json:"body"`
}

// EncodeEvent produces the wire bytes for an AgentEvent.
func EncodeEvent(ev AgentEvent) ([]byte, error) {
	body, err := encodeEventBody(ev)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Kind: string(ev.Kind()), Body: body})
}

// DecodeEvent parses wire bytes back into an AgentEvent. Unknown kinds are
// rejected with a typed error.
func DecodeEvent(data []byte) (AgentEvent, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode event envelope: %w", err)
	}
	return decodeEventBody(EventKind(env.Kind), env.Body)
}

func encodeEventBody(ev AgentEvent) (json.RawMessage, error) {
	switch ev.Kind() {
	case EventSessionCreated, EventSessionUpdated, EventSessionArchived,
		EventTurnStarted, EventTurnCompleted, EventTurnFailed,
		EventPartTextDelta, EventPartReasoningDelta,
		EventPartToolUseStarted, EventPartToolUseCompleted,
		EventPartFileRef, EventPartStepBoundary,
		EventSubAgentSpawned, EventSubAgentCompleted,
		EventPermissionRequested, EventPermissionResolved,
		EventModelSwitched, EventAuthExpired:
		return json.Marshal(ev)
	}
	return nil, fmt.Errorf("agentproto: cannot encode unknown event kind %q", ev.Kind())
}

func decodeEventBody(kind EventKind, body json.RawMessage) (AgentEvent, error) {
	switch kind {
	case EventSessionCreated:
		return unmarshalEvent[SessionCreatedEvent](body)
	case EventSessionUpdated:
		return unmarshalEvent[SessionUpdatedEvent](body)
	case EventSessionArchived:
		return unmarshalEvent[SessionArchivedEvent](body)
	case EventTurnStarted:
		return unmarshalEvent[TurnStartedEvent](body)
	case EventTurnCompleted:
		return unmarshalEvent[TurnCompletedEvent](body)
	case EventTurnFailed:
		return unmarshalEvent[TurnFailedEvent](body)
	case EventPartTextDelta:
		return unmarshalEvent[PartTextDeltaEvent](body)
	case EventPartReasoningDelta:
		return unmarshalEvent[PartReasoningDeltaEvent](body)
	case EventPartToolUseStarted:
		return unmarshalEvent[PartToolUseStartedEvent](body)
	case EventPartToolUseCompleted:
		return unmarshalEvent[PartToolUseCompletedEvent](body)
	case EventPartFileRef:
		return unmarshalEvent[PartFileRefEvent](body)
	case EventPartStepBoundary:
		return unmarshalEvent[PartStepBoundaryEvent](body)
	case EventSubAgentSpawned:
		return unmarshalEvent[SubAgentSpawnedEvent](body)
	case EventSubAgentCompleted:
		return unmarshalEvent[SubAgentCompletedEvent](body)
	case EventPermissionRequested:
		return unmarshalEvent[PermissionRequestedEvent](body)
	case EventPermissionResolved:
		return unmarshalEvent[PermissionResolvedEvent](body)
	case EventModelSwitched:
		return unmarshalEvent[ModelSwitchedEvent](body)
	case EventAuthExpired:
		return unmarshalEvent[AuthExpiredEvent](body)
	}
	return nil, fmt.Errorf("agentproto: unknown event kind %q", kind)
}

func unmarshalEvent[T AgentEvent](body json.RawMessage) (AgentEvent, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("decode event body: %w", err)
	}
	return v, nil
}

// ErrUnknownKind sentinel allows callers to distinguish unknown-discriminator
// errors from JSON shape errors via errors.Is.
var ErrUnknownKind = errors.New("agentproto: unknown kind")
