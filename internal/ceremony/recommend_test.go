package ceremony

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecommend_ContentScanAndGovMerge(t *testing.T) {
	root := t.TempDir()
	// An innocuous-looking .go path whose CONTENT is destructive → must be High.
	if err := os.WriteFile(filepath.Join(root, "cleanup.go"), []byte("package x\n// DROP TABLE users\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := Recommend(root, []string{"cleanup.go"}, nil)
	if rec.Band != RiskHigh || rec.Mode != ModeDeep {
		t.Fatalf("destructive content on an innocuous path must be High/deep, got band %d mode %q", rec.Band, rec.Mode)
	}

	// Governance lookup bumps an ordinary recognized file to Elevated/standard.
	gov := func(f string) GovFacts { return GovFacts{Reversibility: "medium"} }
	if err := os.WriteFile(filepath.Join(root, "ordinary.go"), []byte("package x\nfunc A(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = Recommend(root, []string{"ordinary.go"}, gov)
	if rec.Band != RiskElevated || rec.Mode != ModeStandard {
		t.Fatalf("governed ordinary file should be Elevated/standard, got band %d mode %q", rec.Band, rec.Mode)
	}
	// Without governance, the same file is Low/tactical (the friction win).
	if r := Recommend(root, []string{"ordinary.go"}, nil); r.Band != RiskLow || r.Mode != ModeTactical {
		t.Fatalf("ungoverned ordinary file should be Low/tactical, got band %d mode %q", r.Band, r.Mode)
	}
}

func TestMergeGov_StrongestReversibilityWins(t *testing.T) {
	gov := func(f string) GovFacts {
		switch f {
		case "a":
			return GovFacts{Reversibility: "high"}
		case "b":
			return GovFacts{Reversibility: "low"} // strongest (riskiest)
		default:
			return GovFacts{}
		}
	}
	out := mergeGov([]string{"a", "b", "c"}, gov)
	if out.Reversibility != "low" {
		t.Fatalf("strongest (low) reversibility must win, got %q", out.Reversibility)
	}
}
