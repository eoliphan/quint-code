// Package agentproto is Layer P of the v8 agent stack: the wire format
// shared by the Go agent server (G5) and the TypeScript TUI client. It
// defines AgentEvent (server → client), RPCCommand (client → server), and
// the deterministic JSON encoders/decoders that translate values to the
// SSE/HTTP transport.
//
// The schema is closed: every variant has a known shape and a known kind
// discriminator. Decoders refuse unknown kinds rather than ignoring them,
// so a server upgrade that adds a new event variant without bumping the
// SDK is observable as a typed DecodeError on the client.
//
// This package depends on internal/agentcore for the algebraic Session
// types but does NOT depend on internal/agent (legacy) — the protocol is
// the v8 contract, not a snapshot of legacy state.
package agentproto
