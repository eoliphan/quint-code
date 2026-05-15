package providers

import (
	"context"
	"fmt"

	"github.com/m0n0x41d/haft/internal/agent"
	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentdriver"
	"github.com/m0n0x41d/haft/internal/provider"
)

// AnthropicAdapter mirrors OpenAIAdapter but routes through the
// Anthropic provider. Same multi-step tool loop, same instructions
// + tools wiring. Authentication comes from the ANTHROPIC_API_KEY
// environment variable (or the legacy haft auth file's anthropic
// section).
type AnthropicAdapter struct {
	llm          *provider.AnthropicProvider
	instructions string
	tools        []agent.ToolSchema
}

var _ agentdriver.Provider = (*AnthropicAdapter)(nil)

// NewAnthropicAdapter resolves an API key from env / haft config and
// builds the adapter pinned to the given model.
func NewAnthropicAdapter(model string) (*AnthropicAdapter, error) {
	p, err := provider.NewAnthropic(model, "")
	if err != nil {
		return nil, fmt.Errorf("agentdriver/providers: NewAnthropic: %w", err)
	}
	return &AnthropicAdapter{llm: p}, nil
}

// WithInstructions sets the system prompt the adapter prepends to
// the message list.
func (a *AnthropicAdapter) WithInstructions(instr string) *AnthropicAdapter {
	a.instructions = instr
	return a
}

// WithTools advertises the given tool schemas on every Generate.
func (a *AnthropicAdapter) WithTools(t []agent.ToolSchema) *AnthropicAdapter {
	a.tools = t
	return a
}

// Generate runs the multi-step tool loop. Same shape as the OpenAI
// adapter — see internal/agentdriver/providers/openai.go for the
// loop's design rationale.
func (a *AnthropicAdapter) Generate(
	ctx context.Context,
	model agentcore.ModelChoice,
	history []agentcore.Turn,
	userInput string,
) (<-chan agentdriver.ProviderEvent, chan<- agentdriver.ProviderToolResult, error) {
	messages := buildMessages(a.instructions, history, userInput)
	out := make(chan agentdriver.ProviderEvent, 16)
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
		}

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
				break
			}
			totalTokens += msg.Tokens

			toolCalls := msg.ToolCalls()
			if len(toolCalls) == 0 {
				break
			}

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

// ModelID exposes the wrapped model id.
func (a *AnthropicAdapter) ModelID() string { return a.llm.ModelID() }
