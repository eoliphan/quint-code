package agentdriver

import (
	"context"
	"errors"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// SubAgentSpawnToolName is the tool name the LLM uses to spawn a child
// subagent. The driver routes calls with this name through the
// SubAgentRunner instead of the regular ToolDispatcher, so subagent
// orchestration stays out of the per-tool authorisation policy.
const SubAgentSpawnToolName = "spawn_subagent"

// SubAgentSpawnArgs is the JSON-decoded payload the LLM emits when calling
// SubAgentSpawnToolName. Name selects the subagent definition; Prompt is
// the seed text for the child Session's first turn.
type SubAgentSpawnArgs struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

// ErrNestedSubAgent is returned by the driver when a child subagent
// attempts to spawn another subagent. v8.0 supports one level of nesting
// only — preventing arbitrary spawn depth is the simplest invariant that
// keeps cancel propagation and resource accounting tractable.
var ErrNestedSubAgent = errors.New("agentdriver: nested subagent spawn rejected — single-level only in v8.0")

// ErrNoSubAgentRunner is returned when a spawn_subagent tool call arrives
// but the Driver has no SubAgentRunner configured. Production wiring sets
// SubAgent at construction; tests that do not exercise subagents leave
// it nil and the driver synthesizes an error tool_use_completed so the
// LLM can recover.
var ErrNoSubAgentRunner = errors.New("agentdriver: spawn_subagent tool call arrived but Driver.SubAgent is nil")

// SubAgentRunner spawns and drives a child Session to completion in
// response to a spawn_subagent tool call. Implementations are responsible
// for the full lifecycle: constructing the child Session, calling
// agentcore.AttachSubAgent on the parent, publishing subagent.spawned,
// running the child Driver with IsSubAgent=true so it rejects further
// spawns, calling agentcore.ResolveSubAgent when the child completes,
// publishing subagent.completed, and returning the rendered content the
// parent turn renders as the tool result.
//
// The driver calls Spawn with the parent ctx so cancellation cascades
// naturally. The runner MUST honor ctx.Done(): on cancellation, resolve
// the SubAgentLink with VerdictCanceled, emit subagent.completed, and
// return SubAgentResult with IsError=true and a cancellation marker so
// the parent turn can render the cancellation honestly.
type SubAgentRunner interface {
	Spawn(ctx context.Context, parent agentcore.Session, parentTurn agentcore.TurnID, args SubAgentSpawnArgs) (SubAgentResult, error)
}

// SubAgentResult is the runner's return value. Session is the parent
// Session with the SubAgentLink resolved; the driver wires this back
// into the active turn. Content is the LLM-facing tool result text.
// Verdict mirrors the child Session's terminal verdict. IsError flags
// whether the synthetic tool_use_completed in the parent turn should be
// marked isError=true so the LLM treats the child's output as failure.
type SubAgentResult struct {
	Session agentcore.Session
	Content string
	Verdict agentcore.Verdict
	IsError bool
}
