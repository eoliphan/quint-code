package agentcore

import "time"

// PermissionDecision is the operator's response to a permission request.
// Resolution is terminal — once set, it cannot change. Re-asking for the
// same operation creates a NEW Permission.
type PermissionDecision string

const (
	PermissionPending  PermissionDecision = "pending"
	PermissionApproved PermissionDecision = "approved"
	PermissionDenied   PermissionDecision = "denied"
)

// Permission is a request to run a specific tool invocation, surfaced to
// the operator and resolved by their decision. The Permission record
// participates in the Session graph just like any other entity: it is
// created, eventually resolved, and never mutated after resolution.
type Permission struct {
	ID          PermissionID
	TurnID      TurnID
	ToolCallID  string
	ToolName    string
	Args        []byte
	Decision    PermissionDecision
	Reason      string // operator-provided justification on deny; optional on approve
	RequestedAt time.Time
	ResolvedAt  time.Time // zero until Decision != Pending
}

// IsResolved reports whether the operator has decided.
func (p Permission) IsResolved() bool {
	return p.Decision != PermissionPending
}

// withResolution returns a copy of the Permission marked with the given
// decision at the given time. The receiver is untouched.
func (p Permission) withResolution(d PermissionDecision, reason string, now time.Time) Permission {
	p.Decision = d
	p.Reason = reason
	p.ResolvedAt = now
	return p
}
