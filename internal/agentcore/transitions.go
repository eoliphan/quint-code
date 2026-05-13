package agentcore

import (
	"errors"
	"time"
)

// Errors returned by transition functions.
//
// Each error is sentinel: callers can compare with errors.Is and the wire
// layer (Layer P) maps them to typed RPC error codes. Wrapping via fmt.Errorf
// is fine but the sentinel must remain reachable.
var (
	ErrTurnNotFound        = errors.New("agentcore: turn not found")
	ErrTurnAlreadyTerminal = errors.New("agentcore: turn already terminal")
	ErrTurnNotRunning      = errors.New("agentcore: turn is not running")
	ErrTurnAlreadyRunning  = errors.New("agentcore: another turn is already running")
	ErrPermissionNotFound  = errors.New("agentcore: permission not found")
	ErrPermissionResolved  = errors.New("agentcore: permission already resolved")
	ErrPermissionDecision  = errors.New("agentcore: invalid permission decision")
	ErrSubAgentNotFound    = errors.New("agentcore: subagent not found")
	ErrSubAgentResolved    = errors.New("agentcore: subagent already resolved")
)

// NewSession creates a fresh Session value. It does NOT persist anything;
// persistence is G3's responsibility. The returned Session has empty
// History/Permissions/SubAgents and timestamps set to now.
func NewSession(id SessionID, projectID, title string, model ModelChoice, now time.Time) Session {
	return Session{
		ID:          id,
		ProjectID:   projectID,
		Title:       title,
		Model:       model,
		History:     nil,
		Permissions: map[PermissionID]Permission{},
		SubAgents:   map[SubAgentID]SubAgentLink{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// StartTurn opens a new Running turn on the Session. Returns an error if
// another turn is currently Running — the caller must complete or fail the
// live turn first.
//
// The first Part is appended in the same step so a Turn never exists in a
// partless state on disk; consumers always see at least one Part of context.
func StartTurn(s Session, turnID TurnID, role TurnRole, firstPart Part, now time.Time) (Session, error) {
	if s.liveTurn() != -1 {
		return Session{}, ErrTurnAlreadyRunning
	}
	turn := Turn{
		ID:        turnID,
		Role:      role,
		State:     TurnStateRunning,
		StartedAt: now,
		Parts:     []Part{firstPart},
	}
	return s.withTurnAppended(turn).touch(now), nil
}

// StartSubAgentTurn opens a new Running turn driven by a SubAgent rather
// than a user. Identical to StartTurn but tags the Turn with the
// SubAgentID so the TUI can route it to the parent's footer.
func StartSubAgentTurn(s Session, turnID TurnID, subAgent SubAgentID, firstPart Part, now time.Time) (Session, error) {
	if s.liveTurn() != -1 {
		return Session{}, ErrTurnAlreadyRunning
	}
	turn := Turn{
		ID:         turnID,
		Role:       TurnRoleSubAgent,
		State:      TurnStateRunning,
		StartedAt:  now,
		Parts:      []Part{firstPart},
		SubAgentID: subAgent,
	}
	return s.withTurnAppended(turn).touch(now), nil
}

// AppendPart adds a Part to a Running Turn. Errors if the Turn does not
// exist or is already terminal. The returned Session shares no slices with
// the receiver.
func AppendPart(s Session, turnID TurnID, p Part, now time.Time) (Session, error) {
	for i, turn := range s.History {
		if turn.ID != turnID {
			continue
		}
		if turn.IsTerminal() {
			return Session{}, ErrTurnAlreadyTerminal
		}
		updated := turn.withPart(p)
		return s.withTurnAt(i, updated).touch(now), nil
	}
	return Session{}, ErrTurnNotFound
}

// CompleteTurn marks a Running Turn as Completed with VerdictPass. Errors
// if the Turn is missing or already terminal.
func CompleteTurn(s Session, turnID TurnID, now time.Time) (Session, error) {
	for i, turn := range s.History {
		if turn.ID != turnID {
			continue
		}
		if turn.State != TurnStateRunning {
			return Session{}, ErrTurnNotRunning
		}
		updated := turn.withFinalState(TurnStateCompleted, VerdictPass, "", now)
		return s.withTurnAt(i, updated).touch(now), nil
	}
	return Session{}, ErrTurnNotFound
}

// FailTurn marks a Running Turn as Failed with the given verdict and error
// message. Verdict must be one of VerdictCanceled or VerdictError; passing
// VerdictPass here is rejected.
func FailTurn(s Session, turnID TurnID, verdict Verdict, errMsg string, now time.Time) (Session, error) {
	if verdict != VerdictCanceled && verdict != VerdictError {
		return Session{}, errors.New("agentcore: FailTurn requires VerdictCanceled or VerdictError")
	}
	for i, turn := range s.History {
		if turn.ID != turnID {
			continue
		}
		if turn.State != TurnStateRunning {
			return Session{}, ErrTurnNotRunning
		}
		updated := turn.withFinalState(TurnStateFailed, verdict, errMsg, now)
		return s.withTurnAt(i, updated).touch(now), nil
	}
	return Session{}, ErrTurnNotFound
}

// RequestPermission attaches a pending Permission to the Session. The Turn
// must exist and be Running — permissions on terminal Turns are nonsensical.
// The Permission's ID is supplied by the caller (G3 generates IDs); this
// function does not invent identifiers.
func RequestPermission(s Session, perm Permission, now time.Time) (Session, error) {
	turn, ok := s.FindTurn(perm.TurnID)
	if !ok {
		return Session{}, ErrTurnNotFound
	}
	if turn.IsTerminal() {
		return Session{}, ErrTurnAlreadyTerminal
	}
	perm.Decision = PermissionPending
	perm.RequestedAt = now
	perm.ResolvedAt = time.Time{}
	perms := cloneMap(s.Permissions)
	perms[perm.ID] = perm
	return s.withPermissions(perms).touch(now), nil
}

// ResolvePermission records the operator's decision. Idempotent rejection:
// resolving an already-resolved permission returns ErrPermissionResolved
// rather than overwriting.
func ResolvePermission(s Session, id PermissionID, decision PermissionDecision, reason string, now time.Time) (Session, error) {
	if decision == PermissionPending {
		return Session{}, errors.New("agentcore: cannot resolve permission to pending")
	}
	existing, ok := s.Permissions[id]
	if !ok {
		return Session{}, ErrPermissionNotFound
	}
	if existing.IsResolved() {
		return Session{}, ErrPermissionResolved
	}
	perms := cloneMap(s.Permissions)
	perms[id] = existing.withResolution(decision, reason, now)
	return s.withPermissions(perms).touch(now), nil
}

// AttachSubAgent records that a Turn spawned a child Session. The parent
// turn must exist; it does not have to be Running (a Turn may complete
// before its sub-agent does, in which case the link outlives the Turn).
func AttachSubAgent(s Session, link SubAgentLink, now time.Time) (Session, error) {
	if _, ok := s.FindTurn(link.ParentTurn); !ok {
		return Session{}, ErrTurnNotFound
	}
	link.State = SubAgentLive
	link.SpawnedAt = now
	link.ResolvedAt = time.Time{}
	link.Verdict = ""
	links := cloneSubAgents(s.SubAgents)
	links[link.ID] = link
	return s.withSubAgents(links).touch(now), nil
}

// ResolveSubAgent marks a previously-spawned SubAgent as resolved with the
// given verdict.
func ResolveSubAgent(s Session, id SubAgentID, verdict Verdict, now time.Time) (Session, error) {
	existing, ok := s.SubAgents[id]
	if !ok {
		return Session{}, ErrSubAgentNotFound
	}
	if existing.State == SubAgentResolved {
		return Session{}, ErrSubAgentResolved
	}
	links := cloneSubAgents(s.SubAgents)
	links[id] = existing.withResolution(verdict, now)
	return s.withSubAgents(links).touch(now), nil
}

// SwitchModel returns a copy of the Session with a new ModelChoice. The
// switch takes effect for the NEXT Turn; in-flight Turns keep their
// original choice (the Turn itself doesn't carry the choice — the runtime
// reads s.Model at the moment it dispatches a turn). Switching mid-Turn
// is rejected to avoid ambiguity.
func SwitchModel(s Session, choice ModelChoice, now time.Time) (Session, error) {
	if s.liveTurn() != -1 {
		return Session{}, ErrTurnAlreadyRunning
	}
	s.Model = choice
	return s.touch(now), nil
}

// Rename returns a copy of the Session with a new Title. Pure; safe to
// call regardless of Turn state.
func Rename(s Session, title string, now time.Time) Session {
	s.Title = title
	return s.touch(now)
}

func cloneMap(in map[PermissionID]Permission) map[PermissionID]Permission {
	out := make(map[PermissionID]Permission, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSubAgents(in map[SubAgentID]SubAgentLink) map[SubAgentID]SubAgentLink {
	out := make(map[SubAgentID]SubAgentLink, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
