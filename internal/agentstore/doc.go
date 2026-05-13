// Package agentstore is Layer G3 of the v8 agent stack: append-only event
// log + replay + session lookup. The store persists Layer-P AgentEvents
// to a per-session JSONL journal under ~/.haft/sessions/<id>/events.jsonl
// and reconstructs an agentcore.Session by replaying the journal through
// pure transitions.
//
// The store deliberately persists ONLY state-mutating events, not the
// streaming text/reasoning deltas Layer P broadcasts for live UX. Storing
// every delta would bloat the journal with thousands of micro-events per
// assistant message; the G4 coordinator (M2) will buffer streaming deltas
// in memory and emit a single materialized Part-append event when the
// stream completes. Predicate IsJournalEvent decides routing.
//
// Replay is pure: given a sequence of journal events, replay deterministically
// produces a Session. Acceptance for M1 is the 1000-event replay test in
// store_test.go.
package agentstore
