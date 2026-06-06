package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A Python subclass yields one extends edge to a base resolvable within the
// package; a base that is not a local class (external/third-party) is dropped.
func TestPythonExtendsEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `class Animal:
    def sound(self):
        return ""


class Dog(Animal):
    def sound(self):
        return "woof"


class Robot(Machine):
    pass
`
	rel := filepath.Join("pkg", "models.py")
	if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
		t.Fatal(err)
	}

	py := &PythonLang{}
	edges, err := py.ResolveFileEdges(ctx, root, rel, st)
	if err != nil {
		t.Fatal(err)
	}

	// Exactly one edge: Dog -> Animal. Robot -> Machine is dropped (Machine is
	// not a local class — never invented).
	if len(edges) != 1 {
		t.Fatalf("expected exactly 1 extends edge (Dog->Animal), got %d: %+v", len(edges), edges)
	}
	e := edges[0]
	if e.Kind != EdgeExtends {
		t.Errorf("kind = %q, want extends", e.Kind)
	}
	byName := map[string]CodeSymbol{}
	for _, s := range mustSyms(t, ctx, st, rel) {
		byName[s.ID] = s
	}
	if byName[e.SrcID].Name != "Dog" || byName[e.DstID].Name != "Animal" {
		t.Fatalf("edge endpoints = %s->%s, want Dog->Animal", byName[e.SrcID].Name, byName[e.DstID].Name)
	}
}

// TestPythonHeritage_ImportedBaseNotShadowed is the regression for the
// heritage wrong-edge bug: an imported base must resolve to the imported class,
// never to an unimported same-named class in a sibling module (the decoy).
func TestPythonHeritage_ImportedBaseNotShadowed(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	for _, d := range []string{"pkg", "other"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, src string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("other", "real.py"), "class Animal:\n    pass\n")
	write(filepath.Join("pkg", "decoy.py"), "class Animal:\n    pass\n") // shadow — must NOT win
	zooRel := filepath.Join("pkg", "zoo.py")
	write(zooRel, "from other.real import Animal\n\n\nclass Dog(Animal):\n    pass\n")

	py := &PythonLang{}
	edges, err := py.ResolveFileEdges(ctx, root, zooRel, st)
	if err != nil {
		t.Fatal(err)
	}

	var extendsEdges int
	for _, e := range edges {
		if e.Kind != EdgeExtends {
			continue
		}
		extendsEdges++
		dst, ok, err := st.GetByID(ctx, e.DstID)
		if err != nil {
			t.Fatal(err)
		}
		if !ok || filepath.ToSlash(dst.FilePath) != "other/real.py" {
			t.Errorf("Dog should extend other/real.py::Animal (the import), got %s::%s — decoy shadowed the imported base", dst.FilePath, dst.Name)
		}
	}
	if extendsEdges != 1 {
		t.Errorf("expected exactly 1 extends edge (Dog->imported Animal), got %d", extendsEdges)
	}
}

func mustSyms(t *testing.T, ctx context.Context, st *SymbolStore, rel string) []CodeSymbol {
	t.Helper()
	syms, err := st.GetByFile(ctx, rel)
	if err != nil {
		t.Fatal(err)
	}
	return syms
}
