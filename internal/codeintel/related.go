package codeintel

import (
	"context"
	"fmt"
	"sort"

	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/codebase"
	"github.com/m0n0x41d/haft/internal/graphrank"
	"github.com/m0n0x41d/haft/internal/textsearch"
)

// Phase 2 of dec-20260604-3aaad199: FTS5-seeded Personalized PageRank over the
// FUSED graph (code symbols + call/dispatch edges + reasoning artifacts + their
// links + the artifact<->file<->symbol bridges). Deterministic, no embeddings,
// no second runtime. Surfaces graph-proximity recall — "what's related to this"
// ranked by distance in the fused graph — folded into the existing related action.

// RelatedKind tags a ranked node so the surface can split symbols from artifacts.
type RelatedKind int

const (
	RelatedSymbol RelatedKind = iota
	RelatedArtifact
	relatedFile // connector node; filtered out of results
)

// RelatedNode is one graph-proximity-ranked node (a symbol or an artifact).
type RelatedNode struct {
	ID    string
	Kind  RelatedKind
	Score float64
}

const fileNodePrefix = "file::"

func fileNode(path string) string { return fileNodePrefix + path }

// Fused-graph edge weights. Uniform in v1 — per dec-20260604-3aaad199, weight
// tuning is an OBSERVATION (watched, not optimized), so they are named consts to
// tune later rather than magic literals.
const (
	wCode   = 1.0 // symbol -> symbol call/dispatch edge
	wLink   = 1.0 // artifact -> artifact reasoning link
	wBridge = 1.0 // artifact <-> file, symbol <-> file connector
)

// buildFusedGraph (pure) fuses the code graph, the reasoning graph, and the
// artifact<->file<->symbol bridges into one bidirectional graphrank.Graph, plus
// a node->kind map so ranked results can be split into symbols vs artifacts.
// Edges go BOTH directions: "related" is symmetric — a caller is as related as a
// callee, a governing decision as related as the symbol it governs.
func buildFusedGraph(
	edges []codebase.CodeEdge,
	links []artifact.LinkEdge,
	affected []artifact.AffectedFileRef,
	syms []codebase.SymbolRef,
) (*graphrank.Graph, map[string]RelatedKind) {
	g := graphrank.NewGraph()
	kind := make(map[string]RelatedKind)

	for _, e := range edges {
		g.AddEdge(e.SrcID, e.DstID, wCode)
		g.AddEdge(e.DstID, e.SrcID, wCode)
		kind[e.SrcID] = RelatedSymbol
		kind[e.DstID] = RelatedSymbol
	}
	for _, l := range links {
		g.AddEdge(l.Source, l.Target, wLink)
		g.AddEdge(l.Target, l.Source, wLink)
		kind[l.Source] = RelatedArtifact
		kind[l.Target] = RelatedArtifact
	}
	for _, af := range affected {
		fn := fileNode(af.FilePath)
		g.AddEdge(af.ArtifactID, fn, wBridge)
		g.AddEdge(fn, af.ArtifactID, wBridge)
		kind[af.ArtifactID] = RelatedArtifact
		kind[fn] = relatedFile
	}
	for _, sr := range syms {
		fn := fileNode(sr.FilePath)
		g.AddEdge(sr.ID, fn, wBridge)
		g.AddEdge(fn, sr.ID, wBridge)
		kind[sr.ID] = RelatedSymbol
		kind[fn] = relatedFile
	}
	return g, kind
}

// rankRelated runs PPR from the seeds and returns the top non-seed, non-file
// nodes by score, deterministically (score desc, then id asc). nil seeds or a
// seed that lands on no graph node yields an empty slice.
func rankRelated(g *graphrank.Graph, kind map[string]RelatedKind, seeds map[string]float64, limit int) []RelatedNode {
	scores := g.Rank(seeds, graphrank.DefaultParams())
	out := make([]RelatedNode, 0, len(scores))
	for id, sc := range scores {
		if sc <= 0 {
			continue
		}
		if _, isSeed := seeds[id]; isSeed {
			continue
		}
		k, ok := kind[id]
		if !ok || k == relatedFile {
			continue // drop file connector nodes — they are plumbing, not results
		}
		out = append(out, RelatedNode{ID: id, Kind: k, Score: sc})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// nodeFile / nodeName decompose a symbol node id (file#name#line per
// codebase.NodeID). They return "" for non-symbol ids (artifacts, file nodes).
func nodeFile(id string) string {
	if i := indexByteUntilHash(id); i > 0 {
		return id[:i]
	}
	return ""
}

func nodeName(id string) string {
	first := indexByteUntilHash(id)
	last := lastIndexByteUntilHash(id)
	if first < 0 || last <= first {
		return ""
	}
	return id[first+1 : last]
}

func indexByteUntilHash(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '#' {
			return i
		}
	}
	return -1
}

func lastIndexByteUntilHash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '#' {
			return i
		}
	}
	return -1
}

// isCallableKind reports whether a symbol kind can carry call edges (and so can
// be "exercised by" a test). Types/consts/fields have no callers.
func isCallableKind(kind string) bool {
	switch kind {
	case "func", "function", "method":
		return true
	default:
		return false
	}
}

// SymbolCoverage is one callable symbol of a file plus the test functions that
// exercise it via call edges (dec-20260604-ef966a11). It is STRUCTURAL coverage
// — "exercised / tested by", NOT behavioral "verified": a test may call the
// symbol only as setup. Empty TestedBy means no test exercises it.
type SymbolCoverage struct {
	Symbol   string
	Exported bool
	TestedBy []string
}

// coverageFor (pure) builds the per-callable-symbol coverage map: for each
// callable symbol, the distinct test functions whose call edges reach it.
// Deterministic and order-stable. `callers` maps a symbol id to its inbound edges.
func coverageFor(syms []codebase.CodeSymbol, callers map[string][]codebase.CodeEdge) []SymbolCoverage {
	out := make([]SymbolCoverage, 0, len(syms))
	for _, sym := range syms {
		if !isCallableKind(sym.Kind) {
			continue
		}
		seen := map[string]bool{}
		var tests []string
		for _, e := range callers[sym.ID] {
			f := nodeFile(e.SrcID)
			if f == "" || !textsearch.IsTestPath(f) {
				continue
			}
			name := nodeName(e.SrcID)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			tests = append(tests, name)
		}
		sort.Strings(tests)
		out = append(out, SymbolCoverage{Symbol: sym.Name, Exported: sym.Exported, TestedBy: tests})
	}
	return out
}

// RelatedResult is a ranked node resolved to a display title — what the surface
// renders. Title is the artifact title, or "Name  (file:line)" for a symbol.
type RelatedResult struct {
	RelatedNode
	Title string
}

// RelatedToFile ranks the fused graph by proximity to a file: it seeds the file
// connector node (whose neighbors are the artifacts affecting that file and the
// symbols defined in it) and returns the nearest artifacts + symbols, each
// resolved to a display title. Thin shell — enumerate the stores, delegate to
// the pure builder + ranker, then resolve titles. A node whose title no longer
// resolves (stale id) is dropped rather than shown bare.
func (s *Service) RelatedToFile(ctx context.Context, projectRoot, filePath string, limit int) ([]RelatedResult, error) {
	if _, err := s.EnsureIndex(ctx, projectRoot); err != nil {
		return nil, err
	}
	edges, err := s.edges.AllEdges(ctx)
	if err != nil {
		return nil, err
	}
	links, err := s.art.AllLinks(ctx)
	if err != nil {
		return nil, err
	}
	affected, err := s.art.AllAffectedFiles(ctx)
	if err != nil {
		return nil, err
	}
	syms, err := s.symbols.AllSymbolRefs(ctx)
	if err != nil {
		return nil, err
	}
	g, kind := buildFusedGraph(edges, links, affected, syms)
	// Rank ALL nodes (limit 0); resolveRelated then drops co-file AND test-file
	// symbols (proximity is production-only — tests live in the separate Tested-by
	// lane, dec-20260604-ef966a11), THEN trim — filtering must precede the trim so
	// production neighbors are not crowded out before being chosen.
	ranked := rankRelated(g, kind, map[string]float64{fileNode(filePath): 1}, 0)
	if limit <= 0 {
		limit = 8
	}

	out := make([]RelatedResult, 0, limit)
	for _, n := range ranked {
		title, keep := s.resolveRelated(ctx, n, filePath)
		if !keep {
			continue
		}
		out = append(out, RelatedResult{RelatedNode: n, Title: title})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// resolveRelated renders a ranked node's display title and reports whether to
// keep it. It drops nodes whose id no longer resolves AND symbols defined in the
// seed file itself — co-file symbols are trivially "related" (the caller already
// has the file), so excluding them lets the section surface the non-obvious
// cross-file symbols and graph-reachable artifacts instead.
func (s *Service) resolveRelated(ctx context.Context, n RelatedNode, seedFile string) (string, bool) {
	switch n.Kind {
	case RelatedArtifact:
		a, err := s.art.Get(ctx, n.ID)
		if err != nil || a == nil {
			return "", false
		}
		return a.Meta.Title, true
	case RelatedSymbol:
		sym, ok, err := s.symbols.GetByID(ctx, n.ID)
		if err != nil || !ok {
			return "", false
		}
		if sym.FilePath == seedFile {
			return "", false // co-file: trivially related — drop
		}
		if textsearch.IsTestPath(sym.FilePath) {
			return "", false // tests belong in the Tested-by lane, not proximity
		}
		return fmt.Sprintf("%s  (%s:%d)", sym.Name, sym.FilePath, sym.StartLine), true
	default:
		return "", false
	}
}

// TestedBy returns the structural test-coverage map for a file: each callable
// symbol defined in it and the test functions whose call edges reach it
// (dec-20260604-ef966a11). Thin shell — gather each symbol's inbound edges, then
// delegate to the pure coverageFor. "Exercised by", not "verified".
func (s *Service) TestedBy(ctx context.Context, projectRoot, filePath string) ([]SymbolCoverage, error) {
	if _, err := s.EnsureIndex(ctx, projectRoot); err != nil {
		return nil, err
	}
	syms, err := s.symbols.GetByFile(ctx, filePath)
	if err != nil {
		return nil, err
	}
	callers := make(map[string][]codebase.CodeEdge, len(syms))
	for _, sym := range syms {
		if !isCallableKind(sym.Kind) {
			continue
		}
		in, err := s.edges.InEdges(ctx, sym.ID)
		if err != nil {
			return nil, err
		}
		callers[sym.ID] = in
	}
	return coverageFor(syms, callers), nil
}
