package embedding

import (
	"context"

	"github.com/m0n0x41d/haft/internal/fpf"
)

// openAIAdapter adapts the OpenAI fpf.SemanticEmbedder implementation to the
// Embedder port. OpenAI text-embedding models are symmetric, so Role is
// ignored — query and document text embed identically.
type openAIAdapter struct {
	inner fpf.SemanticEmbedder
}

func newOpenAIAdapter(cfg Config) (Embedder, error) {
	inner, err := NewOpenAI(cfg.Model, cfg.Dim)
	if err != nil {
		return nil, err
	}
	return openAIAdapter{inner: inner}, nil
}

func (a openAIAdapter) Descriptor() Descriptor {
	descriptor := a.inner.Descriptor()
	return Descriptor{
		Provider:   descriptor.Provider,
		Model:      descriptor.Model,
		Dimensions: descriptor.Dimensions,
	}
}

func (a openAIAdapter) Embed(ctx context.Context, _ Role, texts []string) ([][]float32, error) {
	return a.inner.EmbedTexts(ctx, texts)
}

func (a openAIAdapter) Close() error {
	return nil
}
