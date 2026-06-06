package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const aGo = `package p

func Alpha() {
	Shared()
	_ = len("x")
}
`

// TestP1aIntraPackageCallEdges is the P1a gate: an unqualified intra-package
// call resolves to a real edge; a builtin/undefined call resolves to nothing
// (dropped, not invented); and an ambiguous candidate set is dropped, never
// fanned out (the over-resolution guard).
func TestP1aIntraPackageCallEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	edges := NewEdgeStore(st.db)
	if err := edges.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	// Package "p" split across two files (legal Go: each Shared in its own file
	// would collide — so use one Shared in b.go and a caller in a.go).
	mustWrite := func(name, src string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("a.go", aGo)
	mustWriteShared := `package p

func Shared() {}
`
	mustWrite("shared.go", mustWriteShared)

	for _, f := range []string{"a.go", "shared.go"} {
		if err := st.IndexFileSymbols(ctx, root, f); err != nil {
			t.Fatal(err)
		}
	}

	pkgSyms, err := st.GetByName(ctx, "Alpha")
	if err != nil || len(pkgSyms) != 1 {
		t.Fatalf("Alpha node: %v / %v", pkgSyms, err)
	}
	alphaID := pkgSyms[0].ID
	sharedNodes, err := st.GetByName(ctx, "Shared")
	if err != nil || len(sharedNodes) != 1 {
		t.Fatalf("Shared node: %v / %v", sharedNodes, err)
	}
	sharedID := sharedNodes[0].ID

	// Build pkg symbol set (both files), extract a.go's call sites, resolve.
	aSyms, _ := st.GetByFile(ctx, "a.go")
	sSyms, _ := st.GetByFile(ctx, "shared.go")
	pkg := append(append([]CodeSymbol{}, aSyms...), sSyms...)
	sites, err := ExtractCallSites(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	resolved := ResolveIntraPackageCallEdges("a.go", aSyms, pkg, sites)

	// Exactly one edge: Alpha → Shared. The len() call must NOT produce an edge.
	if len(resolved) != 1 {
		t.Fatalf("expected exactly 1 edge (Alpha→Shared); len() must drop. got %d: %+v", len(resolved), resolved)
	}
	if resolved[0].SrcID != alphaID || resolved[0].DstID != sharedID || resolved[0].Kind != EdgeCall || resolved[0].Provenance != ProvenanceStatic {
		t.Fatalf("edge mismatch: %+v (want %s→%s static call)", resolved[0], alphaID, sharedID)
	}

	// Round-trip through the store.
	if err := edges.ReplaceFileEdges(ctx, "a.go", resolved); err != nil {
		t.Fatal(err)
	}
	out, err := edges.OutEdges(ctx, alphaID)
	if err != nil || len(out) != 1 || out[0].DstID != sharedID {
		t.Fatalf("OutEdges(Alpha): %+v / %v", out, err)
	}
	in, err := edges.InEdges(ctx, sharedID)
	if err != nil || len(in) != 1 || in[0].SrcID != alphaID {
		t.Fatalf("InEdges(Shared): %+v / %v", in, err)
	}
}

// TestP1bCrossFileCallEdges is the P1b gate: a qualified call into another
// LOCAL package (pkg.Func) resolves cross-file to that package's exported func;
// an external import (fmt) does not resolve (dropped, not invented).
func TestP1bCrossFileCallEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	mustWrite := func(name, src string) {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("go.mod", "module example.com/proj\n\ngo 1.25\n")
	mustWrite("main.go", `package main

import (
	"fmt"

	"example.com/proj/util"
)

func Run() {
	util.Help()
	fmt.Println("x")
}
`)
	mustWrite("util/util.go", `package util

func Help() {}
`)

	for _, f := range []string{"main.go", "util/util.go"} {
		if err := st.IndexFileSymbols(ctx, root, f); err != nil {
			t.Fatal(err)
		}
	}

	runNodes, _ := st.GetByName(ctx, "Run")
	helpNodes, _ := st.GetByName(ctx, "Help")
	if len(runNodes) != 1 || len(helpNodes) != 1 {
		t.Fatalf("expected 1 Run + 1 Help node, got %d / %d", len(runNodes), len(helpNodes))
	}

	mainSyms, _ := st.GetByFile(ctx, "main.go")
	imports, err := ExtractGoImports(root, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	sites, err := ExtractCallSites(root, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(name string) []CodeSymbol {
		syms, _ := st.GetByName(ctx, name)
		return syms
	}

	edges := ResolveCrossFileCallEdges("main.go", mainSyms, sites, imports, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected exactly 1 cross-file edge (Run→util.Help); fmt.Println must drop. got %d: %+v", len(edges), edges)
	}
	if edges[0].SrcID != runNodes[0].ID || edges[0].DstID != helpNodes[0].ID {
		t.Fatalf("edge mismatch: %+v (want %s→%s)", edges[0], runNodes[0].ID, helpNodes[0].ID)
	}
}

// TestP1aOverResolutionDropsAmbiguous proves the exactly-1-or-drop guard: a
// callee with multiple package candidates yields NO edge (never a fan-out).
func TestP1aOverResolutionDropsAmbiguous(t *testing.T) {
	caller := CodeSymbol{ID: NodeID("a.go", "Caller", 1), FilePath: "a.go", Name: "Caller", Kind: "func", StartLine: 1, EndLine: 5}
	fileSyms := []CodeSymbol{caller}
	pkg := []CodeSymbol{
		caller,
		{ID: NodeID("a.go", "Dup", 10), FilePath: "a.go", Name: "Dup", Kind: "func", StartLine: 10, EndLine: 12},
		{ID: NodeID("b.go", "Dup", 3), FilePath: "b.go", Name: "Dup", Kind: "func", StartLine: 3, EndLine: 5},
	}
	sites := []CallSite{{Callee: "Dup", Line: 2}}

	edges := ResolveIntraPackageCallEdges("a.go", fileSyms, pkg, sites)
	if len(edges) != 0 {
		t.Fatalf("ambiguous callee (2 candidates) must drop, not fan out; got %+v", edges)
	}
}
