package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestSearchSymbols_GeneratedRankedLast confirms a generated stub sharing a name
// with hand-written code ranks after it. The hand-written file is named to sort
// LATER alphabetically than the generated one, so only the generated-file penalty
// (not the path tiebreaker) can put it first.
func TestSearchSymbols_GeneratedRankedLast(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte("type User struct {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("pkg", "auser.pb.go")) // generated, sorts first alphabetically
	write(filepath.Join("pkg", "zuser.go"))    // hand-written, sorts last alphabetically

	hits, err := st.SearchSymbols(ctx, "User", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected both User symbols, got %d", len(hits))
	}
	if filepath.Base(hits[0].FilePath) != "zuser.go" {
		t.Errorf("hand-written zuser.go should rank first, got %s (generated stub surfaced ahead of real impl)", hits[0].FilePath)
	}
}
