package embedding

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSidecarRespawnsAfterFault proves the adapter self-heals: a fake sidecar
// that answers exactly one request then exits forces a mid-session process
// fault, and the second Embed must respawn the process and still return a
// vector — a crash costs one query, not FTS for the rest of the session.
func TestSidecarRespawnsAfterFault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake sidecar script needs a POSIX shell")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available for the fake sidecar")
	}

	// Each launch handshakes, answers one request, then exits — simulating a
	// crash after the first query.
	script := "#!/usr/bin/env bash\n" +
		"printf '{\"ready\":true,\"model\":\"fake\",\"dim\":2}\\n'\n" +
		"IFS= read -r _line || exit 0\n" +
		"printf '{\"id\":1,\"vectors\":[[1.0,0.0]]}\\n'\n" +
		"exit 0\n"

	path := filepath.Join(t.TempDir(), "fake-haft-embed")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}
	t.Setenv(sidecarBinaryEnv, path)
	t.Setenv(sharedSidecarEnv, "0")

	embedder, err := New(Config{Provider: ProviderLocal})
	if err != nil {
		t.Fatalf("New(local) with fake sidecar: %v", err)
	}
	t.Cleanup(func() { _ = embedder.Close() })

	ctx := context.Background()
	first, err := embedder.Embed(ctx, RoleQuery, []string{"hello"})
	if err != nil {
		t.Fatalf("first embed: %v", err)
	}
	if len(first) != 1 || len(first[0]) != 2 {
		t.Fatalf("first embed shape = %v, want one 2-dim vector", first)
	}

	// The process from the first call has exited; this call must respawn and
	// still succeed.
	second, err := embedder.Embed(ctx, RoleDocument, []string{"world"})
	if err != nil {
		t.Fatalf("second embed (after fault) should respawn and succeed: %v", err)
	}
	if len(second) != 1 || len(second[0]) != 2 {
		t.Fatalf("second embed shape = %v, want one 2-dim vector", second)
	}
}
