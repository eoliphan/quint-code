package reff

import (
	"testing"
	"time"
)

func TestComputeED_FreshEvidence(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	validUntil := now.Add(24 * time.Hour)

	if got := ComputeED(validUntil, now, 1.0); got != 0 {
		t.Fatalf("ComputeED(fresh) = %v, want 0", got)
	}
}

func TestComputeED_ExpiredEvidence(t *testing.T) {
	now := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	validUntil := now.Add(-10 * 24 * time.Hour)

	if got := ComputeED(validUntil, now, 0); got != 10.0 {
		t.Fatalf("ComputeED(expired) = %v, want 10.0", got)
	}
}

func TestAggregateED_SumsItems(t *testing.T) {
	now := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	items := []EDItem{
		{ValidUntil: now.Add(-10 * 24 * time.Hour), Now: now, K: 1.0},
		{ValidUntil: now.Add(-5 * 24 * time.Hour), Now: now, K: 0.5},
		{ValidUntil: now.Add(24 * time.Hour), Now: now, K: 1.0},
	}

	if got := AggregateED(items); got != 12.5 {
		t.Fatalf("AggregateED = %v, want 12.5", got)
	}
}

func TestCheckEDBudget(t *testing.T) {
	if alert := CheckEDBudget(10, 30); alert != nil {
		t.Fatalf("expected nil alert within budget, got %+v", alert)
	}

	alert := CheckEDBudget(31, 30)
	if alert == nil {
		t.Fatal("expected alert when debt exceeds budget")
	}
	if alert.Excess != 1 {
		t.Fatalf("excess = %v, want 1", alert.Excess)
	}
}

func TestParseValidUntil_AcceptsDateOnly(t *testing.T) {
	got, ok := ParseValidUntil("2026-04-11")
	if !ok {
		t.Fatal("expected date-only parse to succeed")
	}

	want := time.Date(2026, 4, 11, 23, 59, 59, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("ParseValidUntil(date) = %v, want %v", got, want)
	}
}

func TestScoreEvidence_DateOnlyExpiry(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	got := ScoreEvidence("supports", 3, "2026-04-11", now)

	if got != 0.1 {
		t.Fatalf("ScoreEvidence(date-only expiry) = %v, want 0.1", got)
	}
}

func TestScoreTypedEvidence_AttachedPreservesBaseScore(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	got := ScoreTypedEvidence("attached", "partial", 1, "2026-04-20", now)

	if got != 0.3 {
		t.Fatalf("ScoreTypedEvidence(attached) = %v, want 0.3", got)
	}
}

// TestScoreEvidenceWithCausalBasis_SimulationOnlyCaps verifies CC-B3.9:
// a strong base score (supports + explicit_measure + CL3 = 0.8) gets
// capped at 0.5 when the evidence declares simulation-only as its basis.
func TestScoreEvidenceWithCausalBasis_SimulationOnlyCaps(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	validUntil := now.Add(7 * 24 * time.Hour).Format(time.RFC3339)

	baseline := ScoreTypedEvidence("explicit_measure", "supports", 3, validUntil, now)
	if baseline != 0.8 {
		t.Fatalf("baseline ScoreTypedEvidence = %v, want 0.8", baseline)
	}

	capped := ScoreEvidenceWithCausalBasis(
		"explicit_measure", "supports", 3,
		"simulationOnlyCounterfactualOutputBasis", "",
		validUntil, now,
	)
	if capped != 0.5 {
		t.Fatalf("ScoreEvidenceWithCausalBasis(simulation_only) = %v, want 0.5", capped)
	}

	// alias must also cap
	cappedAlias := ScoreEvidenceWithCausalBasis(
		"explicit_measure", "supports", 3,
		"simulation_only", "",
		validUntil, now,
	)
	if cappedAlias != 0.5 {
		t.Fatalf("alias simulation_only = %v, want 0.5", cappedAlias)
	}
}

// TestScoreEvidenceWithCausalBasis_NonrealizableCaps verifies CC-B3.9 cap
// fires when realizability=nonrealizable even if basis is empty.
func TestScoreEvidenceWithCausalBasis_NonrealizableCaps(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	validUntil := now.Add(7 * 24 * time.Hour).Format(time.RFC3339)

	got := ScoreEvidenceWithCausalBasis(
		"explicit_measure", "supports", 3,
		"", "nonrealizable",
		validUntil, now,
	)
	if got != 0.5 {
		t.Fatalf("nonrealizable realizability = %v, want 0.5", got)
	}
}

// TestScoreEvidenceWithCausalBasis_UnknownDoesNotCap asserts that "unknown"
// realizability does NOT cap — bounded use may still be admissible per C.28.
func TestScoreEvidenceWithCausalBasis_UnknownDoesNotCap(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	validUntil := now.Add(7 * 24 * time.Hour).Format(time.RFC3339)

	got := ScoreEvidenceWithCausalBasis(
		"explicit_measure", "supports", 3,
		"", "unknown",
		validUntil, now,
	)
	if got != 0.8 {
		t.Fatalf("unknown realizability = %v, want 0.8 (no cap)", got)
	}
}

// TestScoreEvidenceWithCausalBasis_NoBasisIdenticalToTypedScore guarantees
// the legacy path: empty basis + empty realizability behaves exactly like
// ScoreTypedEvidence. Legacy artifacts cannot regress.
func TestScoreEvidenceWithCausalBasis_NoBasisIdenticalToTypedScore(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		eType, verdict, validUntil string
		cl                         int
	}{
		{"explicit_measure", "supports", "2026-06-01", 3},
		{"explicit_measure", "supports", "2026-06-01", 2},
		{"measurement", "weakens", "2026-06-01", 1},
		{"attached", "supports", "2026-06-01", 3},
		{"research", "supports", "2026-06-01", 0},
	}

	for _, tc := range cases {
		typed := ScoreTypedEvidence(tc.eType, tc.verdict, tc.cl, tc.validUntil, now)
		withBasis := ScoreEvidenceWithCausalBasis(tc.eType, tc.verdict, tc.cl, "", "", tc.validUntil, now)
		if typed != withBasis {
			t.Fatalf("legacy parity broken for %+v: typed=%v withBasis=%v", tc, typed, withBasis)
		}
	}
}

// TestScoreEvidenceWithCausalBasis_ExpiredEvidenceStillExpired ensures the
// cap floors at 0.5 — it does not raise expired (0.1) evidence.
func TestScoreEvidenceWithCausalBasis_ExpiredEvidenceStillExpired(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	expired := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)

	got := ScoreEvidenceWithCausalBasis(
		"explicit_measure", "supports", 3,
		"simulationOnlyCounterfactualOutputBasis", "",
		expired, now,
	)
	if got != 0.1 {
		t.Fatalf("expired evidence with simulation_only = %v, want 0.1 (no upward cap)", got)
	}
}
