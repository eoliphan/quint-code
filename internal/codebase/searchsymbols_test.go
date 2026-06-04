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

func TestSearchSymbols_FuzzyTypoFallback(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	src := `package p
func Authenticate() {}
func Logout() {}
`
	if err := os.WriteFile(filepath.Join(root, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, "p.go"); err != nil {
		t.Fatal(err)
	}

	// "Athenticate" is missing a 'u' — NOT a substring of "Authenticate", so the
	// substring pass finds nothing and the edit-distance fallback (distance 1)
	// resolves the typo (dec-20260604-3aaad199).
	got, err := st.SearchSymbols(ctx, "Athenticate", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range got {
		if s.Name == "Authenticate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("typo 'Athenticate' should fuzzy-match Authenticate, got %v", got)
	}

	// A far-off query still matches nothing — fuzzy tolerates typos, not garbage
	// (no-wrong-edge: never invent a match).
	if e, _ := st.SearchSymbols(ctx, "qwertyuiopzxcv", 10); len(e) != 0 {
		t.Errorf("distant query must not match, got %d", len(e))
	}
}

func TestSearchSymbols_FieldQualifiedKindFilter(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	// Two symbols share the name "Handler" — a func and a type.
	src := `package p
func Handler() {}
type Handler struct{}
`
	if err := os.WriteFile(filepath.Join(root, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, "p.go"); err != nil {
		t.Fatal(err)
	}

	// No filter: both kinds surface.
	if all, _ := st.SearchSymbols(ctx, "Handler", 10); len(all) < 2 {
		t.Fatalf("bare 'Handler' should match both func and type, got %d", len(all))
	}

	// kind:func narrows to the function only (dec-20260604-3aaad199).
	got, err := st.SearchSymbols(ctx, "Handler kind:func", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("kind:func should still match the Handler func")
	}
	for _, s := range got {
		if s.Kind != "func" {
			t.Errorf("kind:func filter leaked a %q symbol", s.Kind)
		}
	}
}
