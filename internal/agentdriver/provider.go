package agentdriver

import (
	"context"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// Provider abstracts an LLM streaming provider. Implementations adapt the
// existing internal/provider clients (OpenAI, Anthropic, Codex) to the
// driver's coarse-grained event model.
//
// Generate returns two channels:
//   - events (provider → driver): streams text/reasoning deltas, tool
//     calls, and the terminal ProviderTurnDone / ProviderError. The
//     provider owns the lifecycle of this channel and must close it
//     deterministically — the driver does NOT close it.
//   - results (driver → provider): the driver writes a ProviderToolResult
//     for every ProviderToolCall it processed. The provider blocks until
//     it has a result for every tool it dispatched in the current round,
//     then continues with another Stream call carrying the assistant's
//     tool_call + tool_output messages so the model can iterate.
//     The driver MUST close this channel when no more results will be
//     written (turn ended / error path) so the provider's loop doesn't
//     leak.
//
// Cancellation: the driver passes a context derived from the operator's
// session context. When the operator cancels a turn, the driver cancels
// this context; the provider must abort its in-flight HTTP call and
// emit a ProviderError(context.Canceled) or close the channel cleanly.
type Provider interface {
	Generate(
		ctx context.Context,
		model agentcore.ModelChoice,
		history []agentcore.Turn,
		userInput string,
	) (events <-chan ProviderEvent, results chan<- ProviderToolResult, err error)
}

// ProviderToolResult is the driver's reply to a ProviderToolCall.
// content is what the LLM sees as the tool's output; isError signals
// tool-side failure (vs. provider-side stream failure which surfaces
// as ProviderError). callID matches the originating ProviderToolCall.
type ProviderToolResult struct {
	CallID  string
	Name    string
	Content string
	IsError bool
}

// ProviderEvent is the sealed sum type the provider streams. Event order
// is significant: the driver flushes accumulated text into a TextPart
// before processing a tool call so the assistant's output preserves the
// order operators see.
type ProviderEvent interface {
	providerSeal()
}

type providerBase struct{}

func (providerBase) providerSeal() {}

// ProviderTextDelta carries a chunk of streaming assistant text. The
// driver emits PartTextDelta wire events and accumulates a buffer; on the
// next non-text event (tool call, turn done) the driver materializes the
// buffer into a single TextPart-append event for journaling.
type ProviderTextDelta struct {
	providerBase
	Delta string
}

// ProviderReasoningDelta carries hidden chain-of-thought (o1, thinking).
// Treated symmetrically to text deltas: streamed live, materialized on
// flush as a single ReasoningPart.
type ProviderReasoningDelta struct {
	providerBase
	Delta string
}

// ProviderToolCall is the assistant requesting a tool invocation. Args is
// the raw JSON-encoded argument blob as the LLM produced it; the driver
// passes it through the ToolDispatcher unchanged.
type ProviderToolCall struct {
	providerBase
	CallID string
	Name   string
	Args   []byte
}

// ProviderTurnDone marks the assistant's terminal "done" signal. After the
// driver receives this, it flushes any pending text/reasoning, emits
// turn.completed, and stops consuming the channel. Tokens is the
// cumulative token count for the turn (input + output across all
// Stream calls the provider made internally); 0 when the provider
// can't surface a count.
type ProviderTurnDone struct {
	providerBase
	Tokens int
}

// ProviderError is a terminal failure from the provider side. The driver
// flushes pending text and emits turn.failed with the wrapped error
// message.
type ProviderError struct {
	providerBase
	Err error
}
