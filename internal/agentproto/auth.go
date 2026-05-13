package agentproto

// AuthStatusPayload is the body of GET /auth/status. The TUI consumes it
// on first paint so the session header can show "signed in as <model>"
// or "no credentials configured" without making the operator wait for a
// turn to fail. Snake_case JSON shape matches the rest of the agentproto
// wire (see ModelChoice / SessionMeta tagging from the v7.1.0 hardening
// pass, fix-review commit d5001d60).
//
// Provider identifies which backing LLM the auth applies to ("anthropic",
// "openai", "claude-code"). Model is the resolved model id the session
// will use. HasCredentials reports whether usable credentials were found
// at server startup; ExpiresAt is RFC3339 only when the provider exposes
// an expiry signal (OAuth bearer tokens for ChatGPT-Sub, anything else
// leaves it empty).
type AuthStatusPayload struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	HasCredentials bool   `json:"has_credentials"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}
