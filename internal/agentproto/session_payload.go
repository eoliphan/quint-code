package agentproto

import (
	"encoding/json"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// SessionPayload is the wire form of an agentcore.Session. agentcore types
// are not directly JSON-safe — Part is a sealed interface whose concrete
// structs keep ID and CreatedAt in an unexported partBase, so a raw
// encoding/json pass produces entries like {"Text":"..."} with no kind
// discriminator. SessionPayload routes every Part through PartPayload so
// resumed sessions reconstruct on the TUI side.
type SessionPayload struct {
	ID            agentcore.SessionID   `json:"id"`
	ProjectID     string                `json:"project_id"`
	Title         string                `json:"title"`
	Model         agentcore.ModelChoice `json:"model"`
	History       []TurnPayload         `json:"history"`
	Permissions   []PermissionPayload   `json:"permissions"`
	SubAgents     []SubAgentLinkPayload `json:"sub_agents"`
	CreatedAt     timeStamp             `json:"created_at"`
	UpdatedAt     timeStamp             `json:"updated_at"`
	ParentSession agentcore.SessionID   `json:"parent_session,omitempty"`
}

// TurnPayload is the wire form of an agentcore.Turn. Parts are encoded as
// tagged PartPayloads so the TUI can reconstruct PartKind/PartID without
// peeking into the concrete Go type.
type TurnPayload struct {
	ID         agentcore.TurnID     `json:"id"`
	Role       agentcore.TurnRole   `json:"role"`
	State      agentcore.TurnState  `json:"state"`
	Verdict    agentcore.Verdict    `json:"verdict,omitempty"`
	Parts      []PartPayload        `json:"parts"`
	StartedAt  timeStamp            `json:"started_at"`
	EndedAt    *timeStamp           `json:"ended_at,omitempty"`
	SubAgentID agentcore.SubAgentID `json:"sub_agent_id,omitempty"`
	Error      string               `json:"error,omitempty"`
}

// PermissionPayload is the wire form of an agentcore.Permission.
// Permission.Args is documented as a JSON-encoded tool-args blob, so it
// is passed through as json.RawMessage for parity with the event channel
// rather than base64-encoded as []byte would be.
type PermissionPayload struct {
	ID          agentcore.PermissionID       `json:"id"`
	TurnID      agentcore.TurnID             `json:"turn_id"`
	ToolCallID  string                       `json:"tool_call_id"`
	ToolName    string                       `json:"tool_name"`
	Args        json.RawMessage              `json:"args"`
	Decision    agentcore.PermissionDecision `json:"decision"`
	Reason      string                       `json:"reason,omitempty"`
	RequestedAt timeStamp                    `json:"requested_at"`
	ResolvedAt  *timeStamp                   `json:"resolved_at,omitempty"`
}

// SubAgentLinkPayload is the wire form of an agentcore.SubAgentLink.
type SubAgentLinkPayload struct {
	ID            agentcore.SubAgentID    `json:"id"`
	ParentSession agentcore.SessionID     `json:"parent_session"`
	ParentTurn    agentcore.TurnID        `json:"parent_turn"`
	ChildSession  agentcore.SessionID     `json:"child_session"`
	Prompt        string                  `json:"prompt"`
	State         agentcore.SubAgentState `json:"state"`
	Verdict       agentcore.Verdict       `json:"verdict,omitempty"`
	SpawnedAt     timeStamp               `json:"spawned_at"`
	ResolvedAt    *timeStamp              `json:"resolved_at,omitempty"`
}

// EncodeSession produces the wire form of a Session. Every Part is routed
// through EncodePart so the resumed snapshot uses the same envelope shape
// the event stream does. An error in any nested EncodePart aborts the
// encode — partial sessions are never returned.
func EncodeSession(s agentcore.Session) (SessionPayload, error) {
	history := make([]TurnPayload, 0, len(s.History))
	for _, t := range s.History {
		turn, err := encodeTurn(t)
		if err != nil {
			return SessionPayload{}, err
		}
		history = append(history, turn)
	}
	perms := make([]PermissionPayload, 0, len(s.Permissions))
	for _, p := range s.Permissions {
		perms = append(perms, encodePermission(p))
	}
	links := make([]SubAgentLinkPayload, 0, len(s.SubAgents))
	for _, link := range s.SubAgents {
		links = append(links, encodeSubAgentLink(link))
	}
	return SessionPayload{
		ID:            s.ID,
		ProjectID:     s.ProjectID,
		Title:         s.Title,
		Model:         s.Model,
		History:       history,
		Permissions:   perms,
		SubAgents:     links,
		CreatedAt:     timeStamp(s.CreatedAt),
		UpdatedAt:     timeStamp(s.UpdatedAt),
		ParentSession: s.ParentSession,
	}, nil
}

func encodeTurn(t agentcore.Turn) (TurnPayload, error) {
	parts := make([]PartPayload, 0, len(t.Parts))
	for _, p := range t.Parts {
		payload, err := EncodePart(p)
		if err != nil {
			return TurnPayload{}, err
		}
		parts = append(parts, payload)
	}
	var ended *timeStamp
	if !t.EndedAt.IsZero() {
		ts := timeStamp(t.EndedAt)
		ended = &ts
	}
	return TurnPayload{
		ID:         t.ID,
		Role:       t.Role,
		State:      t.State,
		Verdict:    t.Verdict,
		Parts:      parts,
		StartedAt:  timeStamp(t.StartedAt),
		EndedAt:    ended,
		SubAgentID: t.SubAgentID,
		Error:      t.Error,
	}, nil
}

func encodePermission(p agentcore.Permission) PermissionPayload {
	var resolved *timeStamp
	if !p.ResolvedAt.IsZero() {
		ts := timeStamp(p.ResolvedAt)
		resolved = &ts
	}
	return PermissionPayload{
		ID:          p.ID,
		TurnID:      p.TurnID,
		ToolCallID:  p.ToolCallID,
		ToolName:    p.ToolName,
		Args:        json.RawMessage(p.Args),
		Decision:    p.Decision,
		Reason:      p.Reason,
		RequestedAt: timeStamp(p.RequestedAt),
		ResolvedAt:  resolved,
	}
}

func encodeSubAgentLink(l agentcore.SubAgentLink) SubAgentLinkPayload {
	var resolved *timeStamp
	if !l.ResolvedAt.IsZero() {
		ts := timeStamp(l.ResolvedAt)
		resolved = &ts
	}
	return SubAgentLinkPayload{
		ID:            l.ID,
		ParentSession: l.ParentSession,
		ParentTurn:    l.ParentTurn,
		ChildSession:  l.ChildSession,
		Prompt:        l.Prompt,
		State:         l.State,
		Verdict:       l.Verdict,
		SpawnedAt:     timeStamp(l.SpawnedAt),
		ResolvedAt:    resolved,
	}
}
