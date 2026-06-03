package codeintel

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/m0n0x41d/haft/internal/codebase"

	_ "modernc.org/sqlite"
)

func TestShortestPath_FindsAndReportsNoPath(t *testing.T) {
	out := map[string][]codebase.CodeEdge{
		"A": {edge("A", "B")},
		"B": {edge("B", "C")},
		"C": {edge("C", "D")},
		// X is isolated.
	}
	src := fakeEdges{out: out}
	ctx := context.Background()

	path, ok := shortestPath(ctx, src, "A", "C", 7)
	if !ok {
		t.Fatalf("A→C should be reachable")
	}
	got := chainIDs(path)
	if len(got) != 3 || got[0] != "A" || got[1] != "B" || got[2] != "C" {
		t.Fatalf("path A→B→C expected, got %v", got)
	}
	if _, ok := shortestPath(ctx, src, "A", "X", 7); ok {
		t.Fatalf("A→X must be reported unreachable, not bridged")
	}
	if p, ok := shortestPath(ctx, src, "A", "A", 7); !ok || len(p) != 1 {
		t.Fatalf("from==to should be the single node, got %v ok=%v", p, ok)
	}
}

// The decision's hard invariant: when a fuzzy seed is AMBIGUOUS (>1 substring
// match), resolveSeed must return candidates and NEVER silently pick one.
func TestResolveSeed_FuzzyFallbackNeverSilentlyPicks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "cg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := codebase.NewSymbolStore(db)
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	src := `package p
func FrameProblem() {}
func FrameThing() {}
func Lonely() {}
`
	if err := os.WriteFile(filepath.Join(root, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexFileSymbols(ctx, root, "p.go"); err != nil {
		t.Fatal(err)
	}
	svc := &Service{symbols: store}

	// Exact name → seed, not fuzzy.
	seed, cands, fuzzy, err := svc.resolveSeed(ctx, "FrameProblem", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if seed.Name != "FrameProblem" || len(cands) != 0 || fuzzy {
		t.Fatalf("exact name should resolve directly, got seed=%q cands=%d fuzzy=%v", seed.Name, len(cands), fuzzy)
	}

	// Partial name matching >1 → candidates, fuzzy, NO seed (never silently pick).
	seed, cands, fuzzy, err = svc.resolveSeed(ctx, "Frame", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if seed.ID != "" {
		t.Fatalf("ambiguous fuzzy MUST NOT pick a seed silently, got %q", seed.Name)
	}
	if len(cands) != 2 || !fuzzy {
		t.Fatalf("ambiguous fuzzy should return 2 candidates + fuzzy=true, got cands=%d fuzzy=%v", len(cands), fuzzy)
	}

	// Partial name matching exactly 1 → that single fuzzy hit, labeled fuzzy.
	seed, cands, fuzzy, err = svc.resolveSeed(ctx, "Lone", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if seed.Name != "Lonely" || len(cands) != 0 || !fuzzy {
		t.Fatalf("single fuzzy hit should resolve labeled fuzzy, got seed=%q cands=%d fuzzy=%v", seed.Name, len(cands), fuzzy)
	}

	// No match at all → nothing.
	seed, cands, _, err = svc.resolveSeed(ctx, "Zzz", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if seed.ID != "" || len(cands) != 0 {
		t.Fatalf("no match should resolve to nothing, got seed=%q cands=%d", seed.Name, len(cands))
	}
}
