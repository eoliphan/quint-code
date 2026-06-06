package embedding

import "context"

// Role selects the asymmetric prompt treatment a retrieval embedding applies.
// EmbeddingGemma (and E5/BGE-family models) embed a query and the documents it
// should match through different prompt prefixes; symmetric providers ignore
// the Role entirely.
type Role string

const (
	// RoleQuery embeds the search side of a retrieval pair.
	RoleQuery Role = "query"
	// RoleDocument embeds the indexed side of a retrieval pair.
	RoleDocument Role = "document"
)

// Descriptor identifies the provider/model/dimensionality behind an Embedder.
// It is the cache key: when any field changes, previously stored vectors are
// from a different contract and must be re-embedded, never mixed.
type Descriptor struct {
	Provider   string
	Model      string
	Dimensions int
}

// Embedder is the embedding PORT for haft's hybrid recall layer. Adapters
// implement it over interchangeable backends — the local EmbeddingGemma
// sidecar, the OpenAI API, and (future) other HTTP API providers — selected
// via Config at the composition root. Consumers depend only on this interface
// and the New factory, never on a concrete adapter.
//
// Contract: embeddings AUGMENT FTS5+PPR recall, they never replace it. When no
// adapter is available (sidecar absent, provider disabled) the factory returns
// a sentinel error and the caller degrades to FTS5+PPR — recall never hard-
// fails on a missing embedder (decision dec-20260605-fe77b358).
type Embedder interface {
	// Descriptor reports the provider/model/dimension contract.
	Descriptor() Descriptor
	// Embed turns texts into row-aligned vectors for the given retrieval role.
	Embed(ctx context.Context, role Role, texts []string) ([][]float32, error)
	// Close releases any backing process/connection. Idempotent.
	Close() error
}
