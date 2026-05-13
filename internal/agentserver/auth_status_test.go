package agentserver

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/m0n0x41d/haft/internal/agentproto"
)

func TestAuthStatus_DefaultAnonymousPayload(t *testing.T) {
	srv, base, _ := newTestServer(t)
	defer func() { _ = srv.Shutdown(t.Context()) }()

	resp, err := http.Get(base + "/auth/status")
	if err != nil {
		t.Fatalf("GET /auth/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var payload agentproto.AuthStatusPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode: %v (raw=%q)", err, body)
	}
	if payload.Provider != "none" {
		t.Fatalf("default Provider = %q, want \"none\"", payload.Provider)
	}
	if payload.HasCredentials {
		t.Fatal("default HasCredentials must be false")
	}
}

func TestAuthStatus_InjectedProvider(t *testing.T) {
	srv, base, _ := newTestServer(t)
	defer func() { _ = srv.Shutdown(t.Context()) }()
	srv.AuthStatus = AuthStatusFunc(func() agentproto.AuthStatusPayload {
		return agentproto.AuthStatusPayload{
			Provider:       "anthropic",
			Model:          "claude-opus-4-7",
			HasCredentials: true,
			ExpiresAt:      "2026-08-13T00:00:00Z",
		}
	})

	resp, err := http.Get(base + "/auth/status")
	if err != nil {
		t.Fatalf("GET /auth/status: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var payload agentproto.AuthStatusPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode: %v (raw=%q)", err, body)
	}
	if payload.Provider != "anthropic" || payload.Model != "claude-opus-4-7" || !payload.HasCredentials || payload.ExpiresAt == "" {
		t.Fatalf("injected payload not surfaced: %+v", payload)
	}
}

func TestAuthStatus_SnakeCaseWireShape(t *testing.T) {
	srv, base, _ := newTestServer(t)
	defer func() { _ = srv.Shutdown(t.Context()) }()
	srv.AuthStatus = AuthStatusFunc(func() agentproto.AuthStatusPayload {
		return agentproto.AuthStatusPayload{Provider: "x", HasCredentials: true, ExpiresAt: "2026-01-01"}
	})
	resp, err := http.Get(base + "/auth/status")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// Raw key check: snake_case wire shape per agentproto convention.
	for _, key := range []string{`"has_credentials"`, `"expires_at"`, `"provider"`} {
		if !contains(body, key) {
			t.Fatalf("missing wire key %s in %q", key, body)
		}
	}
}

func contains(b []byte, s string) bool {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}
