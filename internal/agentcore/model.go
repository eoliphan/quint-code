package agentcore

// ProviderKind enumerates the LLM provider families haft can speak to.
// Adding a kind is a deliberate breaking change to G1 — the wire format
// in Layer P encodes ProviderKind as a string discriminator and consumers
// will reject unknown values rather than silently ignoring them.
type ProviderKind string

const (
	ProviderOpenAI    ProviderKind = "openai"
	ProviderAnthropic ProviderKind = "anthropic"
	// ProviderCodex is the ChatGPT-Sub OAuth path that reuses the OpenAI
	// API surface but carries chatgpt_account_id auth. It is preserved
	// from internal/cli/login.go and remains the only auth flow that
	// requires a device-code exchange.
	ProviderCodex ProviderKind = "codex"
)

// ModelChoice is the immutable triple a Session pins itself to. Switching
// model mid-session is modeled as the runtime emitting a model.switched
// event and the next Turn binding to a new ModelChoice; the current Turn
// keeps the choice it started with.
type ModelChoice struct {
	Provider ProviderKind `json:"provider"`
	Model    string       `json:"model"` // provider-native model id (e.g. "gpt-5.4", "claude-sonnet-4-6")
	// CredentialKey identifies which stored credential to use without
	// embedding the secret value here. The G1 provider layer dereferences
	// the key against the credential store at call time.
	CredentialKey string `json:"credential_key,omitempty"`
}
