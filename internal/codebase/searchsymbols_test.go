package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchSymbols_SubstringRankAndEmpty(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	src := `package p
func Frame() {}
func FrameProblem() {}
func unframeHelper() {}
func Other() {}
`
	if err := os.WriteFile(filepath.Join(root, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, "p.go"); err != nil {
		t.Fatal(err)
	}

	got, err := st.SearchSymbols(ctx, "frame", 10)
	if err != nil {
		t.Fatal(err)
	}
	// All three names contain "frame" (case-insensitive); Other does not.
	names := map[string]bool{}
	for _, s := range got {
		names[s.Name] = true
	}
	if !names["Frame"] || !names["FrameProblem"] || !names["unframeHelper"] {
		t.Fatalf("substring match should find Frame/FrameProblem/unframeHelper, got %v", names)
	}
	if names["Other"] {
		t.Fatalf("Other must not match 'frame'")
	}
	// Ranking: exact 'Frame' (case-insensitive) ranks first; prefix 'FrameProblem'
	// before the mid-substring 'unframeHelper'.
	if got[0].Name != "Frame" {
		t.Errorf("exact match should rank first, got %q", got[0].Name)
	}
	if len(got) >= 3 && got[len(got)-1].Name != "unframeHelper" {
		t.Errorf("mid-substring match should rank last, got order %v", names)
	}

	// Empty query matches nothing (never "everything").
	if e, _ := st.SearchSymbols(ctx, "   ", 10); len(e) != 0 {
		t.Errorf("empty/whitespace query must match nothing, got %d", len(e))
	}
}
