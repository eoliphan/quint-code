// Package agentserver is Layer G5 of the v8 agent stack: an HTTP server
// that exposes Layer-P RPCCommands as POST endpoints and broadcasts
// AgentEvents as a single SSE stream at /event/global.
//
// The server is process-local: it listens only on 127.0.0.1 with a random
// free port, communicated to the TUI process via env var. There is no
// authentication on the wire — the wire is loopback only and not reachable
// from other hosts.
//
// G5 is deliberately a thin transport layer. It does NOT drive turns
// itself; it accepts an injected Dispatcher (typically backed by the G4
// driver) that owns the agent loop. The server's responsibilities are:
//   - Parse RPC envelopes; reject malformed/unknown commands.
//   - Persist accepted commands' resulting events to the Store (G3).
//   - Broadcast every persisted event over SSE to all subscribed clients.
//   - Replay history on /session/:id requests.
//
// MCP server (haft serve) is a different process, different binary command,
// different code path. There is no shared HTTP listener and no transitive
// coupling between agentserver and internal/cli/serve_*.
package agentserver
