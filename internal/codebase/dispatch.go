package codebase

import (
	gocontext "context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
)

// MethodSig is a method's structural signature for satisfaction matching: name
// plus parameter arity. Arity raises precision over name-only matching (the
// precision cliff) without full type comparison.
type MethodSig struct {
	Name  string
	Arity int
}

// InterfaceDef is a Go interface and its method set, extracted structurally.
// Empty (marker) interfaces are excluded by the extractor — they satisfy
// everything and would only generate noise.
type InterfaceDef struct {
	Name      string
	FilePath  string
	StartLine int
	Methods   []MethodSig
}

// ExtractGoInterfaces extracts each interface type's method set from a Go file.
// Pure relative to file content. Go only.
func ExtractGoInterfaces(projectRoot, relPath string) ([]InterfaceDef, error) {
	ext := filepath.Ext(relPath)
	li, ok := languages[ext]
	if !ok || li.name != "go" {
		return nil, nil
	}
	content, err := os.ReadFile(filepath.Join(projectRoot, relPath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relPath, err)
	}
	if len(content) > 500_000 {
		return nil, nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(li.lang)
	tree, err := parser.ParseCtx(gocontext.Background(), nil, content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relPath, err)
	}
	defer tree.Close()

	q, err := sitter.NewQuery([]byte(`(type_spec name: (type_identifier) @name type: (interface_type) @body)`), li.lang)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	qc.Exec(q, tree.RootNode())

	var defs []InterfaceDef
	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		var name string
		var body *sitter.Node
		for _, c := range m.Captures {
			switch q.CaptureNameForId(c.Index) {
			case "name":
				name = c.Node.Content(content)
			case "body":
				body = c.Node
			}
		}
		if name == "" || body == nil {
			continue
		}

		def := InterfaceDef{Name: name, FilePath: relPath, StartLine: int(body.StartPoint().Row) + 1}
		for i := 0; i < int(body.NamedChildCount()); i++ {
			child := body.NamedChild(i)
			if child == nil {
				continue
			}
			// Go grammar names interface methods "method_spec" (older) or
			// "method_elem" (newer) — accept either; require a name field.
			if child.Type() != "method_spec" && child.Type() != "method_elem" {
				continue
			}
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			arity := 0
			if params := child.ChildByFieldName("parameters"); params != nil {
				for j := 0; j < int(params.NamedChildCount()); j++ {
					if pc := params.NamedChild(j); pc != nil && pc.Type() == "parameter_declaration" {
						arity++
					}
				}
			}
			def.Methods = append(def.Methods, MethodSig{Name: nameNode.Content(content), Arity: arity})
		}
		if len(def.Methods) > 0 { // skip empty / marker interfaces
			defs = append(defs, def)
		}
	}
	return defs, nil
}

// ExtractGoSignatures maps each func/method's start line to its receiver+param
// variable names → declared type name (the simple identifier; *T and pkg.T
// reduce to T). Uses go/ast for accurate types. This is the receiver-type
// inference that bounds dispatch precision: a call x.M() links through an
// interface only when x's DECLARED type is a known interface.
func ExtractGoSignatures(projectRoot, relPath string) (map[int]map[string]string, error) {
	return extractGoSignatures(projectRoot, relPath, TypeFacts{}, false)
}

// ExtractGoSignaturesWithLocals additionally infers the types of LOCAL variables
// (`resolver := registry.ResolverForFile(path)`) via the package's TypeFacts, so
// interface dispatch resolves through `:=`-inferred receivers — not only the
// declared receiver/param ones. Same conservative discipline: an unresolvable
// local contributes no entry.
func ExtractGoSignaturesWithLocals(projectRoot, relPath string, facts TypeFacts) (map[int]map[string]string, error) {
	return extractGoSignatures(projectRoot, relPath, facts, true)
}

// extractGoSignatures maps each function's start line to its variable→type
// table (receiver + params, optionally extended with inferred locals).
func extractGoSignatures(projectRoot, relPath string, facts TypeFacts, withLocals bool) (map[int]map[string]string, error) {
	absPath := filepath.Join(projectRoot, relPath)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, nil, 0)
	if err != nil {
		return nil, nil // skip unparseable
	}
	out := map[int]map[string]string{}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		vars := map[string]string{}
		add := func(field *ast.Field) {
			tn := goTypeName(field.Type)
			if tn == "" {
				return
			}
			for _, n := range field.Names {
				vars[n.Name] = tn
			}
		}
		if fd.Recv != nil {
			for _, field := range fd.Recv.List {
				add(field)
			}
		}
		if fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				add(field)
			}
		}
		if withLocals {
			vars = InferLocalVarTypes(fd, vars, facts)
		}
		out[fset.Position(fd.Pos()).Line] = vars
	}
	return out, nil
}

// goTypeName reduces a type expression to its bare type identifier (*T → T,
// pkg.T → T); returns "" for shapes we don't resolve (maps, slices, funcs).
func goTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return goTypeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}

// TypeMethodSets groups concrete method names by receiver type, from symbols.
func TypeMethodSets(syms []CodeSymbol) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, s := range syms {
		if s.Kind == "method" && s.Receiver != "" {
			if out[s.Receiver] == nil {
				out[s.Receiver] = map[string]bool{}
			}
			out[s.Receiver][s.Name] = true
		}
	}
	return out
}

// Satisfies reports whether a concrete type's method-name set covers every
// method of the interface. Pure. Empty interfaces never satisfy (excluded).
func Satisfies(typeMethods map[string]bool, iface InterfaceDef) bool {
	if len(iface.Methods) == 0 {
		return false
	}
	for _, m := range iface.Methods {
		if !typeMethods[m.Name] {
			return false
		}
	}
	return true
}

// ResolveImplementsEdges synthesizes STATIC implements edges: for each concrete
// type DECLARED in relPath whose method-name set covers an interface's methods,
// emit type -> interface (answering "what implements this interface"). Same
// precision class as interface_dispatch — method-NAME coverage, so a name match
// with a different signature is a possible false positive; hence heuristic
// provenance, not static. Empty interfaces never match (Satisfies excludes them).
// Package-scoped via pkgSyms; reuses TypeMethodSets + Satisfies. Pure.
func ResolveImplementsEdges(relPath string, fileSyms, pkgSyms []CodeSymbol, interfaces map[string]InterfaceDef) []CodeEdge {
	methodSets := TypeMethodSets(pkgSyms)
	// Interface declarations are stored with kind "type" (the extractor does not
	// distinguish interface from struct). The `interfaces` map — from AST
	// extraction — is the source of truth for which names ARE interfaces; here we
	// only need each name's symbol id, so index every type/interface declaration.
	symByName := map[string]CodeSymbol{}
	for _, s := range pkgSyms {
		if s.Kind == "type" || s.Kind == "interface" {
			if _, dup := symByName[s.Name]; !dup {
				symByName[s.Name] = s
			}
		}
	}

	var edges []CodeEdge
	for _, ts := range fileSyms {
		if ts.Kind != "type" && ts.Kind != "struct" {
			continue
		}
		methods := methodSets[ts.Name]
		if len(methods) == 0 {
			continue // a type with no methods implements no non-empty interface
		}
		for name, iface := range interfaces {
			if name == ts.Name {
				continue
			}
			isym, ok := symByName[name]
			if !ok {
				continue
			}
			if Satisfies(methods, iface) {
				edges = append(edges, CodeEdge{
					SrcID:      ts.ID,
					DstID:      isym.ID,
					Kind:       EdgeImplements,
					FilePath:   relPath,
					Provenance: ProvenanceHeuristic,
				})
			}
		}
	}
	return edges
}

// ResolveInterfaceDispatchEdges synthesizes interface_dispatch edges (heuristic
// provenance) from a call site through an interface to its concrete impls. The
// precision bound: it fires ONLY when the call's receiver variable has a
// DECLARED type (from signatures) that is a KNOWN interface declaring the called
// method. Where that doesn't hold, NO edge — the boundary stays "unresolved
// dispatch". Multiple satisfying impls produce multiple edges (the true dispatch
// fan, not ambiguity). Pure given its injected lookups.
// implSymbols MUST be scoped to a single package. Receiver names are bare
// (package-stripped), so a whole-repo impl lookup would collide unrelated types
// that happen to share a receiver name (e.g. db.Store vs artifact.Store) and emit
// a WRONG dispatch edge — the precision-cliff failure this scoping prevents.
func ResolveInterfaceDispatchEdges(
	filePath string,
	fileSymbols []CodeSymbol,
	callSites []CallSite,
	signatures map[int]map[string]string,
	interfaces map[string]InterfaceDef,
	implSymbols []CodeSymbol,
) []CodeEdge {
	typeMethods := TypeMethodSets(implSymbols)
	methodIndex := map[string]CodeSymbol{} // receiver \x00 name -> node (unique within one package)
	for _, s := range implSymbols {
		if s.Kind == "method" && s.Receiver != "" {
			methodIndex[s.Receiver+"\x00"+s.Name] = s
		}
	}

	seen := map[string]bool{}
	var edges []CodeEdge
	for _, cs := range callSites {
		if cs.Qualifier == "" {
			continue // x.M() is qualified by the receiver var
		}
		caller := enclosingSymbol(fileSymbols, cs.Line)
		if caller == nil {
			continue
		}
		vars := signatures[caller.StartLine]
		if vars == nil {
			continue
		}
		declaredType, ok := vars[cs.Qualifier]
		if !ok {
			continue // receiver var's type unknown → drop (no guessing)
		}
		iface, ok := interfaces[declaredType]
		if !ok {
			continue // declared type isn't a known interface → not dispatch
		}
		declares := false
		for _, m := range iface.Methods {
			if m.Name == cs.Callee {
				declares = true
				break
			}
		}
		if !declares {
			continue
		}
		for recv, methods := range typeMethods {
			if !methods[cs.Callee] || !Satisfies(methods, iface) {
				continue
			}
			node, ok := methodIndex[recv+"\x00"+cs.Callee]
			if !ok || node.ID == caller.ID {
				continue
			}
			key := caller.ID + "->" + node.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, CodeEdge{
				SrcID:      caller.ID,
				DstID:      node.ID,
				Kind:       EdgeInterfaceDispatch,
				FilePath:   filePath,
				Line:       cs.Line,
				Provenance: ProvenanceHeuristic,
			})
		}
	}
	return edges
}
