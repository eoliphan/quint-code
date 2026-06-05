package codebase

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	typescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// heritageRel is one class/interface and one base it inherits, with whether the
// relation is `extends` (class extends class, interface extends interface) or
// `implements` (class implements interface).
type heritageRel struct {
	subType  string
	baseName string
	extends  bool
}

// tsGrammarForExt picks the tree-sitter grammar: TypeScript for .ts/.tsx/.mts,
// JavaScript for .js/.jsx/.mjs. nil for anything else.
func tsGrammarForExt(ext string) *sitter.Language {
	switch strings.ToLower(ext) {
	case ".ts", ".tsx", ".mts":
		return typescript.GetLanguage()
	case ".js", ".jsx", ".mjs":
		return javascript.GetLanguage()
	}
	return nil
}

// extractTSHeritage parses a JS/TS file and returns each class/interface heritage
// relation (extends / implements), reading the explicit clauses from the AST.
// Pure relative to file content. JS has only class `extends`; TS adds class
// `implements` and interface `extends`.
func extractTSHeritage(root *sitter.Node, content []byte) []heritageRel {
	var out []heritageRel
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "class_declaration":
			if name := declName(n, content); name != "" {
				for _, heritage := range childrenOfType(n, "class_heritage") {
					for _, base := range identChildren(firstChildOfType(heritage, "extends_clause"), content) {
						out = append(out, heritageRel{name, base, true})
					}
					for _, base := range identChildren(firstChildOfType(heritage, "implements_clause"), content) {
						out = append(out, heritageRel{name, base, false})
					}
				}
			}
		case "interface_declaration":
			if name := declName(n, content); name != "" {
				for _, base := range identChildren(firstChildOfType(n, "extends_type_clause"), content) {
					out = append(out, heritageRel{name, base, true})
				}
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(root)
	return out
}

// declName returns the first direct identifier / type_identifier child — the
// class or interface name (heritage clauses come after, so they are not hit).
func declName(n *sitter.Node, content []byte) string {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() == "type_identifier" || c.Type() == "identifier" {
			return c.Content(content)
		}
	}
	return ""
}

func firstChildOfType(n *sitter.Node, typ string) *sitter.Node {
	if n == nil {
		return nil
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		if c := n.NamedChild(i); c.Type() == typ {
			return c
		}
	}
	return nil
}

func childrenOfType(n *sitter.Node, typ string) []*sitter.Node {
	var out []*sitter.Node
	for i := 0; i < int(n.NamedChildCount()); i++ {
		if c := n.NamedChild(i); c.Type() == typ {
			out = append(out, c)
		}
	}
	return out
}

// identChildren returns the base names directly under a heritage clause:
// identifier / type_identifier (Base), and the head of a generic_type (Base<T>).
func identChildren(clause *sitter.Node, content []byte) []string {
	if clause == nil {
		return nil
	}
	var out []string
	for i := 0; i < int(clause.NamedChildCount()); i++ {
		c := clause.NamedChild(i)
		switch c.Type() {
		case "identifier", "type_identifier":
			out = append(out, c.Content(content))
		case "generic_type":
			if name := declName(c, content); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// ResolveFileEdges makes JSTSLang an EdgeResolver. It emits two edge families:
//   - `extends` / `implements` edges from the explicit JS/TS heritage clauses,
//     resolved directory-locally (heuristic provenance — no import analysis);
//   - `call` edges, resolved through relative-import analysis (file-local defs,
//     named imports `{Foo}`, and namespaced `ns.foo()` calls) with the same
//     exactly-1-or-drop discipline (static provenance).
//
// A base or call that does not resolve to exactly one symbol is dropped, never
// guessed. Default imports and instance-method calls (`obj.method()`) are left
// unresolved — their target cannot be named soundly from the AST alone.
func (j *JSTSLang) ResolveFileEdges(ctx context.Context, projectRoot, relPath string, symbols SymbolView) ([]CodeEdge, error) {
	lang := tsGrammarForExt(filepath.Ext(relPath))
	if lang == nil {
		return nil, nil
	}
	content, err := os.ReadFile(filepath.Join(projectRoot, relPath))
	if err != nil {
		return nil, nil
	}

	// Parse the file ONCE; every extractor reads the shared tree.
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
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

	lookup := func(name string) []CodeSymbol {
		s, _ := symbols.GetByName(ctx, name)
		return s
	}
	imports := extractTSImports(root, content, filepath.Dir(relPath), loadTSProjectResolution(projectRoot))

	var edges []CodeEdge
	edges = append(edges, tsHeritageEdges(root, content, relPath, fileSyms, imports, lookup)...)

	calls, callbacks := extractTSCallsAndCallbacks(root, content, lang)
	callEdges := resolveTSCallEdges(relPath, fileSyms, pkgSyms, calls, callbacks, imports, lookup)
	edges = append(edges, callEdges...)

	// Intra-file EventEmitter dispatch: pair .on("e", h) with .emit("e").
	regs, dispatches := extractTSEmitterSites(root, content, lang)
	edges = append(edges, synthesizeEmitterEdges(relPath, fileSyms, regs, dispatches, edgePairs(callEdges))...)

	return dedupeEdges(edges), nil
}

// tsHeritageEdges resolves `extends` / `implements` edges from explicit heritage
// clauses with IMPORT AWARENESS: a base resolves to a same-file class/interface
// or to an explicitly imported one, never to an unimported same-named type in a
// sibling module. A base that resolves to neither (external / unresolved) or to
// both (a shadowed redefinition) is dropped — never guessed. Pure.
func tsHeritageEdges(root *sitter.Node, content []byte, relPath string, fileSyms []CodeSymbol, imports tsImports, lookup func(string) []CodeSymbol) []CodeEdge {
	rels := extractTSHeritage(root, content)
	if len(rels) == 0 {
		return nil
	}

	fileByName := map[string]CodeSymbol{}
	for _, s := range fileSyms {
		if s.Kind == "class" || s.Kind == "interface" {
			fileByName[s.Name] = s
		}
	}

	var edges []CodeEdge
	for _, r := range rels {
		sub, ok := fileByName[r.subType]
		if !ok || r.baseName == r.subType {
			continue
		}
		dst := resolveTSBaseType(r.baseName, fileByName, imports, lookup)
		if dst == nil {
			continue
		}
		kind := EdgeImplements
		if r.extends {
			kind = EdgeExtends
		}
		edges = append(edges, CodeEdge{
			SrcID:      sub.ID,
			DstID:      dst.ID,
			Kind:       kind,
			FilePath:   relPath,
			Provenance: ProvenanceHeuristic,
		})
	}
	return edges
}

// resolveTSBaseType resolves a base type name to exactly one class/interface with
// import awareness: an explicit named import is authoritative for the name in this
// file (resolved cross-module), else a same-file type. A name that is BOTH
// imported and locally defined, or NEITHER, is dropped. Without this an unimported
// same-named type in a sibling module would wrongly shadow the imported base.
func resolveTSBaseType(b string, fileByName map[string]CodeSymbol, imports tsImports, lookup func(string) []CodeSymbol) *CodeSymbol {
	bind, imported := imports.names[b]
	local, isLocal := fileByName[b]
	if imported && isLocal {
		return nil // shadowed redefinition — ambiguous, drop
	}
	if imported {
		var cands []CodeSymbol
		for _, s := range lookup(bind.orig) {
			if (s.Kind == "class" || s.Kind == "interface") && tsFileMatchesBase(s.FilePath, bind.base) {
				cands = append(cands, s)
			}
		}
		if len(cands) != 1 {
			return nil
		}
		return &cands[0]
	}
	if isLocal {
		return &local
	}
	return nil
}
