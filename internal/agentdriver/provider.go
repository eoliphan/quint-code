package agentdriver

import (
	"context"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// Provider abstracts an LLM streaming provider. Implementations adapt the
// existing internal/provider clients (OpenAI, Anthropic, Codex) to the
// driver's coarse-grained event model.
//
// Generate returns a channel that the driver consumes until it closes or
// emits a ProviderTurnDone or ProviderError. The provider owns the
// lifecycle of the channel and must close it deterministically — the
// driver does NOT close it.
//
// Cancellation: the driver passes a context derived from the operator's
// session context. When the operator cancels a turn, the driver cancels
// this context; the provider must abort its in-flight HTTP call and
// emit a ProviderError(context.Canceled) or close the channel cleanly.
type Provider interface {
	Generate(ctx context.Context, model agentcore.ModelChoice, history []agentcore.Turn, userInput string) (<-chan ProviderEvent, error)
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
// turn.completed, and stops consuming the channel.
type ProviderTurnDone struct {
	providerBase
}

// ProviderError is a terminal failure from the provider side. The driver
// flushes pending text and emits turn.failed with the wrapped error
// message.
type ProviderError struct {
	providerBase
	Err error
}
