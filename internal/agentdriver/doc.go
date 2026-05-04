// Package agentdriver is Layer G4 of the v8 agent stack: pure turn
// orchestration on top of agentcore (G2) and agentstore (G3). One Driver
// drives one Session through one turn at a time, emitting Layer-P events
// to a sink as the turn progresses.
//
// The driver does NOT speak to LLM providers or tool implementations
// directly. It composes against three minimal interfaces:
//
//   - Provider — streams ProviderEvents (text deltas, reasoning deltas,
//     tool calls, terminal events) for a given history.
//   - ToolDispatcher — authorizes and executes tool invocations.
//   - EventSink — receives every Layer-P AgentEvent the driver wants to
//     journal and/or broadcast.
//
// Production wiring lives in internal/agentdriver/wiring.go (M2c): it
// adapts existing internal/provider and internal/tools to these
// interfaces. The legacy internal/agentloop coordinator is NOT touched
// by this package; v8 ships them side by side and migration happens at
// the haft-agent CLI command boundary in M4.
package agentdriver
