package artifact

import (
	"context"
	"strings"
	"testing"
)

func TestNoteFactMode_RationaleOptional(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	haftDir := t.TempDir()

	// A fact with observations and a source, NO rationale, is valid and renders
	// as a fact (Observations section, no Rationale section).
	in := NoteInput{
		Title:        "MCP server is per-session, not a daemon",
		Observations: []string{"host spawns the MCP server per session", "no guaranteed long-lived process"},
		Evidence:     "internal/mcp + the MCP spec",
	}
	if v := ValidateNote(ctx, store, in); !v.OK {
		t.Fatalf("a fact with observations should be valid, got: %v", v.Warnings)
	}
	note, _, err := CreateNote(ctx, store, haftDir, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note.Body, "## Observations") {
		t.Errorf("fact body should have an Observations section: %q", note.Body)
	}
	if strings.Contains(note.Body, "## Rationale") {
		t.Errorf("fact with no rationale should have no Rationale section: %q", note.Body)
	}

	// A content-free note (no observation, no source, no rationale) is rejected.
	if v := ValidateNote(ctx, store, NoteInput{Title: "empty"}); v.OK {
		t.Errorf("a content-free note must be invalid")
	}
}

func TestNoteAnchors_PersistAsLinksAndRejectDead(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	haftDir := t.TempDir()

	dec := &Artifact{
		Meta: Meta{ID: "dec-20260604-target", Kind: KindDecisionRecord, Title: "Target decision"},
		Body: "x",
	}
	if err := store.Create(ctx, dec); err != nil {
		t.Fatal(err)
	}

	// An anchored fact: the typed anchor persists as a real link and surfaces as
	// a backlink of the anchored decision.
	note, _, err := CreateNote(ctx, store, haftDir, NoteInput{
		Title:        "fact governing the target",
		Observations: []string{"this constrains the decision"},
		Anchors:      []NoteAnchor{{Type: "governs", Ref: "dec-20260604-target"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	backs, err := store.GetBacklinks(ctx, "dec-20260604-target")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range backs {
		if b.Ref == note.Meta.ID && b.Type == "governs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("anchor should surface as a 'governs' backlink of the decision, got %v", backs)
	}

	// A dead anchor (target does not exist) rejects the whole note — no dead edge.
	if _, _, err := CreateNote(ctx, store, haftDir, NoteInput{
		Title:        "fact with a dead anchor",
		Observations: []string{"x"},
		Anchors:      []NoteAnchor{{Type: "about", Ref: "dec-does-not-exist"}},
	}); err == nil {
		t.Fatal("a note anchored to a non-existent target must be rejected")
	}
}

func TestNoteSymbolAnchors_PersistAndSurface(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	haftDir := t.TempDir()

	note, _, err := CreateNote(ctx, store, haftDir, NoteInput{
		Title:        "fact about SearchSymbols",
		Observations: []string{"it tolerates typos via bounded edit distance"},
		AffectedSymbols: []AffectedSymbol{{
			FilePath:   "internal/codebase/symstore.go",
			SymbolName: "SearchSymbols",
			SymbolKind: "method",
			Line:       230,
			EndLine:    266,
			Hash:       "deadbeef",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The fact surfaces at the exact symbol via the symbol-granular join.
	hits, err := store.SearchByAffectedSymbol(ctx, "SearchSymbols", "internal/codebase/symstore.go")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.Meta.ID == note.Meta.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("symbol-anchored fact should surface via SearchByAffectedSymbol, got %v", hits)
	}
}
