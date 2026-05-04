package agentdriver

import "context"

// ToolDispatcher abstracts tool authorisation + execution. The driver
// calls Authorize before invoking a tool; if Authorize returns
// AuthorisationDeferred, the driver opens a Permission record and waits
// on the dispatcher's external resolution (the HTTP layer surfaces the
// permission via SSE and resolves it via POST).
//
// The split exists because some tools — read, glob, grep — are pre-approved
// by policy and never need a permission round-trip. Bash, write, edit
// always require approval in checkpointed mode. The dispatcher is the
// single source of truth for that policy.
type ToolDispatcher interface {
	Authorize(ctx context.Context, name string, args []byte) AuthorisationVerdict

	// Run executes the tool. Called after Authorize returns
	// AuthorisationGranted. Returns the rendered content (passed to the
	// LLM as the tool result) and isError indicating tool-layer failure
	// (NOT non-zero exit codes — those are part of normal output).
	Run(ctx context.Context, name string, args []byte) (content string, isError bool, err error)
}

// AuthorisationVerdict is the dispatcher's pre-execution verdict.
type AuthorisationVerdict int

const (
	// AuthorisationGranted means the tool may run without operator approval.
	AuthorisationGranted AuthorisationVerdict = iota
	// AuthorisationRequiresPrompt means the driver must open a Permission
	// record and wait on the dispatcher's PermissionGate before executing.
	AuthorisationRequiresPrompt
	// AuthorisationDenied means the tool is forbidden by policy. The driver
	// emits a synthetic tool_use_completed with isError=true and a denial
	// message so the LLM can reason about it.
	AuthorisationDenied
)
