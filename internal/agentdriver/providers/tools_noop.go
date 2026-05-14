package providers

import (
	"context"
	"fmt"

	"github.com/m0n0x41d/haft/internal/agentdriver"
)

// NoOpTools is the ToolDispatcher v8.0 alpha ships with: every tool the
// LLM asks for is denied at the authorize step. This is the SAFE
// default — the type system forces the driver to have a ToolDispatcher
// configured, but exposing real tools without per-tool permission UX
// is the larger work that the agentcore PermissionGate + agentdriver
// tools layer can carry once we wire concrete tool surfaces.
//
// LLMs occasionally hallucinate tool calls even when none are
// advertised; this dispatcher closes that path safely instead of
// crashing the turn.
type NoOpTools struct{}

var _ agentdriver.ToolDispatcher = NoOpTools{}

// Authorize denies every tool. The driver will then emit a synthetic
// tool_use_completed with isError=true so the LLM can recover.
func (NoOpTools) Authorize(_ context.Context, _ string, _ []byte) agentdriver.AuthorisationVerdict {
	return agentdriver.AuthorisationDenied
}

// Run is unreachable for NoOpTools — Authorize never returns Granted —
// but the interface requires it. Returning an explicit error makes a
// future regression that calls Run anyway loud rather than silent.
func (NoOpTools) Run(_ context.Context, name string, _ []byte) (string, bool, error) {
	return "", true, fmt.Errorf("agentdriver/providers: NoOpTools.Run called for %q (v8.0 alpha exposes no tools)", name)
}
