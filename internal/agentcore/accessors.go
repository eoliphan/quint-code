package agentcore

// Read-only TUI accessors on Session and Turn.
//
// The Session struct exports its fields (History, Permissions, SubAgents,
// Model) so internal transitions can read them, but a TUI consumer that
// reaches in directly risks mutating shared state — slice append into
// spare capacity, map insert, etc. — and silently corrupting the parent
// Session value. These accessors return defensive copies the caller
// cannot reach back into.
//
// Sealed sum variants (Part) are returned as interface values, so the
// caller cannot extend the closed set; the underlying value types remain
// the only carriers. Permissions and SubAgentLink are structs and copy
// by value at the slice / map boundary.

// Turns returns a defensive copy of the Session's Turn history. The
// returned slice is freshly allocated; appending to it does not affect
// the Session value.
func (s Session) Turns() []Turn {
	out := make([]Turn, len(s.History))
	copy(out, s.History)
	return out
}

// Parts returns a defensive copy of the named Turn's Parts. Returns nil
// when the Turn is absent. Mutating the returned slice does not affect
// the Session value.
func (s Session) Parts(id TurnID) []Part {
	turn, ok := s.FindTurn(id)
	if !ok {
		return nil
	}
	return turn.PartsCopy()
}

// PermissionsList returns a defensive copy of the Session's pending and
// resolved Permissions as a slice. Map iteration order is not part of
// the Session's identity, so the slice order is the natural map order
// (callers that need a stable order MUST sort).
func (s Session) PermissionsList() []Permission {
	out := make([]Permission, 0, len(s.Permissions))
	for _, p := range s.Permissions {
		out = append(out, p)
	}
	return out
}

// SubAgentsList returns a defensive copy of the Session's SubAgentLinks
// as a slice. Map iteration order is not stable; callers that need a
// stable order MUST sort.
func (s Session) SubAgentsList() []SubAgentLink {
	out := make([]SubAgentLink, 0, len(s.SubAgents))
	for _, l := range s.SubAgents {
		out = append(out, l)
	}
	return out
}

// ModelChoice returns the Session's current ModelChoice by value. The
// underlying type is a value type so this is already defensive — the
// method exists for symmetry with the other accessors and for use as a
// stable read-only API the TUI can rely on across refactors.
func (s Session) ModelChoice() ModelChoice {
	return s.Model
}

// PartsCopy returns a defensive copy of the Turn's Parts. Provided as a
// Turn method so callers iterating Turns() can ask each Turn for its
// Parts without referencing the originating Session.
func (t Turn) PartsCopy() []Part {
	out := make([]Part, len(t.Parts))
	copy(out, t.Parts)
	return out
}
