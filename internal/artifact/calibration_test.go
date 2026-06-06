package artifact

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func floatPtr(v float64) *float64 { return &v }

// TestCalibrationProfile_DecideVerifyScoreLoop exercises the full
// decide(probability) → verify(outcome) → score loop through real persistence:
// a decision is stored with claims carrying elicited probabilities and verified
// statuses, then CalibrationProfile re-reads the store and decomposed-Brier scores
// only the verified, probabilistic claims (dec-20260603-c3c7fa88).
func TestCalibrationProfile_DecideVerifyScoreLoop(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	df := DecisionFields{
		SelectedTitle: "calibration loop",
		WhySelected:   "exercise the score loop",
		Claims: []DecisionClaim{
			// verified + probabilistic → forecasts
			{Claim: "a", Observable: "o", Threshold: "t", Status: ClaimStatusSupported, Probability: floatPtr(0.9)},
			{Claim: "b", Observable: "o", Threshold: "t", Status: ClaimStatusRefuted, Probability: floatPtr(0.8)},
			// probabilistic but not yet verified → excluded (no binary outcome)
			{Claim: "c", Observable: "o", Threshold: "t", Status: ClaimStatusUnverified, Probability: floatPtr(0.7)},
			// verified but no probability → excluded (no forecast)
			{Claim: "d", Observable: "o", Threshold: "t", Status: ClaimStatusSupported},
		},
	}
	sd, err := json.Marshal(df)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	dec := &Artifact{
		Meta: Meta{
			ID: "dec-20260604-calib", Kind: KindDecisionRecord, Status: StatusActive,
			Title: "calibration loop", CreatedAt: now, UpdatedAt: now,
		},
		Body:           "body",
		StructuredData: string(sd),
	}
	if err := store.Create(ctx, dec); err != nil {
		t.Fatal(err)
	}

	v, err := CalibrationProfile(ctx, store)
	if err != nil {
		t.Fatal(err)
	}

	// Only the two verified + probabilistic claims form forecasts:
	// {0.9, supported→1} and {0.8, refuted→0}.
	if v.Components.N != 2 {
		t.Fatalf("expected 2 forecasts (verified + probabilistic only), got %d", v.Components.N)
	}
	// MeanBrier = ((0.9−1)² + (0.8−0)²)/2 = (0.01 + 0.64)/2 = 0.325
	if math.Abs(v.Components.MeanBrier-0.325) > 1e-9 {
		t.Errorf("MeanBrier = %v, want 0.325", v.Components.MeanBrier)
	}
	// Mean forecast 0.85 vs base rate 0.5 → forecasts run high → overconfident.
	if v.Direction != "overconfident" {
		t.Errorf("expected overconfident, got %s (bias %v)", v.Direction, v.Bias)
	}
}

// TestCalibrationProfile_EmptyIsClean confirms a project with no probabilistic
// claims yields an empty, non-erroring profile (cold start).
func TestCalibrationProfile_EmptyIsClean(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	v, err := CalibrationProfile(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if v.Components.N != 0 {
		t.Errorf("empty project should yield 0 forecasts, got %d", v.Components.N)
	}
}

// TestProbabilityRoundTrips confirms an elicited probability survives the
// input → store → read path without migration (additive structured_data field).
func TestProbabilityRoundTrips(t *testing.T) {
	df := DecisionFields{
		Claims: []DecisionClaim{
			{Claim: "x", Observable: "o", Threshold: "t", Probability: floatPtr(0.65)},
		},
	}
	sd, err := json.Marshal(df)
	if err != nil {
		t.Fatal(err)
	}
	a := &Artifact{StructuredData: string(sd)}
	got := a.UnmarshalDecisionFields()
	if len(got.Claims) != 1 || got.Claims[0].Probability == nil {
		t.Fatalf("probability lost on round-trip: %+v", got.Claims)
	}
	if *got.Claims[0].Probability != 0.65 {
		t.Errorf("probability = %v, want 0.65", *got.Claims[0].Probability)
	}
}
