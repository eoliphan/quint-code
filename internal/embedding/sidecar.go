package embedding

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/m0n0x41d/haft/logger"
)

// ErrSidecarUnavailable signals the haft-embed binary is not installed. Callers
// MUST degrade to FTS5+PPR recall on this error (decision invariant) — it is
// the expected state on a default install without the optional sidecar.
var ErrSidecarUnavailable = errors.New("embedding sidecar (haft-embed) not found")

const (
	sidecarBinaryName = "haft-embed"
	// DefaultLocalModel is the int8-quantized EmbeddingGemma: identical retrieval
	// quality to the fp32 model (measured R@10 parity) but a ~304MB first-use
	// download instead of ~1.1GB. The shipped FPF vectors are baked under this
	// same model id, so the runtime query embedder must default to it to match the
	// baked contract (a mismatch silently degrades semantic search to FTS).
	DefaultLocalModel = "embeddinggemma-300m-q"
)

// sidecarAdapter is the stdio local Embedder adapter: it owns a long-lived
// haft-embed child process (the model loads once) and serializes
// request/response lines over its stdio pipe. It is self-healing — if the
// process faults mid-session it respawns once and retries, so a sidecar crash
// costs a single failed query, not FTS5+PPR for the rest of the session.
// Implements the Embedder port.
type sidecarAdapter struct {
	binary string
	args   []string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	nextID uint64
	desc   Descriptor
	alive  bool
	closed bool
}

type sidecarRequest struct {
	ID    uint64   `json:"id"`
	Task  string   `json:"task"`
	Texts []string `json:"texts"`
}

type sidecarResponse struct {
	ID      uint64      `json:"id"`
	Vectors [][]float32 `json:"vectors"`
	Error   string      `json:"error"`
}

type sidecarHandshake struct {
	Ready bool   `json:"ready"`
	Model string `json:"model"`
	Dim   int    `json:"dim"`
	Error string `json:"error"`
}

type sidecarSpec struct {
	Model    string
	CacheDir string
	Dim      int
	Args     []string
}

func newSidecarAdapter(cfg Config) (Embedder, error) {
	binary, ok := locateSidecar()
	if !ok {
		return nil, ErrSidecarUnavailable
	}

	spec := sidecarSpecFromConfig(cfg)
	if sharedSidecarEnabled() {
		adapter, err := newSharedSidecarAdapter(binary, spec)
		if err == nil {
			return adapter, nil
		}
		logger.Info().Err(err).Msg("shared embedding sidecar unavailable — falling back to stdio child")
	}
	return newStdioSidecarAdapter(binary, spec)
}

func sidecarSpecFromConfig(cfg Config) sidecarSpec {
	model := cfg.Model
	if model == "" {
		model = DefaultLocalModel
	}
	cacheDir := resolveCacheDir(cfg.CacheDir)
	args := []string{"--model", model, "--cache-dir", cacheDir}
	if cfg.Dim > 0 {
		args = append(args, "--dim", strconv.Itoa(cfg.Dim))
	}
	return sidecarSpec{Model: model, CacheDir: cacheDir, Dim: cfg.Dim, Args: args}
}

func newStdioSidecarAdapter(binary string, spec sidecarSpec) (Embedder, error) {
	adapter := &sidecarAdapter{binary: binary, args: spec.Args}
	adapter.mu.Lock()
	err := adapter.spawn()
	adapter.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return adapter, nil
}

// spawn launches (or relaunches) the sidecar process and reads its handshake,
// installing the live pipes. Caller must hold a.mu.
func (a *sidecarAdapter) spawn() error {
	cmd := exec.Command(a.binary, a.args...)
	cmd.Stderr = os.Stderr // model-download progress / errors stay visible

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("sidecar stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("sidecar stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start sidecar: %w", err)
	}

	reader := bufio.NewReader(stdout)
	handshake, err := readHandshake(reader)
	if err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	a.cmd = cmd
	a.stdin = stdin
	a.reader = reader
	a.desc = Descriptor{Provider: ProviderLocal, Model: handshake.Model, Dimensions: handshake.Dim}
	a.alive = true
	return nil
}

// readHandshake blocks on the sidecar's first line. On a cold start this waits
// out the one-time model download, so no read deadline is imposed here.
func readHandshake(reader *bufio.Reader) (sidecarHandshake, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return sidecarHandshake{}, fmt.Errorf("sidecar handshake: %w", err)
	}
	var handshake sidecarHandshake
	if err := json.Unmarshal(line, &handshake); err != nil {
		return sidecarHandshake{}, fmt.Errorf("sidecar handshake decode: %w", err)
	}
	if !handshake.Ready {
		return sidecarHandshake{}, fmt.Errorf("sidecar failed to start: %s", handshake.Error)
	}
	return handshake, nil
}

func (a *sidecarAdapter) Descriptor() Descriptor {
	return a.desc
}

func (a *sidecarAdapter) Embed(ctx context.Context, role Role, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, errors.New("embedding sidecar closed")
	}

	vectors, err := a.embedOnce(role, texts)
	if err == nil {
		return vectors, nil
	}
	if a.alive {
		// Process is fine — a request-level error (bad response / count
		// mismatch). Respawning would not help, so surface it.
		return nil, err
	}

	// The process faulted mid-session. Respawn once and retry so a crash costs
	// a single failed query, not FTS5+PPR for the rest of the session.
	logger.Warn().Err(err).Msg("embedding sidecar faulted — respawning")
	a.killLocked()
	if respawnErr := a.spawn(); respawnErr != nil {
		return nil, fmt.Errorf("sidecar respawn failed: %w", respawnErr)
	}
	return a.embedOnce(role, texts)
}

// embedOnce performs one request/response round-trip. An IO/protocol fault marks
// the process dead (alive=false) so Embed can respawn; a valid error response
// leaves the process alive.
func (a *sidecarAdapter) embedOnce(role Role, texts []string) ([][]float32, error) {
	if !a.alive {
		return nil, errors.New("embedding sidecar not running")
	}

	a.nextID++
	request := sidecarRequest{ID: a.nextID, Task: taskFor(role), Texts: texts}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode embed request: %w", err)
	}
	if _, err := a.stdin.Write(append(payload, '\n')); err != nil {
		a.alive = false
		return nil, fmt.Errorf("write embed request: %w", err)
	}

	line, err := a.reader.ReadBytes('\n')
	if err != nil {
		a.alive = false
		return nil, fmt.Errorf("read embed response: %w", err)
	}
	var response sidecarResponse
	if err := json.Unmarshal(line, &response); err != nil {
		a.alive = false
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("sidecar embed: %s", response.Error)
	}
	if len(response.Vectors) != len(texts) {
		return nil, fmt.Errorf("sidecar returned %d vectors for %d texts", len(response.Vectors), len(texts))
	}
	return response.Vectors, nil
}

// killLocked force-terminates the current process. Caller must hold a.mu.
func (a *sidecarAdapter) killLocked() {
	if a.stdin != nil {
		_ = a.stdin.Close()
	}
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
		_ = a.cmd.Wait()
	}
	a.alive = false
}

func (a *sidecarAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	if a.stdin != nil {
		_ = a.stdin.Close() // EOF tells the sidecar to exit cleanly
	}
	if a.cmd != nil {
		return a.cmd.Wait()
	}
	return nil
}

func taskFor(role Role) string {
	if role == RoleQuery {
		return "query"
	}
	return "document"
}

func resolveCacheDir(configured string) string {
	if configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".haft", "models")
}

// sidecarBinaryEnv overrides binary discovery with an explicit path.
const sidecarBinaryEnv = "HAFT_EMBED_BIN"

// sharedSidecarEnv disables per-user daemon sharing when set to 0/false/no.
const sharedSidecarEnv = "HAFT_EMBED_SHARED"

func sharedSidecarEnabled() bool {
	value := strings.TrimSpace(os.Getenv(sharedSidecarEnv))
	switch strings.ToLower(value) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// locateSidecar resolves the haft-embed binary: an explicit HAFT_EMBED_BIN
// override, then the installed runtime location (mirroring open-sleigh under
// ~/.haft/runtimes), then dev build outputs when running from the haft repo,
// then PATH.
func locateSidecar() (string, bool) {
	if override := os.Getenv(sidecarBinaryEnv); override != "" {
		if isExecutableFile(override) {
			return override, true
		}
		return "", false
	}
	for _, candidate := range sidecarCandidates() {
		if isExecutableFile(candidate) {
			return candidate, true
		}
	}
	if resolved, err := exec.LookPath(sidecarBinaryName); err == nil {
		return resolved, true
	}
	return "", false
}

func sidecarCandidates() []string {
	candidates := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		base := filepath.Join(home, ".haft", "runtimes", "haft-embed", "current")
		candidates = append(candidates,
			filepath.Join(base, "bin", sidecarBinaryName),
			filepath.Join(base, sidecarBinaryName),
		)
	}
	candidates = append(candidates,
		filepath.Join("embed-sidecar", "target", "release", sidecarBinaryName),
		filepath.Join("embed-sidecar", "target", "debug", sidecarBinaryName),
	)
	return candidates
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}
