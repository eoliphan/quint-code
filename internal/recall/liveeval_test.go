package recall

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/embedding"
)

// liveQuery is a paraphrased operator question (deliberately different vocabulary
// than the target decision's text) mapped to a substring that uniquely picks the
// target decision file. Reused from the phase-3 R@k spike.
type liveQuery struct {
	text   string
	target string
}

var liveQueries = []liveQuery{
	{"how do we show which functions are exercised by tests in the graph", "ef966a11"},
	{"scoring whether our past predictions were overconfident", "c3c7fa88"},
	{"finding a symbol when I mistype its name", "1c108a55"},
	{"representing call relationships between functions across files", "5825abc6"},
	{"stop a risky database migration from being treated as a quick low-effort change", "0a6edafd"},
	{"showing when the code has diverged from what a recorded decision assumed", "9fdd33ed"},
	{"attach a remembered fact to a specific place in the code", "26be1e4b"},
	{"filter out low-confidence machine-written justifications", "732219b6"},
	{"order the ungoverned modules by how many things depend on them", "e4b86938"},
	{"the plan for adding meaning-based search to the tool later", "3aaad199"},
	{"only run the tests that a change actually touches", "ef966a11"},
	{"measure if the agent is systematically too sure of itself", "c3c7fa88"},
	{"connect a class to the parent it inherits from", "5825abc6"},
	{"warn me before I make an irreversible choice about authentication", "0a6edafd"},
	{"remember a project fact without a full decision record", "26be1e4b"},
	{"rank search hits so generated stub files sink below hand-written code", "ef966a11"},
}

// TestLiveDecisionRecallEval measures prediction #1 of dec-20260605-fe77b358:
// does HYBRID (FTS5 + EmbeddingGemma RRF) beat FTS5-alone on the live decision
// corpus by >=10% R@10? It runs the SHIPPED code (store.Search + recall.Hybrid)
// with the real sidecar. Skips (never fails) when the sidecar or the live
// corpus is absent — it is an evaluation harness, not a portable unit test.
func TestLiveDecisionRecallEval(t *testing.T) {
	if testing.Short() {
		t.Skip("live recall eval skipped in -short")
	}

	decisionsDir := filepath.Join("..", "..", ".haft", "decisions")
	files, err := filepath.Glob(filepath.Join(decisionsDir, "*.md"))
	if err != nil || len(files) == 0 {
		t.Skipf("live decision corpus not found at %s — skipping eval", decisionsDir)
	}

	embedder, err := embedding.New(embedding.Config{Provider: embedding.ProviderLocal})
	if embedding.Degraded(err) {
		t.Skipf("sidecar unavailable, eval needs the production embedder: %v", err)
	}
	if err != nil {
		t.Fatalf("embedder: %v", err)
	}
	_ = embedder.Close() // hybrid spawns its own via the factory below

	conn := testDB(t)
	store := artifact.NewStore(conn)
	ctx := context.Background()
	loadDecisionCorpus(t, ctx, store, files)

	hybrid := NewHybrid(store, func() (embedding.Embedder, error) {
		return embedding.New(embedding.Config{Provider: embedding.ProviderLocal})
	}, conn)
	if err := hybrid.Warm(ctx); err != nil {
		t.Fatalf("warm hybrid corpus: %v", err)
	}

	ftsRanks := make([]int, 0, len(liveQueries))
	hybridRanks := make([]int, 0, len(liveQueries))
	for _, query := range liveQueries {
		ftsRanks = append(ftsRanks, rankOfTarget(ctx, t, store, query, false, nil))
		hybridRanks = append(hybridRanks, rankOfTarget(ctx, t, store, query, true, hybrid))
	}

	fts := recallReport(ftsRanks)
	hyb := recallReport(hybridRanks)
	t.Logf("corpus=%d decisions  queries=%d", len(files), len(liveQueries))
	t.Logf("%-8s %10s %10s %8s", "metric", "FTS5", "Hybrid", "delta")
	for _, k := range []int{1, 3, 5, 10} {
		t.Logf("R@%-6d %10.2f %10.2f %+8.2f", k, fts.at[k], hyb.at[k], hyb.at[k]-fts.at[k])
	}
	t.Logf("%-8s %10.3f %10.3f %+8.3f", "MRR", fts.mrr, hyb.mrr, hyb.mrr-fts.mrr)

	gain := hyb.at[10] - fts.at[10]
	t.Logf("PREDICTION #1 (>=10%% R@10 gain): R@10 delta = %+.0f%% -> %s",
		gain*100, verdict(gain))
}

func loadDecisionCorpus(t *testing.T, ctx context.Context, store *artifact.Store, files []string) {
	t.Helper()
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		title, body := splitFrontmatterTitle(string(raw))

		art := &artifact.Artifact{Body: body}
		art.Meta.ID = id
		art.Meta.Kind = artifact.KindDecisionRecord
		art.Meta.Title = title
		if err := store.Create(ctx, art); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
}

var headingPattern = regexp.MustCompile(`(?m)^#\s+(.+)$`)

// splitFrontmatterTitle strips YAML frontmatter, lifts the first markdown
// heading as the title, and returns the remaining body — mirroring how a stored
// decision separates Title from content.
func splitFrontmatterTitle(text string) (string, string) {
	if strings.HasPrefix(text, "---") {
		if parts := strings.SplitN(text, "---", 3); len(parts) == 3 {
			text = parts[2]
		}
	}
	title := ""
	if match := headingPattern.FindStringSubmatch(text); match != nil {
		title = strings.TrimSpace(match[1])
	}
	return title, strings.TrimSpace(text)
}

const rankMiss = 1 << 30

// rankOfTarget returns the 0-based rank of the target decision in the search
// results, or rankMiss when it is absent from the ranked list.
func rankOfTarget(ctx context.Context, t *testing.T, store *artifact.Store, query liveQuery, useHybrid bool, hybrid *Hybrid) int {
	t.Helper()
	var results []*artifact.Artifact
	var err error
	if useHybrid {
		results, err = hybrid.Search(ctx, query.text, 100)
	} else {
		results, err = store.Search(ctx, query.text, 100)
	}
	if err != nil {
		t.Fatalf("search %q: %v", query.text, err)
	}
	for rank, item := range results {
		if strings.Contains(item.Meta.ID, query.target) {
			return rank
		}
	}
	return rankMiss
}

type report struct {
	at  map[int]float64
	mrr float64
}

func recallReport(ranks []int) report {
	out := report{at: map[int]float64{}}
	for _, k := range []int{1, 3, 5, 10} {
		hits := 0
		for _, rank := range ranks {
			if rank < k {
				hits++
			}
		}
		out.at[k] = float64(hits) / float64(len(ranks))
	}
	for _, rank := range ranks {
		if rank < rankMiss {
			out.mrr += 1.0 / float64(rank+1)
		}
	}
	out.mrr /= float64(len(ranks))
	return out
}

func verdict(gain float64) string {
	if gain >= 0.10 {
		return "HOLDS — embeddings earn their place"
	}
	if gain >= 0 {
		return "PARTIAL — positive but below the 10% threshold"
	}
	return "FAILS — hybrid did not beat FTS5 (rollback trigger)"
}
