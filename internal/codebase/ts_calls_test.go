package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestTSCrossModuleCallEdges verifies import-resolved call edges for TypeScript: a
// named import `{ helper }` and a namespace `ns.shared()` both resolve to their
// cross-file definitions, while an unresolved name and an instance-method call are
// dropped (no wrong edge).
func TestTSCrossModuleCallEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	bar := `export function helper() { return 1 }
export function shared() { return 2 }
`
	main := `import { helper } from './bar'
import * as bar from './bar'

function run() {
  helper()
  bar.shared()
  missing()
  obj.method()
}
`
	write := func(rel, src string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("pkg", "bar.ts"), bar)
	mainRel := filepath.Join("pkg", "main.ts")
	write(mainRel, main)

	jsts := &JSTSLang{}
	edges, err := jsts.ResolveFileEdges(ctx, root, mainRel, st)
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
		t.Errorf("missing named-import call edge run->helper; got %v", calls)
	}
	if !calls["run->shared"] {
		t.Errorf("missing namespace call edge run->shared; got %v", calls)
	}
	if len(calls) != 2 {
		t.Errorf("expected exactly 2 call edges (missing() and obj.method() dropped), got %d: %v", len(calls), calls)
	}
}

// TestTSCallbackEdges confirms a function passed as a callback argument gets a
// heuristic callback edge from the enclosing function, while a non-function
// argument is dropped.
func TestTSCallbackEdges(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	handlers := `export function onClick() { return 1 }
`
	main := `import { onClick } from './handlers'

function setup(btn) {
  btn.addEventListener('click', onClick)
  console.log(btn)
}
`
	write := func(rel, src string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("pkg", "handlers.ts"), handlers)
	mainRel := filepath.Join("pkg", "main.ts")
	write(mainRel, main)

	jsts := &JSTSLang{}
	edges, err := jsts.ResolveFileEdges(ctx, root, mainRel, st)
	if err != nil {
		t.Fatal(err)
	}

	cb := 0
	found := false
	for _, e := range edges {
		if e.Kind != EdgeCallback {
			continue
		}
		cb++
		if edgeName(t, ctx, st, e.SrcID) == "setup" && edgeName(t, ctx, st, e.DstID) == "onClick" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing callback edge setup->onClick")
	}
	if cb != 1 {
		t.Errorf("expected exactly 1 callback edge (console.log(btn) arg is not a function), got %d", cb)
	}
}
