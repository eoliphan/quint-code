package providers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/m0n0x41d/haft/internal/agent"
	"github.com/m0n0x41d/haft/internal/agentdriver"
	"github.com/m0n0x41d/haft/internal/tools"
)

// RegistryDispatcher adapts the legacy *tools.Registry — the same
// registry the v7 haft agent uses — to agentdriver.ToolDispatcher.
//
// Authorize policy: every tool that the v7 ReadOnlyRegistry strips
// out (bash, write, edit, multiedit, haft_problem, haft_solution,
// haft_decision, haft_commission, haft_note) requires an operator
// prompt; everything else (read, glob, grep, lsp_*, haft_query,
// haft_refresh, etc.) auto-approves.
//
// Run hands argsJSON to Registry.Execute and folds the resulting
// agent.ToolResult into the (content, isError) shape the driver
// expects. Errors returned by Execute are translated into a
// tool-side failure (isError=true, content=err.Error()) so the LLM
// can reason about the failure instead of crashing the turn.
type RegistryDispatcher struct {
	Registry *tools.Registry
}

var _ agentdriver.ToolDispatcher = (*RegistryDispatcher)(nil)

// writeClassTools mirrors tools.Registry.ReadOnlyRegistry's exclusion
// set. Kept as a private constant rather than imported so a future
// per-tool policy table (e.g. "edit auto-approves under
// .haft/scratch/*") can be added without modifying the upstream
// registry's read-only filter.
var writeClassTools = map[string]bool{
	"bash":            true,
	"write":           true,
	"edit":            true,
	"multiedit":       true,
	"haft_problem":    true,
	"haft_solution":   true,
	"haft_decision":   true,
	"haft_commission": true,
	"haft_note":       true,
}

// Authorize classifies a tool call. write-class tools require an
// operator prompt; read-class tools auto-approve. Unknown tool names
// surface as denied so a hallucinated tool call collapses into a
// synthetic tool_use_completed instead of waiting forever on a
// permission gate that will never fire.
func (d *RegistryDispatcher) Authorize(_ context.Context, name string, _ []byte) agentdriver.AuthorisationVerdict {
	if d.Registry == nil {
		return agentdriver.AuthorisationDenied
	}
	if !d.knownTool(name) {
		return agentdriver.AuthorisationDenied
	}
	if writeClassTools[name] {
		return agentdriver.AuthorisationRequiresPrompt
	}
	return agentdriver.AuthorisationGranted
}

// Run dispatches to the registry. The provider hands us args as
// []byte; Registry.Execute wants a JSON string, so we re-string it
// (and validate that it's valid JSON — the OpenAI Responses API
// occasionally emits empty args as "" or whitespace which would fail
// downstream parsers).
func (d *RegistryDispatcher) Run(ctx context.Context, name string, args []byte) (string, bool, error) {
	if d.Registry == nil {
		return "", true, fmt.Errorf("agentdriver/providers: RegistryDispatcher.Registry is nil")
	}
	argsJSON := normaliseToolArgs(args)
	result, err := d.Registry.Execute(ctx, name, argsJSON)
	if err != nil {
		return err.Error(), true, nil
	}
	return result.DisplayText, false, nil
}

// Schemas exposes the registry's tool schemas in the shape the
// provider adapter forwards to the OpenAI Responses API. Returned in
// registration order so the LLM sees the same tool list every turn.
func (d *RegistryDispatcher) Schemas() []agent.ToolSchema {
	if d.Registry == nil {
		return nil
	}
	return d.Registry.List()
}

func (d *RegistryDispatcher) knownTool(name string) bool {
	for _, s := range d.Registry.List() {
		if s.Name == name {
			return true
		}
	}
	return false
}

// normaliseToolArgs guarantees a JSON-decodable string. The LLM may
// emit "" for a no-argument call; downstream parsers in
// internal/tools choke on that and the driver would surface a
// tool-side failure for a perfectly valid invocation. Replace empty
// / whitespace input with "{}" so the registry parses a zero-arg
// call correctly.
func normaliseToolArgs(args []byte) string {
	if len(args) == 0 {
		return "{}"
	}
	// Cheap whitespace probe; avoids unmarshal allocation for the
	// hot path of well-formed JSON.
	allWS := true
	for _, b := range args {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			allWS = false
			break
		}
	}
	if allWS {
		return "{}"
	}
	// Validate so a broken arg blob doesn't crash registry parsers.
	if !json.Valid(args) {
		return "{}"
	}
	return string(args)
}
