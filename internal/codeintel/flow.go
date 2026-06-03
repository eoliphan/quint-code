package codeintel

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/codebase"
	"github.com/m0n0x41d/haft/internal/contextgraph"
	"github.com/m0n0x41d/haft/internal/graph"
)

// FusedHop is a reached symbol with its traversal metadata AND the reasoning
// graph fused onto it — the core promise of haft's code graph: never a bare
// code node, always "what calls this AND what was decided about it".
type FusedHop struct {
	Symbol     codebase.CodeSymbol
	Distance   int
	ViaKind    codebase.EdgeKind
	Provenance codebase.Provenance
	Context    contextgraph.CodeContext
}

// Governed reports whether any reasoning touches this hop — symbol-level or, as
// the honest fallback, the enclosing module. A blank here means "nothing
// decided", never "lookup skipped".
func (h FusedHop) Governed() bool {
	return len(h.Context.Decisions) > 0 || len(h.Context.ModuleDecisions) > 0
}

// FlowResult is the outcome of a callers/callees/impact query. When the seed
// name resolves to multiple symbols and nothing disambiguates, Ambiguous lists
// the candidates and Hops is empty — an honest "which one?" instead of a
// wrong-seed answer (the keystone discipline: overloads are never conflated).
type FlowResult struct {
	Seed      codebase.CodeSymbol
	SeedFound bool
	Ambiguous []codebase.CodeSymbol
	Direction Direction
	Depth     int
	Hops      []FusedHop
	ColdBuilt bool // a one-time index build ran to answer this query
}

// Service is the imperative shell of the code-graph query surface: it owns the
// stores and composes the pure traverser with the fusion layer. Stores are
// stateless over the shared DB, so one per request is fine.
type Service struct {
	scanner *codebase.Scanner
	symbols *codebase.SymbolStore
	edges   *codebase.EdgeStore
	art     *artifact.Store
	graph   *graph.Store
}

// NewService wires a code-graph service over the artifact store's DB.
func NewService(store *artifact.Store) *Service {
	db := store.DB()
	return &Service{
		scanner: codebase.NewScanner(db),
		symbols: codebase.NewSymbolStore(db),
		edges:   codebase.NewEdgeStore(db),
		art:     store,
		graph:   graph.NewStore(db),
	}
}

// buildMu serializes index builds across the process. Service is created per
// request (and one runs at server startup), so the lock cannot live on the
// struct — it must be package-scoped. With double-checked staleness inside the
// lock, a query arriving during the startup rebuild waits, then sees a fresh
// index instead of triggering a second build.
var buildMu sync.Mutex

// EnsureIndex (re)builds the symbol + edge layers when the index is empty OR the
// source tree has changed since the last build (fingerprint mismatch). Returns
// whether a build ran. The staleness check is a stat-only fingerprint — cheap on
// the warm path (no rebuild, just a tree walk) — so the index stays fresh
// without a manual rebuild after code changes. (A full rebuild on staleness;
// closure-scoped partial rebuild is a later latency optimization — edges are
// non-local, so per-file rebuild would be unsound.)
func (s *Service) EnsureIndex(ctx context.Context, projectRoot string) (bool, error) {
	buildMu.Lock()
	defer buildMu.Unlock()

	if err := s.symbols.EnsureSchema(ctx); err != nil {
		return false, err
	}
	if err := s.edges.EnsureSchema(ctx); err != nil {
		return false, err
	}
	if err := s.scanner.EnsureIndexMetaSchema(ctx); err != nil {
		return false, err
	}

	stale, fp, err := s.indexStale(ctx, projectRoot)
	if err != nil {
		return false, err
	}
	if !stale {
		return false, nil
	}
	if _, err := s.scanner.ScanSymbols(ctx, projectRoot); err != nil {
		return false, fmt.Errorf("index (symbols): %w", err)
	}
	if _, err := s.scanner.ScanEdges(ctx, projectRoot); err != nil {
		return false, fmt.Errorf("index (edges): %w", err)
	}
	if err := s.scanner.SetFingerprint(ctx, fp); err != nil {
		return false, fmt.Errorf("record index fingerprint: %w", err)
	}
	return true, nil
}

// indexStale reports whether the index must be rebuilt: never built (no
// fingerprint recorded — set only after a successful build, so it correctly
// distinguishes "never built" from "built but zero edges resolved"), or a
// current source fingerprint that differs from the stored one. Returns the
// freshly-computed fingerprint so the caller can store it after a rebuild.
func (s *Service) indexStale(ctx context.Context, projectRoot string) (bool, string, error) {
	fp, err := s.scanner.SourceFingerprint(projectRoot)
	if err != nil {
		return false, "", err
	}
	stored, err := s.scanner.StoredFingerprint(ctx)
	if err != nil {
		return false, fp, err
	}
	if stored == "" {
		return true, fp, nil // never built
	}
	return stored != fp, fp, nil
}

// Flow runs a callers/callees traversal from the named seed, fusing the
// reasoning graph onto every reached symbol. Impact is the Callers direction
// (who breaks if this changes); Callees is the forward dependency set.
func (s *Service) Flow(ctx context.Context, projectRoot, name, file string, line, depth int, dir Direction) (FlowResult, error) {
	cold, err := s.EnsureIndex(ctx, projectRoot)
	if err != nil {
		return FlowResult{}, err
	}
	seed, ambiguous, err := s.resolveSeed(ctx, name, file, line)
	if err != nil {
		return FlowResult{}, err
	}
	res := FlowResult{Direction: dir, Depth: depth, ColdBuilt: cold}
	if len(ambiguous) > 0 {
		res.Ambiguous = ambiguous
		return res, nil
	}
	if seed.ID == "" {
		return res, nil // not found — SeedFound stays false
	}
	res.Seed = seed
	res.SeedFound = true

	hops, err := Traverse(ctx, s.edges, seed.ID, dir, depth, MaxResults)
	if err != nil {
		return FlowResult{}, err
	}
	for _, h := range hops {
		fused, ok, err := s.fuse(ctx, h)
		if err != nil {
			return FlowResult{}, err
		}
		if !ok {
			continue // edge to a node no longer in the symbol table — drop, don't fabricate
		}
		res.Hops = append(res.Hops, fused)
	}
	sort.SliceStable(res.Hops, func(i, j int) bool { return res.Hops[i].Distance < res.Hops[j].Distance })
	return res, nil
}

// fuse resolves a hop's node id back to its symbol and attaches the reasoning
// graph. ok=false when the symbol is gone (the edge is stale) — the caller
// drops it rather than emitting a half-resolved row.
func (s *Service) fuse(ctx context.Context, h Hop) (FusedHop, bool, error) {
	sym, ok, err := s.symbols.GetByID(ctx, h.NodeID)
	if err != nil || !ok {
		return FusedHop{}, false, err
	}
	cc, err := contextgraph.FetchCodeContext(ctx, s.art, s.graph, contextgraph.Target{
		File:   sym.FilePath,
		Symbol: sym.Name,
		Line:   sym.StartLine,
	})
	if err != nil {
		return FusedHop{}, false, err
	}
	return FusedHop{
		Symbol:     sym,
		Distance:   h.Distance,
		ViaKind:    h.ViaKind,
		Provenance: h.Provenance,
		Context:    cc,
	}, true, nil
}

// resolveSeed maps a (name, file, line) request to a single seed symbol, or to
// a candidate list when the name is ambiguous and nothing disambiguates it.
func (s *Service) resolveSeed(ctx context.Context, name, file string, line int) (codebase.CodeSymbol, []codebase.CodeSymbol, error) {
	// Most precise: a file + line covers exactly one symbol body.
	if file != "" && line > 0 {
		syms, err := s.symbols.GetByFile(ctx, file)
		if err != nil {
			return codebase.CodeSymbol{}, nil, err
		}
		if sym, ok := symbolCoveringLine(syms, line); ok {
			return sym, nil, nil
		}
	}
	candidates, err := s.symbols.GetByName(ctx, name)
	if err != nil {
		return codebase.CodeSymbol{}, nil, err
	}
	if file != "" {
		candidates = filterByFile(candidates, file)
	}
	switch len(candidates) {
	case 0:
		return codebase.CodeSymbol{}, nil, nil
	case 1:
		return candidates[0], nil, nil
	default:
		return codebase.CodeSymbol{}, candidates, nil
	}
}

// symbolCoveringLine returns the innermost symbol whose [StartLine, EndLine]
// covers line (max StartLine wins for nested bodies). Pure.
func symbolCoveringLine(syms []codebase.CodeSymbol, line int) (codebase.CodeSymbol, bool) {
	best := codebase.CodeSymbol{}
	found := false
	for _, sym := range syms {
		if sym.StartLine <= line && line <= sym.EndLine {
			if !found || sym.StartLine > best.StartLine {
				best = sym
				found = true
			}
		}
	}
	return best, found
}

func filterByFile(syms []codebase.CodeSymbol, file string) []codebase.CodeSymbol {
	out := make([]codebase.CodeSymbol, 0, len(syms))
	for _, sym := range syms {
		if sym.FilePath == file {
			out = append(out, sym)
		}
	}
	return out
}
