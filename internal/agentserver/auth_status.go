package agentserver

import (
	"net/http"

	"github.com/m0n0x41d/haft/internal/agentproto"
)

// AuthStatusProvider reports the current auth state for the
// /auth/status endpoint. Production wiring delegates to the configured
// LLM provider (anthropic, openai, claude-code). When Server.AuthStatus
// is nil the endpoint returns a default anonymous payload so the TUI
// can still render an honest "no credentials configured" header in
// dev / test deployments.
type AuthStatusProvider interface {
	Status() agentproto.AuthStatusPayload
}

// AuthStatusFunc adapts a plain function into AuthStatusProvider.
type AuthStatusFunc func() agentproto.AuthStatusPayload

func (f AuthStatusFunc) Status() agentproto.AuthStatusPayload { return f() }

func (s *Server) handleAuthStatus(w http.ResponseWriter, _ *http.Request) {
	payload := agentproto.AuthStatusPayload{
		Provider:       "none",
		Model:          "",
		HasCredentials: false,
	}
	if s.AuthStatus != nil {
		payload = s.AuthStatus.Status()
	}
	writeJSON(w, http.StatusOK, payload)
}
