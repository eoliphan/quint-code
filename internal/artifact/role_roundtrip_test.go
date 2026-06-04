package artifact

import (
	"context"
	"testing"
)

// TestCharacterizeProblem_PreservesRoleAndValidUntil pins where indicator roles
// (constraint/observation) and valid_until are lost between characterize and the
// read path that /h-compare consumes (characterizedDimensionsForProblem).
//
// Observed bug: stored characterization tables show Role="target" for every
// dimension (even ones explicitly set constraint/observation) and no Valid Until,
// while scale_type/unit/polarity/how_to_measure survive — which cascades into
// /h-compare ignoring indicator roles. This test isolates the ARTIFACT layer:
// if it passes, the drop is upstream (MCP arg decode); if it fails, it is here.
func TestCharacterizeProblem_PreservesRoleAndValidUntil(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	haftDir := t.TempDir()

	prob, _, err := FrameProblem(ctx, store, haftDir, ProblemFrameInput{
		Title:  "Role round-trip",
		Signal: "indicator roles must survive characterize -> compare read",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = CharacterizeProblem(ctx, store, haftDir, CharacterizeInput{
		ProblemRef: prob.Meta.ID,
		Dimensions: []ComparisonDimension{
			{Name: "throughput", Role: "target", Polarity: "higher_better", ScaleType: "ratio"},
			{Name: "must_pass", Role: "constraint", Polarity: "true_better", ScaleType: "binary", ValidUntil: "2026-09-03"},
			{Name: "watch_only", Role: "observation", Polarity: "lower_better", ScaleType: "ordinal"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Re-fetch from the store to exercise the real characterize -> persist -> reload
	// path that /h-compare uses (it does not reuse the returned artifact in memory).
	reloaded, err := store.Get(ctx, prob.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}

	dims := characterizedDimensionsForProblem(reloaded)
	byName := make(map[string]charDim, len(dims))
	for _, d := range dims {
		byName[d.Name] = d
	}

	if got := byName["must_pass"].Role; got != "constraint" {
		t.Errorf("constraint role dropped on characterize round-trip: got %q, want %q", got, "constraint")
	}
	if got := byName["watch_only"].Role; got != "observation" {
		t.Errorf("observation role dropped on characterize round-trip: got %q, want %q", got, "observation")
	}
	if got := byName["must_pass"].ValidUntil; got == "" {
		t.Errorf("valid_until dropped on characterize round-trip for must_pass: got empty, want 2026-09-03")
	}
}
