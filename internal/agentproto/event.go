package agentproto

import (
	"encoding/json"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// EventKind discriminates AgentEvent variants in the JSON wire format.
// Adding a kind is a deliberate breaking change: the TS SDK validates
// against the same string set and rejects unknowns.
type EventKind string

const (
	EventSessionCreated  EventKind = "session.created"
	EventSessionUpdated  EventKind = "session.updated"
	EventSessionArchived EventKind = "session.archived"

	EventTurnStarted   EventKind = "turn.started"
	EventTurnCompleted EventKind = "turn.completed"
	EventTurnFailed    EventKind = "turn.failed"

	EventPartTextDelta          EventKind = "part.text.delta"
	EventPartTextCompleted      EventKind = "part.text.completed"
	EventPartReasoningDelta     EventKind = "part.reasoning.delta"
	EventPartReasoningCompleted EventKind = "part.reasoning.completed"
	EventPartToolUseStarted     EventKind = "part.tool_use.started"
	EventPartToolUseCompleted   EventKind = "part.tool_use.completed"
	EventPartFileRef            EventKind = "part.file_ref"
	EventPartStepBoundary       EventKind = "part.step.boundary"

	EventSubAgentSpawned   EventKind = "subagent.spawned"
	EventSubAgentCompleted EventKind = "subagent.completed"

	EventPermissionRequested EventKind = "permission.requested"
	EventPermissionResolved  EventKind = "permission.resolved"

	EventModelSwitched EventKind = "model.switched"
	EventAuthExpired   EventKind = "auth.expired"
)

// AgentEvent is the sealed union of all things the server can broadcast.
// Each event carries SessionID; turn-scoped events also carry TurnID.
// Wall-clock timestamp is always present so a TUI joining mid-stream can
// place events on its timeline without inferring from arrival order.
type AgentEvent interface {
	Kind() EventKind
	Session() agentcore.SessionID
	Timestamp() time.Time
	eventSeal()
}

type eventBase struct {
	SessionID agentcore.SessionID `json:"session_id"`
	At        time.Time           `json:"at"`
}

func (b eventBase) Session() agentcore.SessionID { return b.SessionID }
func (b eventBase) Timestamp() time.Time         { return b.At }
func (eventBase) eventSeal()                     {}

// SessionCreatedEvent: server created a new Session row, history empty,
// model pinned. Sent once per Session lifetime.
//
// ProjectID is the free-form project identifier the session belongs to.
// It is journaled here so replay reconstructs the same Session value the
// caller observed at creation; meta.json carries a redundant copy for
// fast listing.
type SessionCreatedEvent struct {
	eventBase
	ProjectID string                `json:"project_id,omitempty"`
	Title     string                `json:"title"`
	Model     agentcore.ModelChoice `json:"model"`
}

func (SessionCreatedEvent) Kind() EventKind { return EventSessionCreated }

// SessionUpdatedEvent: title or other metadata changed.
type SessionUpdatedEvent struct {
	eventBase
	Title string `json:"title"`
}

func (SessionUpdatedEvent) Kind() EventKind { return EventSessionUpdated }

// SessionArchivedEvent: session moved to archived state. The TUI removes it
// from the active list but keeps it loadable for replay.
type SessionArchivedEvent struct {
	eventBase
	Reason string `json:"reason"`
}

func (SessionArchivedEvent) Kind() EventKind { return EventSessionArchived }

type turnEventBase struct {
	eventBase
	TurnID agentcore.TurnID `json:"turn_id"`
}

// TurnStartedEvent: a new turn opened. Carries the role and the first part.
type TurnStartedEvent struct {
	turnEventBase
	Role      agentcore.TurnRole   `json:"role"`
	SubAgent  agentcore.SubAgentID `json:"sub_agent_id,omitempty"`
	FirstPart PartPayload          `json:"first_part"`
}

func (TurnStartedEvent) Kind() EventKind { return EventTurnStarted }

// TurnCompletedEvent: assistant finished cleanly.
type TurnCompletedEvent struct {
	turnEventBase
}

func (TurnCompletedEvent) Kind() EventKind { return EventTurnCompleted }

// TurnFailedEvent: terminal failure. Verdict is VerdictCanceled or
// VerdictError; the message describes the failure for operator UX.
type TurnFailedEvent struct {
	turnEventBase
	Verdict agentcore.Verdict `json:"verdict"`
	Message string            `json:"message"`
}

func (TurnFailedEvent) Kind() EventKind { return EventTurnFailed }

// PartTextDeltaEvent: a chunk of streaming text was appended to the turn.
// Delta is the new bytes only — the TUI accumulates from prior deltas.
type PartTextDeltaEvent struct {
	turnEventBase
	PartID agentcore.PartID `json:"part_id"`
	Delta  string           `json:"delta"`
}

func (PartTextDeltaEvent) Kind() EventKind { return EventPartTextDelta }

// PartTextCompletedEvent: the assistant finished a streaming text part.
// Carries the full materialized text so replay reconstructs the same
// TextPart the driver appended; deltas alone are wire-only and would be
// lost on reload.
type PartTextCompletedEvent struct {
	turnEventBase
	PartID agentcore.PartID `json:"part_id"`
	Text   string           `json:"text"`
}

func (PartTextCompletedEvent) Kind() EventKind { return EventPartTextCompleted }

// PartReasoningDeltaEvent: hidden reasoning chunk (o1, thinking blocks).
type PartReasoningDeltaEvent struct {
	turnEventBase
	PartID agentcore.PartID `json:"part_id"`
	Delta  string           `json:"delta"`
}

func (PartReasoningDeltaEvent) Kind() EventKind { return EventPartReasoningDelta }

// PartReasoningCompletedEvent: assistant finished a streaming reasoning
// part. See PartTextCompletedEvent for rationale; reasoning is journaled
// so replayed sessions match the in-memory value Drive returns.
type PartReasoningCompletedEvent struct {
	turnEventBase
	PartID agentcore.PartID `json:"part_id"`
	Text   string           `json:"text"`
}

func (PartReasoningCompletedEvent) Kind() EventKind { return EventPartReasoningCompleted }

// PartToolUseStartedEvent: an assistant tool call has been emitted but not
// yet executed. Args carries the JSON-encoded parameters as the LLM
// produced them; json.RawMessage preserves the JSON shape on the wire
// instead of base64-encoding the bytes.
type PartToolUseStartedEvent struct {
	turnEventBase
	PartID     agentcore.PartID `json:"part_id"`
	ToolCallID string           `json:"tool_call_id"`
	ToolName   string           `json:"tool_name"`
	Args       json.RawMessage  `json:"args"`
}

func (PartToolUseStartedEvent) Kind() EventKind { return EventPartToolUseStarted }

// PartToolUseCompletedEvent: a tool result arrived. Content is the textual
// rendering the LLM will see; IsError flags tool-layer failures (NOT
// non-zero exit codes).
type PartToolUseCompletedEvent struct {
	turnEventBase
	PartID     agentcore.PartID `json:"part_id"`
	ToolCallID string           `json:"tool_call_id"`
	ToolName   string           `json:"tool_name"`
	Content    string           `json:"content"`
	IsError    bool             `json:"is_error"`
}

func (PartToolUseCompletedEvent) Kind() EventKind { return EventPartToolUseCompleted }

// PartFileRefEvent: a tool attached a file to the turn.
type PartFileRefEvent struct {
	turnEventBase
	PartID   agentcore.PartID `json:"part_id"`
	Path     string           `json:"path"`
	MIMEType string           `json:"mime_type"`
	Bytes    int64            `json:"bytes"`
}

func (PartFileRefEvent) Kind() EventKind { return EventPartFileRef }

// PartStepBoundaryEvent: providers that batch tool calls into agentic
// "steps" emit one of these between steps. Lets the TUI fold sections.
type PartStepBoundaryEvent struct {
	turnEventBase
	PartID agentcore.PartID `json:"part_id"`
	Label  string           `json:"label"`
}

func (PartStepBoundaryEvent) Kind() EventKind { return EventPartStepBoundary }

// SubAgentSpawnedEvent: parent turn opened a child session. Both IDs are
// emitted so the TUI can attach the footer view immediately.
type SubAgentSpawnedEvent struct {
	turnEventBase
	SubAgentID   agentcore.SubAgentID `json:"sub_agent_id"`
	ChildSession agentcore.SessionID  `json:"child_session_id"`
	Prompt       string               `json:"prompt"`
}

func (SubAgentSpawnedEvent) Kind() EventKind { return EventSubAgentSpawned }

// SubAgentCompletedEvent: child session finished. Verdict is
// VerdictPass / VerdictCanceled / VerdictError.
type SubAgentCompletedEvent struct {
	turnEventBase
	SubAgentID agentcore.SubAgentID `json:"sub_agent_id"`
	Verdict    agentcore.Verdict    `json:"verdict"`
}

func (SubAgentCompletedEvent) Kind() EventKind { return EventSubAgentCompleted }

// PermissionRequestedEvent: agent needs operator approval to run a tool.
// Args is json.RawMessage so the operator UI sees the actual tool
// arguments rather than a base64 blob.
type PermissionRequestedEvent struct {
	turnEventBase
	PermissionID agentcore.PermissionID `json:"permission_id"`
	ToolCallID   string                 `json:"tool_call_id"`
	ToolName     string                 `json:"tool_name"`
	Args         json.RawMessage        `json:"args"`
}

func (PermissionRequestedEvent) Kind() EventKind { return EventPermissionRequested }

// PermissionResolvedEvent: operator decided. Decision is approved/denied.
type PermissionResolvedEvent struct {
	turnEventBase
	PermissionID agentcore.PermissionID       `json:"permission_id"`
	Decision     agentcore.PermissionDecision `json:"decision"`
	Reason       string                       `json:"reason,omitempty"`
}

func (PermissionResolvedEvent) Kind() EventKind { return EventPermissionResolved }

// ModelSwitchedEvent: session's pinned ModelChoice changed. Takes effect
// for the next turn.
type ModelSwitchedEvent struct {
	eventBase
	Model agentcore.ModelChoice `json:"model"`
}

func (ModelSwitchedEvent) Kind() EventKind { return EventModelSwitched }

// AuthExpiredEvent: a stored credential (codex, OpenAI, Anthropic) has
// expired or been revoked. SessionID may be empty if the auth failure is
// global rather than scoped to a single session.
type AuthExpiredEvent struct {
	eventBase
	Provider agentcore.ProviderKind `json:"provider"`
	Detail   string                 `json:"detail"`
}

func (AuthExpiredEvent) Kind() EventKind { return EventAuthExpired }
