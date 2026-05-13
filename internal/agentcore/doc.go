// Package agentcore is Layer G2 of the v8 agent stack: pure algebraic types
// for Session/Turn/Part/Permission/SubAgentLink/ModelChoice and the
// pure transitions that move a Session from one state to the next.
//
// All values in this package are immutable. Every transition function takes
// a Session and returns a NEW Session — no field mutation, no shared slice
// state. Errors are returned, never thrown. Side effects (disk, network,
// time) are forbidden here; they live at G0/G1/G3/G5.
//
// This package coexists with the legacy [internal/agent] package during the
// v8 migration. Legacy types remain authoritative for the current coordinator
// (internal/agentloop). Once M2 cuts the coordinator over to G4, legacy
// agent.Session/Message will be deprecated.
//
// Inexpressible (by design):
//   - Mutating an existing Turn or Part.
//   - Recording a Part without a Turn.
//   - Recording a Turn without a Session.
//   - Resolving a Permission that was never requested.
//   - Completing a Turn that is already complete.
//   - Attaching a SubAgent without naming the parent Turn.
//
// Each is rejected by the type system (sealed interfaces, opaque IDs) or by
// the transition function returning a typed error.
package agentcore
