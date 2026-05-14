package cli

import (
	"time"

	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/config"
)

// realAuthStatus loads ~/.haft/config.yaml and resolves the active
// provider's credential state for the TUI's /auth/status endpoint.
// Used by both `haft v8 serve` and `haft agent --v8` so the home
// screen renders the same auth state regardless of entrypoint.
func realAuthStatus() agentproto.AuthStatusPayload {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return agentproto.AuthStatusPayload{
			Provider:       "none",
			Model:          "",
			HasCredentials: false,
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
		}
	}
	auth := cfg.GetAuth(providerID)
	has := auth.APIKey != "" || auth.AccessToken != ""
	payload := agentproto.AuthStatusPayload{
		Provider:       providerID,
		Model:          model,
		HasCredentials: has,
	}
	if auth.ExpiresAt > 0 {
		payload.ExpiresAt = time.Unix(auth.ExpiresAt, 0).UTC().Format(time.RFC3339)
	}
	return payload
}
