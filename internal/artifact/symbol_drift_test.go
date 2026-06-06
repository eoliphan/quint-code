package artifact

import "testing"

// TestSymbolVerdict pins the deterministic session-start triage floor:
// a decision is auto-baselineable ONLY when every modified file is provably
// additive; any governed-symbol modification/removal, deleted file, or
// unanalyzable change must route to the operator (dec-20260605-413e2600
// invariants 1 and 5).
func TestSymbolVerdict(t *testing.T) {
	added := func(name string) SymbolDriftItem {
		return SymbolDriftItem{SymbolName: name, SymbolKind: "func", Status: "added"}
	}
	modified := func(name string) SymbolDriftItem {
		return SymbolDriftItem{SymbolName: name, SymbolKind: "func", Status: "modified"}
	}
	removed := func(name string) SymbolDriftItem {
		return SymbolDriftItem{SymbolName: name, SymbolKind: "func", Status: "removed"}
	}

	cases := []struct {
		name  string
		files []DriftItem
		want  string
	}{
		{
			name:  "added file only is additive",
			files: []DriftItem{{Path: "new.go", Status: DriftAdded}},
			want:  SymbolVerdictAdditiveOnly,
		},
		{
			name:  "modified file with only added symbols is additive",
			files: []DriftItem{{Path: "a.go", Status: DriftModified, Symbols: []SymbolDriftItem{added("Foo"), added("Bar")}}},
			want:  SymbolVerdictAdditiveOnly,
		},
		{
			name:  "a modified symbol body forces operator review",
			files: []DriftItem{{Path: "a.go", Status: DriftModified, Symbols: []SymbolDriftItem{added("Foo"), modified("Bar")}}},
			want:  SymbolVerdictGovernedModified,
		},
		{
			name:  "a removed symbol forces operator review",
			files: []DriftItem{{Path: "a.go", Status: DriftModified, Symbols: []SymbolDriftItem{removed("Bar")}}},
			want:  SymbolVerdictGovernedModified,
		},
		{
			name:  "a deleted file forces operator review",
			files: []DriftItem{{Path: "gone.go", Status: DriftMissing}},
			want:  SymbolVerdictGovernedModified,
		},
		{
			name:  "modified file with no symbol evidence fails safe to review",
			files: []DriftItem{{Path: "a.go", Status: DriftModified}},
			want:  SymbolVerdictNeedsReview,
		},
		{
			name:  "no-baseline file fails safe to review",
			files: []DriftItem{{Path: "a.go", Status: DriftNoBaseline}},
			want:  SymbolVerdictNeedsReview,
		},
		{
			name: "any governed modification dominates additive siblings",
			files: []DriftItem{
				{Path: "a.go", Status: DriftAdded},
				{Path: "b.go", Status: DriftModified, Symbols: []SymbolDriftItem{added("New")}},
				{Path: "c.go", Status: DriftModified, Symbols: []SymbolDriftItem{modified("Old")}},
			},
			want: SymbolVerdictGovernedModified,
		},
		{
			name: "one unanalyzable file taints an otherwise-additive decision",
			files: []DriftItem{
				{Path: "a.go", Status: DriftModified, Symbols: []SymbolDriftItem{added("New")}},
				{Path: "b.bin", Status: DriftModified}, // no symbol evidence
			},
			want: SymbolVerdictNeedsReview,
		},
		{
			name:  "empty report is review, never additive",
			files: nil,
			want:  SymbolVerdictNeedsReview,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := DriftReport{Files: tc.files}
			if got := report.SymbolVerdict(); got != tc.want {
				t.Fatalf("SymbolVerdict() = %q, want %q", got, tc.want)
			}
		})
	}
}
