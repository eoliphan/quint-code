---
name: h-status
description: |
  Project FPF state dashboard. Use when the operator wants situational awareness — "where are we", "what's stale", "what's pending", "project status", "show me the FPF state", "what needs attention". Read-only surface that consolidates active problems, decisions, refresh-due artifacts, work commissions, recent notes, and module coverage.
when_to_use: |
  Operator wants to see the FPF state at a glance. Cheap and read-only — no commitments. For deep history use h-search; for verifying a single decision use h-verify; for managing commission lifecycle use h-commission.
argument-hint: "[optional: context name to filter]"
allowed-tools: mcp__haft__haft_query
---

# h-status — Project FPF state dashboard

You are surfacing the current FPF state via `mcp__haft__haft_query(action="status")`. Read-only — no kernel writes.

## Step 1 — Call the kernel

```
mcp__haft__haft_query(
  action="status",
  context="<optional context filter>"
)
```

Or for richer visualization (terminal-friendly board view):

```
mcp__haft__haft_query(action="board")
```

## Step 2 — Interpret the response

The status payload contains:

- **Shipped / Healthy** — recent decisions that are still active and within valid_until window
- **Pending** — decisions awaiting implementation, baseline, or measure
- **Unassessed** — decisions without recent measurement
- **Refresh Due** — decisions/evidence past valid_until, drift detected, or R_eff degraded; also epistemic debt budget if exceeded
- **WorkCommissions Need Attention** — blocked, stale, or expired commissions
- **Backlog** — active problems without committed decisions
- **Addressed** — recently closed problems with their decisions
- **Recent Notes** — last 5 micro-decisions
- **Module Coverage** — per-module decision density (governed / blind / stale)

The response also includes a navigation strip with available next actions (e.g., `/h-refresh`).

## Step 3 — Present to operator

Surface the response as-is — the kernel formats it already. Highlight items that need operator attention:

- Refresh-due items → recommend `/h-verify` for the specific decisions
- Blocked work commissions → recommend `/h-commission` with `action=show` and operator review
- Epistemic debt budget exceeded → recommend running `/h-verify` on the highest-debt decisions
- Blind modules (no decisions) → recommend `/h-frame` if upcoming work targets that module
- Stale decisions → recommend `/h-refresh` action=waive (with new evidence) or `action=supersede` (with replacement decision)

## Step 4 — Optional: cross-link to related decisions when context given

If the operator's context mentions a specific file or module, also call:

```
mcp__haft__haft_query(action="related", file="<file path>")
```

To surface decisions whose affected_files include that path.

## What NOT to do

- Do NOT auto-fix anything from this skill. h-status is read-only.
- Do NOT silently filter out refresh-due items — the operator needs to see epistemic debt to triage.
- Do NOT use this skill for full-text search across decisions — that's h-search via `mcp__haft__haft_query(action="search", query="...")`.
- Do NOT call this skill on every turn. Auto-trigger is fine when the operator explicitly asks; constant polling pollutes context with stale state.

## FPF spec references

- B.3.4 — Evidence Decay & Epistemic Debt (drives refresh-due classification)
- F.10 — Three Ladders: Evidence / Standard / Requirement status
- VER-02 — Decay (valid_until expiry semantics)
- X-WLNK — Weakest-link aggregation for R_eff degradation surfacing

Look up via `mcp__haft__haft_query(action="fpf", query="B.3.4")`.
