package codebase

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// tsNameImport binds a name introduced by `import { Orig as Local } from './m'`
// to the resolved target-module base path and the original exported name.
type tsNameImport struct {
	base string // project-relative module path without extension (e.g. "pkg/bar")
	orig string // the name as exported by the target module
}

// tsImports is the resolved relative-import surface of one JS/TS file:
//   - names:      local name -> {target base, original name} for `import { N }`
//   - namespaces: alias       -> target base                  for `import * as ns`
//
// Bare (non-relative) imports are external dependencies and are not recorded —
// they never resolve to a project node.
type tsImports struct {
	names      map[string]tsNameImport
	namespaces map[string]string
}

// extractTSImports reads the relative `import ... from './path'` statements of a
// JS/TS file and resolves each module specifier to a project-relative base path
// (extension stripped) against the importing file's directory. Pure.
func extractTSImports(content []byte, lang *sitter.Language, fileDir string) tsImports {
	res := tsImports{names: map[string]tsNameImport{}, namespaces: map[string]string{}}
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return res
	}
	defer tree.Close()

	root := tree.RootNode()
	for i := 0; i < int(root.NamedChildCount()); i++ {
		n := root.NamedChild(i)
		if n.Type() != "import_statement" {
			continue
		}
		source := n.ChildByFieldName("source")
		if source == nil {
			continue
		}
		raw := strings.Trim(source.Content(content), "'\"`")
		if !strings.HasPrefix(raw, ".") {
			continue // external dependency — never a project node
		}
		base := strings.TrimPrefix(filepath.ToSlash(filepath.Join(fileDir, raw)), "/")
		clause := firstChildOfType(n, "import_clause")
		if clause == nil {
			continue
		}
		collectTSImportClause(clause, content, base, &res)
	}
	return res
}

// collectTSImportClause records the bindings of one import clause: named imports
// `{ A, B as C }`, a namespace `* as ns`, or a bare default (skipped — the export
// name is not knowable from the import site).
func collectTSImportClause(clause *sitter.Node, content []byte, base string, res *tsImports) {
	for i := 0; i < int(clause.NamedChildCount()); i++ {
		c := clause.NamedChild(i)
		switch c.Type() {
		case "named_imports":
			for j := 0; j < int(c.NamedChildCount()); j++ {
				spec := c.NamedChild(j)
				if spec.Type() != "import_specifier" {
					continue
				}
				name := spec.ChildByFieldName("name")
				if name == nil {
					continue
				}
				local := name.Content(content)
				if alias := spec.ChildByFieldName("alias"); alias != nil {
					local = alias.Content(content)
				}
				res.names[local] = tsNameImport{base: base, orig: name.Content(content)}
			}
		case "namespace_import":
			if id := firstChildOfType(c, "identifier"); id != nil {
				res.namespaces[id.Content(content)] = base
			}
		}
	}
}

// extractTSCalls walks a JS/TS file's call expressions. An unqualified call
// (`foo()`) has empty Qualifier; a member call (`ns.foo()`, `obj.foo()`) carries
// the object's text as Qualifier. Pure relative to content.
func extractTSCalls(content []byte, lang *sitter.Language) []CallSite {
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil
	}
	defer tree.Close()

	q, err := sitter.NewQuery([]byte(`(call_expression) @c`), lang)
	if err != nil {
		return nil
	}
	defer q.Close()
	qc := sitter.NewQueryCursor()
	qc.Exec(q, tree.RootNode())

	var sites []CallSite
	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		for _, capture := range m.Captures {
			fn := capture.Node.ChildByFieldName("function")
			if fn == nil {
				continue
			}
			var callee, qualifier string
			switch fn.Type() {
			case "identifier":
				callee = fn.Content(content)
			case "member_expression":
				prop := fn.ChildByFieldName("property")
				obj := fn.ChildByFieldName("object")
				if prop == nil || obj == nil {
					continue
				}
				callee = prop.Content(content)
				qualifier = obj.Content(content)
			default:
				continue
			}
			if callee == "" {
				continue
			}
			sites = append(sites, CallSite{Callee: callee, Qualifier: qualifier, Line: int(capture.Node.StartPoint().Row) + 1})
		}
	}
	return sites
}

// tsFileMatchesBase reports whether a stored file path is the module named by a
// project-relative base — either `<base>.<ext>` or `<base>/index.<ext>`.
func tsFileMatchesBase(file, base string) bool {
	p := filepath.ToSlash(file)
	p = strings.TrimSuffix(p, filepath.Ext(p))
	return p == base || p == base+"/index"
}

// resolveTSImportedName finds the single func/class symbol named `name` defined in
// the target module base, or nil if zero/ambiguous. The cross-module primitive.
func resolveTSImportedName(name, base string, lookup func(string) []CodeSymbol) *CodeSymbol {
	var cands []CodeSymbol
	for _, s := range lookup(name) {
		if (s.Kind == "func" || s.Kind == "class") && tsFileMatchesBase(s.FilePath, base) {
			cands = append(cands, s)
		}
	}
	if len(cands) != 1 {
		return nil
	}
	return &cands[0]
}

// resolveTSCallEdges resolves call sites to definition nodes with the
// exactly-1-or-drop guard. Unqualified resolution order: a file-local def, then a
// named import, then a directory-local def. A member call resolves only when the
// qualifier is a namespace import. Pure — lookup injected so it stays storeless.
func resolveTSCallEdges(relPath string, fileSyms, pkgSyms []CodeSymbol, calls []CallSite, imports tsImports, lookup func(name string) []CodeSymbol) []CodeEdge {
	fileDefs := map[string][]CodeSymbol{}
	for _, s := range fileSyms {
		if s.Kind == "func" || s.Kind == "class" {
			fileDefs[s.Name] = append(fileDefs[s.Name], s)
		}
	}
	pkgDefs := map[string][]CodeSymbol{}
	for _, s := range pkgSyms {
		if s.Kind == "func" || s.Kind == "class" {
			pkgDefs[s.Name] = append(pkgDefs[s.Name], s)
		}
	}

	seen := map[string]bool{}
	var edges []CodeEdge
	for _, cs := range calls {
		caller := enclosingSymbol(fileSyms, cs.Line)
		if caller == nil {
			continue
		}
		var dst *CodeSymbol
		if cs.Qualifier == "" {
			dst = resolveTSUnqualified(cs.Callee, fileDefs, pkgDefs, imports, lookup)
		} else {
			dst = resolveTSNamespaceCall(cs.Qualifier, cs.Callee, imports, lookup)
		}
		if dst == nil || dst.ID == caller.ID {
			continue
		}
		key := caller.ID + "->" + dst.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, CodeEdge{
			SrcID:      caller.ID,
			DstID:      dst.ID,
			Kind:       EdgeCall,
			FilePath:   relPath,
			Line:       cs.Line,
			Provenance: ProvenanceStatic,
		})
	}
	return edges
}

func resolveTSUnqualified(callee string, fileDefs, pkgDefs map[string][]CodeSymbol, imports tsImports, lookup func(string) []CodeSymbol) *CodeSymbol {
	local := fileDefs[callee]
	b, imported := imports.names[callee]
	if len(local) == 1 && !imported {
		return &local[0]
	}
	if imported {
		return resolveTSImportedName(b.orig, b.base, lookup)
	}
	if pkg := pkgDefs[callee]; len(pkg) == 1 {
		return &pkg[0]
	}
	return nil
}

func resolveTSNamespaceCall(qualifier, callee string, imports tsImports, lookup func(string) []CodeSymbol) *CodeSymbol {
	base, ok := imports.namespaces[qualifier]
	if !ok {
		return nil // qualifier is a receiver / instance / unknown — drop
	}
	return resolveTSImportedName(callee, base, lookup)
}
