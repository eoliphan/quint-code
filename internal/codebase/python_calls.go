package codebase

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

// pyNameImport binds a name introduced by `from MODULE import NAME [as LOCAL]`
// to the module path fragment it came from and the original exported name.
type pyNameImport struct {
	frag string // module path fragment, slash-separated (e.g. "pkg/mod")
	orig string // the name as defined in the target module
}

// pythonImports is the resolved import surface of one Python file:
//   - names:   local name -> {module fragment, original name} for `from M import N`
//   - modules: qualifier  -> module fragment              for `import M [as alias]`
type pythonImports struct {
	names   map[string]pyNameImport
	modules map[string]string
}

// dottedToFrag turns a dotted Python module path ("a.b.c") into a slash path
// fragment ("a/b/c").
func dottedToFrag(dotted string) string {
	return strings.ReplaceAll(strings.TrimSpace(dotted), ".", "/")
}

// lastDotSegment returns the final segment of a (possibly dotted) name.
func lastDotSegment(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// pyModuleFrag resolves an `import_from_statement` module_name node to a slash
// path fragment, accounting for relative imports: leading dots in import_prefix
// walk up from the importing file's directory. Returns "" if unresolvable.
func pyModuleFrag(mod *sitter.Node, content []byte, fileDir string) string {
	switch mod.Type() {
	case "dotted_name":
		return dottedToFrag(mod.Content(content))
	case "relative_import":
		level := 0
		rest := ""
		for i := 0; i < int(mod.NamedChildCount()); i++ {
			c := mod.NamedChild(i)
			switch c.Type() {
			case "import_prefix":
				level = strings.Count(c.Content(content), ".")
			case "dotted_name":
				rest = dottedToFrag(c.Content(content))
			}
		}
		base := filepath.ToSlash(fileDir)
		for i := 1; i < level; i++ {
			base = filepath.ToSlash(filepath.Dir(base))
		}
		if base == "." {
			base = ""
		}
		if rest == "" {
			return base
		}
		return strings.TrimPrefix(filepath.ToSlash(filepath.Join(base, rest)), "/")
	}
	return ""
}

// extractPythonImports reads the import statements of a Python file and resolves
// each to a module path fragment. Pure relative to content + fileDir.
func extractPythonImports(content []byte, fileDir string) pythonImports {
	res := pythonImports{names: map[string]pyNameImport{}, modules: map[string]string{}}
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return res
	}
	defer tree.Close()

	root := tree.RootNode()
	for i := 0; i < int(root.NamedChildCount()); i++ {
		n := root.NamedChild(i)
		switch n.Type() {
		case "import_statement":
			collectPythonImportStmt(n, content, &res)
		case "import_from_statement":
			collectPythonFromStmt(n, content, fileDir, &res)
		}
	}
	return res
}

// collectPythonImportStmt handles `import a.b` and `import a.b as m` — both bind a
// qualifier (the dotted text, or the alias) to the target module fragment.
func collectPythonImportStmt(n *sitter.Node, content []byte, res *pythonImports) {
	for j := 0; j < int(n.NamedChildCount()); j++ {
		c := n.NamedChild(j)
		switch c.Type() {
		case "dotted_name":
			res.modules[c.Content(content)] = dottedToFrag(c.Content(content))
		case "aliased_import":
			name := c.ChildByFieldName("name")
			alias := c.ChildByFieldName("alias")
			if name != nil && alias != nil {
				res.modules[alias.Content(content)] = dottedToFrag(name.Content(content))
			}
		}
	}
}

// collectPythonFromStmt handles `from M import N [as L]` — each imported name binds
// to (module fragment, original name). The module_name node is skipped by byte
// range; wildcard imports bind nothing (the names are not enumerable).
func collectPythonFromStmt(n *sitter.Node, content []byte, fileDir string, res *pythonImports) {
	mod := n.ChildByFieldName("module_name")
	if mod == nil {
		return
	}
	frag := pyModuleFrag(mod, content, fileDir)
	if frag == "" {
		return
	}
	for j := 0; j < int(n.NamedChildCount()); j++ {
		c := n.NamedChild(j)
		if c.StartByte() == mod.StartByte() && c.EndByte() == mod.EndByte() {
			continue // the module_name node itself
		}
		switch c.Type() {
		case "dotted_name", "identifier":
			orig := lastDotSegment(c.Content(content))
			res.names[orig] = pyNameImport{frag: frag, orig: orig}
		case "aliased_import":
			name := c.ChildByFieldName("name")
			alias := c.ChildByFieldName("alias")
			if name != nil && alias != nil {
				res.names[alias.Content(content)] = pyNameImport{frag: frag, orig: lastDotSegment(name.Content(content))}
			}
		}
	}
}

// extractPythonCalls walks a Python file's call expressions. An unqualified call
// (`foo()`) has empty Qualifier; an attribute call (`m.foo()`, `a.b.foo()`) carries
// the object's full text as Qualifier. Pure relative to content.
func extractPythonCalls(content []byte) []CallSite {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil
	}
	defer tree.Close()

	q, err := sitter.NewQuery([]byte(`(call) @c`), python.GetLanguage())
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
			case "attribute":
				attr := fn.ChildByFieldName("attribute")
				obj := fn.ChildByFieldName("object")
				if attr == nil || obj == nil {
					continue
				}
				callee = attr.Content(content)
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

// pyFileMatchesModule reports whether a stored file path is the module named by a
// slash fragment — either `<frag>.py` or `<frag>/__init__.py` (suffix-matched,
// since the project's package root is unknown).
func pyFileMatchesModule(file, frag string) bool {
	p := filepath.ToSlash(file)
	return p == frag+".py" || strings.HasSuffix(p, "/"+frag+".py") ||
		p == frag+"/__init__.py" || strings.HasSuffix(p, "/"+frag+"/__init__.py")
}

// resolvePythonCallEdges resolves call sites to definition nodes with the
// exactly-1-or-drop guard. Resolution order for an unqualified call: a file-local
// def, then a `from M import N` binding, then a package-local def. A qualified
// call resolves only when the qualifier is an imported module. Pure — symbols +
// calls + imports in, edges out (lookup injected so the resolver stays storeless).
func resolvePythonCallEdges(relPath string, fileSyms, pkgSyms []CodeSymbol, calls []CallSite, imports pythonImports, lookup func(name string) []CodeSymbol) []CodeEdge {
	// A callable target is a function/method (Kind "func") or a class (calling it
	// constructs an instance — a real call edge).
	fileFuncs := map[string][]CodeSymbol{}
	for _, s := range fileSyms {
		if s.Kind == "func" || s.Kind == "class" {
			fileFuncs[s.Name] = append(fileFuncs[s.Name], s)
		}
	}
	pkgFuncs := map[string][]CodeSymbol{}
	for _, s := range pkgSyms {
		if s.Kind == "func" || s.Kind == "class" {
			pkgFuncs[s.Name] = append(pkgFuncs[s.Name], s)
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
			dst = resolvePyUnqualified(cs.Callee, fileFuncs, pkgFuncs, imports, lookup)
		} else {
			dst = resolvePyModuleCall(cs.Qualifier, cs.Callee, imports, lookup)
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

// resolveImportedName finds the single func symbol named `name` defined in the
// module fragment, or nil if zero/ambiguous. The cross-module primitive.
func resolveImportedName(name, frag string, lookup func(string) []CodeSymbol) *CodeSymbol {
	var cands []CodeSymbol
	for _, s := range lookup(name) {
		if (s.Kind == "func" || s.Kind == "class") && pyFileMatchesModule(s.FilePath, frag) {
			cands = append(cands, s)
		}
	}
	if len(cands) != 1 {
		return nil
	}
	return &cands[0]
}

func resolvePyUnqualified(callee string, fileFuncs, pkgFuncs map[string][]CodeSymbol, imports pythonImports, lookup func(string) []CodeSymbol) *CodeSymbol {
	local := fileFuncs[callee]
	_, imported := imports.names[callee]
	if len(local) == 1 && !imported {
		return &local[0]
	}
	if imported {
		b := imports.names[callee]
		return resolveImportedName(b.orig, b.frag, lookup)
	}
	if pkg := pkgFuncs[callee]; len(pkg) == 1 {
		return &pkg[0]
	}
	return nil
}

func resolvePyModuleCall(qualifier, callee string, imports pythonImports, lookup func(string) []CodeSymbol) *CodeSymbol {
	frag, ok := imports.modules[qualifier]
	if !ok {
		return nil // qualifier is a receiver / instance / unknown — drop
	}
	return resolveImportedName(callee, frag, lookup)
}
