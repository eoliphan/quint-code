package tools

import (
	"slices"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/artifact"
)

func TestDetectExploreWarnings_DisguisedDuplicateTitles(t *testing.T) {
	input := artifact.ExploreInput{
		Variants: []artifact.Variant{
			{Title: "Cache responses", WeakestLink: "cache invalidation"},
			{Title: "Cache responses with TTL", WeakestLink: "ttl tuning"},
		},
	}
	warnings := detectExploreWarnings(input, "")
	if !containsSubstring(warnings, "rewordings") {
		t.Fatalf("expected disguised-duplicate warning, got %v", warnings)
	}
}

func TestDetectExploreWarnings_WLNKRepeatsTitle(t *testing.T) {
	input := artifact.ExploreInput{
		Variants: []artifact.Variant{
			{Title: "Use cache", WeakestLink: "use cache"},
			{Title: "Restructure pipeline", WeakestLink: "data layout coupling"},
		},
	}
	warnings := detectExploreWarnings(input, "")
	if !containsSubstring(warnings, "WLNK repeats the title") {
		t.Fatalf("expected wlnk-repeats-title warning, got %v", warnings)
	}
}

func TestDetectExploreWarnings_ParityMissingFor3PlusVariants(t *testing.T) {
	input := artifact.ExploreInput{
		Variants: []artifact.Variant{
			{Title: "A", WeakestLink: "x"},
			{Title: "B", WeakestLink: "y"},
			{Title: "C", WeakestLink: "z"},
		},
	}
	warnings := detectExploreWarnings(input, "")
	if !containsSubstring(warnings, "parity_rules empty") {
		t.Fatalf("expected parity-missing warning, got %v", warnings)
	}
}

func TestDetectExploreWarnings_NoSteppingStoneWithoutRationale(t *testing.T) {
	input := artifact.ExploreInput{
		Variants: []artifact.Variant{
			{Title: "A", WeakestLink: "x"},
			{Title: "B", WeakestLink: "y"},
		},
	}
	warnings := detectExploreWarnings(input, "equal compute budget")
	if !containsSubstring(warnings, "stepping_stone") {
		t.Fatalf("expected no-stepping-stone warning, got %v", warnings)
	}
}

func TestDetectExploreWarnings_CleanInputProducesNoWarnings(t *testing.T) {
	input := artifact.ExploreInput{
		Variants: []artifact.Variant{
			{Title: "Cache", WeakestLink: "cache invalidation under schema change"},
			{Title: "Restructure", WeakestLink: "migration risk", SteppingStone: true, SteppingStoneBasis: "enables future X"},
		},
	}
	if w := detectExploreWarnings(input, "equal budget"); len(w) > 0 {
		t.Fatalf("expected no warnings on clean input, got %v", w)
	}
}

func TestDetectCompareWarnings_SingleDimension(t *testing.T) {
	input := artifact.CompareInput{}
	input.Results.Dimensions = []string{"latency"}
	input.Results.PolicyApplied = "min latency"
	warnings := detectCompareWarnings(input)
	if !containsSubstring(warnings, "1 dimension") {
		t.Fatalf("expected single-dim warning, got %v", warnings)
	}
}

func TestDetectCompareWarnings_PolicyMissing(t *testing.T) {
	input := artifact.CompareInput{}
	input.Results.Dimensions = []string{"latency", "cost"}
	warnings := detectCompareWarnings(input)
	if !containsSubstring(warnings, "policy_applied empty") {
		t.Fatalf("expected policy-missing warning, got %v", warnings)
	}
}

func TestDetectCompareWarnings_SelectedNotInNonDominated(t *testing.T) {
	input := artifact.CompareInput{}
	input.Results.Dimensions = []string{"latency", "cost"}
	input.Results.PolicyApplied = "min cost"
	input.Results.SelectedRef = "var-x"
	input.Results.NonDominatedSet = []string{"var-a", "var-b"}
	warnings := detectCompareWarnings(input)
	if !containsSubstring(warnings, "NOT in non_dominated_set") {
		t.Fatalf("expected dominated-selected warning, got %v", warnings)
	}
}

func TestDetectDecideWarnings_AllMandatoryFieldsEmpty(t *testing.T) {
	input := artifact.DecideInput{}
	warnings := detectDecideWarnings(input)
	wantSubs := []string{"weakest_link empty", "selection_policy empty", "counterargument empty", "no predictions declared", "rollback steps empty"}
	for _, sub := range wantSubs {
		if !containsSubstring(warnings, sub) {
			t.Errorf("expected warning containing %q, got %v", sub, warnings)
		}
	}
}

func TestDetectDecideWarnings_NoVerifyAfterOnPredictions(t *testing.T) {
	input := artifact.DecideInput{
		WeakestLink:     "x",
		SelectionPolicy: "y",
		CounterArgument: "z",
		Predictions: []artifact.PredictionInput{
			{Claim: "X drops 30%", Observable: "p95"},
		},
		Rollback: &artifact.RollbackSpec{Steps: []string{"revert"}},
	}
	warnings := detectDecideWarnings(input)
	if !containsSubstring(warnings, "verify_after") {
		t.Fatalf("expected verify_after warning, got %v", warnings)
	}
}

func containsSubstring(items []string, needle string) bool {
	return slices.ContainsFunc(items, func(s string) bool {
		return strings.Contains(s, needle)
	})
}
