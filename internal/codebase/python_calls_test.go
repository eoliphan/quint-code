package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestPythonCrossModuleCallEdges verifies import-resolved call edges: a name
// imported via `from M import N` and a module-qualified `m.foo()` both resolve to
// their cross-file definitions, while an unresolved name and an instance-method
// call are dropped (no wrong edge).
func TestPythonCrossModuleCallEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	helpers := `def helper():
    return 1


def shared():
    return 2
`
	main := `from pkg.helpers import helper
import pkg.helpers as h


def run():
    helper()
    h.shared()
    missing()
    self.method()
`
	write := func(rel, src string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("pkg", "helpers.py"), helpers)
	mainRel := filepath.Join("pkg", "main.py")
	write(mainRel, main)

	py := &PythonLang{}
	edges, err := py.ResolveFileEdges(ctx, root, mainRel, st)
	if err != nil {
		t.Fatal(err)
	}

	calls := map[string]bool{}
	for _, e := range edges {
		if e.Kind == EdgeCall {
			calls[edgeName(t, ctx, st, e.SrcID)+"->"+edgeName(t, ctx, st, e.DstID)] = true
		}
	}
	if !calls["run->helper"] {
		t.Errorf("missing cross-module from-import call edge run->helper; got %v", calls)
	}
	if !calls["run->shared"] {
		t.Errorf("missing module-qualified call edge run->shared; got %v", calls)
	}
	if len(calls) != 2 {
		t.Errorf("expected exactly 2 call edges (missing() and self.method() dropped), got %d: %v", len(calls), calls)
	}
}

// TestPythonCallEdges_ClassAndExternalDrop confirms a cross-module class
// construction resolves to a call edge, while a call qualified by an external
// (non-project) module is dropped — no file backs `os`, so no edge is invented.
func TestPythonCallEdges_ClassAndExternalDrop(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	models := `class Animal:
    pass
`
	zoo := `import os
from pkg.models import Animal


def build():
    Animal()
    os.getcwd()
`
	write := func(rel, src string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("pkg", "models.py"), models)
	zooRel := filepath.Join("pkg", "zoo.py")
	write(zooRel, zoo)

	py := &PythonLang{}
	edges, err := py.ResolveFileEdges(ctx, root, zooRel, st)
	if err != nil {
		t.Fatal(err)
	}

	calls := map[string]bool{}
	for _, e := range edges {
		if e.Kind == EdgeCall {
			calls[edgeName(t, ctx, st, e.SrcID)+"->"+edgeName(t, ctx, st, e.DstID)] = true
		}
	}
	if !calls["build->Animal"] {
		t.Errorf("missing cross-module class-construction call edge build->Animal; got %v", calls)
	}
	if len(calls) != 1 {
		t.Errorf("expected exactly 1 call edge (os.getcwd() dropped as external), got %d: %v", len(calls), calls)
	}
}

// edgeName resolves a symbol ID to its name via a whole-project lookup.
func edgeName(t *testing.T, ctx context.Context, st *SymbolStore, id string) string {
	t.Helper()
	for _, dir := range []string{"pkg"} {
		syms, err := st.GetByDir(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range syms {
			if s.ID == id {
				return s.Name
			}
		}
	}
	return id
}
