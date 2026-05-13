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

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/agentserver"
	"github.com/m0n0x41d/haft/internal/agentstore"
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

	var sessionCounter atomic.Int64
	dispatcher := &agentserver.StoreDispatcher{
		Store: store,
		IDGen: func() agentcore.SessionID {
			id := uuid.NewString()
			if id == "" {
				id = fmt.Sprintf("sess-%d", sessionCounter.Add(1))
			}
			return agentcore.SessionID(id)
		},
		Now: time.Now,
	}

	srv := agentserver.NewServer("127.0.0.1:0", store, dispatcher, nil)
	srv.AuthStatus = agentserver.AuthStatusFunc(func() agentproto.AuthStatusPayload {
		// v8.0 dev path: no LLM driver wired here yet (production wiring
		// adapts internal/provider to agentdriver.Provider in a later
		// slice). Report honest "none" so the TUI renders a clear
		// "no credentials configured" banner.
		return agentproto.AuthStatusPayload{
			Provider:       "none",
			Model:          "",
			HasCredentials: false,
		}
	})

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
	cmd := exec.Command(bunPath, "run", entry)
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
