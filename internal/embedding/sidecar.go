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
	"sync"
)

// ErrSidecarUnavailable signals the haft-embed binary is not installed. Callers
// MUST degrade to FTS5+PPR recall on this error (decision invariant) — it is
// the expected state on a default install without the optional sidecar.
var ErrSidecarUnavailable = errors.New("embedding sidecar (haft-embed) not found")

const (
	sidecarBinaryName = "haft-embed"
	defaultLocalModel = "embeddinggemma-300m"
)

// sidecarAdapter is the local Embedder adapter: it owns a long-lived haft-embed
// child process (the model loads once) and serializes request/response lines
// over its stdio pipe. Implements the Embedder port.
type sidecarAdapter struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	mu     sync.Mutex
	nextID uint64
	desc   Descriptor
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

func newSidecarAdapter(cfg Config) (Embedder, error) {
	binary, ok := locateSidecar()
	if !ok {
		return nil, ErrSidecarUnavailable
	}

	model := cfg.Model
	if model == "" {
		model = defaultLocalModel
	}
	args := []string{"--model", model, "--cache-dir", resolveCacheDir(cfg.CacheDir)}
	if cfg.Dim > 0 {
		args = append(args, "--dim", strconv.Itoa(cfg.Dim))
	}

	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr // model-download progress / errors stay visible

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("sidecar stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sidecar stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sidecar: %w", err)
	}

	reader := bufio.NewReader(stdout)
	handshake, err := readHandshake(reader)
	if err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}

	return &sidecarAdapter{
		cmd:    cmd,
		stdin:  stdin,
		reader: reader,
		desc:   Descriptor{Provider: ProviderLocal, Model: handshake.Model, Dimensions: handshake.Dim},
	}, nil
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

	a.nextID++
	request := sidecarRequest{ID: a.nextID, Task: taskFor(role), Texts: texts}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode embed request: %w", err)
	}
	if _, err := a.stdin.Write(append(payload, '\n')); err != nil {
		return nil, fmt.Errorf("write embed request: %w", err)
	}

	line, err := a.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}
	var response sidecarResponse
	if err := json.Unmarshal(line, &response); err != nil {
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

func (a *sidecarAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	_ = a.stdin.Close() // EOF tells the sidecar to exit cleanly
	return a.cmd.Wait()
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
