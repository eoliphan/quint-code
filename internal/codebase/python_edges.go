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

// extractPythonClassBases parses a Python file and returns each class with the
// identifier names in its superclass list. Pure relative to file content; uses
// the same tree-sitter grammar as symbol extraction. A base that is not a plain
// identifier or a `pkg.Base` attribute (e.g. a keyword arg metaclass=…, or a
// subscript Generic[T]) yields nothing for that base — never a guessed name.
func extractPythonClassBases(content []byte) []classBases {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil
	}
	defer tree.Close()

	q, err := sitter.NewQuery([]byte("(class_definition) @c"), python.GetLanguage())
	if err != nil {
		return nil
	}
	defer q.Close()
	qc := sitter.NewQueryCursor()
	qc.Exec(q, tree.RootNode())

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

// ResolveFileEdges makes PythonLang an EdgeResolver: it emits `extends` edges
// (subclass -> base) for class inheritance resolvable WITHIN the file's package.
// A base that does not resolve to exactly one local class (stdlib / third-party /
// cross-module / ambiguous) is dropped — an unresolved base is an absent edge,
// never a wrong one. Call edges are deferred (Python's dynamic dispatch cannot be
// resolved soundly from the AST alone). Heuristic provenance: the base is explicit
// in source, but name resolution here is package-scoped without import analysis.
func (p *PythonLang) ResolveFileEdges(ctx context.Context, projectRoot, relPath string, symbols SymbolView) ([]CodeEdge, error) {
	content, err := os.ReadFile(filepath.Join(projectRoot, relPath))
	if err != nil {
		return nil, nil
	}
	classes := extractPythonClassBases(content)
	if len(classes) == 0 {
		return nil, nil
	}

	fileSyms, err := symbols.GetByFile(ctx, relPath)
	if err != nil {
		return nil, err
	}
	pkgSyms, err := symbols.GetByDir(ctx, filepath.Dir(relPath))
	if err != nil {
		return nil, err
	}

	// Package-local class symbols by name (for base resolution) and this file's
	// class symbols (for the subclass endpoint).
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
	return dedupeEdges(edges), nil
}
