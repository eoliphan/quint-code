package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestEdgeResolverPort_GoComposesAndIsPluggable verifies the language seam: the
// registry picks the Go adapter by extension, the adapter composes its resolvers
// behind the EdgeResolver port (using the SymbolView read-port), AND languages
// without an adapter yet return nil gracefully — the architecture is pluggable,
// not Go-locked.
func TestEdgeResolverPort_GoComposesAndIsPluggable(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	write := func(name, src string) {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/proj\n\ngo 1.25\n")
	write("main.go", "package main\n\nimport \"example.com/proj/util\"\n\nfunc Run() { util.Help() }\n")
	write("util/util.go", "package util\n\nfunc Help() {}\n")
	for _, f := range []string{"main.go", "util/util.go"} {
		if err := st.IndexFileSymbols(ctx, root, f); err != nil {
			t.Fatal(err)
		}
	}

	reg := NewRegistry()

	resolver := reg.ResolverForFile("main.go")
	if resolver == nil || resolver.Language() != "go" {
		t.Fatalf("expected a Go edge resolver for .go, got %v", resolver)
	}

	// *SymbolStore is passed as the SymbolView read-port.
	edges, err := resolver.ResolveFileEdges(ctx, root, "main.go", st)
	if err != nil {
		t.Fatal(err)
	}
	runNodes, _ := st.GetByName(ctx, "Run")
	helpNodes, _ := st.GetByName(ctx, "Help")
	found := false
	for _, e := range edges {
		if e.SrcID == runNodes[0].ID && e.DstID == helpNodes[0].ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("composed resolver should produce Run→util.Help, got %+v", edges)
	}

	// Python now has an edge resolver too (class-inheritance `extends` edges).
	if py := reg.ResolverForFile("x.py"); py == nil || py.Language() != "python" {
		t.Fatalf("Python should now have an edge resolver, got %v", py)
	}

	// THE SEAM still holds for languages without an adapter yet — nil gracefully,
	// rather than a Go-specific assumption. Adding one is one adapter + one
	// registry line (as Go and Python did).
	if reg.ResolverForFile("x.rs") != nil {
		t.Fatal("Rust must have no edge resolver yet — the pluggable seam")
	}
	if reg.ResolverForFile("x.c") != nil {
		t.Fatal("C must have no edge resolver yet — the pluggable seam")
	}
}
