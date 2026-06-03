package present

import (
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/codebase"
	"github.com/m0n0x41d/haft/internal/codeintel"
	"github.com/m0n0x41d/haft/internal/contextgraph"
)

func TestExploreResponse_SpineBlastSourceFused(t *testing.T) {
	res := codeintel.ExploreResult{
		Seed:        codebase.CodeSymbol{Name: "FrameProblem", FilePath: "internal/artifact/problem.go", StartLine: 147},
		SeedFound:   true,
		SeedBody:    "func FrameProblem() error { return nil }",
		SeedBodyOK:  true,
		BridgesUsed: 1,
		Chain: []codeintel.ChainStep{
			{
				Symbol:   codebase.CodeSymbol{Name: "FrameProblem", FilePath: "internal/artifact/problem.go", StartLine: 147},
				Distance: 0,
				Context:  contextgraph.CodeContext{Decisions: []*artifact.Artifact{decArtifact("dec-seed", "framing rule")}},
			},
			{
				Symbol:     codebase.CodeSymbol{Name: "Create", Receiver: "Store", FilePath: "internal/artifact/store.go", StartLine: 44},
				Distance:   1,
				ViaKind:    codebase.EdgeInterfaceDispatch,
				Provenance: codebase.ProvenanceHeuristic,
				Context:    contextgraph.CodeContext{Decisions: []*artifact.Artifact{decArtifact("dec-store", "store contract")}},
			},
		},
		BlastRadius: []codeintel.FusedHop{
			{Symbol: codebase.CodeSymbol{Name: "handleProblem", FilePath: "internal/cli/serve.go", StartLine: 400}, Context: contextgraph.CodeContext{Decisions: []*artifact.Artifact{decArtifact("dec-cli", "cli")}}},
		},
	}

	out := ExploreResponse(res, "FrameProblem", "go")

	if !strings.Contains(out, "spine of 2") {
		t.Errorf("chain length not reported:\n%s", out)
	}
	if !strings.Contains(out, "dec-seed") || !strings.Contains(out, "dec-store") {
		t.Errorf("per-on-chain-symbol fusion must render (the moat):\n%s", out)
	}
	if !strings.Contains(out, "heuristic boundary") {
		t.Errorf("dispatch bridge on the spine must be flagged:\n%s", out)
	}
	if !strings.Contains(out, "Blast radius") || !strings.Contains(out, "handleProblem") || !strings.Contains(out, "dec-cli") {
		t.Errorf("blast radius + covering decisions must render:\n%s", out)
	}
	if !strings.Contains(out, "func FrameProblem()") {
		t.Errorf("verbatim seed source must be included (0–1 Read sufficiency):\n%s", out)
	}
}

func TestExploreResponse_UnresolvedBoundaryHonest(t *testing.T) {
	res := codeintel.ExploreResult{
		Seed:          codebase.CodeSymbol{Name: "Dispatch", FilePath: "x.go", StartLine: 1},
		SeedFound:     true,
		UnresolvedEnd: true,
		SeedBodyOK:    false,
		Chain: []codeintel.ChainStep{
			{Symbol: codebase.CodeSymbol{Name: "Dispatch", FilePath: "x.go", StartLine: 1}},
		},
	}
	out := ExploreResponse(res, "Dispatch", "go")
	if !strings.Contains(out, "unresolved dispatch boundary") {
		t.Errorf("a chain stopping at a boundary must say so, not imply completeness:\n%s", out)
	}
	if strings.Contains(out, "```go") {
		t.Errorf("unverified seed body must not be printed:\n%s", out)
	}
}

func TestExploreResponse_AmbiguousAndNotFound(t *testing.T) {
	amb := ExploreResponse(codeintel.ExploreResult{Ambiguous: []codebase.CodeSymbol{{Name: "S", FilePath: "a.go", StartLine: 1}, {Name: "S", FilePath: "b.go", StartLine: 2}}}, "S", "go")
	if !strings.Contains(amb, "ambiguous") {
		t.Errorf("ambiguous seed must be surfaced:\n%s", amb)
	}
	nf := ExploreResponse(codeintel.ExploreResult{}, "Nope", "go")
	if !strings.Contains(nf, "not found") {
		t.Errorf("missing seed must say not found:\n%s", nf)
	}
}
