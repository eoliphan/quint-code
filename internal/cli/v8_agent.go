package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/m0n0x41d/haft/db"
	"github.com/m0n0x41d/haft/internal/agent"
	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentdriver"
	"github.com/m0n0x41d/haft/internal/agentdriver/providers"
	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/agentserver"
	"github.com/m0n0x41d/haft/internal/agentstore"
	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/config"
	"github.com/m0n0x41d/haft/internal/lsp"
	"github.com/m0n0x41d/haft/internal/mcpclient"
	"github.com/m0n0x41d/haft/internal/project"
	"github.com/m0n0x41d/haft/internal/tools"
)

// runAgentV8 is the v8 stack spawn lifecycle. Boots agentserver on
// 127.0.0.1:0, captures the chosen port, spawns the Bun TUI process
// with HAFT_AGENT_PORT in env, propagates stdin / stdout / stderr,
// waits for the TUI to exit, then shuts the server down.
//
// SIGINT / SIGTERM to the parent cascade to the Bun child via the
// process group (Unix); the server's Shutdown then drains and exits.
// Leak invariant: no orphan Bun nor agentserver after parent exit.
func runAgentV8(projectRoot string) error {
	storeRoot := filepath.Join(projectRoot, ".haft", "agent-store")
	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		return fmt.Errorf("create agent store root: %w", err)
	}
	store, err := agentstore.NewStore(storeRoot)
	if err != nil {
		return fmt.Errorf("open agentstore: %w", err)
	}
	defer func() { _ = store.Close() }()

	idGen := newSessionIDGen()

	dispatcher, drvCleanup, drvErr := buildDispatcher(store, idGen, projectRoot)
	if drvErr != nil {
		// Credentials missing or provider construction failed. Fall back to
		// session lifecycle only — the home screen will surface the
		// "no credentials" state via /auth/status and `haft login` fixes it.
		fmt.Fprintf(os.Stderr, "haft agent --v8: driver init skipped (%v); turns will be rejected until `haft login` succeeds\n", drvErr)
		dispatcher = &agentserver.StoreDispatcher{Store: store, IDGen: idGen, Now: time.Now}
	}
	if drvCleanup != nil {
		defer drvCleanup()
	}

	srv := agentserver.NewServer("127.0.0.1:0", store, dispatcher, nil)
	srv.AuthStatus = agentserver.AuthStatusFunc(realAuthStatus)

	// agentdriver.Dispatcher publishes events through the same Hub the
	// server hosts; rewire it now that the server's Hub is final. The
	// StoreDispatcher fallback path has no Hub coupling.
	if drv, ok := dispatcher.(*agentdriver.Dispatcher); ok {
		drv.Driver.Sink = &agentdriver.CombinedSink{
			Store: store,
			Broadcast: func(ev agentproto.AgentEvent) {
				srv.Hub.Publish(ev)
			},
		}
	}

	boundAddr, srvErrCh, err := srv.Start()
	if err != nil {
		return fmt.Errorf("start agentserver: %w", err)
	}

	port := portFromAddr(boundAddr)
	if port == 0 {
		return fmt.Errorf("agentserver returned unparseable addr %q", boundAddr)
	}

	tuiEntry, tuiCwd, err := findV8TUIEntry(projectRoot)
	if err != nil {
		_ = srv.Shutdown(context.Background())
		return err
	}

	tuiCmd, err := spawnV8TUI(tuiEntry, tuiCwd, port)
	if err != nil {
		_ = srv.Shutdown(context.Background())
		return err
	}

	// Race: wait for Bun TUI exit OR signal OR server fatal error.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	tuiDone := make(chan error, 1)
	go func() { tuiDone <- tuiCmd.Wait() }()

	var firstErr error
	select {
	case sig := <-sigCh:
		// Forward to the Bun process group so it tears down cleanly.
		_ = tuiCmd.Process.Signal(sig)
		select {
		case <-tuiDone:
		case <-time.After(2 * time.Second):
			_ = tuiCmd.Process.Kill()
			<-tuiDone
		}
	case err := <-tuiDone:
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) && ee.ExitCode() != 0 {
				firstErr = fmt.Errorf("tui exited with code %d", ee.ExitCode())
			} else {
				firstErr = fmt.Errorf("tui wait: %w", err)
			}
		}
	case err := <-srvErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			firstErr = fmt.Errorf("agentserver: %w", err)
		}
		_ = tuiCmd.Process.Signal(syscall.SIGTERM)
		<-tuiDone
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("agentserver shutdown: %w", err)
	}
	return firstErr
}

// newSessionIDGen returns a SessionID generator that prefers uuid and
// falls back to a monotonically increasing counter when the random
// reader is unavailable. Shared by Driver and Store dispatchers.
func newSessionIDGen() func() agentcore.SessionID {
	var counter atomic.Int64
	return func() agentcore.SessionID {
		id := uuid.NewString()
		if id == "" {
			id = fmt.Sprintf("sess-%d", counter.Add(1))
		}
		return agentcore.SessionID(id)
	}
}

// buildDispatcher constructs the production DriverDispatcher when
// credentials resolve cleanly. Returns the wired Dispatcher, a
// cleanup closure that the caller defers (closes the DB handle
// opened for the haft kernel tools), and an error path the caller
// uses to fall back to lifecycle-only mode.
//
// What this wires:
//   - OpenAI / codex provider (reads ~/.haft/config.yaml)
//   - Full FPF system prompt via agent.BuildSystemPrompt(Lemniscate=true)
//   - project context + workflow prefix — same prompt the legacy
//     `haft agent` builds.
//   - tools.Registry pre-populated with builtin tools (bash, read,
//     write, edit, multiedit, glob, grep, fetch) + haft kernel tools
//     (haft_problem, haft_solution, haft_decision, haft_query,
//     haft_refresh, haft_note) calling the underlying Go functions
//     directly — no MCP/JSON-RPC veneer.
//   - RegistryDispatcher as the agentdriver.ToolDispatcher; write
//     tools route through the PermissionGate, read tools auto-approve.
//
// Non-OpenAI providers return an explicit error rather than guessing.
// The Anthropic adapter is a follow-up slice.
func buildDispatcher(
	store *agentstore.Store,
	idGen func() agentcore.SessionID,
	projectRoot string,
) (agentserver.Dispatcher, func(), error) {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return nil, nil, fmt.Errorf("config.Load: %w", err)
	}
	model := cfg.Model
	if model == "" {
		return nil, nil, errors.New("no model configured (set HAFT_MODEL or run `haft login`)")
	}
	providerID := config.ProviderForModel(model)
	if providerID == "" {
		return nil, nil, fmt.Errorf("cannot determine provider for model %q", model)
	}
	if providerID != "openai" && providerID != "anthropic" {
		return nil, nil, fmt.Errorf("v8 driver supports openai/codex and anthropic; got provider %q for model %q", providerID, model)
	}
	auth := cfg.GetAuth(providerID)
	if auth.APIKey == "" && auth.AccessToken == "" {
		return nil, nil, fmt.Errorf("no credentials for provider %q — run `haft login`", providerID)
	}

	// Open the project DB so haft kernel tools (problem/solution/
	// decision/query/refresh/note) can read+write the artifact store
	// the same way `haft agent` does. The handle is owned by this
	// function; the cleanup closure surfaces back to runAgentV8 so
	// the caller can defer-close at the right scope.
	haftDir := filepath.Join(projectRoot, ".haft")
	projCfg, perr := project.Load(haftDir)
	if perr != nil || projCfg == nil {
		return nil, nil, fmt.Errorf("project not initialized — run `haft init` first (%v)", perr)
	}
	dbPath, derr := projCfg.DBPath()
	if derr != nil {
		return nil, nil, fmt.Errorf("resolve DB path: %w", derr)
	}
	database, derr := db.NewStore(dbPath)
	if derr != nil {
		return nil, nil, fmt.Errorf("open DB: %w", derr)
	}
	cleanup := func() { _ = database.Close() }

	artStore := artifact.NewStore(database.GetRawDB())

	toolRegistry := tools.NewRegistry(projectRoot)
	toolRegistry.Register(tools.NewHaftProblemTool(artStore, haftDir))
	toolRegistry.Register(tools.NewHaftSolutionTool(artStore, haftDir, toolRegistry))
	toolRegistry.Register(tools.NewHaftDecisionTool(artStore, haftDir, projectRoot, toolRegistry))
	toolRegistry.Register(tools.NewHaftQueryTool(artStore, buildFPFSearchFunc()))
	toolRegistry.Register(tools.NewHaftRefreshTool(artStore, haftDir, projectRoot))
	toolRegistry.Register(tools.NewHaftNoteTool(artStore, haftDir))

	// LSP tools — same shape as legacy haft agent. The manager spins
	// up language servers lazily on the first diagnostics/references
	// call; nothing extra to start here. Status callbacks go nowhere
	// in v8 (no Bus); a future slice can pipe server state to a
	// status-bar item.
	lspManager := lsp.NewManager(projectRoot, lsp.DefaultConfigs())
	toolRegistry.Register(tools.NewLSPDiagnosticsTool(lspManager, projectRoot))
	toolRegistry.Register(tools.NewLSPReferencesTool(lspManager, projectRoot))
	toolRegistry.Register(tools.NewLSPRestartTool(lspManager))

	// Worktree isolation tools — agent can spawn a temporary git
	// worktree, work there, and exit it (committing or discarding).
	// Same shape the legacy haft agent registers.
	worktreeState := tools.NewWorktreeState(projectRoot)
	toolRegistry.Register(tools.NewEnterWorktreeTool(worktreeState))
	toolRegistry.Register(tools.NewExitWorktreeTool(worktreeState))

	// External MCP servers. Reads ~/.claude.json mcpServers +
	// project-local .mcp.json. Each server's advertised tools are
	// surfaced into toolRegistry with a "<server>__" prefix so two
	// servers can't collide on a tool name. Per-server start
	// failures are logged to stderr but don't abort the agent —
	// the operator still gets the rest of the stack.
	mcpCfgs, _ := mcpclient.LoadConfig(projectRoot)
	mcpMgr := mcpclient.Start(context.Background(), mcpCfgs)
	for _, te := range mcpMgr.ToolExecutors() {
		toolRegistry.Register(te)
	}
	prevCleanup := cleanup
	cleanup = func() {
		_ = mcpMgr.Close()
		prevCleanup()
	}

	// Build the FPF-aware system prompt. project.LoadWorkflow may
	// return nil if no workflow is configured; we tolerate that
	// silently — the prefix is purely additive.
	cwd, _ := os.Getwd()
	systemPrompt := agent.BuildSystemPrompt(agent.PromptConfig{
		ProjectRoot: projectRoot,
		Cwd:         cwd,
		Lemniscate:  true,
	}) + agent.LoadProjectContext(projectRoot)
	if workflow, werr := project.LoadWorkflow(projectRoot); werr == nil && workflow != nil {
		systemPrompt = workflow.PromptPrefix() + "\n\n" + systemPrompt
	}

	var llm agentdriver.Provider
	if providerID == "anthropic" {
		a, aerr := providers.NewAnthropicAdapter(model)
		if aerr != nil {
			cleanup()
			return nil, nil, fmt.Errorf("anthropic adapter: %w", aerr)
		}
		a.WithInstructions(systemPrompt).WithTools(toolRegistry.List())
		llm = a
	} else {
		o, oerr := providers.NewOpenAIAdapter(model)
		if oerr != nil {
			cleanup()
			return nil, nil, fmt.Errorf("OpenAI adapter: %w", oerr)
		}
		o.WithInstructions(systemPrompt).WithTools(toolRegistry.List())
		llm = o
	}

	toolDispatcher := &providers.RegistryDispatcher{Registry: toolRegistry}

	driver := &agentdriver.Driver{
		Provider: llm,
		Tools:    toolDispatcher,
		IDGen: func(kind string) string {
			id := uuid.NewString()
			if id == "" {
				return fmt.Sprintf("%s-fallback", kind)
			}
			return fmt.Sprintf("%s-%s", kind, id)
		},
		Now: time.Now,
	}
	perms := agentdriver.NewPermissionGate()

	// NewDispatcher wires Driver.Sink against a placeholder Hub it
	// constructs from the (nil, fn) closure below; the real Hub is
	// substituted in runAgentV8 once srv.Hub is the canonical one.
	hubPlaceholder := agentserver.NewHub()
	disp := agentdriver.NewDispatcher(store, hubPlaceholder, driver, perms)
	disp.IDGen = idGen
	disp.Now = time.Now
	return disp, cleanup, nil
}

// findV8TUIEntry locates the v8 TUI entrypoint AND its bun cwd. The
// cwd matters because @opentui/core ships a platform-specific native
// module (@opentui/core-<os>-<arch>) that bun resolves from the cwd's
// node_modules at runtime — a bare single-file bundle without an
// adjacent node_modules cannot start.
//
// Search order:
//  1. ~/.haft/tui-v8/current/ as an installed package directory
//     (must contain src/main.tsx + node_modules — t17 install pipeline
//     will produce this; broken single-file bundle from the v8.0 alpha
//     install is no longer supported).
//  2. <projectRoot>/tui/src/main.tsx in the Haft repo (dev mode —
//     bun resolves modules from <projectRoot>/tui/node_modules).
//
// Returns (entry, cwd, error). The caller passes cwd as cmd.Dir so
// bun's module resolver finds the native binding.
func findV8TUIEntry(projectRoot string) (string, string, error) {
	installedDir := filepath.Join(homeDir(), ".haft", "tui-v8", "current")
	installedEntry := filepath.Join(installedDir, "src", "main.tsx")
	installedNodeModules := filepath.Join(installedDir, "node_modules")

	if _, err := os.Stat(installedEntry); err == nil {
		if _, err := os.Stat(installedNodeModules); err == nil {
			return installedEntry, installedDir, nil
		}
	}

	repoTuiDir := filepath.Join(projectRoot, "tui")
	repoEntry := filepath.Join(repoTuiDir, "src", "main.tsx")
	repoNodeModules := filepath.Join(repoTuiDir, "node_modules")

	if _, err := os.Stat(repoEntry); err == nil {
		if _, err := os.Stat(repoNodeModules); err == nil {
			return repoEntry, repoTuiDir, nil
		}
		return "", "", fmt.Errorf("v8 TUI source at %s but node_modules missing — run `cd %s && bun install`", repoEntry, repoTuiDir)
	}

	return "", "", fmt.Errorf("v8 TUI not found; expected at %s (installed) or %s (haft repo dev). For v8.0 alpha, run `haft agent --v8` from the haft source tree", installedDir, repoTuiDir)
}

// spawnV8TUI starts the Bun process with HAFT_AGENT_PORT in env and
// cmd.Dir set to the TUI package root so bun resolves node_modules
// (including the platform-specific @opentui/core-<os>-<arch> binding)
// correctly. stdin / stdout / stderr inherit from the parent so the
// TUI owns the terminal directly.
func spawnV8TUI(entry, cwd string, port int) (*exec.Cmd, error) {
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		return nil, fmt.Errorf("bun not found in PATH (required for v8 TUI)")
	}
	// --conditions=browser forces Bun's package.json `exports`
	// resolver to pick the SolidJS client/reactive runtime
	// (dist/solid.js → real createSignal/createContext + effect graph)
	// instead of its SSR build (dist/server.js, which exposes stub
	// no-op implementations and crashes the moment a context provider
	// or signal updates). Bun's `--conditions` ADDS conditions on top
	// of its defaults (bun, node, import), and the resolver picks the
	// FIRST matching export key in JSON declaration order. solid-js's
	// exports declare "browser" BEFORE "node", so passing
	// --conditions=browser makes it win over the node→server.js path.
	// @opentui/solid is OpenTUI's custom Solid renderer; it mounts the
	// reactive tree against an OpenTUI Renderable graph rather than
	// the DOM, so the "browser" build is the correct target even
	// though no browser is involved.
	cmd := exec.Command(bunPath, "run", "--conditions=browser", entry)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), fmt.Sprintf("HAFT_AGENT_PORT=%d", port))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn bun: %w", err)
	}
	return cmd, nil
}
