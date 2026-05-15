package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/config"
)

// realAuthStatus loads ~/.haft/config.yaml and resolves the active
// provider's credential state for the TUI's /auth/status endpoint.
// Used by both `haft v8 serve` and `haft agent --v8` so the home
// screen + status bar render the same auth + project context
// regardless of entrypoint.
func realAuthStatus() agentproto.AuthStatusPayload {
	root, branch := projectContextFromCwd()

	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return agentproto.AuthStatusPayload{
			Provider:       "none",
			Model:          "",
			HasCredentials: false,
			ProjectRoot:    root,
			GitBranch:      branch,
		}
	}
	model := cfg.Model
	providerID := config.ProviderForModel(model)
	if providerID == "" {
		configured := cfg.ConfiguredProviders()
		if len(configured) > 0 {
			providerID = configured[0]
		}
	}
	if providerID == "" {
		return agentproto.AuthStatusPayload{
			Provider:       "none",
			Model:          model,
			HasCredentials: false,
			ProjectRoot:    root,
			GitBranch:      branch,
		}
	}
	auth := cfg.GetAuth(providerID)
	has := auth.APIKey != "" || auth.AccessToken != ""
	payload := agentproto.AuthStatusPayload{
		Provider:       providerID,
		Model:          model,
		HasCredentials: has,
		ProjectRoot:    root,
		GitBranch:      branch,
	}
	if auth.ExpiresAt > 0 {
		payload.ExpiresAt = time.Unix(auth.ExpiresAt, 0).UTC().Format(time.RFC3339)
	}
	return payload
}

// projectContextFromCwd returns (project_root, git_branch) for the
// status bar. project_root is the cwd; git_branch is read via
// `git rev-parse --abbrev-ref HEAD` with a hard 200ms timeout so a
// stuck git invocation never blocks an /auth/status response. Both
// fields are best-effort — missing values fall through as "".
func projectContextFromCwd() (string, string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	// Resolve to absolute path so tilde / symlink shortcuts don't
	// confuse the renderer's path-shortening.
	abs, aerr := filepath.Abs(cwd)
	if aerr != nil {
		abs = cwd
	}
	branch := gitBranch(abs)
	return abs, branch
}

// gitBranch returns the current branch name, or "" when the path is
// not inside a git repo, when git is not on PATH, or when the
// command takes longer than 200ms. The status bar tolerates an empty
// branch field gracefully.
func gitBranch(root string) string {
	if root == "" {
		return ""
	}
	cmd := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Stderr = nil
	done := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		out, err := cmd.Output()
		done <- struct {
			out []byte
			err error
		}{out, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return ""
		}
		b := strings.TrimSpace(string(r.out))
		if b == "HEAD" {
			// detached HEAD — show the short hash instead
			hashCmd := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD")
			if hash, err := hashCmd.Output(); err == nil {
				return strings.TrimSpace(string(hash))
			}
			return ""
		}
		return b
	case <-time.After(200 * time.Millisecond):
		_ = cmd.Process.Kill()
		return ""
	}
}
