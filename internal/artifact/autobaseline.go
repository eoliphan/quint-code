package artifact

// Auto-baseline disposition floor (dec-20260606-9b4a4c52).
//
// This is the SAFETY core of the conservative auto-baseline gate: a pure,
// deterministic mapping from a decision's drift report to one of three
// dispositions. It keys off the kernel-computed SymbolVerdict (the symbol-level
// floor from dec-20260605-413e2600) and is conservative by construction —
// a governed-symbol change can NEVER map to AutoResolveSilent.
//
// What this file intentionally does NOT do (separate slices, by design):
//   - It does not mutate any baseline. Acting on AutoResolveSilent (re-baseline
//     + prior-snapshot retention for reversibility) is the mutation slice.
//   - It does not extend monitoring to a governed symbol's callee-closure. The
//     closure refinement (note-20260606-694247bb) is deferred while the code
//     graph is intra-package only (cross-package edges = P1b, not yet built),
//     so over the current graph it is a recall tweak, not a safety guarantee.
//
// The disposition floor here is the part that makes the operator's requirement
// true regardless of those slices: drift touching a governed symbol is never
// silently blessed.

// AutoBaselineAction is the deterministic disposition of one decision's drift
// under the conservative floor.
type AutoBaselineAction string

const (
	// AutoResolveSilent: every drift is provably additive (new symbols only).
	// Safe to re-baseline without operator review. By construction this is
	// never assigned to a governed-symbol modification/removal/file-deletion.
	AutoResolveSilent AutoBaselineAction = "auto_resolve_silent"
	// StageForConfirm: a governed symbol body was modified/removed (or a file
	// deleted). Staged into the confirm-digest — visible and reversible, never
	// silently baselined.
	StageForConfirm AutoBaselineAction = "stage_for_confirm"
	// SurfaceForReview: benignity could not be proven (no baseline, no symbol
	// evidence, unanalyzable change). Fail-safe to the operator.
	SurfaceForReview AutoBaselineAction = "surface_for_review"
)

// DriftDisposition pairs a drift report with its deterministic action + reason.
type DriftDisposition struct {
	Report DriftReport
	Action AutoBaselineAction
	Reason string
}

// verdictToAction is the table-driven core: each kernel verdict has exactly one
// disposition. Keeping it a table (not branching) makes the total mapping
// auditable at a glance and keeps the safety property obvious — governed_modified
// maps to StageForConfirm, never AutoResolveSilent.
var verdictToAction = map[string]struct {
	action AutoBaselineAction
	reason string
}{
	SymbolVerdictAdditiveOnly:     {AutoResolveSilent, "every drift is provably additive (new symbols only)"},
	SymbolVerdictGovernedModified: {StageForConfirm, "a governed symbol body was modified/removed or a file was deleted"},
	SymbolVerdictNeedsReview:      {SurfaceForReview, "benignity could not be proven (no symbol evidence / unanalyzable)"},
}

// ClassifyAutoBaseline maps each drift report to its deterministic disposition.
// Pure and side-effect-free: no store, no filesystem, no agent judgment.
func ClassifyAutoBaseline(reports []DriftReport) []DriftDisposition {
	out := make([]DriftDisposition, 0, len(reports))
	for _, r := range reports {
		out = append(out, classifyOne(r))
	}
	return out
}

// classifyOne applies the fail-safe precondition then the verdict table.
func classifyOne(r DriftReport) DriftDisposition {
	// Fail-safe precondition: a report with no baseline cannot be proven benign,
	// independent of any symbol verdict.
	if !r.HasBaseline {
		return DriftDisposition{Report: r, Action: SurfaceForReview, Reason: "no baseline to compare against — fail-safe to review"}
	}
	ar := verdictToAction[r.SymbolVerdict()]
	return DriftDisposition{Report: r, Action: ar.action, Reason: ar.reason}
}

// PartitionDispositions splits dispositions into the three action buckets in a
// single pass — the shape the confirm-digest renders from.
func PartitionDispositions(ds []DriftDisposition) (silent, confirm, review []DriftDisposition) {
	bucket := map[AutoBaselineAction]*[]DriftDisposition{
		AutoResolveSilent: &silent,
		StageForConfirm:   &confirm,
		SurfaceForReview:  &review,
	}
	for _, d := range ds {
		b := bucket[d.Action]
		*b = append(*b, d)
	}
	return silent, confirm, review
}
