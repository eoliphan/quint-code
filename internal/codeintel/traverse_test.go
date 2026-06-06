package codeintel

import (
	"context"
	"testing"

	"github.com/m0n0x41d/haft/internal/codebase"
)

type fakeEdges struct {
	out, in map[string][]codebase.CodeEdge
}

func (f fakeEdges) OutEdges(_ context.Context, id string) ([]codebase.CodeEdge, error) {
	return f.out[id], nil
}
func (f fakeEdges) InEdges(_ context.Context, id string) ([]codebase.CodeEdge, error) {
	return f.in[id], nil
}

func edge(s, d string) codebase.CodeEdge {
	return codebase.CodeEdge{SrcID: s, DstID: d, Kind: codebase.EdgeCall}
}

func TestTraverse_DepthVisitedDirection(t *testing.T) {
	// A→B→C (with a C→A cycle) and A→D.
	out := map[string][]codebase.CodeEdge{
		"A": {edge("A", "B"), edge("A", "D")},
		"B": {edge("B", "C")},
		"C": {edge("C", "A")},
	}
	in := map[string][]codebase.CodeEdge{
		"B": {edge("A", "B")},
		"D": {edge("A", "D")},
		"C": {edge("B", "C")},
		"A": {edge("C", "A")},
	}
	src := fakeEdges{out: out, in: in}
	ctx := context.Background()

	ids := func(hs []Hop) map[string]bool {
		m := map[string]bool{}
		for _, h := range hs {
			m[h.NodeID] = true
		}
		return m
	}

	// Callees depth 2 → {B, C, D}; the C→A cycle must not loop.
	d2, _ := Traverse(ctx, src, "A", Callees, 2, 20)
	if g := ids(d2); !g["B"] || !g["C"] || !g["D"] || len(d2) != 3 {
		t.Fatalf("callees depth2 = {B,C,D}, got %+v", d2)
	}
	// Depth 1 → only direct {B, D}.
	d1, _ := Traverse(ctx, src, "A", Callees, 1, 20)
	if g := ids(d1); g["C"] || !g["B"] || !g["D"] || len(d1) != 2 {
		t.Fatalf("callees depth1 = {B,D}, got %+v", d1)
	}
	// Callers of C → reaches B (depth1) and A (depth2).
	c, _ := Traverse(ctx, src, "C", Callers, 5, 20)
	if g := ids(c); !g["B"] || !g["A"] {
		t.Fatalf("callers of C should reach B and A, got %+v", c)
	}
	// Limit clamps the result count.
	lim, _ := Traverse(ctx, src, "A", Callees, 5, 1)
	if len(lim) != 1 {
		t.Fatalf("limit 1 should cap at 1, got %d", len(lim))
	}
}
