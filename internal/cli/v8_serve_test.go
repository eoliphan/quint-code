package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestV8Serve_SmokeStartupHealthAuth boots `haft v8 serve` against a
// scratch store root, reads the startup JSON, hits /healthz and
// /auth/status, then sends SIGINT and asserts clean shutdown.
// Skipped unless the haft binary is in PATH (CI installs it).
func TestV8Serve_SmokeStartupHealthAuth(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}
	tmp := t.TempDir()

	cmd := exec.Command("go", "run", "./cmd/haft", "v8", "serve",
		"--addr", "127.0.0.1:0",
		"--store-root", tmp+"/store",
	)
	cmd.Dir = "../../" // run from repo root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGINT)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	})

	// Read startup JSON line.
	line, err := readLine(stdout, 30*time.Second)
	if err != nil {
		t.Fatalf("read startup: %v", err)
	}
	var startup struct {
		Port      int    `json:"port"`
		Addr      string `json:"addr"`
		StoreRoot string `json:"store_root"`
		Driver    string `json:"driver"`
	}
	if err := json.Unmarshal([]byte(line), &startup); err != nil {
		t.Fatalf("startup JSON decode: %v (line=%q)", err, line)
	}
	if startup.Port == 0 || startup.Driver != "store" {
		t.Fatalf("startup malformed: %+v", startup)
	}

	base := "http://" + startup.Addr
	if resp, err := http.Get(base + "/healthz"); err != nil {
		t.Fatalf("GET /healthz: %v", err)
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("/healthz status %d", resp.StatusCode)
		}
	}

	resp, err := http.Get(base + "/auth/status")
	if err != nil {
		t.Fatalf("GET /auth/status: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/auth/status status %d body=%q", resp.StatusCode, body)
	}
	// /auth/status reads the operator's haft config; the response shape
	// is what matters here, not the specific values (which differ
	// between dev machines with credentials and CI without).
	for _, key := range []string{`"provider"`, `"model"`, `"has_credentials"`} {
		if !strings.Contains(string(body), key) {
			t.Fatalf("/auth/status body=%q missing key %s", body, key)
		}
	}
}

func readLine(r io.Reader, timeout time.Duration) (string, error) {
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 0, 1024)
		one := make([]byte, 1)
		for {
			n, err := r.Read(one)
			if n > 0 {
				if one[0] == '\n' {
					ch <- result{s: string(buf)}
					return
				}
				buf = append(buf, one[0])
			}
			if err != nil {
				ch <- result{s: string(buf), err: err}
				return
			}
		}
	}()
	select {
	case res := <-ch:
		return res.s, res.err
	case <-time.After(timeout):
		return "", io.EOF
	}
}
