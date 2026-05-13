package agentproto

import (
	"encoding/json"
	"fmt"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// RPCKind discriminates RPCCommand variants. Like EventKind, the set is
// closed; the server rejects unknown commands with a typed error.
type RPCKind string

const (
	RPCSessionCreate     RPCKind = "session.create"
	RPCSessionList       RPCKind = "session.list"
	RPCSessionResume     RPCKind = "session.resume"
	RPCSessionRename     RPCKind = "session.rename"
	RPCTurnSubmit        RPCKind = "turn.submit"
	RPCTurnCancel        RPCKind = "turn.cancel"
	RPCPermissionRespond RPCKind = "permission.respond"
	RPCModelSet          RPCKind = "model.set"
	RPCSubAgentAttach    RPCKind = "subagent.attach"
)

// RPCCommand is the sealed set of mutations the TUI may request. Every
// command is scoped to either a project (session.create/list) or a
// session (everything else); the server enforces scoping at dispatch.
type RPCCommand interface {
	Kind() RPCKind
	rpcSeal()
}

type rpcBase struct{}

func (rpcBase) rpcSeal() {}

// SessionCreateCmd: open a fresh session. Title is the operator-provided
// display label; empty title is allowed and rendered as "Untitled session".
type SessionCreateCmd struct {
	rpcBase
	ProjectID string                `json:"project_id"`
	Title     string                `json:"title"`
	Model     agentcore.ModelChoice `json:"model"`
}

func (SessionCreateCmd) Kind() RPCKind { return RPCSessionCreate }

// SessionListCmd: list sessions within a project. Server returns metadata
// only; full history is fetched separately via session.resume.
type SessionListCmd struct {
	rpcBase
	ProjectID string `json:"project_id"`
}

func (SessionListCmd) Kind() RPCKind { return RPCSessionList }

// SessionResumeCmd: load a session's full event history. Sent when the
// TUI selects a session from the list view.
type SessionResumeCmd struct {
	rpcBase
	SessionID agentcore.SessionID `json:"session_id"`
}

func (SessionResumeCmd) Kind() RPCKind { return RPCSessionResume }

// SessionRenameCmd: change the operator-visible title.
type SessionRenameCmd struct {
	rpcBase
	SessionID agentcore.SessionID `json:"session_id"`
	Title     string              `json:"title"`
}

func (SessionRenameCmd) Kind() RPCKind { return RPCSessionRename }

// TurnSubmitCmd: send a user message and start a new turn. Server validates
// no other turn is running; returns a typed error if one is.
type TurnSubmitCmd struct {
	rpcBase
	SessionID agentcore.SessionID `json:"session_id"`
	Text      string              `json:"text"`
}

func (TurnSubmitCmd) Kind() RPCKind { return RPCTurnSubmit }

// TurnCancelCmd: stop a running turn. Server transitions the turn to
// Failed/VerdictCanceled and aborts any in-flight tool calls.
type TurnCancelCmd struct {
	rpcBase
	SessionID agentcore.SessionID `json:"session_id"`
	TurnID    agentcore.TurnID    `json:"turn_id"`
}

func (TurnCancelCmd) Kind() RPCKind { return RPCTurnCancel }

// PermissionRespondCmd: operator approves or denies a permission request.
// Reason is optional on approve, recommended on deny.
type PermissionRespondCmd struct {
	rpcBase
	SessionID    agentcore.SessionID          `json:"session_id"`
	PermissionID agentcore.PermissionID       `json:"permission_id"`
	Decision     agentcore.PermissionDecision `json:"decision"`
	Reason       string                       `json:"reason,omitempty"`
}

func (PermissionRespondCmd) Kind() RPCKind { return RPCPermissionRespond }

// ModelSetCmd: pin the session to a different ModelChoice. Takes effect
// for the next turn; rejected if a turn is currently running.
type ModelSetCmd struct {
	rpcBase
	SessionID agentcore.SessionID   `json:"session_id"`
	Model     agentcore.ModelChoice `json:"model"`
}

func (ModelSetCmd) Kind() RPCKind { return RPCModelSet }

// SubAgentAttachCmd: external orchestrator attaches a sub-agent record to
// a parent turn. Used when a tool spawns a child session out-of-band; the
// agentloop coordinator (G4) calls this internally so the TUI sees the
// link before any child events flow.
type SubAgentAttachCmd struct {
	rpcBase
	ParentSession agentcore.SessionID  `json:"parent_session_id"`
	ParentTurn    agentcore.TurnID     `json:"parent_turn_id"`
	ChildSession  agentcore.SessionID  `json:"child_session_id"`
	SubAgentID    agentcore.SubAgentID `json:"sub_agent_id"`
	Prompt        string               `json:"prompt"`
}

func (SubAgentAttachCmd) Kind() RPCKind { return RPCSubAgentAttach }

// EncodeCommand serializes an RPCCommand to wire bytes.
func EncodeCommand(cmd RPCCommand) ([]byte, error) {
	body, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Kind: string(cmd.Kind()), Body: body})
}

// DecodeCommand parses wire bytes back to an RPCCommand. Unknown kinds
// are rejected; nil is never returned without an error.
func DecodeCommand(data []byte) (RPCCommand, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode command envelope: %w", err)
	}
	switch RPCKind(env.Kind) {
	case RPCSessionCreate:
		return unmarshalCmd[SessionCreateCmd](env.Body)
	case RPCSessionList:
		return unmarshalCmd[SessionListCmd](env.Body)
	case RPCSessionResume:
		return unmarshalCmd[SessionResumeCmd](env.Body)
	case RPCSessionRename:
		return unmarshalCmd[SessionRenameCmd](env.Body)
	case RPCTurnSubmit:
		return unmarshalCmd[TurnSubmitCmd](env.Body)
	case RPCTurnCancel:
		return unmarshalCmd[TurnCancelCmd](env.Body)
	case RPCPermissionRespond:
		return unmarshalCmd[PermissionRespondCmd](env.Body)
	case RPCModelSet:
		return unmarshalCmd[ModelSetCmd](env.Body)
	case RPCSubAgentAttach:
		return unmarshalCmd[SubAgentAttachCmd](env.Body)
	}
	return nil, fmt.Errorf("agentproto: unknown command kind %q", env.Kind)
}

func unmarshalCmd[T RPCCommand](body json.RawMessage) (RPCCommand, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("decode command body: %w", err)
	}
	return v, nil
}
