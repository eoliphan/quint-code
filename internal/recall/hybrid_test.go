package recall

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/db"
	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/embedding"
)

// fakeEmbedder maps text topics to fixed orthogonal unit vectors so the test
// controls the cosine ranking exactly. It counts Embed calls to prove caching.
type fakeEmbedder struct {
	calls int
}

func (f *fakeEmbedder) Descriptor() embedding.Descriptor {
	return embedding.Descriptor{Provider: "fake", Model: "topic-v1", Dimensions: 3}
}

func (f *fakeEmbedder) Embed(_ context.Context, _ embedding.Role, texts []string) ([][]float32, error) {
	f.calls++
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = topicVector(text)
	}
	return out, nil
}

func (f *fakeEmbedder) Close() error { return nil }

func staticFactory(embedder embedding.Embedder) func() (embedding.Embedder, error) {
	return func() (embedding.Embedder, error) { return embedder, nil }
}

func topicVector(text string) []float32 {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "embedding"), strings.Contains(lower, "vector"), strings.Contains(lower, "fastembed"):
		return []float32{1, 0, 0}
	case strings.Contains(lower, "migration"), strings.Contains(lower, "database"):
		return []float32{0, 1, 0}
	case strings.Contains(lower, "auth"), strings.Contains(lower, "oauth"), strings.Contains(lower, "token"):
		return []float32{0, 0, 1}
	default:
		return []float32{0, 0, 0}
	}
}

// fakeSource returns a fixed FTS ordering and a fixed corpus.
type fakeSource struct {
	corpus   []*artifact.Artifact
	ftsOrder []*artifact.Artifact
}

func (s fakeSource) Search(_ context.Context, _ string, limit int) ([]*artifact.Artifact, error) {
	return truncate(s.ftsOrder, limit), nil
}

func (s fakeSource) ListByKind(_ context.Context, kind artifact.Kind, _ int) ([]*artifact.Artifact, error) {
	var out []*artifact.Artifact
	for _, item := range s.corpus {
		if item.Meta.Kind == kind {
			out = append(out, item)
		}
	}
	return out, nil
}

func decisionArtifact(id, title, body string) *artifact.Artifact {
	item := &artifact.Artifact{Body: body}
	item.Meta.ID = id
	item.Meta.Title = title
	item.Meta.Kind = artifact.KindDecisionRecord
	return item
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	store, err := db.NewStore(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store.GetRawDB()
}

// TestHybridPromotesSemanticHit is the killer case: the document that FTS ranks
// LAST is the one semantically closest to the query, and fusion lifts it to the
// top — exactly the gap embeddings exist to close.
func TestHybridPromotesSemanticHit(t *testing.T) {
	d1 := decisionArtifact("dec-1", "Migration safety gate", "block a risky database migration from tactical mode")
	d2 := decisionArtifact("dec-2", "Rust embedding sidecar", "fastembed gemma produces local vectors")
	d3 := decisionArtifact("dec-3", "Auth token rotation", "rotate oauth secrets on a schedule")

	source := fakeSource{
		corpus:   []*artifact.Artifact{d1, d2, d3},
		ftsOrder: []*artifact.Artifact{d1, d3, d2}, // d2 ranked last by keyword
	}
	embedder := &fakeEmbedder{}
	hybrid := NewHybrid(source, staticFactory(embedder), testDB(t))

	results, err := hybrid.Search(context.Background(), "how do we run vectors locally", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 || results[0].Meta.ID != "dec-2" {
		t.Fatalf("expected dec-2 (semantic match) promoted to top, got %s", orderIDs(results))
	}
}

// TestHybridCachesCorpusEmbeddings proves the second warm reuses the cache: a
// fresh Hybrid over the same DB embeds only the query, not the corpus again.
func TestHybridCachesCorpusEmbeddings(t *testing.T) {
	conn := testDB(t)
	corpus := []*artifact.Artifact{
		decisionArtifact("dec-1", "Rust embedding sidecar", "fastembed gemma local vectors"),
		decisionArtifact("dec-2", "Auth token rotation", "rotate oauth secrets"),
	}
	source := fakeSource{corpus: corpus, ftsOrder: corpus}

	first := &fakeEmbedder{}
	if _, err := NewHybrid(source, staticFactory(first), conn).Search(context.Background(), "vectors", 5); err != nil {
		t.Fatalf("first search: %v", err)
	}
	// First run: one corpus document batch + one query = 2 calls.
	if first.calls != 2 {
		t.Fatalf("first run embed calls = %d, want 2 (corpus + query)", first.calls)
	}

	second := &fakeEmbedder{}
	if _, err := NewHybrid(source, staticFactory(second), conn).Search(context.Background(), "vectors", 5); err != nil {
		t.Fatalf("second search: %v", err)
	}
	// Second run: corpus hits the cache, so only the query is embedded.
	if second.calls != 1 {
		t.Fatalf("second run embed calls = %d, want 1 (query only — corpus cached)", second.calls)
	}
}

// TestHybridDegradesWithoutEmbedder proves a nil embedder falls straight through
// to the store's FTS ordering.
func TestHybridDegradesWithoutEmbedder(t *testing.T) {
	corpus := []*artifact.Artifact{
		decisionArtifact("dec-1", "A", "alpha"),
		decisionArtifact("dec-2", "B", "beta"),
	}
	source := fakeSource{corpus: corpus, ftsOrder: corpus}
	hybrid := NewHybrid(source, nil, testDB(t))

	results, err := hybrid.Search(context.Background(), "anything", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if orderIDs(results) != "dec-1,dec-2" {
		t.Fatalf("nil embedder should pass through FTS order, got %s", orderIDs(results))
	}
}

func TestFuseRRF(t *testing.T) {
	// id "x" is rank0 in list A and rank0 in list B → must win.
	fused := fuseRRF([][]string{{"x", "y"}, {"x", "z"}}, rrfK)
	if fused[0] != "x" {
		t.Fatalf("RRF should rank the doubly-top-ranked id first, got %v", fused)
	}
}

func orderIDs(items []*artifact.Artifact) string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.Meta.ID
	}
	return strings.Join(ids, ",")
}
