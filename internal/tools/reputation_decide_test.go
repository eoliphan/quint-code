package tools

import (
	"strings"
	"testing"
)

// decideArgsWithWhy builds a valid decide arg set (mirrors the working
// decide example) with a caller-supplied why_selected, so tests vary only
// the rationale text.
func decideArgsWithWhy(fixture decisionToolFixture, whySelected string) map[string]any {
	return completeDecisionArgs(map[string]any{
		"action":         "decide",
		"problem_ref":    fixture.problem.Meta.ID,
		"portfolio_ref":  fixture.comparedPortfolio.Meta.ID,
		"selected_title": "gRPC",
		"why_selected":   whySelected,
		"weakest_link":   "Operational confidence still depends on limited production-grade evidence.",
		"invariants":     []string{"p99 latency remains below 50ms during cutover"},
		"admissibility":  []string{"No silent message loss during protocol migration"},
		"affected_files": []string{"internal/transport/grpc.go"},
		"predictions": []map[string]any{
			{"claim": "Latency stays under 50ms", "observable": "publish latency p99", "threshold": "< 50ms"},
			{"claim": "Throughput stays above 100k events/sec", "observable": "throughput", "threshold": "> 100k events/sec"},
		},
		"rollback": map[string]any{"triggers": []string{"Error budget exceeds 2% during canary"}},
	})
}

// TestHaftDecisionTool_DecideSurfacesReputationWarning is the end-to-end
// assertion for FPF E.9.DA:4.4b: a decision whose rationale leans on
// popularity/standard-status must surface the advisory in the real decide
// response.
func TestHaftDecisionTool_DecideSurfacesReputationWarning(t *testing.T) {
	fixture := setupDecisionToolFixture(t)

	result, err := fixture.tool.Execute(fixture.ctx, mustJSON(t,
		decideArgsWithWhy(fixture, "gRPC is the industry standard and everyone uses it")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.DisplayText, "Reputation signals") {
		t.Fatalf("expected reputation warning in decide output, got:\n%s", result.DisplayText)
	}
	if !strings.Contains(result.DisplayText, "industry standard") {
		t.Fatalf("decide output missing detected phrase:\n%s", result.DisplayText)
	}
}

// TestHaftDecisionTool_DecideContentReasonNoReputationWarning guards the
// boundary: a genuine content reason must NOT trip the warning.
func TestHaftDecisionTool_DecideContentReasonNoReputationWarning(t *testing.T) {
	fixture := setupDecisionToolFixture(t)

	result, err := fixture.tool.Execute(fixture.ctx, mustJSON(t,
		decideArgsWithWhy(fixture, "gRPC gives strict schemas and bidirectional streaming our ordering guarantee needs")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.DisplayText, "Reputation signals") {
		t.Fatalf("did not expect reputation warning for a content reason, got:\n%s", result.DisplayText)
	}
}
