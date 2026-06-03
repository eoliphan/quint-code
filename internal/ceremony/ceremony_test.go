package ceremony

import "testing"

// The cardinal invariant: a DETECTED high-risk change is NEVER recommended
// tactical. Covers the decision's prediction-1 across the high-risk classes.
func TestClassify_HighRiskNeverTactical(t *testing.T) {
	cases := []struct {
		name string
		in   Input
	}{
		{"sql file", Input{Files: []string{"db/schema.sql"}}},
		{"migration path", Input{Files: []string{"internal/store/migrations/0007_add.go"}}},
		{"auth one-liner (tiny fan-out — leaf must not read as safe)", Input{Files: []string{"internal/auth/token.go"}}},
		{"public api proto", Input{Files: []string{"api/v1/users.proto"}}},
		{"infra terraform", Input{Files: []string{"deploy/main.tf"}}},
		{"destructive SQL content on innocuous path", Input{Files: []string{"internal/repo/cleanup.go"}, ContentHighRisk: map[string]string{"internal/repo/cleanup.go": "DROP TABLE"}}},
		{"governing decision low reversibility", Input{Files: []string{"internal/x/x.go"}, Gov: GovFacts{Reversibility: "low"}}},
	}
	for _, c := range cases {
		rec := Classify(c.in)
		if rec.Band != RiskHigh {
			t.Errorf("%s: expected RiskHigh, got band %d", c.name, rec.Band)
		}
		if rec.Mode == ModeTactical {
			t.Errorf("%s: HIGH-risk recommended tactical — safety floor breached", c.name)
		}
		if rec.Mode != ModeDeep {
			t.Errorf("%s: high-risk should map to deep, got %q", c.name, rec.Mode)
		}
	}
}

// Prediction-2: risk detection does NOT depend on call-graph edges. The package
// has no call-graph input at all, so a destructive migration and a one-line
// auth-path change are flagged High purely from path/content — by construction.
func TestClassify_NoCallGraphDependency(t *testing.T) {
	mig := Classify(Input{Files: []string{"migrations/2026_drop_users.sql"}})
	authOneLiner := Classify(Input{Files: []string{"internal/auth/session.go"}})
	if mig.Band != RiskHigh || authOneLiner.Band != RiskHigh {
		t.Fatalf("destructive migration and auth one-liner must be High without any call-graph signal: %d / %d", mig.Band, authOneLiner.Band)
	}
}

// Unknown (unrecognized/config/non-Go) must ESCALATE — ask, default upward to
// standard, never silently tactical.
func TestClassify_UnknownEscalatesNeverTactical(t *testing.T) {
	rec := Classify(Input{Files: []string{"deploy/values.yaml", "scripts/run.py"}})
	if rec.Band != RiskUnknown {
		t.Fatalf("unrecognized files should be Unknown, got band %d", rec.Band)
	}
	if !rec.Ask {
		t.Errorf("blind floor must set Ask=true")
	}
	if rec.Mode == ModeTactical {
		t.Errorf("blind floor must NOT default tactical, got %q", rec.Mode)
	}
	if rec.Mode != ModeStandard || rec.Question == "" {
		t.Errorf("blind floor should default standard + pose a question, got mode=%q q=%q", rec.Mode, rec.Question)
	}
}

// The friction win: an ordinary recognized Go edit with no high-risk signal is
// Low → tactical.
func TestClassify_OrdinaryGoIsTactical(t *testing.T) {
	rec := Classify(Input{Files: []string{"internal/present/flow.go", "internal/present/flow_test.go"}})
	if rec.Band != RiskLow || rec.Mode != ModeTactical {
		t.Fatalf("ordinary Go edit should be Low/tactical, got band %d mode %q", rec.Band, rec.Mode)
	}
}

// Aggregation priority: High > Unknown > Elevated > Low — and an Unknown file
// is never masked by a lower recognized signal.
func TestClassify_AggregatePriority(t *testing.T) {
	// Low + Unknown → Unknown (the unknown file escalates, not masked by the Go file).
	if rec := Classify(Input{Files: []string{"internal/x/x.go", "deploy/values.yaml"}}); rec.Band != RiskUnknown {
		t.Errorf("Low+Unknown should escalate to Unknown, got %d", rec.Band)
	}
	// High + Unknown → High (already going deep; no need to ask).
	if rec := Classify(Input{Files: []string{"db/x.sql", "deploy/values.yaml"}}); rec.Band != RiskHigh || rec.Ask {
		t.Errorf("High+Unknown should be High with Ask=false, got band %d ask %v", rec.Band, rec.Ask)
	}
	// Elevated (governance medium) alone → standard.
	if rec := Classify(Input{Files: []string{"internal/x/x.go"}, Gov: GovFacts{Reversibility: "medium"}}); rec.Band != RiskElevated || rec.Mode != ModeStandard {
		t.Errorf("medium-reversibility governance should be Elevated/standard, got band %d mode %q", rec.Band, rec.Mode)
	}
}
