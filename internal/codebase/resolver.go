package codebase

import (
	"context"
	"path/filepath"
)

// SymbolView is the read port over the symbol-node store that an EdgeResolver
// needs to resolve call targets to nodes (scoped per file / package / name).
// *SymbolStore satisfies it — resolvers depend on this abstraction, not the
// concrete store (hexagonal).
type SymbolView interface {
	GetByFile(ctx context.Context, filePath string) ([]CodeSymbol, error)
	GetByDir(ctx context.Context, dir string) ([]CodeSymbol, error)
	GetByName(ctx context.Context, name string) ([]CodeSymbol, error)
}

// EdgeResolver extracts the code→code edges originating in ONE file. It is the
// per-language PORT: the code graph is NOT Go-specific. Go is the first adapter;
// Python / TypeScript / Rust / … each add an EdgeResolver implementation plus a
// registry entry — exactly like the existing ModuleDetector / ImportParser
// adapters. Orchestration (scan, traversal, tools) codes against this interface,
// never against a language. An unresolved call/dispatch yields no edge.
type EdgeResolver interface {
	Language() string
	Extensions() []string
	ResolveFileEdges(ctx context.Context, projectRoot, relPath string, symbols SymbolView) ([]CodeEdge, error)
}

// ResolveFileEdges is the Go adapter's implementation of EdgeResolver — it
// composes the Go-specific extraction + resolution (call-site, intra-package,
// cross-file qualified, interface dispatch) behind the port. The Go-specific
// internals live in callsites.go / dispatch.go; nothing outside this method
// knows the language is Go.
func (g *GoLang) ResolveFileEdges(ctx context.Context, projectRoot, relPath string, symbols SymbolView) ([]CodeEdge, error) {
	fileSyms, err := symbols.GetByFile(ctx, relPath)
	if err != nil {
		return nil, err
	}
	pkgSyms, err := symbols.GetByDir(ctx, filepath.Dir(relPath))
	if err != nil {
		return nil, err
	}
	sites, err := ExtractCallSites(projectRoot, relPath)
	if err != nil {
		return nil, err
	}

	var edges []CodeEdge

	// 1. intra-package unqualified calls
	edges = append(edges, ResolveIntraPackageCallEdges(relPath, fileSyms, pkgSyms, sites)...)

	// 2. cross-file qualified calls (pkg.Func)
	imports, err := ExtractGoImports(projectRoot, relPath)
	if err != nil {
		return nil, err
	}
	lookup := func(name string) []CodeSymbol {
		s, _ := symbols.GetByName(ctx, name)
		return s
	}
	edges = append(edges, ResolveCrossFileCallEdges(relPath, fileSyms, sites, imports, lookup)...)

	// 3. interface dispatch (interfaces + impls scoped to the package)
	interfaces := map[string]InterfaceDef{}
	for _, pf := range distinctFiles(pkgSyms) {
		defs, _ := ExtractGoInterfaces(projectRoot, pf)
		for _, d := range defs {
			interfaces[d.Name] = d
		}
	}
	sigs, err := ExtractGoSignatures(projectRoot, relPath)
	if err != nil {
		return nil, err
	}
	edges = append(edges, ResolveInterfaceDispatchEdges(relPath, fileSyms, sites, sigs, interfaces, pkgSyms)...)

	return dedupeEdges(edges), nil
}

func distinctFiles(syms []CodeSymbol) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range syms {
		if !seen[s.FilePath] {
			seen[s.FilePath] = true
			out = append(out, s.FilePath)
		}
	}
	return out
}

func dedupeEdges(edges []CodeEdge) []CodeEdge {
	seen := make(map[string]bool, len(edges))
	out := make([]CodeEdge, 0, len(edges))
	for _, e := range edges {
		k := e.SrcID + "\x00" + e.DstID + "\x00" + string(e.Kind)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}
