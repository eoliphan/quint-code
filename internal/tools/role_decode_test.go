package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/artifact"
)

// TestJSONDecodeArg_PreservesDimensionRoleAndValidUntil checks the MCP arg-decode
// helper: when the tool receives a dimensions array (as the framework delivers it,
// []any of map[string]any), role + valid_until must survive decoding into
// CharacterizeInput.Dimensions.
func TestJSONDecodeArg_PreservesDimensionRoleAndValidUntil(t *testing.T) {
	args := map[string]any{
		"dimensions": []any{
			map[string]any{
				"name":        "must_pass",
				"role":        "constraint",
				"polarity":    "true_better",
				"scale_type":  "binary",
				"valid_until": "2026-09-03",
			},
			map[string]any{
				"name":     "watch_only",
				"role":     "observation",
				"polarity": "lower_better",
			},
		},
	}

	var input artifact.CharacterizeInput
	if ok := jsonDecodeArg(args, "dimensions", &input.Dimensions); !ok {
		t.Fatal("jsonDecodeArg returned false")
	}
	if len(input.Dimensions) != 2 {
		t.Fatalf("decoded %d dimensions, want 2", len(input.Dimensions))
	}
	if got := input.Dimensions[0].Role; got != "constraint" {
		t.Errorf("role dropped in decode: got %q, want constraint", got)
	}
	if got := input.Dimensions[0].ValidUntil; got == "" {
		t.Errorf("valid_until dropped in decode: got empty, want 2026-09-03")
	}
	if got := input.Dimensions[1].Role; got != "observation" {
		t.Errorf("observation role dropped in decode: got %q, want observation", got)
	}
}

// TestHaftProblemTool_CharacterizePreservesRoleAndValidUntil exercises the FULL tool
// path (Execute(argsJSON) -> jsonDecodeArg -> CharacterizeProblem -> store), which is
// what /h-compare ultimately depends on. If this passes, the entire current haft
// source preserves indicator roles + valid_until, and the observed runtime drop
// (stored tables showing Role=target, no Valid Until) is a STALE running `haft serve`
// binary, not a source bug — i.e. the fix is rebuild/reinstall, not a code change.
func TestHaftProblemTool_CharacterizePreservesRoleAndValidUntil(t *testing.T) {
	store := setupHaftToolStore(t)
	ctx := context.Background()
	haftDir := t.TempDir()
	tool := NewHaftProblemTool(store, haftDir)

	frameRes, err := tool.Execute(ctx, mustJSON(t, map[string]any{
		"action": "frame",
		"title":  "Role path",
		"signal": "indicator roles must survive the full tool path",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if frameRes.Meta == nil || frameRes.Meta.ArtifactRef == "" {
		t.Fatal("frame did not return an artifact ref")
	}
	probID := frameRes.Meta.ArtifactRef

	_, err = tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":      "characterize",
		"problem_ref": probID,
		"dimensions": []any{
			map[string]any{"name": "must_pass", "role": "constraint", "polarity": "true_better", "scale_type": "binary", "valid_until": "2026-09-03"},
			map[string]any{"name": "watch_only", "role": "observation", "polarity": "lower_better", "scale_type": "ordinal"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := store.Get(ctx, probID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reloaded.Body, "constraint") {
		t.Errorf("constraint role dropped through the tool path; body:\n%s", reloaded.Body)
	}
	if !strings.Contains(reloaded.Body, "observation") {
		t.Errorf("observation role dropped through the tool path; body:\n%s", reloaded.Body)
	}
	if !strings.Contains(reloaded.Body, "2026-09-03") {
		t.Errorf("valid_until dropped through the tool path; body:\n%s", reloaded.Body)
	}
}
