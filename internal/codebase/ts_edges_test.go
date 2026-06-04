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
