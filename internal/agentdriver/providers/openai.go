// Package providers contains agentdriver.Provider adapters over the
// concrete LLM clients in internal/provider. The adapter is the only
// thing keeping internal/agentdriver decoupled from any single SDK.
package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/m0n0x41d/haft/internal/agent"
	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentdriver"
	"github.com/m0n0x41d/haft/internal/provider"
)

// OpenAIAdapter wraps provider.OpenAIProvider so it satisfies
// agentdriver.Provider. The wrapped client already resolves ChatGPT-Sub
// OAuth tokens (codex auth), API keys, and the codex CLI fallback —
// reuse all of that rather than re-implementing the auth ladder.
type OpenAIAdapter struct {
	llm *provider.OpenAIProvider
}

var _ agentdriver.Provider = (*OpenAIAdapter)(nil)

// NewOpenAIAdapter builds an adapter pinned to the given model. It fails
// fast if no usable credential is found — same surface as the legacy
// `haft` command path.
func NewOpenAIAdapter(model string) (*OpenAIAdapter, error) {
	p, err := provider.NewOpenAI(model)
	if err != nil {
		return nil, fmt.Errorf("agentdriver/providers: NewOpenAI: %w", err)
	}
	return &OpenAIAdapter{llm: p}, nil
}

// Generate is the agentdriver.Provider entry point. It launches the
// underlying Stream call on a goroutine and bridges the callback-based
// StreamDelta protocol into the ProviderEvent channel the driver
// consumes. The channel is closed when:
//
//   - the underlying Stream returns successfully (ProviderTurnDone
//     emitted just before close);
//   - the underlying Stream returns an error (ProviderError emitted
//     just before close);
//   - ctx is canceled (ProviderError(ctx.Err()) emitted, close).
//
// Per the Provider contract, the caller does NOT close the channel.
func (a *OpenAIAdapter) Generate(
	ctx context.Context,
	model agentcore.ModelChoice,
	history []agentcore.Turn,
	userInput string,
) (<-chan agentdriver.ProviderEvent, error) {
	messages := buildMessages(history, userInput)
	out := make(chan agentdriver.ProviderEvent, 16)

	go func() {
		defer close(out)
		handler := func(d provider.StreamDelta) {
			if d.Text != "" {
				select {
				case out <- agentdriver.ProviderTextDelta{Delta: d.Text}:
				case <-ctx.Done():
				}
			}
			if d.Thinking != "" {
				select {
				case out <- agentdriver.ProviderReasoningDelta{Delta: d.Thinking}:
				case <-ctx.Done():
				}
			}
			for _, tc := range d.ToolCalls {
				if tc.ID == "" || tc.Name == "" {
					continue
				}
				select {
				case out <- agentdriver.ProviderToolCall{
					CallID: tc.ID,
					Name:   tc.Name,
					Args:   []byte(tc.ArgsDelta),
				}:
				case <-ctx.Done():
				}
			}
		}

		_, err := a.llm.Stream(ctx, messages, nil, handler)
		if err != nil {
			out <- agentdriver.ProviderError{Err: err}
			return
		}
		out <- agentdriver.ProviderTurnDone{}
	}()

	return out, nil
}

// buildMessages converts journaled Turns + the new user input into the
// flat []agent.Message shape the OpenAI SDK consumes.
//
// Convention from internal/agentdriver/driver.go: a TurnRoleUser turn's
// Parts[0] is the user's input TextPart; subsequent TextParts and
// ReasoningParts belong to the assistant; ToolUsePart / ToolResultPart
// also belong to the assistant / tool roles respectively. Closed Turns
// have either Verdict=Pass or Verdict=Canceled — both are valid history.
//
// v8.0 alpha is conversation-only: we drop ReasoningParts and tool
// parts from history because no tools are wired yet, and the legacy
// chat completions API does not accept reasoning messages as input. The
// adapter still streams reasoning deltas live for display.
// defaultInstructions is the minimal system prompt the Codex backend
// requires. ChatGPT's /backend-api/codex/responses returns
// `{"detail":"Instructions are required"}` 400 when the request omits
// Instructions; the platform /v1/responses API allows omission, but the
// adapter has to satisfy both call paths from the same code.
const defaultInstructions = "You are haft, a conversational coding assistant. Answer concisely; defer to the user's intent."

func buildMessages(history []agentcore.Turn, userInput string) []agent.Message {
	out := make([]agent.Message, 0, len(history)*2+2)
	out = append(out, agent.Message{
		Role:  agent.RoleSystem,
		Parts: []agent.Part{agent.TextPart{Text: defaultInstructions}},
	})
	for _, turn := range history {
		if !turn.IsTerminal() {
			continue
		}
		userText, assistantText := splitTurnText(turn)
		if userText != "" {
			out = append(out, agent.Message{
				Role:  agent.RoleUser,
				Parts: []agent.Part{agent.TextPart{Text: userText}},
			})
		}
		if assistantText != "" {
			out = append(out, agent.Message{
				Role:  agent.RoleAssistant,
				Parts: []agent.Part{agent.TextPart{Text: assistantText}},
			})
		}
	}
	out = append(out, agent.Message{
		Role:  agent.RoleUser,
		Parts: []agent.Part{agent.TextPart{Text: userInput}},
	})
	return out
}

// splitTurnText pulls the user's first TextPart out and concatenates
// every other TextPart as assistant output. ReasoningParts and tool
// parts are skipped — see buildMessages doc for the reasoning.
func splitTurnText(turn agentcore.Turn) (string, string) {
	var userText string
	var assistant strings.Builder
	gotUser := false
	for _, p := range turn.Parts {
		tp, ok := p.(agentcore.TextPart)
		if !ok {
			continue
		}
		if !gotUser {
			userText = tp.Text
			gotUser = true
			continue
		}
		if assistant.Len() > 0 {
			assistant.WriteByte('\n')
		}
		assistant.WriteString(tp.Text)
	}
	return userText, assistant.String()
}

// ModelID exposes the wrapped model identifier for callers that need
// to log it. Not part of the agentdriver.Provider contract.
func (a *OpenAIAdapter) ModelID() string { return a.llm.ModelID() }
