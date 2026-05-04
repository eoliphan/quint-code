package agentserver

import (
	"context"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
)

// Dispatcher is the contract between the HTTP/SSE transport and the agent
// driver. The transport parses the wire bytes, validates the envelope, and
// hands the typed command to a Dispatcher. The Dispatcher decides what
// events to emit; the transport persists them and broadcasts them.
//
// Dispatcher implementations are typically:
//   - StoreDispatcher (this package) — minimal: handles session lifecycle
//     CRUD without an LLM. Used for tests and for the bootstrap sessions
//     listing endpoint.
//   - DriverDispatcher (internal/agentdriver) — full: turns submitted via
//     RPCTurnSubmit drive the LLM through G4 and produce a stream of
//     events.
//
// The interface is single-method by design: every command flows through
// one Dispatch call. A Dispatcher that doesn't recognize a command should
// return ErrUnsupportedCommand rather than silently no-op'ing.
type Dispatcher interface {
	Dispatch(ctx context.Context, cmd agentproto.RPCCommand) (DispatchResult, error)
}

// DispatchResult carries everything the transport needs after a command
// runs: events that should be journaled+broadcast (in order), and an
// optional response payload returned over HTTP for synchronous queries
// like session.list.
type DispatchResult struct {
	// Events to journal and broadcast. May be empty for query commands
	// (e.g. session.list); may contain many for a turn that streamed.
	Events []agentproto.AgentEvent

	// Response is the HTTP body for query commands. Nil for fire-and-forget
	// mutations whose only response is the SSE stream.
	Response any

	// SessionID identifies which session's journal the events should land
	// in. For session.create, this is the new session's ID even though
	// the events themselves carry it; for cross-session commands (none
	// today), the transport rejects multi-target results.
	SessionID agentcore.SessionID
}
