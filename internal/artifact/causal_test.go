package artifact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseCausalSupportBasis_AcceptsAliasesAndCanonicalizes(t *testing.T) {
	cases := []struct {
		input string
		want  CausalEvidenceSupportBasis
	}{
		{"", ""},
		{"   ", ""},
		{"observational", CausalBasisObservational},
		{"observational_association", CausalBasisObservational},
		{"observationalAssociationSupportBasis", CausalBasisObservational},
		{"interventional", CausalBasisInterventional},
		{"interventional-action", CausalBasisInterventional},
		{"realized_counterfactual", CausalBasisRealizedCounterfactual},
		{"realizedCounterfactualSample", CausalBasisRealizedCounterfactual},
		{"identified_estimate", CausalBasisIdentifiedEstimate},
		{"identifiedCounterfactualEstimate", CausalBasisIdentifiedEstimate},
		{"simulation_only", CausalBasisSimulationOnly},
		{"SIMULATION", CausalBasisSimulationOnly},
		{"simulationOnlyCounterfactualOutputBasis", CausalBasisSimulationOnly},
	}

	for _, tc := range cases {
		got, err := ParseCausalSupportBasis(tc.input)
		if err != nil {
			t.Fatalf("ParseCausalSupportBasis(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("ParseCausalSupportBasis(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	if _, err := ParseCausalSupportBasis("randomgarbage"); err == nil {
		t.Fatal("ParseCausalSupportBasis accepted invalid value")
	}
}

func TestParseRealizabilityVerdict_AcceptsAliases(t *testing.T) {
	cases := []struct {
		input string
		want  RealizabilityVerdict
	}{
		{"", ""},
		{"  ", ""},
		{"realizable", RealizabilityRealizable},
		{"REALIZABLE", RealizabilityRealizable},
		{"nonrealizable", RealizabilityNonrealizable},
		{"non-realizable", RealizabilityNonrealizable},
		{"not_realizable", RealizabilityNonrealizable},
		{"unrealizable", RealizabilityNonrealizable},
		{"unknown", RealizabilityUnknown},
	}

	for _, tc := range cases {
		got, err := ParseRealizabilityVerdict(tc.input)
		if err != nil {
			t.Fatalf("ParseRealizabilityVerdict(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("ParseRealizabilityVerdict(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	if _, err := ParseRealizabilityVerdict("partially-realizable"); err == nil {
		t.Fatal("ParseRealizabilityVerdict accepted invalid value")
	}
}

func TestNewDecisionClaims_PreservesRealizability(t *testing.T) {
	inputs := []PredictionInput{
		{Claim: "intervention X reduces error rate", Observable: "p99 error", Threshold: "< 1%", Realizability: "nonrealizable"},
		{Claim: "training run finishes", Observable: "wallclock", Threshold: "< 4h"}, // unset
	}

	got := newDecisionClaims(inputs)
	if len(got) != 2 {
		t.Fatalf("expected 2 claims, got %d", len(got))
	}
	if got[0].Realizability != RealizabilityNonrealizable {
		t.Fatalf("claim 0 Realizability = %q, want %q", got[0].Realizability, RealizabilityNonrealizable)
	}
	if got[1].Realizability != "" {
		t.Fatalf("claim 1 Realizability = %q, want empty", got[1].Realizability)
	}
}

func TestNormalizeDecisionClaims_PreservesRealizability(t *testing.T) {
	claims := []DecisionClaim{
		{Claim: "x", Observable: "y", Threshold: "z", Realizability: "  realizable  "},
		{Claim: "a", Observable: "b", Threshold: "c"},
	}

	got := normalizeDecisionClaims(claims)
	if len(got) != 2 {
		t.Fatalf("expected 2 claims, got %d", len(got))
	}
	if got[0].Realizability != RealizabilityRealizable {
		t.Fatalf("claim 0 Realizability = %q, want %q", got[0].Realizability, RealizabilityRealizable)
	}
	if got[1].Realizability != "" {
		t.Fatalf("claim 1 Realizability = %q, want empty", got[1].Realizability)
	}
}

func TestDecisionPredictionsFromClaims_PreservesRealizability(t *testing.T) {
	claims := []DecisionClaim{
		{ID: "claim-001", Claim: "x", Observable: "y", Threshold: "z", Realizability: RealizabilityUnknown},
	}

	predictions := decisionPredictionsFromClaims(claims)
	if len(predictions) != 1 {
		t.Fatalf("expected 1 prediction, got %d", len(predictions))
	}
	if predictions[0].Realizability != RealizabilityUnknown {
		t.Fatalf("Realizability = %q, want %q", predictions[0].Realizability, RealizabilityUnknown)
	}
}

func TestDecisionClaimsFromPredictions_PreservesRealizability(t *testing.T) {
	predictions := []DecisionPrediction{
		{Claim: "x", Observable: "y", Threshold: "z", Realizability: RealizabilityRealizable},
	}

	claims := decisionClaimsFromPredictions(predictions)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}
	if claims[0].Realizability != RealizabilityRealizable {
		t.Fatalf("Realizability = %q, want %q", claims[0].Realizability, RealizabilityRealizable)
	}
}

func TestEvidenceItem_RoundTripPreservesCausalSupportBasis(t *testing.T) {
	original := EvidenceItem{
		ID:                 "evid-001",
		Type:               "measurement",
		Content:            "X intervention reduced y by 12%",
		Verdict:            "supports",
		CongruenceLevel:    3,
		CausalSupportBasis: CausalBasisInterventional,
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"causal_support_basis":"interventionalActionSupportBasis"`) {
		t.Fatalf("encoded JSON missing canonical basis: %s", encoded)
	}

	var decoded EvidenceItem
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.CausalSupportBasis != CausalBasisInterventional {
		t.Fatalf("round-trip basis = %q, want %q", decoded.CausalSupportBasis, CausalBasisInterventional)
	}

	emptyOriginal := EvidenceItem{ID: "evid-002", Type: "general", Content: "no basis", Verdict: "supports"}
	emptyEncoded, err := json.Marshal(emptyOriginal)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if strings.Contains(string(emptyEncoded), "causal_support_basis") {
		t.Fatalf("omitempty broken for empty basis: %s", emptyEncoded)
	}
}
