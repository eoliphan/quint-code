package present

import (
	"encoding/json"
	"testing"

	"github.com/m0n0x41d/haft/internal/artifact"
)

func mkDecision(t *testing.T, claims []artifact.DecisionClaim) *artifact.Artifact {
	t.Helper()
	sd, err := json.Marshal(artifact.DecisionFields{Claims: claims})
	if err != nil {
		t.Fatal(err)
	}
	return &artifact.Artifact{StructuredData: string(sd)}
}

// TestDecisionVerificationTag is the trust-decay signal: a governing decision
// surfaces how many of its predictions remain unverified, so unchecked rationale
// does not read as authoritative.
func TestDecisionVerificationTag(t *testing.T) {
	mixed := mkDecision(t, []artifact.DecisionClaim{
		{Claim: "a", Observable: "o", Threshold: "t", Status: artifact.ClaimStatusSupported},
		{Claim: "b", Observable: "o", Threshold: "t", Status: artifact.ClaimStatusUnverified},
		{Claim: "c", Observable: "o", Threshold: "t", Status: artifact.ClaimStatusUnverified},
	})
	if got := decisionVerificationTag(mixed); got != " · 2/3 predictions unverified" {
		t.Errorf("mixed = %q, want ' · 2/3 predictions unverified'", got)
	}

	allVerified := mkDecision(t, []artifact.DecisionClaim{
		{Claim: "a", Observable: "o", Threshold: "t", Status: artifact.ClaimStatusSupported},
	})
	if got := decisionVerificationTag(allVerified); got != "" {
		t.Errorf("all-verified = %q, want empty", got)
	}

	if got := decisionVerificationTag(&artifact.Artifact{}); got != "" {
		t.Errorf("no-claims = %q, want empty", got)
	}
}
