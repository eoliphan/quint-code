package codeintel

import (
	"context"
	"sort"

	"github.com/m0n0x41d/haft/internal/codebase"
	"github.com/m0n0x41d/haft/internal/contextgraph"
)

// Explore budget. A single-call answer must stay focused: the spine is a deep
// but bounded path; at most one heuristic dispatch "bridge" may be crossed, so
// the chain never strings together multiple weak hops into a misleadingly-
// complete flow. ChainVisitBudget bounds the longest-path search (it degrades
// to a deep partial path on huge graphs rather than blowing up — the latency
// objective wins over provable-longest).
const (
	MaxChainHops     = 7
	MaxChainBridges  = 1
	ChainVisitBudget = 800
)

// ChainStep is one symbol on the explore spine, fused with the reasoning
// governing exactly it. Distance 0 is the seed; ViaKind/Provenance describe the
// edge that reached it (zero-valued for the seed).
type ChainStep struct {
	Symbol     codebase.CodeSymbol
	Distance   int
	ViaKind    codebase.EdgeKind
	Provenance codebase.Provenance
	Context    contextgraph.CodeContext
}

// Bridge reports whether this step crossed a heuristic interface-dispatch
// boundary — the one place the chain's honesty matters most.
func (c ChainStep) Bridge() bool {
	return c.ViaKind == codebase.EdgeInterfaceDispatch || c.Provenance == codebase.ProvenanceHeuristic
}

// ExploreResult is the single-call capstone: the connected spine (each on-chain
// symbol fused), the blast radius (who breaks if the seed changes, with covering
// decisions), and the seed's verbatim, freshness-revalidated source. Enough to
// answer "how does this flow work and what was decided about it" at 0–1 Read.
type ExploreResult struct {
	Seed          codebase.CodeSymbol
	SeedFound     bool
	Ambiguous     []codebase.CodeSymbol
	Fuzzy         bool // seed/candidates came from the fuzzy fallback (no exact-name match)
	Chain         []ChainStep
	UnresolvedEnd bool // the spine stops at a dispatch boundary it could not cross
	BridgesUsed   int
	BlastRadius   []FusedHop
	SeedBody      string
	SeedBodyOK    bool
	ColdBuilt     bool
}

// Explore assembles the capstone view for a seed: the deepest connected call
// chain (fused), the immediate blast radius, and verbatim seed source.
func (s *Service) Explore(ctx context.Context, projectRoot, name, file string, line int) (ExploreResult, error) {
	cold, err := s.EnsureIndex(ctx, projectRoot)
	if err != nil {
		return ExploreResult{}, err
	}
	seed, candidates, fuzzy, err := s.resolveSeed(ctx, name, file, line)
	if err != nil {
		return ExploreResult{}, err
	}
	res := ExploreResult{ColdBuilt: cold, Fuzzy: fuzzy}
	if len(candidates) > 0 {
		res.Ambiguous = candidates
		return res, nil
	}
	if seed.ID == "" {
		return res, nil
	}
	res.Seed = seed
	res.SeedFound = true

	hops, unresolvedEnd, bridges, err := longestChain(ctx, s.edges, seed.ID, MaxChainHops, MaxChainBridges)
	if err != nil {
		return ExploreResult{}, err
	}
	res.UnresolvedEnd = unresolvedEnd
	res.BridgesUsed = bridges
	for i, h := range hops {
		sym, ok, err := s.symbols.GetByID(ctx, h.NodeID)
		if err != nil {
			return ExploreResult{}, err
		}
		if !ok {
			continue
		}
		cc, err := contextgraph.FetchCodeContext(ctx, s.art, s.graph, contextgraph.Target{
			File:   sym.FilePath,
			Symbol: sym.Name,
			Line:   sym.StartLine,
		})
		if err != nil {
			return ExploreResult{}, err
		}
		res.Chain = append(res.Chain, ChainStep{Symbol: sym, Distance: i, ViaKind: h.ViaKind, Provenance: h.Provenance, Context: cc})
	}

	blast, err := s.trail(ctx, seed.ID, Callers)
	if err != nil {
		return ExploreResult{}, err
	}
	res.BlastRadius = blast

	body, ok, _, freshSym, err := s.freshBody(ctx, projectRoot, seed)
	if err != nil {
		return ExploreResult{}, err
	}
	res.Seed = freshSym
	res.SeedBody = string(body)
	res.SeedBodyOK = ok
	return res, nil
}

// BagLeg is the connection between two adjacent seeds in a multi-seed explore:
// the fused step sequence From … To, or Connected=false when no static path
// exists (honest — never bridged with a guess). Reversed means the path runs
// To→From (we still report it, naming the direction).
type BagLeg struct {
	From      codebase.CodeSymbol
	To        codebase.CodeSymbol
	Steps     []ChainStep
	Connected bool
	Reversed  bool
}

// ExploreBagResult is the multi-seed explore: how a bag of named symbols
// connects. Each adjacent pair is a leg with its connecting path (or an honest
// no-path). Seeds that did not resolve to a single symbol are listed so the
// caller disambiguates them rather than the bag silently dropping them.
type ExploreBagResult struct {
	Seeds      []codebase.CodeSymbol
	Unresolved []string
	Legs       []BagLeg
	ColdBuilt  bool
}

// ExploreBag connects a bag of >=2 seed names: resolve each (exact, else
// unambiguous fuzzy) and find the shortest connecting path between each adjacent
// pair over the call/dispatch edges, trying both directions. A pair with no
// static path is reported as not connected — never bridged with a guess.
func (s *Service) ExploreBag(ctx context.Context, projectRoot string, names []string) (ExploreBagResult, error) {
	cold, err := s.EnsureIndex(ctx, projectRoot)
	if err != nil {
		return ExploreBagResult{}, err
	}
	res := ExploreBagResult{ColdBuilt: cold}
	var seeds []codebase.CodeSymbol
	for _, n := range names {
		seed, candidates, _, err := s.resolveSeed(ctx, n, "", 0)
		if err != nil {
			return ExploreBagResult{}, err
		}
		if seed.ID == "" || len(candidates) > 0 {
			res.Unresolved = append(res.Unresolved, n) // not found or ambiguous — can't place in the bag
			continue
		}
		seeds = append(seeds, seed)
	}
	res.Seeds = seeds
	if len(seeds) < 2 {
		return res, nil
	}
	for i := 0; i+1 < len(seeds); i++ {
		from, to := seeds[i], seeds[i+1]
		leg := BagLeg{From: from, To: to}
		if path, ok := shortestPath(ctx, s.edges, from.ID, to.ID, MaxChainHops); ok {
			leg.Connected = true
			if leg.Steps, err = s.fuseChain(ctx, path); err != nil {
				return ExploreBagResult{}, err
			}
		} else if path, ok := shortestPath(ctx, s.edges, to.ID, from.ID, MaxChainHops); ok {
			leg.Connected = true
			leg.Reversed = true
			if leg.Steps, err = s.fuseChain(ctx, path); err != nil {
				return ExploreBagResult{}, err
			}
		}
		res.Legs = append(res.Legs, leg)
	}
	return res, nil
}

// fuseChain resolves a node-id path to symbols and fuses each. Distance is the
// position in the EMITTED chain (not the raw hop index), so it stays contiguous
// even if a stale/deleted node is skipped mid-path.
func (s *Service) fuseChain(ctx context.Context, hops []chainHop) ([]ChainStep, error) {
	steps := make([]ChainStep, 0, len(hops))
	for _, h := range hops {
		sym, ok, err := s.symbols.GetByID(ctx, h.NodeID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		cc, err := contextgraph.FetchCodeContext(ctx, s.art, s.graph, contextgraph.Target{File: sym.FilePath, Symbol: sym.Name, Line: sym.StartLine})
		if err != nil {
			return nil, err
		}
		steps = append(steps, ChainStep{Symbol: sym, Distance: len(steps), ViaKind: h.ViaKind, Provenance: h.Provenance, Context: cc})
	}
	return steps, nil
}

// shortestPath is a bounded BFS from fromID to toID over out-edges, returning
// the node sequence (from … to inclusive) or ok=false if unreachable within
// maxHops. Each non-seed hop carries the edge that reached it. Pure relative to
// the EdgeSource.
func shortestPath(ctx context.Context, src EdgeSource, fromID, toID string, maxHops int) ([]chainHop, bool) {
	if fromID == toID {
		return []chainHop{{NodeID: fromID}}, true
	}
	visited := map[string]bool{fromID: true}
	reachedBy := map[string]chainHop{}
	pred := map[string]string{}
	type frontier struct {
		id    string
		depth int
	}
	queue := []frontier{{fromID, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= maxHops {
			continue
		}
		edges, err := src.OutEdges(ctx, cur.id)
		if err != nil {
			return nil, false
		}
		for _, e := range edges {
			if visited[e.DstID] {
				continue
			}
			visited[e.DstID] = true
			reachedBy[e.DstID] = chainHop{NodeID: e.DstID, ViaKind: e.Kind, Provenance: e.Provenance}
			pred[e.DstID] = cur.id
			if e.DstID == toID {
				return reconstructPath(fromID, toID, reachedBy, pred), true
			}
			queue = append(queue, frontier{e.DstID, cur.depth + 1})
		}
	}
	return nil, false
}

// reconstructPath walks predecessors from toID back to fromID and reverses.
func reconstructPath(fromID, toID string, reachedBy map[string]chainHop, pred map[string]string) []chainHop {
	var rev []chainHop
	for n := toID; n != fromID; n = pred[n] {
		rev = append(rev, reachedBy[n])
	}
	rev = append(rev, chainHop{NodeID: fromID})
	out := make([]chainHop, len(rev))
	for i := range rev {
		out[len(rev)-1-i] = rev[i]
	}
	return out
}

// chainHop is a pure node-id step on the spine — the shell resolves it to a
// symbol and fuses it. The seed itself is the zero-valued first hop.
type chainHop struct {
	NodeID     string
	ViaKind    codebase.EdgeKind
	Provenance codebase.Provenance
}

// chainSuffix is a candidate continuation from a node during the longest-path
// search (the path AFTER the current node), plus how many bridges it crossed
// and whether it stopped at a bridge it could not cross.
type chainSuffix struct {
	path          []chainHop
	bridges       int
	endUnresolved bool
}

// longestChain finds the deepest simple path of out-edges from seedID, up to
// maxHops, crossing at most maxBridges heuristic dispatch edges. Pure relative
// to the EdgeSource. unresolvedEnd is true when the chosen path stops at a node
// whose only continuation would cross one bridge too many — surfaced so the
// chain prints "unresolved dispatch here", never a falsely-complete flow.
func longestChain(ctx context.Context, src EdgeSource, seedID string, maxHops, maxBridges int) (hops []chainHop, unresolvedEnd bool, bridges int, err error) {
	visited := map[string]bool{seedID: true}
	budget := ChainVisitBudget

	var dfs func(node string, depth, usedBridges int) chainSuffix
	dfs = func(node string, depth, usedBridges int) chainSuffix {
		if depth >= maxHops || budget <= 0 {
			return chainSuffix{}
		}
		edges, e := src.OutEdges(ctx, node)
		if e != nil {
			err = e
			return chainSuffix{}
		}
		sortChainEdges(edges)
		best := chainSuffix{}
		sawCappedBridge := false
		for _, ed := range edges {
			next := ed.DstID
			if visited[next] {
				continue
			}
			isBridge := ed.Kind == codebase.EdgeInterfaceDispatch || ed.Provenance == codebase.ProvenanceHeuristic
			if isBridge && usedBridges >= maxBridges {
				sawCappedBridge = true
				continue
			}
			budget--
			visited[next] = true
			sub := dfs(next, depth+1, usedBridges+boolInt(isBridge))
			visited[next] = false

			cand := chainSuffix{
				path:          append([]chainHop{{NodeID: next, ViaKind: ed.Kind, Provenance: ed.Provenance}}, sub.path...),
				bridges:       boolInt(isBridge) + sub.bridges,
				endUnresolved: sub.endUnresolved,
			}
			// Prefer the more INFORMATIVE spine: a flow that crosses an interface
			// boundary (the moat — reasoning across dynamic dispatch) is worth a
			// couple of static hops, but a much deeper static flow still wins.
			if chainScore(cand) > chainScore(best) {
				best = cand
			}
		}
		if len(best.path) == 0 && sawCappedBridge {
			best.endUnresolved = true
		}
		return best
	}

	tail := dfs(seedID, 0, 0)
	chain := append([]chainHop{{NodeID: seedID}}, tail.path...)
	return chain, tail.endUnresolved, tail.bridges, err
}

// sortChainEdges orders a node's out-edges deterministically: resolved static
// calls before heuristic dispatch (so the spine prefers solid ground), then by
// destination id for stable output.
func sortChainEdges(edges []codebase.CodeEdge) {
	sort.SliceStable(edges, func(i, j int) bool {
		bi := edges[i].Provenance == codebase.ProvenanceHeuristic
		bj := edges[j].Provenance == codebase.ProvenanceHeuristic
		if bi != bj {
			return !bi // static first
		}
		return edges[i].DstID < edges[j].DstID
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// chainScore values a candidate suffix: its length plus a bonus per interface
// boundary crossed, so a cross-interface flow beats a marginally-longer static
// one without letting a single bridge outweigh a genuinely deeper static flow.
func chainScore(s chainSuffix) int {
	return len(s.path) + 2*s.bridges
}
