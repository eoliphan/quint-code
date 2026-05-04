package agentcore

import "time"

// TurnRole identifies who initiated a Turn. The agent itself never opens
// a Turn — Turns are opened by user messages or by sub-agent prompts.
type TurnRole string

const (
	TurnRoleUser     TurnRole = "user"
	TurnRoleSubAgent TurnRole = "sub_agent"
)

// TurnState is the lifecycle state of a Turn.
//
// Running   — created, parts being appended.
// Completed — assistant finished, no more parts will be appended.
// Failed    — terminal with an error attached (provider error, cancel, panic).
//
// The set is closed. There is no Paused — pause-on-permission is modeled
// by an open Permission, not by a Turn state.
type TurnState string

const (
	TurnStateRunning   TurnState = "running"
	TurnStateCompleted TurnState = "completed"
	TurnStateFailed    TurnState = "failed"
)

// Verdict is attached to a completed or failed Turn. For successful turns
// VerdictPass is the only value; failure modes live in VerdictFail variants.
type Verdict string

const (
	VerdictPass     Verdict = "pass"
	VerdictCanceled Verdict = "canceled"
	VerdictError    Verdict = "error"
)

// Turn is one user-or-subagent → assistant exchange. Immutable: every
// transition that adds a Part returns a NEW Turn whose Parts slice is a
// fresh allocation. Tests verify this property; see transitions_test.go.
type Turn struct {
	ID        TurnID
	Role      TurnRole
	State     TurnState
	Verdict   Verdict // empty until State != Running
	Parts     []Part  // append-only
	StartedAt time.Time
	EndedAt   time.Time // zero until State != Running
	// SubAgentID is non-empty only for Role=TurnRoleSubAgent; it identifies
	// the parent linkage stored on the parent session, not the child.
	SubAgentID SubAgentID
	// Error is the failure detail when State=Failed. Empty otherwise.
	Error string
}

// IsTerminal reports whether the Turn has finished and no further parts
// will be appended to it.
func (t Turn) IsTerminal() bool {
	return t.State == TurnStateCompleted || t.State == TurnStateFailed
}

// LastPartOfKind returns the most recent Part of the given kind, or nil if
// no such Part is present. Used by transition logic that needs to coalesce
// streaming text deltas; the agentcore package itself does NOT coalesce —
// that policy lives at G4 if desired.
func (t Turn) LastPartOfKind(kind PartKind) Part {
	for i := len(t.Parts) - 1; i >= 0; i-- {
		if t.Parts[i].Kind() == kind {
			return t.Parts[i]
		}
	}
	return nil
}

// withPart returns a copy of the Turn with the given Part appended. The
// new Parts slice is freshly allocated; the receiver's slice is untouched.
func (t Turn) withPart(p Part) Turn {
	parts := make([]Part, len(t.Parts)+1)
	copy(parts, t.Parts)
	parts[len(t.Parts)] = p
	t.Parts = parts
	return t
}

// withFinalState returns a copy of the Turn marked terminal with the given
// verdict at the given time. The receiver is untouched.
func (t Turn) withFinalState(state TurnState, verdict Verdict, errMsg string, now time.Time) Turn {
	t.State = state
	t.Verdict = verdict
	t.EndedAt = now
	t.Error = errMsg
	return t
}
