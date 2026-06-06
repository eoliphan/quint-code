package embedding

import (
	"context"
	"math"
	"testing"
)

// TestSidecarEmbedderEndToEnd drives the Embedder port against a real haft-embed
// process. It skips (does not fail) when the sidecar binary is absent, since
// degrading to FTS5+PPR is the contract on a default install.
func TestSidecarEmbedderEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sidecar E2E in -short mode (loads the embedding model)")
	}

	embedder, err := New(Config{Provider: ProviderLocal})
	if Degraded(err) {
		t.Skipf("embedding sidecar unavailable, degrading is expected: %v", err)
	}
	if err != nil {
		t.Fatalf("New(local): %v", err)
	}
	t.Cleanup(func() { _ = embedder.Close() })

	descriptor := embedder.Descriptor()
	if descriptor.Dimensions <= 0 {
		t.Fatalf("descriptor reports non-positive dim: %+v", descriptor)
	}

	ctx := context.Background()
	queries, err := embedder.Embed(ctx, RoleQuery, []string{
		"how do we run embeddings locally inside haft without a network call",
	})
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	documents, err := embedder.Embed(ctx, RoleDocument, []string{
		"A Rust fastembed sidecar running EmbeddingGemma augments FTS5 and PPR recall locally",
		"The cat sat lazily on the warm mat in the afternoon sun",
	})
	if err != nil {
		t.Fatalf("embed documents: %v", err)
	}

	if got := len(queries[0]); got != descriptor.Dimensions {
		t.Fatalf("query vector dim = %d, want %d", got, descriptor.Dimensions)
	}

	relevant := cosine(queries[0], documents[0])
	irrelevant := cosine(queries[0], documents[1])
	if relevant <= irrelevant {
		t.Fatalf("expected relevant doc to outscore irrelevant: relevant=%.4f irrelevant=%.4f", relevant, irrelevant)
	}
	t.Logf("dim=%d cos(relevant)=%.4f cos(irrelevant)=%.4f separation=%.4f",
		descriptor.Dimensions, relevant, irrelevant, relevant-irrelevant)
}

func cosine(left, right []float32) float64 {
	if len(left) != len(right) {
		return 0
	}
	var dot, normLeft, normRight float64
	for i := range left {
		dot += float64(left[i]) * float64(right[i])
		normLeft += float64(left[i]) * float64(left[i])
		normRight += float64(right[i]) * float64(right[i])
	}
	if normLeft == 0 || normRight == 0 {
		return 0
	}
	return dot / (math.Sqrt(normLeft) * math.Sqrt(normRight))
}
