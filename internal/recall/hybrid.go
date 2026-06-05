// Package recall composes haft's keyword/graph artifact store with the optional
// embedding port into one hybrid retrieval layer. Embeddings AUGMENT FTS5+PPR;
// they never replace it. With no embedder, or on any embedder fault, Search
// degrades to the store's own FTS ranking — recall never hard-fails on the
// optional semantic layer (decision dec-20260605-fe77b358).
package recall

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"

	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/embedding"
)

const (
	rrfK                  = 60.0 // standard Reciprocal Rank Fusion constant
	candidatePoolFactor   = 4    // pull this many × limit candidates before fusing
	minCandidatePool      = 24
	minSemanticSimilarity = 0.15 // cosine floor — below this a doc is a non-match, not a weak hit
)

// corpusKinds scopes the semantic index to decisions + notes — the prose where
// keyword recall misses paraphrase (operator scope: "start small").
var corpusKinds = []artifact.Kind{artifact.KindDecisionRecord, artifact.KindNote}

// ArtifactSource is the slice of the artifact store the hybrid layer needs.
type ArtifactSource interface {
	Search(ctx context.Context, query string, limit int) ([]*artifact.Artifact, error)
	ListByKind(ctx context.Context, kind artifact.Kind, limit int) ([]*artifact.Artifact, error)
}

// Searcher is the narrow retrieval surface the server's search handlers depend
// on. Both *artifact.Store (FTS only) and *Hybrid (FTS+semantic) satisfy it,
// so the call site stays agnostic to whether embeddings are active.
type Searcher interface {
	Search(ctx context.Context, query string, limit int) ([]*artifact.Artifact, error)
}

// Hybrid fuses FTS ranking from the store with cosine ranking from an in-memory
// embedding index. The embedder is resolved lazily (so server startup never
// blocks on a first-run model download) and the index is built on first Search,
// cached to the artifact_embeddings table so restarts re-embed only changed
// artifacts.
type Hybrid struct {
	source      ArtifactSource
	newEmbedder func() (embedding.Embedder, error)
	cache       vectorCache

	mu       sync.Mutex
	embedder embedding.Embedder
	index    *embedding.Index
	byID     map[string]*artifact.Artifact
	warmed   bool
}

// NewHybrid builds the hybrid layer. newEmbedder is invoked once, on the first
// Search, to spawn/connect the embedder — a nil factory (or one that errors,
// e.g. the sidecar is absent) makes Search transparently delegate to the
// store's FTS ranking for the rest of the session.
func NewHybrid(source ArtifactSource, newEmbedder func() (embedding.Embedder, error), db *sql.DB) *Hybrid {
	return &Hybrid{source: source, newEmbedder: newEmbedder, cache: vectorCache{db: db}}
}

// Search returns artifacts for the query, fusing keyword and semantic ranking.
// It always falls back to FTS-only on any semantic-path issue, never erroring
// out recall for a missing/faulty embedder.
func (h *Hybrid) Search(ctx context.Context, query string, limit int) ([]*artifact.Artifact, error) {
	if limit <= 0 {
		limit = 20
	}
	if h.newEmbedder == nil {
		return h.source.Search(ctx, query, limit)
	}
	if err := h.warm(ctx); err != nil || h.index == nil || h.index.Len() == 0 {
		return h.source.Search(ctx, query, limit)
	}

	pool := max(limit*candidatePoolFactor, minCandidatePool)

	ftsResults, err := h.source.Search(ctx, query, pool)
	if err != nil {
		return nil, err
	}

	queryVectors, err := h.embedder.Embed(ctx, embedding.RoleQuery, []string{query})
	if err != nil || len(queryVectors) == 0 {
		return truncate(ftsResults, limit), nil
	}
	semantic := h.index.Search(queryVectors[0], pool)

	fused := fuseRRF([][]string{idsOf(ftsResults), semanticIDs(semantic, minSemanticSimilarity)}, rrfK)
	return h.resolve(fused, ftsResults, limit), nil
}

// warm lazily embeds the decision/note corpus (reusing cached vectors for
// unchanged artifacts) and builds the in-memory cosine index. Idempotent.
func (h *Hybrid) warm(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.warmed {
		return nil
	}

	if h.embedder == nil {
		embedder, err := h.newEmbedder()
		if err != nil {
			// Sidecar absent / provider disabled — degrade to FTS for this
			// session rather than retrying the spawn on every search.
			h.warmed = true
			return nil
		}
		h.embedder = embedder
	}

	descriptor := h.embedder.Descriptor()
	cached, err := h.cache.load(ctx, descriptor)
	if err != nil {
		return err
	}

	corpus, err := h.loadCorpus(ctx)
	if err != nil {
		return err
	}

	byID := make(map[string]*artifact.Artifact, len(corpus))
	vectors := make(map[string][]float32, len(corpus))
	var missIDs, missHashes, missTexts []string

	for _, item := range corpus {
		byID[item.Meta.ID] = item
		text := corpusText(item)
		if text == "" {
			continue
		}
		hash := contentHash(text)
		if hit, ok := cached[item.Meta.ID]; ok && hit.ContentHash == hash {
			vectors[item.Meta.ID] = hit.Vector
			continue
		}
		missIDs = append(missIDs, item.Meta.ID)
		missHashes = append(missHashes, hash)
		missTexts = append(missTexts, text)
	}

	if err := h.embedMisses(ctx, descriptor, missIDs, missHashes, missTexts, vectors); err != nil {
		return err
	}

	index := embedding.NewIndex(len(vectors))
	for id, vector := range vectors {
		index.Add(id, vector)
	}

	h.index = index
	h.byID = byID
	h.warmed = true
	return nil
}

func (h *Hybrid) loadCorpus(ctx context.Context) ([]*artifact.Artifact, error) {
	var corpus []*artifact.Artifact
	for _, kind := range corpusKinds {
		items, err := h.source.ListByKind(ctx, kind, 0)
		if err != nil {
			return nil, err
		}
		corpus = append(corpus, items...)
	}
	return corpus, nil
}

// embedMisses embeds the not-cached artifacts as documents and writes them
// back to the cache, populating vectors in place.
func (h *Hybrid) embedMisses(
	ctx context.Context,
	descriptor embedding.Descriptor,
	ids, hashes, texts []string,
	vectors map[string][]float32,
) error {
	if len(ids) == 0 {
		return nil
	}
	embedded, err := h.embedder.Embed(ctx, embedding.RoleDocument, texts)
	if err != nil {
		return err
	}
	if len(embedded) != len(ids) {
		return fmt.Errorf("embedded %d vectors for %d corpus misses", len(embedded), len(ids))
	}
	for index, id := range ids {
		vectors[id] = embedded[index]
		if err := h.cache.store(ctx, descriptor, id, hashes[index], embedded[index]); err != nil {
			return err
		}
	}
	return nil
}

// resolve maps the fused id ordering back to artifacts, preferring the corpus
// lookup but falling back to FTS result objects for non-corpus kinds.
func (h *Hybrid) resolve(orderedIDs []string, ftsResults []*artifact.Artifact, limit int) []*artifact.Artifact {
	lookup := make(map[string]*artifact.Artifact, len(h.byID)+len(ftsResults))
	maps.Copy(lookup, h.byID)
	for _, item := range ftsResults {
		lookup[item.Meta.ID] = item
	}

	out := make([]*artifact.Artifact, 0, limit)
	for _, id := range orderedIDs {
		item, ok := lookup[id]
		if !ok {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// fuseRRF merges ranked id-lists by Reciprocal Rank Fusion: an id's score is
// the sum over lists of 1/(k + rank), so a strong hit in either list surfaces
// even if the other list misses it entirely. Robust to bm25/cosine scale gap.
func fuseRRF(ranked [][]string, k float64) []string {
	score := make(map[string]float64)
	for _, list := range ranked {
		for rank, id := range list {
			score[id] += 1.0 / (k + float64(rank) + 1.0)
		}
	}

	ids := make([]string, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		if score[ids[i]] == score[ids[j]] {
			return ids[i] < ids[j]
		}
		return score[ids[i]] > score[ids[j]]
	})
	return ids
}

func idsOf(items []*artifact.Artifact) []string {
	ids := make([]string, len(items))
	for index, item := range items {
		ids[index] = item.Meta.ID
	}
	return ids
}

// semanticIDs keeps only candidates at or above the cosine floor — a doc below
// it is a non-match and must not earn RRF rank credit that would let keyword-
// unrelated noise ride into the fused results.
func semanticIDs(scored []embedding.Scored, minScore float64) []string {
	ids := make([]string, 0, len(scored))
	for _, item := range scored {
		if item.Score < minScore {
			continue
		}
		ids = append(ids, item.ID)
	}
	return ids
}

func truncate(items []*artifact.Artifact, limit int) []*artifact.Artifact {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func corpusText(item *artifact.Artifact) string {
	return strings.TrimSpace(item.Meta.Title + "\n" + item.Body)
}

func contentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
