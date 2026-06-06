package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A concrete type whose method-name set covers an interface yields a single
// implements edge (type -> interface); a type with no methods yields none, and
// the interface itself is never a source.
func TestImplementsEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	src := `package p

type Animal interface {
	Sound() string
}

type Dog struct{}

func (d Dog) Sound() string { return "woof" }

type Rock struct{}
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

	interfaces := map[string]InterfaceDef{}
	defs, err := ExtractGoInterfaces(root, "p.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		interfaces[d.Name] = d
	}

	edges := ResolveImplementsEdges("p.go", syms, syms, interfaces)

	// Exactly one edge: Dog -> Animal. Rock (no methods) must not implement;
	// the Animal interface itself is never a source.
	if len(edges) != 1 {
		t.Fatalf("expected exactly 1 implements edge (Dog->Animal), got %d: %+v", len(edges), edges)
	}
	e := edges[0]
	if e.Kind != EdgeImplements {
		t.Errorf("kind = %q, want implements", e.Kind)
	}
	if e.Provenance != ProvenanceHeuristic {
		t.Errorf("provenance = %q, want heuristic (name-coverage is not signature-checked)", e.Provenance)
	}
	byID := map[string]CodeSymbol{}
	for _, s := range syms {
		byID[s.ID] = s
	}
	if byID[e.SrcID].Name != "Dog" || byID[e.DstID].Name != "Animal" {
		t.Fatalf("edge endpoints = %s->%s, want Dog->Animal", byID[e.SrcID].Name, byID[e.DstID].Name)
	}
}
