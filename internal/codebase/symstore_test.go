package codebase

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

const sampleGo = `package sample

type Foo struct{}
type Bar struct{}

func (f *Foo) Do() string { return "foo" }

func (b *Bar) Do() int { return 42 }

func Free() {}
`

func newSymbolStore(t *testing.T) (*SymbolStore, string) {
	t.Helper()
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "cg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := NewSymbolStore(db)
	if err := st.EnsureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st, root
}

// TestScanSymbols_PopulatesAcrossFiles checks the bulk-walk indexer reaches
// supported source files across directories.
func TestScanSymbols_PopulatesAcrossFiles(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.go"), []byte("package sub\nfunc Beta() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := NewScanner(st.db).ScanSymbols(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("expected >=2 files indexed, got %d", n)
	}
	if alpha, err := st.GetByName(ctx, "Alpha"); err != nil || len(alpha) != 1 {
		t.Fatalf("Alpha not indexed: %v / %v", alpha, err)
	}
	if beta, err := st.GetByName(ctx, "Beta"); err != nil || len(beta) != 1 {
		t.Fatalf("Beta (nested dir) not indexed: %v / %v", beta, err)
	}
}

func TestSymbolStore_GetByDirIncludesRootPackageFiles(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	files := map[string]string{
		"a.go":     "package p\nfunc Alpha() {}\n",
		"b.go":     "package p\nfunc Beta() {}\n",
		"sub/c.go": "package sub\nfunc Gamma() {}\n",
	}
	for name, src := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, name); err != nil {
			t.Fatal(err)
		}
	}

	syms, err := st.GetByDir(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, sym := range syms {
		got[sym.Name] = true
	}
	if !got["Alpha"] || !got["Beta"] {
		t.Fatalf("expected root package symbols Alpha and Beta, got %+v", syms)
	}
	if got["Gamma"] {
		t.Fatalf("expected nested package symbol Gamma to be excluded, got %+v", syms)
	}
}

// TestSymbolStore_OverloadNodesAndByteExactSlice is the P0 gate for the node
// store: two same-name methods persist as DISTINCT nodes, and each node's
// stored byte offsets slice the file back to its EXACT body.
func TestSymbolStore_OverloadNodesAndByteExactSlice(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	rel := "sample.go"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(sampleGo), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
		t.Fatal(err)
	}

	// GATE 2: two overloaded methods → two distinct nodes.
	dos, err := st.GetByName(ctx, "Do")
	if err != nil {
		t.Fatal(err)
	}
	if len(dos) != 2 {
		t.Fatalf("expected 2 distinct 'Do' nodes (overloads), got %d: %+v", len(dos), dos)
	}
	if dos[0].StartLine == dos[1].StartLine {
		t.Fatalf("overload nodes must differ by start_line (identity), got same: %+v", dos)
	}
	recv := map[string]bool{dos[0].Receiver: true, dos[1].Receiver: true}
	if !recv["Foo"] || !recv["Bar"] {
		t.Fatalf("expected receivers Foo + Bar, got %v", recv)
	}

	// GATE 1: byte-exact slice round-trip.
	content, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	wantByReceiver := map[string]string{
		"Foo": `func (f *Foo) Do() string { return "foo" }`,
		"Bar": `func (b *Bar) Do() int { return 42 }`,
	}
	for _, sym := range dos {
		body, ok := SliceBody(content, sym)
		if !ok {
			t.Fatalf("SliceBody failed for %s.Do", sym.Receiver)
		}
		if string(body) != wantByReceiver[sym.Receiver] {
			t.Fatalf("byte-exact mismatch for %s.Do:\n got: %q\nwant: %q", sym.Receiver, string(body), wantByReceiver[sym.Receiver])
		}
	}

	// Freshness: fresh right after index; stale after an edit.
	stale, err := st.FileSymbolsStale(ctx, root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("file should not be stale immediately after indexing")
	}
	edited := sampleGo + "\nfunc Added() {}\n"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err = st.FileSymbolsStale(ctx, root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatal("file should be stale after adding a symbol")
	}
	// rebuild-on-demand restores freshness.
	if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
		t.Fatal(err)
	}
	stale, err = st.FileSymbolsStale(ctx, root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("file should be fresh after re-indexing")
	}
}
