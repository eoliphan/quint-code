package present

import (
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/codebase"
	"github.com/m0n0x41d/haft/internal/codeintel"
	"github.com/m0n0x41d/haft/internal/contextgraph"
	"github.com/m0n0x41d/haft/internal/graph"
)

func decArtifact(id, title string) *artifact.Artifact {
	a := &artifact.Artifact{}
	a.Meta.ID = id
	a.Meta.Title = title
	a.Meta.Kind = artifact.KindDecisionRecord
	a.Meta.Status = artifact.StatusActive
	return a
}

// The P2 gate, asserted on the presentation contract: a hop governed only at
// the module level must render "module governed by dec-Y", never blank — a
// blank row reads as "safe to change", the exact failure the fusion exists to
// prevent.
func TestFlowResponse_GovernanceNeverBlank(t *testing.T) {
	res := codeintel.FlowResult{
		Seed:      codebase.CodeSymbol{Name: "Edit", FilePath: "internal/x/edit.go", StartLine: 10},
		SeedFound: true,
		Direction: codeintel.Callers,
		Depth:     2,
		Hops: []codeintel.FusedHop{
			{ // symbol-level governance
				Symbol:   codebase.CodeSymbol{Name: "Apply", FilePath: "internal/x/apply.go", StartLine: 40},
				Distance: 1,
				ViaKind:  codebase.EdgeCall,
				Context:  contextgraph.CodeContext{Decisions: []*artifact.Artifact{decArtifact("dec-AAA", "apply contract")}},
			},
			{ // module-level fallback only
				Symbol:     codebase.CodeSymbol{Name: "Dispatch", FilePath: "internal/y/run.go", StartLine: 70},
				Distance:   2,
				ViaKind:    codebase.EdgeInterfaceDispatch,
				Provenance: codebase.ProvenanceHeuristic,
				Context:    contextgraph.CodeContext{Module: "internal/y", Governed: true, ModuleDecisions: []graph.Node{{ID: "dec-BBB", Name: "y boundary"}}},
			},
			{ // genuinely ungoverned
				Symbol:   codebase.CodeSymbol{Name: "helper", FilePath: "internal/z/util.go", StartLine: 5},
				Distance: 2,
				ViaKind:  codebase.EdgeCall,
				Context:  contextgraph.CodeContext{},
			},
		},
	}

	out := FlowResponse(res, "impact", "Edit")

	if !strings.Contains(out, "dec-AAA") || !strings.Contains(out, "governs:") {
		t.Errorf("symbol-level decision not surfaced:\n%s", out)
	}
	if !strings.Contains(out, "module governed by") || !strings.Contains(out, "dec-BBB") {
		t.Errorf("module-governed hop rendered blank — the exact P2 failure mode:\n%s", out)
	}
	if !strings.Contains(out, "no recorded reasoning") {
		t.Errorf("ungoverned hop should be explicit, not silent:\n%s", out)
	}
	if !strings.Contains(out, "heuristic") {
		t.Errorf("heuristic dispatch edge must be flagged:\n%s", out)
	}
	if !strings.Contains(out, "2 carry recorded reasoning") {
		t.Errorf("governed count wrong (want 2 of 3):\n%s", out)
	}
}

// An ambiguous seed must list candidates for disambiguation, never silently
// traverse one (the keystone discipline at the query surface).
func TestFlowResponse_AmbiguousSeedSurfacesCandidates(t *testing.T) {
	res := codeintel.FlowResult{
		Direction: codeintel.Callers,
		Ambiguous: []codebase.CodeSymbol{
			{Name: "Search", FilePath: "internal/artifact/store.go", StartLine: 100, Receiver: "Store"},
			{Name: "Search", FilePath: "internal/db/store.go", StartLine: 55, Receiver: "Store"},
		},
	}
	out := FlowResponse(res, "callers", "Search")
	if !strings.Contains(out, "ambiguous") {
		t.Errorf("ambiguous seed not announced:\n%s", out)
	}
	if !strings.Contains(out, "internal/artifact/store.go:100") || !strings.Contains(out, "internal/db/store.go:55") {
		t.Errorf("both candidates must be listed with file:line:\n%s", out)
	}
}

func TestFlowResponse_SeedNotFound(t *testing.T) {
	out := FlowResponse(codeintel.FlowResult{Direction: codeintel.Callees}, "callees", "Nope")
	if !strings.Contains(out, "not found") {
		t.Errorf("missing seed should say not found:\n%s", out)
	}
}
