package cli

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/fpf"
	_ "modernc.org/sqlite"
)

// proseEvalQuery is a PARAPHRASED conceptual question whose best answer is a
// specific FPF *prose* section (not a compiled pattern card), targeted by its
// stable section id. The wording is deliberately steered away from the section's
// own headings/keywords so FTS must match on different words — the case where
// semantic recall earns its keep. Many targets (the conceptual essays) carry an
// empty pattern_id, so they are INVISIBLE to the deterministic pattern-filtered
// FTS path and reachable only through the hybrid's section-level arm.
type proseEvalQuery struct {
	text   string
	target int // section id
}

var proseEvalQueries = []proseEvalQuery{
	{"how should I reason decisively when I only have partial information about the situation", 5},
	{"moving past merely spotting flawed reasoning toward actually constructing sound arguments", 8},
	{"giving shapeless private reasoning a concrete shareable auditable form", 9},
	{"when a casual everyday sentence turns into a load-bearing mechanism at an interface or contract", 13},
	{"a way of thinking that travels across separate fields instead of being yet another specialist jargon", 16},
	{"complex efforts fail because the thinking competencies are misaligned, not because facts are missing", 18},
	{"the mistake of treating a body of proof as if it had physical plugs and connectors", 54},
	{"why a loose field label like healthcare is not the same as one precise scoped context", 63},
	{"a capability only carries meaning inside the setting it was defined in and is undefined elsewhere", 83},
	{"the error of claiming a document enforced a procedure all by itself", 89},
	{"evidence from category theory and microservices that components with declared edges compose safely", 56},
	{"the algebraic rules that keep parts composing cleanly without the recursion drifting", 1891},
	{"how the composition law echoes renormalization-group coarse-graining from physics", 1900},
	{"a calculus for locating a piece of knowledge by how formal, how broad, and how trustworthy it is", 2350},
	{"gauging how formal a claim is — would a machine reject it if it were wrong", 2354},
}

// TestFPFProseRecallEval measures the OTHER half of the FPF semantic question:
// does baking section vectors recover the right *prose* explanation from a
// reworded conceptual question that keyword search misses? This is the eval that
// decides whether the full-corpus bake (prose + cards) earns its cost over the
// pattern-cards-only bake — answering it with evidence, not assertion. Matches by
// stable section id (essays have no pattern_id). Reads HAFT_FPF_EVAL_DB (default
// fpf.db); skips without baked vectors or sidecar. FPF methodology only, no leak.
func TestFPFProseRecallEval(t *testing.T) {
	if testing.Short() {
		t.Skip("fpf prose eval skipped in -short")
	}

	dbPath := "fpf.db"
	if p := strings.TrimSpace(os.Getenv("HAFT_FPF_EVAL_DB")); p != "" {
		dbPath = p
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Skipf("open fpf.db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, _, _, count, _ := fpf.SpecEmbeddingContract(db); count == 0 {
		t.Skip("fpf.db has no baked vectors — run the indexer with the sidecar first")
	}

	original := openFPFDBFunc
	openFPFDBFunc = func() (*sql.DB, func(), error) {
		conn, err := sql.Open("sqlite", dbPath)
		return conn, func() { _ = conn.Close() }, err
	}
	defer func() { openFPFDBFunc = original }()

	hybrid := buildFPFHybrid()
	if hybrid == nil {
		t.Skip("embeddings disabled by config")
	}
	if err := hybrid.Warm(); err != nil {
		t.Skipf("fpf semantic warm unavailable: %v", err)
	}

	ftsRanks := make([]int, 0, len(proseEvalQueries))
	hybRanks := make([]int, 0, len(proseEvalQueries))
	t.Logf("%-72s %4s %4s", "paraphrased conceptual question (-> prose section)", "FTS", "HYB")
	for _, q := range proseEvalQueries {
		fts, ferr := fpf.SearchSpecWithOptions(db, q.text, fpf.SpecSearchOptions{Limit: 100})
		if ferr != nil {
			t.Fatalf("fts %q: %v", q.text, ferr)
		}
		hyb, herr := hybrid.Search(db, q.text, 100)
		if herr != nil {
			t.Fatalf("hybrid %q: %v", q.text, herr)
		}
		fr := sectionRank(fts, q.target)
		hr := sectionRank(hyb, q.target)
		ftsRanks = append(ftsRanks, fr)
		hybRanks = append(hybRanks, hr)
		t.Logf("%-72s %4s %4s", clipFPF(q.text, 72), rankStrFPF(fr), rankStrFPF(hr))
	}

	fts := fpfRecallAtK(ftsRanks)
	hyb := fpfRecallAtK(hybRanks)
	t.Logf("\nqueries=%d (paraphrased -> FPF prose section)", len(proseEvalQueries))
	t.Logf("%-8s %8s %8s", "metric", "FTS", "Hybrid")
	for _, k := range []int{1, 3, 5, 10} {
		t.Logf("R@%-6d %8.2f %8.2f", k, fts.at[k], hyb.at[k])
	}
	t.Logf("%-8s %8.3f %8.3f", "MRR", fts.mrr, hyb.mrr)
	gain := hyb.at[10] - fts.at[10]
	t.Logf("\nPROSE RECALL (hybrid R@10 vs FTS): R@10 delta %+.0f%%", gain*100)
	if gain < 0 {
		t.Errorf("REGRESSION: hybrid prose R@10 %.2f < FTS R@10 %.2f", hyb.at[10], fts.at[10])
	}
}

func sectionRank(results []fpf.SpecSearchResult, target int) int {
	for rank, r := range results {
		if r.SectionID == target {
			return rank
		}
	}
	return fpfRankMiss
}
