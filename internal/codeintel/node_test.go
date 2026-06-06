package codeintel

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/codebase"

	_ "modernc.org/sqlite"
)

func TestMatchSymbol_ClosestLineSameReceiver(t *testing.T) {
	orig := codebase.CodeSymbol{Name: "Do", Receiver: "Store", StartLine: 40}
	syms := []codebase.CodeSymbol{
		{Name: "Do", Receiver: "Cache", StartLine: 41}, // wrong receiver
		{Name: "Do", Receiver: "Store", StartLine: 48}, // shifted by edit — should win
		{Name: "Other", Receiver: "Store", StartLine: 42},
	}
	got, ok := matchSymbol(syms, orig)
	if !ok || got.StartLine != 48 || got.Receiver != "Store" {
		t.Fatalf("expected Store.Do@48, got %+v (ok=%v)", got, ok)
	}
	if _, ok := matchSymbol(syms, codebase.CodeSymbol{Name: "Gone"}); ok {
		t.Fatalf("removed symbol should not match")
	}
}

func TestMethodsOfAndContainerKind(t *testing.T) {
	syms := []codebase.CodeSymbol{
		{Name: "Get", Receiver: "Store"},
		{Name: "Set", Receiver: "Store"},
		{Name: "Free", Receiver: ""},
		{Name: "Get", Receiver: "Cache"},
	}
	got := methodsOf(syms, "Store")
	if len(got) != 2 {
		t.Fatalf("Store has 2 methods, got %d", len(got))
	}
	for _, k := range []string{"type", "interface", "class", "struct"} {
		if !isContainerKind(k) {
			t.Fatalf("%q should be a container kind", k)
		}
	}
	if isContainerKind("func") || isContainerKind("method") {
		t.Fatalf("callables are not container kinds")
	}
}

// The P3 keystone, end-to-end: a file edited after indexing must yield a
// byte-exact body from the NEW content (re-indexed), never the stale offsets'
// bytes — proving re-read + re-hash, not a stored-hash compare.
func TestFreshBody_ReReadsEditedFile(t *testing.T) {
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
	svc := &Service{symbols: store}

	rel := "x.go"
	v1 := "package x\n\nfunc Target() int { return 1 }\n"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexFileSymbols(ctx, root, rel); err != nil {
		t.Fatal(err)
	}
	syms, _ := store.GetByName(ctx, "Target")
	if len(syms) != 1 {
		t.Fatalf("expected 1 Target, got %d", len(syms))
	}
	stale := syms[0]

	// Edit: prepend lines (shifts the symbol down) AND change the body.
	v2 := "package x\n\n// inserted\n// lines\nfunc Target() int { return 42 }\n"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}

	body, fresh, reindexed, freshSym, err := svc.freshBody(ctx, root, stale)
	if err != nil {
		t.Fatal(err)
	}
	if !reindexed {
		t.Fatalf("an edited file must trigger a re-index")
	}
	if !fresh {
		t.Fatalf("after re-index the body must verify byte-exact")
	}
	if !strings.Contains(string(body), "return 42") {
		t.Fatalf("body must reflect the EDITED source, got %q", body)
	}
	if freshSym.StartLine <= stale.StartLine {
		t.Fatalf("re-resolved symbol should have shifted down: stale=%d fresh=%d", stale.StartLine, freshSym.StartLine)
	}
	// The stale offsets, sliced from the NEW content, would NOT verify — the
	// whole point of re-reading.
	if _, ok := codebase.VerifyBody([]byte(v2), stale); ok {
		t.Fatalf("stale offsets should fail verification against edited content")
	}
}
