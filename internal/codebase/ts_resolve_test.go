package codebase

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestTSPathAliasResolution confirms a tsconfig `paths` alias (`@/*` → `src/*`)
// lets an aliased import resolve to a cross-module call edge — and that the
// tsconfig is parsed despite JSONC comments and trailing commas.
func TestTSPathAliasResolution(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	tsconfig := `{
  // editor-style comment
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"], /* the src alias */
    },
  },
}`
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(tsconfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, src string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("src", "util.ts"), "export function helper() { return 1 }\n")
	appRel := filepath.Join("src", "app.ts")
	write(appRel, "import { helper } from '@/util'\n\nfunction run() {\n  helper()\n}\n")

	jsts := &JSTSLang{}
	edges, err := jsts.ResolveFileEdges(ctx, root, appRel, st)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCallEdge(t, ctx, st, edges, "run", "helper") {
		t.Errorf("aliased import '@/util' should resolve to call edge run->helper; got %v", edges)
	}
}

// TestTSWorkspaceResolution confirms a monorepo workspace package import
// (`@scope/ui`) resolves to the member directory's index module.
func TestTSWorkspaceResolution(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"workspaces":["packages/*"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "packages", "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "packages", "ui", "package.json"), []byte(`{"name":"@scope/ui"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(rel, src string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("packages", "ui", "index.ts"), "export function widget() { return 1 }\n")
	mainRel := filepath.Join("app", "main.ts")
	write(mainRel, "import { widget } from '@scope/ui'\n\nfunction run() {\n  widget()\n}\n")

	jsts := &JSTSLang{}
	edges, err := jsts.ResolveFileEdges(ctx, root, mainRel, st)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCallEdge(t, ctx, st, edges, "run", "widget") {
		t.Errorf("workspace import '@scope/ui' should resolve to call edge run->widget; got %v", edges)
	}
}

func TestStripJSONC(t *testing.T) {
	in := `{
  // line comment
  "a": 1, /* block */
  "b": "http://not-a-comment", // url inside string preserved
  "c": [1, 2,],
}`
	out := stripJSONC(in)
	var got struct {
		A int    `json:"a"`
		B string `json:"b"`
		C []int  `json:"c"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stripped JSONC must parse: %v\n--- stripped ---\n%s", err, out)
	}
	if got.A != 1 || got.B != "http://not-a-comment" || len(got.C) != 2 {
		t.Errorf("parsed wrong: %+v (url must survive, trailing commas gone)", got)
	}
}

// hasCallEdge reports whether edges contain a call edge srcName->dstName.
func hasCallEdge(t *testing.T, ctx context.Context, st *SymbolStore, edges []CodeEdge, srcName, dstName string) bool {
	t.Helper()
	for _, e := range edges {
		if e.Kind == EdgeCall && edgeName(t, ctx, st, e.SrcID) == srcName && edgeName(t, ctx, st, e.DstID) == dstName {
			return true
		}
	}
	return false
}
