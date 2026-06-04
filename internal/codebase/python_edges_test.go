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

func mustSyms(t *testing.T, ctx context.Context, st *SymbolStore, rel string) []CodeSymbol {
	t.Helper()
	syms, err := st.GetByFile(ctx, rel)
	if err != nil {
		t.Fatal(err)
	}
	return syms
}
