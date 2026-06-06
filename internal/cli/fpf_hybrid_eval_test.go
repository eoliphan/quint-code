package cli

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/fpf"
	_ "modernc.org/sqlite"
)

// fpfEvalQuery is a PARAPHRASED "how do I think about X" question mapped to the
// FPF pattern that answers it — deliberately reworded away from the pattern's
// own heading/keywords, so FTS must match on different words. Authored from the
// 66 pattern headings; haft-only methodology, no operator data.
type fpfEvalQuery struct {
	text   string
	target string // pattern_id
}

var fpfEvalQueries = []fpfEvalQuery{
	{"how do I find the part of a design that breaks first", "X-WLNK"},
	{"comparing two options fairly so the comparison is not rigged", "CMP-01"},
	{"deciding the rule for choosing before I look at the results", "CMP-02"},
	{"telling apart what we planned from what actually runs in production", "X-DESIGNRUN"},
	{"how do I undo a change cheaply if it turns out wrong", "DEC-05"},
	{"stating in advance what observable outcome would prove me right or wrong", "DEC-06"},
	{"how do I know when the problem is actually solved", "FRAME-03"},
	{"naming the real anomaly instead of jumping straight to a fix", "FRAME-01"},
	{"for each candidate approach, what most plausibly sinks it", "EXP-04"},
	{"why the assistant should not pick the architecture on its own", "X-TRANSFORMER"},
	{"how trust in old proof erodes as time passes", "VER-02"},
	{"scoring how much I should trust a claim overall", "VER-03"},
	{"labeling whether a number is a hard limit, a thing to optimize, or just watched", "CHR-01"},
	{"keeping the strongest argument against my own choice on the record", "DEC-08"},
	{"noticing when an option opens a question we had not asked before", "EXP-08"},
	{"is this an optimization, a diagnosis, a search, or a synthesis", "FRAME-05"},
	{"finding the set of options where none strictly beats another", "CMP-03"},
	{"stopping the work from quietly growing beyond what we agreed", "X-SCOPE"},
	{"what must stay true at every moment no matter what changes", "DEC-04"},
	{"picking a problem that is neither trivial nor impossibly hard", "FRAME-07"},
	{"deciding whether a sentence is a fact, a plan, or a definition", "X-STATEMENT-TYPE"},
	{"making sure a metric is not being gamed or faked", "VER-05"},
	{"when to bet on more compute and data over clever hand-built rules", "X-BITTER-LESSON"},
	{"the load-bearing chain of evidence a conclusion rests on", "CHR-05"},
	{"preferring changes that are easy to reverse later", "DEC-09"},
}

// TestFPFHybridRecallEval is the ship gate for FPF-spec semantic search: does the
// hybrid (FTS + baked section vectors) beat the deterministic FTS at recovering
// the right pattern from a reworded "how do I think about X" question? Reads the
// on-disk internal/cli/fpf.db (must carry baked vectors) and the local sidecar;
// skips otherwise. Local-only, no-leak (methodology patterns, not user data).
func TestFPFHybridRecallEval(t *testing.T) {
	if testing.Short() {
		t.Skip("fpf hybrid eval skipped in -short")
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

	ftsRanks := make([]int, 0, len(fpfEvalQueries))
	hybRanks := make([]int, 0, len(fpfEvalQueries))
	t.Logf("%-66s %4s %4s", "paraphrased thinking question", "FTS", "HYB")
	for _, q := range fpfEvalQueries {
		fts, ferr := fpf.SearchSpecWithOptions(db, q.text, fpf.SpecSearchOptions{Limit: 100})
		if ferr != nil {
			t.Fatalf("fts %q: %v", q.text, ferr)
		}
		hyb, herr := hybrid.Search(db, q.text, 100)
		if herr != nil {
			t.Fatalf("hybrid %q: %v", q.text, herr)
		}
		fr := patternRank(fts, q.target)
		hr := patternRank(hyb, q.target)
		ftsRanks = append(ftsRanks, fr)
		hybRanks = append(hybRanks, hr)
		t.Logf("%-66s %4s %4s", clipFPF(q.text, 66), rankStrFPF(fr), rankStrFPF(hr))
	}

	fts := fpfRecallAtK(ftsRanks)
	hyb := fpfRecallAtK(hybRanks)
	t.Logf("\nqueries=%d (paraphrased -> FPF pattern)", len(fpfEvalQueries))
	t.Logf("%-8s %8s %8s", "metric", "FTS", "Hybrid")
	for _, k := range []int{1, 3, 5, 10} {
		t.Logf("R@%-6d %8.2f %8.2f", k, fts.at[k], hyb.at[k])
	}
	t.Logf("%-8s %8.3f %8.3f", "MRR", fts.mrr, hyb.mrr)
	gain := hyb.at[10] - fts.at[10]
	t.Logf("\nSHIP GATE (hybrid R@10 >= FTS +10%%): R@10 delta %+.0f%% -> %s", gain*100, fpfVerdict(gain))
	if gain < 0 {
		t.Errorf("REGRESSION: hybrid R@10 %.2f < FTS R@10 %.2f", hyb.at[10], fts.at[10])
	}
}

const fpfRankMiss = 1 << 30

func patternRank(results []fpf.SpecSearchResult, target string) int {
	for rank, r := range results {
		if r.PatternID == target {
			return rank
		}
	}
	return fpfRankMiss
}

type fpfReport struct {
	at  map[int]float64
	mrr float64
}

func fpfRecallAtK(ranks []int) fpfReport {
	out := fpfReport{at: map[int]float64{}}
	for _, k := range []int{1, 3, 5, 10} {
		hits := 0
		for _, r := range ranks {
			if r < k {
				hits++
			}
		}
		out.at[k] = float64(hits) / float64(len(ranks))
	}
	for _, r := range ranks {
		if r < fpfRankMiss {
			out.mrr += 1.0 / float64(r+1)
		}
	}
	out.mrr /= float64(len(ranks))
	return out
}

func fpfVerdict(gain float64) string {
	if gain >= 0.10 {
		return "CLEARS — semantic FPF search earns the default"
	}
	if gain > 0 {
		return "POSITIVE BUT BELOW 10% — ship DORMANT (FTS default), do not lower the gate"
	}
	return "NO GAIN over FTS"
}

func rankStrFPF(rank int) string {
	if rank >= fpfRankMiss {
		return "—"
	}
	digits := []byte{}
	if rank == 0 {
		return "0"
	}
	for rank > 0 {
		digits = append([]byte{byte('0' + rank%10)}, digits...)
		rank /= 10
	}
	return string(digits)
}

func clipFPF(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
