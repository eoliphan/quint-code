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

func TestNodeResponse_OverloadsBodyTrailFusion(t *testing.T) {
	view := codeintel.NodeView{
		Name:  "Do",
		Found: true,
		Overloads: []codeintel.NodeOverload{
			{ // verified body + symbol-level governance + trail
				Symbol:  codebase.CodeSymbol{Name: "Do", Receiver: "Foo", FilePath: "a.go", StartLine: 10, EndLine: 12, Kind: "method"},
				Body:    "func (f *Foo) Do() string { return \"x\" }",
				BodyOK:  true,
				Context: contextgraph.CodeContext{Decisions: []*artifact.Artifact{decArtifact("dec-1", "foo rule")}},
				Callees: []codeintel.FusedHop{{Symbol: codebase.CodeSymbol{Name: "helper", FilePath: "a.go", StartLine: 30}, ViaKind: codebase.EdgeCall}},
			},
			{ // unverified body must NOT print source
				Symbol: codebase.CodeSymbol{Name: "Do", Receiver: "Bar", FilePath: "b.go", StartLine: 5, EndLine: 7, Kind: "method"},
				Body:   "stale bytes",
				BodyOK: false,
			},
		},
	}

	out := NodeResponse(view, "go")

	if !strings.Contains(out, "2 definitions") {
		t.Errorf("should report 2 overloads:\n%s", out)
	}
	if !strings.Contains(out, "return \"x\"") {
		t.Errorf("verified body must be shown:\n%s", out)
	}
	if strings.Contains(out, "stale bytes") {
		t.Errorf("UNVERIFIED body must NOT be printed:\n%s", out)
	}
	if !strings.Contains(out, "could not be verified") {
		t.Errorf("unverified overload must say so:\n%s", out)
	}
	if !strings.Contains(out, "dec-1") {
		t.Errorf("symbol-level fusion must render:\n%s", out)
	}
	if !strings.Contains(out, "Calls:") || !strings.Contains(out, "helper") {
		t.Errorf("callee trail must render:\n%s", out)
	}
}

func TestNodeResponse_ContainerMembersAndModuleGovernance(t *testing.T) {
	view := codeintel.NodeView{
		Name:  "Store",
		Found: true,
		Overloads: []codeintel.NodeOverload{{
			Symbol:  codebase.CodeSymbol{Name: "Store", FilePath: "store.go", StartLine: 1, EndLine: 3, Kind: "type"},
			Body:    "type Store struct{}",
			BodyOK:  true,
			Context: contextgraph.CodeContext{Module: "internal/x", Governed: true, ModuleDecisions: []graph.Node{{ID: "dec-9", Name: "x boundary"}}},
			Members: []codebase.CodeSymbol{{Name: "Get", FilePath: "store.go", StartLine: 5}, {Name: "Set", FilePath: "store.go", StartLine: 9}},
		}},
	}
	out := NodeResponse(view, "go")
	if !strings.Contains(out, "Members:") || !strings.Contains(out, "Get") || !strings.Contains(out, "Set") {
		t.Errorf("container member outline missing:\n%s", out)
	}
	if !strings.Contains(out, "Module governed by") || !strings.Contains(out, "dec-9") {
		t.Errorf("module-level governance must render, not blank:\n%s", out)
	}
}

func TestNodeResponse_NotFound(t *testing.T) {
	out := NodeResponse(codeintel.NodeView{Name: "Nope"}, "go")
	if !strings.Contains(out, "not found") {
		t.Errorf("missing node should say not found:\n%s", out)
	}
}
