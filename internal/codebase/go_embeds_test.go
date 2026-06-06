package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Struct embedding and interface embedding each yield an embeds edge to the
// package-local embedded type; a named field is not an embedding.
func TestGoEmbedsEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	src := `package p

type Base struct{}

func (Base) Hello() {}

type Widget struct {
	Base
	name string
}

type Reader interface{ Read() }

type Writer interface{ Write() }

type ReadWriter interface {
	Reader
	Writer
}
`
	if err := os.WriteFile(filepath.Join(root, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, "p.go"); err != nil {
		t.Fatal(err)
	}
	syms, err := st.GetByFile(ctx, "p.go")
	if err != nil {
		t.Fatal(err)
	}

	edges := ResolveEmbedsEdges("p.go", syms, syms, ExtractGoEmbeds(root, "p.go"))

	// Expect Widget->Base, ReadWriter->Reader, ReadWriter->Writer. The named
	// field `name string` is NOT an embedding.
	got := map[string]bool{}
	byID := map[string]CodeSymbol{}
	for _, s := range syms {
		byID[s.ID] = s
	}
	for _, e := range edges {
		if e.Kind != EdgeEmbeds {
			t.Errorf("kind = %q, want embeds", e.Kind)
		}
		got[byID[e.SrcID].Name+"->"+byID[e.DstID].Name] = true
	}
	for _, want := range []string{"Widget->Base", "ReadWriter->Reader", "ReadWriter->Writer"} {
		if !got[want] {
			t.Errorf("missing embeds edge %s; got %v", want, got)
		}
	}
	if len(edges) != 3 {
		t.Fatalf("expected exactly 3 embeds edges, got %d: %v", len(edges), got)
	}
}
