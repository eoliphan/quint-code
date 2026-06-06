package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const dispatchGo = `package p

type Greeter interface {
	Greet()
}

type Eng struct{}

func (e Eng) Greet() {}

type Fr struct{}

func (f Fr) Greet() {}

type Partial struct{}

func (x Partial) Other() {}

func Run(g Greeter) {
	g.Greet()
}
`

func findMethodID(syms []CodeSymbol, recv, name string) string {
	for _, s := range syms {
		if s.Kind == "method" && s.Receiver == recv && s.Name == name {
			return s.ID
		}
	}
	return ""
}

func ifaceMap(defs []InterfaceDef) map[string]InterfaceDef {
	m := map[string]InterfaceDef{}
	for _, d := range defs {
		m[d.Name] = d
	}
	return m
}

// TestP1cInterfaceDispatch is the P1c gate: a call x.M() where x's DECLARED type
// is a known interface resolves to ALL structurally-satisfying impls (the
// dispatch fan), and a type that does NOT satisfy the interface is excluded.
func TestP1cInterfaceDispatch(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	rel := "p.go"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(dispatchGo), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
		t.Fatal(err)
	}

	fileSyms, _ := st.GetByFile(ctx, rel)
	defs, err := ExtractGoInterfaces(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	sigs, err := ExtractGoSignatures(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := ExtractCallSites(root, rel)
	if err != nil {
		t.Fatal(err)
	}

	// implSymbols scoped to this file (= the package).
	edges := ResolveInterfaceDispatchEdges(rel, fileSyms, sites, sigs, ifaceMap(defs), fileSyms)

	if len(edges) != 2 {
		t.Fatalf("expected 2 dispatch edges (Eng+Fr Greet), got %d: %+v", len(edges), edges)
	}
	dst := map[string]bool{}
	for _, e := range edges {
		if e.Kind != EdgeInterfaceDispatch || e.Provenance != ProvenanceHeuristic {
			t.Fatalf("dispatch edge must be interface_dispatch/heuristic: %+v", e)
		}
		dst[e.DstID] = true
	}
	if !dst[findMethodID(fileSyms, "Eng", "Greet")] || !dst[findMethodID(fileSyms, "Fr", "Greet")] {
		t.Fatalf("expected edges to Eng.Greet + Fr.Greet, got %v", dst)
	}
	if dst[findMethodID(fileSyms, "Partial", "Other")] {
		t.Fatal("Partial does NOT satisfy Greeter (no Greet) — must be excluded")
	}
}

// TestP1cConcreteReceiverNotDispatch: a call on a receiver whose declared type
// is a concrete type (not an interface) produces NO dispatch edge.
func TestP1cConcreteReceiverNotDispatch(t *testing.T) {
	src := `package p

type Eng struct{}

func (e Eng) Greet() {}

func Run(e Eng) {
	e.Greet()
}
`
	st, root := newSymbolStore(t)
	ctx := context.Background()
	rel := "c.go"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
		t.Fatal(err)
	}
	fileSyms, _ := st.GetByFile(ctx, rel)
	defs, _ := ExtractGoInterfaces(root, rel) // none
	sigs, _ := ExtractGoSignatures(root, rel)
	sites, _ := ExtractCallSites(root, rel)
	edges := ResolveInterfaceDispatchEdges(rel, fileSyms, sites, sigs, ifaceMap(defs), fileSyms)
	if len(edges) != 0 {
		t.Fatalf("concrete receiver must yield no dispatch edge, got %+v", edges)
	}
}
