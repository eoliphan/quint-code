package artifact

import (
	"context"
	"testing"
	"time"
)

// TestSearchByAffectedSymbol covers the symbol-granular fusion join: given a
// symbol (optionally scoped to a file), return the artifacts touching it.
func TestSearchByAffectedSymbol(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	mk := func(id, title string) *Artifact {
		return &Artifact{Meta: Meta{
			ID: id, Kind: KindDecisionRecord, Status: StatusActive,
			Title: title, CreatedAt: now, UpdatedAt: now,
		}, Body: "body"}
	}
	if err := store.Create(ctx, mk("dec-login", "JWT over sessions")); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, mk("dec-cache", "RWMutex cache")); err != nil {
		t.Fatal(err)
	}

	// Same symbol name "Login" in two different files — the disambiguation case.
	if err := store.SetAffectedSymbols(ctx, "dec-login", []AffectedSymbol{
		{FilePath: "auth/login.go", SymbolName: "Login", SymbolKind: "func"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAffectedSymbols(ctx, "dec-cache", []AffectedSymbol{
		{FilePath: "cache/store.go", SymbolName: "Login", SymbolKind: "func"},
	}); err != nil {
		t.Fatal(err)
	}

	// Unscoped: both "Login" symbols match.
	all, err := store.SearchByAffectedSymbol(ctx, "Login", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 artifacts for unscoped symbol 'Login', got %d", len(all))
	}

	// File-scoped: only the auth/login.go decision.
	scoped, err := store.SearchByAffectedSymbol(ctx, "Login", "auth/login.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].Meta.ID != "dec-login" {
		t.Fatalf("expected only dec-login scoped to auth/login.go, got %+v", scoped)
	}

	// Unknown symbol: empty.
	none, err := store.SearchByAffectedSymbol(ctx, "Nonexistent", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no artifacts for unknown symbol, got %d", len(none))
	}
}
