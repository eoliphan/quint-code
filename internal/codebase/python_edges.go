package codebase

import (
	"context"
	"os"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

// classBases is one Python class declaration and the base-class names listed in
// its superclass clause.
type classBases struct {
	name  string
	bases []string
}

// extractPythonClassBases returns each class in a parsed Python file with the
// identifier names in its superclass list. Pure relative to (root, content). A
// base that is not a plain identifier or a `pkg.Base` attribute (e.g. a keyword
// arg metaclass=…, or a subscript Generic[T]) yields nothing for that base —
// never a guessed name.
func extractPythonClassBases(root *sitter.Node, content []byte) []classBases {
	q, err := sitter.NewQuery([]byte("(class_definition) @c"), python.GetLanguage())
	if err != nil {
		return nil
	}
	defer q.Close()
	qc := sitter.NewQueryCursor()
	qc.Exec(q, root)

	var out []classBases
	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		for _, capture := range m.Captures {
			cd := capture.Node
			nameNode := cd.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			cb := classBases{name: nameNode.Content(content)}
			if sc := cd.ChildByFieldName("superclasses"); sc != nil {
				for i := 0; i < int(sc.NamedChildCount()); i++ {
					if name := simpleBaseName(sc.NamedChild(i), content); name != "" {
						cb.bases = append(cb.bases, name)
					}
				}
			}
			out = append(out, cb)
		}
	}
	return out
}

// simpleBaseName returns the resolvable identifier of a base expression: a plain
// identifier (Base) or the trailing attribute of pkg.Base. Anything else (keyword
// arg, subscript, call) yields "" — skipped, not guessed.
func simpleBaseName(n *sitter.Node, content []byte) string {
	switch n.Type() {
	case "identifier":
		return n.Content(content)
	case "attribute":
		if attr := n.ChildByFieldName("attribute"); attr != nil {
			return attr.Content(content)
		}
	}
	return ""
}

// ResolveFileEdges makes PythonLang an EdgeResolver. It emits two edge families:
//   - `extends` edges (subclass -> base) from class inheritance, resolved within
//     the file's package (heuristic provenance — no import analysis on bases);
//   - `call` edges, resolved through import analysis (file-local defs, names
//     imported via `from M import N`, and module-qualified `m.foo()` calls) with
//     the same exactly-1-or-drop discipline (static provenance).
//
// Every unresolved base or call is an absent edge, never a guessed one. Dynamic
// instance-method dispatch (`obj.method()` where obj is not an imported module)
// is deliberately dropped — it cannot be typed soundly from the AST alone.
func (p *PythonLang) ResolveFileEdges(ctx context.Context, projectRoot, relPath string, symbols SymbolView) ([]CodeEdge, error) {
	content, err := os.ReadFile(filepath.Join(projectRoot, relPath))
	if err != nil {
		return nil, nil
	}

	// Parse the file ONCE; every extractor reads the shared tree.
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, nil
	}
	defer tree.Close()
	root := tree.RootNode()

	fileSyms, err := symbols.GetByFile(ctx, relPath)
	if err != nil {
		return nil, err
	}
	pkgSyms, err := symbols.GetByDir(ctx, filepath.Dir(relPath))
	if err != nil {
		return nil, err
	}

	var edges []CodeEdge
	edges = append(edges, pythonHeritageEdges(root, content, relPath, fileSyms, pkgSyms)...)

	lookup := func(name string) []CodeSymbol {
		s, _ := symbols.GetByName(ctx, name)
		return s
	}
	imports := extractPythonImports(root, content, filepath.Dir(relPath))
	calls, callbacks := extractPythonCallsAndCallbacks(root, content)
	edges = append(edges, resolvePythonCallEdges(relPath, fileSyms, pkgSyms, calls, callbacks, imports, lookup)...)

	return dedupeEdges(edges), nil
}

// pythonHeritageEdges resolves class-inheritance `extends` edges within the
// package: a base that does not resolve to exactly one local class (stdlib /
// third-party / cross-module / ambiguous) is dropped, never guessed. Pure.
func pythonHeritageEdges(root *sitter.Node, content []byte, relPath string, fileSyms, pkgSyms []CodeSymbol) []CodeEdge {
	classes := extractPythonClassBases(root, content)
	if len(classes) == 0 {
		return nil
	}

	pkgClasses := map[string][]CodeSymbol{}
	for _, s := range pkgSyms {
		if s.Kind == "class" {
			pkgClasses[s.Name] = append(pkgClasses[s.Name], s)
		}
	}
	fileClass := map[string]CodeSymbol{}
	for _, s := range fileSyms {
		if s.Kind == "class" {
			fileClass[s.Name] = s
		}
	}

	var edges []CodeEdge
	for _, c := range classes {
		sub, ok := fileClass[c.name]
		if !ok {
			continue
		}
		for _, b := range c.bases {
			if b == c.name {
				continue
			}
			cands := pkgClasses[b]
			if len(cands) != 1 {
				continue // unresolved (external) or ambiguous — drop, don't guess
			}
			edges = append(edges, CodeEdge{
				SrcID:      sub.ID,
				DstID:      cands[0].ID,
				Kind:       EdgeExtends,
				FilePath:   relPath,
				Provenance: ProvenanceHeuristic,
			})
		}
	}
	return edges
}
