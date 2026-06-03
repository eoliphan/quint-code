package artifact

import (
	"context"
	"strings"
	"testing"
)

func TestNonDiscriminatingDimensionWarnings_TargetAllEqualWarns(t *testing.T) {
	w := nonDiscriminatingDimensionWarnings(
		[]string{"A", "B"},
		map[string]map[string]string{"A": {"d": "same"}, "B": {"d": "same"}},
		[]string{"d"},
		map[string]charDim{}, // empty → role defaults to target
	)
	if len(w) != 1 || !strings.Contains(w[0], "'d'") || !strings.Contains(w[0], "does not discriminate") {
		t.Fatalf("expected one non-discriminating warning for a target dim, got %v", w)
	}
}

func TestNonDiscriminatingDimensionWarnings_ConstraintAllEqualSkipped(t *testing.T) {
	char := map[string]charDim{normalizeArtifactKey("d"): {Name: "d", Role: "constraint"}}
	w := nonDiscriminatingDimensionWarnings(
		[]string{"A", "B"},
		map[string]map[string]string{"A": {"d": "pass"}, "B": {"d": "pass"}},
		[]string{"d"},
		char,
	)
	if len(w) != 0 {
		t.Fatalf("a constraint all variants satisfy identically (all-pass) must not warn, got %v", w)
	}
}

func TestNonDiscriminatingDimensionWarnings_ObservationSkipped(t *testing.T) {
	char := map[string]charDim{normalizeArtifactKey("d"): {Name: "d", Role: "observation"}}
	w := nonDiscriminatingDimensionWarnings(
		[]string{"A", "B"},
		map[string]map[string]string{"A": {"d": "x"}, "B": {"d": "x"}},
		[]string{"d"},
		char,
	)
	if len(w) != 0 {
		t.Fatalf("observation dimensions are watch-only and must not warn, got %v", w)
	}
}

func TestNonDiscriminatingDimensionWarnings_DiscriminatingTargetClean(t *testing.T) {
	w := nonDiscriminatingDimensionWarnings(
		[]string{"A", "B"},
		map[string]map[string]string{"A": {"d": "fast"}, "B": {"d": "slow"}},
		[]string{"d"},
		map[string]charDim{},
	)
	if len(w) != 0 {
		t.Fatalf("a discriminating target must not warn, got %v", w)
	}
}

func TestNonDiscriminatingDimensionWarnings_SingleScoredSkipped(t *testing.T) {
	w := nonDiscriminatingDimensionWarnings(
		[]string{"A", "B"},
		map[string]map[string]string{"A": {"d": "x"}}, // B has no score → missing-data, not non-discriminating
		[]string{"d"},
		map[string]charDim{},
	)
	if len(w) != 0 {
		t.Fatalf("a single scored variant is missing-data, not non-discriminating; got %v", w)
	}
}

// End-to-end: a target dimension that scores identically across all variants
// must surface a non-discriminating warning in the real comparison body,
// while the genuinely discriminating dimensions stay quiet.
func TestCompareSolutions_FlagsNonDiscriminatingTargetDimension(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	haftDir := t.TempDir()

	portfolio, _, _ := ExploreSolutions(ctx, store, haftDir, ExploreInput{
		Variants: []Variant{
			testVariant("Alpha", "weaker on latency", "Trade latency for cost"),
			testVariant("Beta", "weaker on cost", "Trade cost for latency"),
		},
		NoSteppingStoneRationale: "Both are direct end-state candidates.",
	})

	input := CompareInput{
		PortfolioRef: portfolio.Meta.ID,
		Results: ComparisonResult{
			Dimensions: []string{"latency", "cost", "footprint"},
			Scores: map[string]map[string]string{
				"Alpha": {"latency": "42ms", "cost": "$100", "footprint": "medium"},
				"Beta":  {"latency": "18ms", "cost": "$300", "footprint": "medium"},
			},
			NonDominatedSet: []string{"Alpha", "Beta"},
			ParetoTradeoffs: []ParetoTradeoffNote{
				{Variant: "Alpha", Summary: "Cheaper, slower."},
				{Variant: "Beta", Summary: "Faster, costlier."},
			},
			PolicyApplied: "Balance latency against cost.",
			SelectedRef:   "Beta",
		},
	}

	a, _, err := CompareSolutions(ctx, store, haftDir, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.Body, "footprint") || !strings.Contains(a.Body, "does not discriminate") {
		t.Fatalf("expected non-discriminating warning for 'footprint':\n%s", a.Body)
	}
	if strings.Contains(a.Body, "'latency' scores identically") || strings.Contains(a.Body, "'cost' scores identically") {
		t.Fatalf("discriminating dimensions latency/cost must not be flagged:\n%s", a.Body)
	}
}
