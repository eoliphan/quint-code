package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TypeScript class extends/implements and interface extends each yield edges to
// package-local bases; a base not declared locally (imported/external) is dropped.
func TestTSHeritageEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `class Animal {}
interface Walker {}
interface Runner {}
class Dog extends Animal implements Walker, Runner {}
interface FastRunner extends Runner {}
class Cat extends External {}
`
	rel := filepath.Join("src", "zoo.ts")
	if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
		t.Fatal(err)
	}
	syms, err := st.GetByFile(ctx, rel)
	if err != nil {
		t.Fatal(err)
	}

	js := &JSTSLang{}
	edges, err := js.ResolveFileEdges(ctx, root, rel, st)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]CodeSymbol{}
	for _, s := range syms {
		byID[s.ID] = s
	}
	got := map[string]string{} // "Sub->Base" -> kind
	for _, e := range edges {
		got[byID[e.SrcID].Name+"->"+byID[e.DstID].Name] = string(e.Kind)
	}

	want := map[string]string{
		"Dog->Animal":        "extends",
		"Dog->Walker":        "implements",
		"Dog->Runner":        "implements",
		"FastRunner->Runner": "extends",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("edge %s = %q, want %q; all=%v", k, got[k], v, got)
		}
	}
	// Cat extends External must be dropped (External is not a local class).
	if _, ok := got["Cat->External"]; ok {
		t.Errorf("Cat->External must be dropped (external base), got %v", got)
	}
	if len(edges) != 4 {
		t.Fatalf("expected exactly 4 heritage edges, got %d: %v", len(edges), got)
	}
}

// TestTSHeritage_ImportedBaseNotShadowed is the regression for the heritage
// wrong-edge bug: an imported base type must resolve to the imported one, never
// to an unimported same-named type in a sibling module (the decoy).
func TestTSHeritage_ImportedBaseNotShadowed(t *testing.T) {
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
	write(filepath.Join("other", "real.ts"), "export class Animal {}\n")
	write(filepath.Join("pkg", "decoy.ts"), "export class Animal {}\n") // shadow — must NOT win
	zooRel := filepath.Join("pkg", "zoo.ts")
	write(zooRel, "import { Animal } from '../other/real'\n\nclass Dog extends Animal {}\n")

	js := &JSTSLang{}
	edges, err := js.ResolveFileEdges(ctx, root, zooRel, st)
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
		if !ok || filepath.ToSlash(dst.FilePath) != "other/real.ts" {
			t.Errorf("Dog should extend other/real.ts::Animal (the import), got %s::%s — decoy shadowed the imported base", dst.FilePath, dst.Name)
		}
	}
	if extendsEdges != 1 {
		t.Errorf("expected exactly 1 extends edge (Dog->imported Animal), got %d", extendsEdges)
	}
}
