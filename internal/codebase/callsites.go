package codebase

import (
	gocontext "context"
	"fmt"
	"os"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
)

// CallSite is one extracted call expression: the called name, an optional
// qualifier (the selector operand — a package alias or receiver), and the line.
// The enclosing caller symbol is resolved later by line containment, not here.
type CallSite struct {
	Callee    string
	Qualifier string // "" = unqualified (intra-package candidate); non-"" = qualified (P1b)
	Line      int    // 1-based
}

// ExtractCallSites walks a Go file's call expressions. Go only for P1a — other
// languages return nil (node extraction still works for them; edges don't yet).
// Pure relative to the file content; no DB.
func ExtractCallSites(projectRoot, relPath string) ([]CallSite, error) {
	ext := filepath.Ext(relPath)
	langInfo, ok := languages[ext]
	if !ok || langInfo.name != "go" {
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
	parser.SetLanguage(langInfo.lang)
	tree, err := parser.ParseCtx(gocontext.Background(), nil, content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relPath, err)
	}
	defer tree.Close()

	q, err := sitter.NewQuery([]byte(`(call_expression) @call`), langInfo.lang)
	if err != nil {
		return nil, err
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
			call := capture.Node
			fn := call.ChildByFieldName("function")
			if fn == nil {
				continue
			}
			var callee, qualifier string
			switch fn.Type() {
			case "identifier":
				callee = fn.Content(content)
			case "selector_expression":
				field := fn.ChildByFieldName("field")
				if field == nil {
					continue
				}
				callee = field.Content(content)
				if operand := fn.ChildByFieldName("operand"); operand != nil {
					qualifier = operand.Content(content)
				}
			default:
				continue
			}
			if callee == "" {
				continue
			}
			sites = append(sites, CallSite{
				Callee:    callee,
				Qualifier: qualifier,
				Line:      int(call.StartPoint().Row) + 1,
			})
		}
	}
	return sites, nil
}

// ResolveIntraPackageCallEdges resolves UNQUALIFIED call sites in a file to
// package-level definitions, against the package's symbol set. The over-
// resolution guard is exactly-1-or-drop: a callee with zero or multiple package
// candidates yields NO edge (an unresolved call is honest; a fanned-out wrong
// edge is corrosive). Qualified calls (receiver/package selectors) are left for
// P1b. Pure — symbols + call sites in, edges out.
func ResolveIntraPackageCallEdges(filePath string, fileSymbols, pkgSymbols []CodeSymbol, callSites []CallSite) []CodeEdge {
	byName := map[string][]CodeSymbol{}
	for _, s := range pkgSymbols {
		if s.Kind == "func" || s.Kind == "method" {
			byName[s.Name] = append(byName[s.Name], s)
		}
	}

	seen := map[string]bool{}
	var edges []CodeEdge
	for _, cs := range callSites {
		if cs.Qualifier != "" {
			continue // qualified → P1b (selector resolution)
		}
		caller := enclosingSymbol(fileSymbols, cs.Line)
		if caller == nil {
			continue
		}
		cands := byName[cs.Callee]
		if len(cands) != 1 {
			continue // 0 = undefined/builtin; >1 = ambiguous — DROP, never fan out
		}
		dst := cands[0]
		if dst.ID == caller.ID {
			continue // skip trivial self-recursion edges
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
			FilePath:   filePath,
			Line:       cs.Line,
			Provenance: ProvenanceStatic,
		})
	}
	return edges
}

// enclosingSymbol returns the innermost func/method whose body covers the line.
func enclosingSymbol(syms []CodeSymbol, line int) *CodeSymbol {
	var best *CodeSymbol
	for i := range syms {
		s := &syms[i]
		if s.Kind != "func" && s.Kind != "method" {
			continue
		}
		if line >= s.StartLine && line <= s.EndLine {
			if best == nil || (s.EndLine-s.StartLine) < (best.EndLine-best.StartLine) {
				best = s
			}
		}
	}
	return best
}
