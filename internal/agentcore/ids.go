package agentcore

// Typed identifiers prevent cross-domain ID confusion at compile time.
// SessionID, TurnID, PartID, PermissionID are not interchangeable strings;
// the compiler rejects passing one where another is expected.

type SessionID string

type TurnID string

type PartID string

type PermissionID string

type SubAgentID string

func (s SessionID) String() string    { return string(s) }
func (t TurnID) String() string       { return string(t) }
func (p PartID) String() string       { return string(p) }
func (p PermissionID) String() string { return string(p) }
func (s SubAgentID) String() string   { return string(s) }
