package agentcore

import "time"

// Session is the durable, immutable record of an interactive agent run.
//
// Every Turn ever started in this Session is in History — including failed
// turns, canceled turns, and the live one if any. There is no separate
// "active turn" pointer; the live Turn is the one whose State is Running.
// At most one Turn is ever Running at a time; the type system does not
// enforce this (it would require a more elaborate state machine), but the
// transition functions in transitions.go reject overlap.
//
// Permissions and SubAgents are stored as flat collections keyed by their
// IDs. They do not live inside Turns because their lifecycles outlast the
// turn that created them (an operator may resolve a Permission asynchronously
// from the next prompt; a sub-agent may finish after the parent turn ended).
//
// Title is operator-editable display text; the runtime never derives Title
// automatically. A Session with no Title is rendered as "Untitled session"
// in the TUI.
type Session struct {
	ID        SessionID
	ProjectID string // free-form identifier, typically the .haft project id
	Title     string
	Model     ModelChoice
	History   []Turn
	// Permissions and SubAgents are stable maps. Map iteration order is not
	// part of the value's identity; transition functions return Sessions whose
	// maps are fresh allocations to preserve immutability.
	Permissions map[PermissionID]Permission
	SubAgents   map[SubAgentID]SubAgentLink
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// ParentSession is set on child sessions spawned by SubAgentLink. The
	// parent's SubAgents map carries the link; the child references back
	// here for replay and for the TUI to follow upward navigation.
	ParentSession SessionID
}

// liveTurn returns the index of the Running turn, or -1 if no turn is live.
func (s Session) liveTurn() int {
	for i := range s.History {
		if s.History[i].State == TurnStateRunning {
			return i
		}
	}
	return -1
}

// HasLiveTurn reports whether the Session currently has a Running turn.
// Callers outside agentcore use this to reject submits synchronously when
// the journal already shows a turn in flight — e.g. after a process restart
// where the in-memory dispatcher map is empty but the persisted history is
// not.
func (s Session) HasLiveTurn() bool {
	return s.liveTurn() != -1
}

// FindTurn returns the Turn with the given ID and whether it was found.
// Lookup is linear — Sessions are short enough in practice that a map
// would only add allocation overhead.
func (s Session) FindTurn(id TurnID) (Turn, bool) {
	for _, t := range s.History {
		if t.ID == id {
			return t, true
		}
	}
	return Turn{}, false
}

// withTurnAt returns a copy of the Session with the History entry at index i
// replaced by replacement. The original History slice is not modified.
func (s Session) withTurnAt(i int, replacement Turn) Session {
	history := make([]Turn, len(s.History))
	copy(history, s.History)
	history[i] = replacement
	s.History = history
	return s
}

// withTurnAppended returns a copy of the Session with the given Turn added.
func (s Session) withTurnAppended(t Turn) Session {
	history := make([]Turn, len(s.History)+1)
	copy(history, s.History)
	history[len(s.History)] = t
	s.History = history
	return s
}

// withPermissions returns a copy of the Session whose Permissions map is a
// fresh allocation containing the provided entries.
func (s Session) withPermissions(perms map[PermissionID]Permission) Session {
	clone := make(map[PermissionID]Permission, len(perms))
	for k, v := range perms {
		clone[k] = v
	}
	s.Permissions = clone
	return s
}

// withSubAgents returns a copy of the Session whose SubAgents map is a
// fresh allocation containing the provided entries.
func (s Session) withSubAgents(links map[SubAgentID]SubAgentLink) Session {
	clone := make(map[SubAgentID]SubAgentLink, len(links))
	for k, v := range links {
		clone[k] = v
	}
	s.SubAgents = clone
	return s
}

// touch returns a copy of the Session with UpdatedAt set to now. Used by
// every transition; centralized here so transitions.go stays focused on
// invariant checks.
func (s Session) touch(now time.Time) Session {
	s.UpdatedAt = now
	return s
}
