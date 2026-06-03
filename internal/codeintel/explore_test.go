package codeintel

import (
	"context"
	"testing"

	"github.com/m0n0x41d/haft/internal/codebase"
)

// dedge is a heuristic interface-dispatch edge (a "bridge").
func dedge(s, d string) codebase.CodeEdge {
	return codebase.CodeEdge{SrcID: s, DstID: d, Kind: codebase.EdgeInterfaceDispatch, Provenance: codebase.ProvenanceHeuristic}
}

func chainIDs(hops []chainHop) []string {
	out := make([]string, len(hops))
	for i, h := range hops {
		out[i] = h.NodeID
	}
	return out
}

func TestLongestChain_DeepestStaticPath(t *testing.T) {
	out := map[string][]codebase.CodeEdge{
		"A": {edge("A", "B"), edge("A", "X")}, // B-branch is deeper than X
		"B": {edge("B", "C")},
		"C": {edge("C", "D")},
	}
	src := fakeEdges{out: out}
	hops, unresolved, bridges, err := longestChain(context.Background(), src, "A", 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := chainIDs(hops)
	if len(got) != 4 || got[0] != "A" || got[3] != "D" {
		t.Fatalf("deepest path A→B→C→D expected, got %v", got)
	}
	if unresolved || bridges != 0 {
		t.Fatalf("static path: unresolved=%v bridges=%d", unresolved, bridges)
	}
}

func TestLongestChain_MaxHopsCap(t *testing.T) {
	out := map[string][]codebase.CodeEdge{
		"A": {edge("A", "B")},
		"B": {edge("B", "C")},
		"C": {edge("C", "D")},
	}
	hops, _, _, _ := longestChain(context.Background(), fakeEdges{out: out}, "A", 2, 1)
	if got := chainIDs(hops); len(got) != 3 { // seed + 2 hops
		t.Fatalf("maxHops 2 should cap chain at 3 nodes, got %v", got)
	}
}

// One bridge may be crossed; a second is refused and the chain reports that it
// ends at an unresolved dispatch boundary rather than crossing it silently.
func TestLongestChain_BridgeBudgetAndUnresolvedEnd(t *testing.T) {
	out := map[string][]codebase.CodeEdge{
		"A": {dedge("A", "B")}, // bridge 1 — allowed
		"B": {dedge("B", "C")}, // bridge 2 — refused (budget 1)
	}
	hops, unresolved, bridges, err := longestChain(context.Background(), fakeEdges{out: out}, "A", 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := chainIDs(hops)
	if len(got) != 2 || got[1] != "B" {
		t.Fatalf("chain should cross exactly one bridge to B, got %v", got)
	}
	if bridges != 1 {
		t.Fatalf("expected 1 bridge used, got %d", bridges)
	}
	if !unresolved {
		t.Fatalf("chain must report it ends at an unresolved dispatch boundary")
	}
}

func TestLongestChain_PrefersStaticOverBridge(t *testing.T) {
	// From A: a static edge to a shallow node, and a bridge to a deep chain.
	// Static is preferred at equal opportunity, but longest still wins overall;
	// here the static branch is shorter, so the bridge chain (longer) is chosen
	// — verifying length dominates, with the bridge counted.
	out := map[string][]codebase.CodeEdge{
		"A": {edge("A", "S"), dedge("A", "B")},
		"B": {edge("B", "C")},
		"C": {edge("C", "D")},
	}
	hops, _, bridges, _ := longestChain(context.Background(), fakeEdges{out: out}, "A", 7, 1)
	if got := chainIDs(hops); len(got) != 4 || got[1] != "B" {
		t.Fatalf("longer bridge path A→B→C→D should win, got %v", got)
	}
	if bridges != 1 {
		t.Fatalf("bridge crossing must be counted, got %d", bridges)
	}
}
