---
name: h-decide
description: |
  Record a binding DecisionRecord with full FPF discipline (E.9 DRR). MANUAL ONLY — the operator must explicitly type /h-decide. This skill must NOT be auto-invoked: per FPF X-TRANSFORMER (Transformer Mandate), the human principal records binding choices, not the agent.
when_to_use: |
  Operator has finished framing/exploring/comparing and is committing to a chosen variant. For tactical reversible changes (<2-week blast radius), pass _mode="tactical" with explicit _skips and _skip_reason. For irreversible/security/cross-team changes, pass _mode="deep" — all DRR fields required, no skips accepted.
argument-hint: "[selected variant title or short choice text]"
disable-model-invocation: true
allowed-tools: mcp__haft__haft_decision mcp__haft__haft_query
---

# h-decide — Record a Decision (manual only, Transformer Mandate)

You are recording a `DecisionRecord` via `mcp__haft__haft_decision(action="decide", ...)`. The operator invoked this manually (`disable-model-invocation: true` enforces that structurally per FPF X-TRANSFORMER).

This is the binding moment. The DecisionRecord becomes the authoritative
choice that downstream commissions, runtime runs, and verification cycles
reference. Take it seriously.

## Required arguments for standard mode (default)

Call `mcp__haft__haft_decision(action="decide", ...)` with at minimum:

- `problem_ref` (or `problem_refs` for multi-problem) — the ProblemCard(s) this addresses
- `portfolio_ref` — the SolutionPortfolio whose variants you compared
- `selected_title` — the chosen variant title (must match a variant in the portfolio)
- `why_selected` — rationale for the choice
- `selection_policy` — the explicit policy used to choose (FPF CMP-02: declared BEFORE scoring, Anti-Goodhart)
- `weakest_link` — what most plausibly breaks this choice (FPF X-WLNK)
- `counterargument` — the strongest argument AGAINST this decision (FPF DEC-08: self-deception check)
- `why_not_others` — `[{variant: "...", reason: "..."}]` for at least one rejected alternative
- `rollback` — `{triggers: [...], steps: [...], blast_radius: "..."}` — at least one trigger required
- `predictions` — `[{claim, observable, threshold, verify_after?}]` — falsifiable claims the verify loop will check
- `invariants` — what MUST hold at all times
- `affected_files` — scope of this decision (governance + drift tracking)
- `valid_until` — RFC3339 date when this decision should be re-evaluated

For deep mode (`mode: "deep"`), also provide rich `evidence_requirements` and `refresh_triggers`.

## Tactical mode — explicit skip mechanism

If this is a reversible change with <2-week blast radius, switch to tactical mode and acknowledge skipped fields explicitly:

```json
{
  "action": "decide",
  "mode": "tactical",
  "selected_title": "...",
  "why_selected": "...",
  "_skips": ["selection_policy", "counterargument", "rollback"],
  "_skip_reason": "5-line config change reversible by file revert; full DRR ceremony exceeds blast radius"
}
```

The kernel rejects `_skips` in standard/deep mode and requires `_skip_reason` whenever `_skips` is non-empty. Skip field names must be in the allowlist (selection_policy, counterargument, weakest_link, why_not_others, rollback, predictions, invariants, evidence_requirements, refresh_triggers, affected_files, why_selected). `selected_title` cannot be skipped — a decision without identity has no substrate.

## When the kernel returns an error

The MCP server validates and returns structured errors of the form:

```
FPF discipline violation: decision in <mode> mode is incomplete.

Missing required fields:
- <field> — <hint>

How to proceed:
- Option 1: Provide the missing fields and retry the call.
- Option 2: ... (tactical mode skip option)

References:
- FPF E.9 — Design Rationale Record minimum kernel
- ...
```

Read the response, decide which option fits the change's actual blast radius, and retry. Do NOT bypass by silently omitting `_skip_reason` or fabricating fields.

## After successful decide

The kernel returns the new decision ID (e.g. `dec-20260525-...`). Suggested next steps for the operator:

- `mcp__haft__haft_decision(action="baseline", decision_ref="dec-...")` — snapshot affected files for drift detection
- For verification later: `/h-verify` (invokes haft_decision measure + evidence)
- For autonomous execution: `/h-commission` (creates WorkCommission within autonomy envelope)

## What NOT to do

- Do not invoke this skill from another skill — operator must explicitly type `/h-decide` (structural enforcement via `disable-model-invocation: true`).
- Do not record decisions on behalf of the operator without their explicit /h-decide invocation.
- Do not combine multiple distinct decisions in one call — each binding choice gets its own DRR.
- Do not skip fields silently by omitting them — use the explicit `_skips` + `_skip_reason` mechanism so the bypass is auditable.
- Do not fabricate `verify_after` dates to bypass prediction validation; if you don't know when to verify, omit `verify_after` (kernel accepts predictions without it; some FPF discipline still lost).
- Do not record a decision that contradicts an active prior decision without superseding it first via `mcp__haft__haft_refresh(action="supersede", ...)`.

## FPF spec references

- E.9 — Design Rationale Record method
- DEC-01 — Decision record structure (problem frame + decision + rationale + consequences)
- DEC-04 — Invariants
- DEC-05 — Rollback (triggers + steps + blast radius + timeline)
- DEC-06 — Predictions (falsifiable claims with verify_after)
- DEC-08 — Counterargument (self-deception check)
- X-TRANSFORMER — Transformer Mandate (human principal decides)
- CMP-02 — Selection policy declared BEFORE scoring (Anti-Goodhart)
- X-WLNK — Weakest link per claim

Look up full pattern text via `mcp__haft__haft_query(action="fpf", query="E.9")`.
