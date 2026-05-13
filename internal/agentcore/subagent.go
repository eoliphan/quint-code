package agentcore

import "time"

// SubAgentLink records that a Turn in a parent Session spawned a child
// Session. The child runs as a fully independent Session — it has its own
// history, its own Turns, its own Permissions — but the link lets the TUI
// render it as a nested footer beneath the parent Turn that opened it.
//
// Lifecycle parity with Permission: a SubAgentLink is created at spawn
// time and resolved when the child Session finishes. Once resolved, the
// link is immutable.
type SubAgentLink struct {
	ID            SubAgentID
	ParentSession SessionID
	ParentTurn    TurnID
	ChildSession  SessionID
	Prompt        string
	State         SubAgentState
	Verdict       Verdict // empty until State=Resolved
	SpawnedAt     time.Time
	ResolvedAt    time.Time // zero until State=Resolved
}

// SubAgentState is closed: a link is either live (child still running) or
// resolved (child terminated, verdict attached).
type SubAgentState string

const (
	SubAgentLive     SubAgentState = "live"
	SubAgentResolved SubAgentState = "resolved"
)

// withResolution returns a copy of the link with the given verdict and
// resolution timestamp. Receiver untouched.
func (s SubAgentLink) withResolution(v Verdict, now time.Time) SubAgentLink {
	s.State = SubAgentResolved
	s.Verdict = v
	s.ResolvedAt = now
	return s
}
