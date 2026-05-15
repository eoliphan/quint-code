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
	llm          *provider.OpenAIProvider
	instructions string
	tools        []agent.ToolSchema
}

var _ agentdriver.Provider = (*OpenAIAdapter)(nil)

// NewOpenAIAdapter builds an adapter pinned to the given model. It fails
// fast if no usable credential is found — same surface as the legacy
// `haft` command path. The instructions string is the system prompt
// the provider sends as the OpenAI Responses Instructions field;
// tools is the schema list advertised to the LLM. Both default-empty
// at construction time and are wired via WithInstructions / WithTools
// so the constructor signature stays stable.
func NewOpenAIAdapter(model string) (*OpenAIAdapter, error) {
	p, err := provider.NewOpenAI(model)
	if err != nil {
		return nil, fmt.Errorf("agentdriver/providers: NewOpenAI: %w", err)
	}
	return &OpenAIAdapter{llm: p}, nil
}

// WithInstructions overrides the system prompt. Empty string keeps
// the minimum-codex-required defaultInstructions stub.
func (a *OpenAIAdapter) WithInstructions(instr string) *OpenAIAdapter {
	a.instructions = instr
	return a
}

// WithTools advertises the given tool schemas on every Generate call.
// Tool dispatch happens through agentdriver.ToolDispatcher — this
// only controls what the LLM SEES.
func (a *OpenAIAdapter) WithTools(t []agent.ToolSchema) *OpenAIAdapter {
	a.tools = t
	return a
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
// Per the Provider contract, the caller does NOT close the events
// channel. The caller MUST close the results channel when no more
// tool results will arrive so the multi-step loop can exit cleanly.
func (a *OpenAIAdapter) Generate(
	ctx context.Context,
	model agentcore.ModelChoice,
	history []agentcore.Turn,
	userInput string,
) (<-chan agentdriver.ProviderEvent, chan<- agentdriver.ProviderToolResult, error) {
	messages := buildMessages(a.instructions, history, userInput)
	out := make(chan agentdriver.ProviderEvent, 16)
	// Buffered to the same depth as the events channel so a slow
	// driver doesn't deadlock when the provider has emitted several
	// tool calls in quick succession.
	results := make(chan agentdriver.ProviderToolResult, 16)

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
			// Tool call deltas streamed by the SDK are partial; the
			// FINAL list comes back from Stream's returned message.
			// Ignore them here to avoid double-counting.
		}

		// Multi-step loop. Each iteration runs one model generation
		// and dispatches any tool calls back through the driver.
		// Loop exits when the model returns with no tool calls.
		// Safety cap: 20 rounds prevents an unbounded loop if a
		// pathological tool keeps producing the same call.
		const maxRounds = 20
		totalTokens := 0
		for round := 0; round < maxRounds; round++ {
			msg, err := a.llm.Stream(ctx, messages, a.tools, handler)
			if err != nil {
				select {
				case out <- agentdriver.ProviderError{Err: err}:
				case <-ctx.Done():
				}
				return
			}
			if msg == nil {
				// Shouldn't happen — Stream returns a message or an
				// error — but defend against it rather than
				// deadlocking the driver.
				break
			}
			totalTokens += msg.Tokens

			toolCalls := msg.ToolCalls()
			if len(toolCalls) == 0 {
				// Terminal generation — model produced text only.
				break
			}

			// Emit every tool call onto the events channel so the
			// driver journals + dispatches them. The provider then
			// blocks until it gets a result for each, in order.
			for _, tc := range toolCalls {
				if tc.ToolCallID == "" || tc.ToolName == "" {
					continue
				}
				select {
				case out <- agentdriver.ProviderToolCall{
					CallID: tc.ToolCallID,
					Name:   tc.ToolName,
					Args:   []byte(tc.Arguments),
				}:
				case <-ctx.Done():
					return
				}
			}

			// Append the assistant's tool_call message and the
			// tool_output rows so the next Stream call sees the
			// model's last move + the operator/registry's response.
			messages = append(messages, *msg)
			for range toolCalls {
				var res agentdriver.ProviderToolResult
				select {
				case res = <-results:
				case <-ctx.Done():
					return
				}
				messages = append(messages, agent.Message{
					Role: agent.RoleTool,
					Parts: []agent.Part{
						agent.ToolResultPart{
							ToolCallID: res.CallID,
							ToolName:   res.Name,
							Content:    res.Content,
							IsError:    res.IsError,
						},
					},
				})
			}
		}

		select {
		case out <- agentdriver.ProviderTurnDone{Tokens: totalTokens}:
		case <-ctx.Done():
		}
	}()

	return out, results, nil
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
// adapter has to satisfy both call paths from the same code. The
// caller is expected to override this via WithInstructions for any
// non-trivial agent — without the full BuildSystemPrompt the model
// has no idea it is the haft agent operating under FPF discipline.
const defaultInstructions = "You are haft, a conversational coding assistant. Answer concisely; defer to the user's intent."

func buildMessages(instructions string, history []agentcore.Turn, userInput string) []agent.Message {
	if instructions == "" {
		instructions = defaultInstructions
	}
	out := make([]agent.Message, 0, len(history)*2+2)
	out = append(out, agent.Message{
		Role:  agent.RoleSystem,
		Parts: []agent.Part{agent.TextPart{Text: instructions}},
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
