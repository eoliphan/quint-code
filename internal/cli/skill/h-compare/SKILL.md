---
name: h-compare
description: |
  Compare candidate variants under parity discipline and produce a Pareto front (NOT a scalar winner). Use when the operator signals comparison intent — "compare A and B", "trade-off between X and Y", "which is better", "Pareto for these options". Per FPF CMP-01/CMP-02: selection policy declared BEFORE scoring, parity_plan stated explicitly, dim-wise scoring (one evaluator per dimension applies one scale to all variants) prevents anchoring bias and produces fair comparison.
when_to_use: |
  A SolutionPortfolio exists with ≥2 variants and the operator wants to evaluate. For generating variants first use h-explore. For committing to the winner use h-decide (manual-only per Transformer Mandate).
argument-hint: "[portfolio-ref or comparison topic]"
allowed-tools: Agent mcp__haft__haft_problem mcp__haft__haft_solution mcp__haft__haft_query
---

# h-compare — Fair comparison with Pareto front

You are running the FPF compare workflow: characterize dimensions → declare parity plan → declare selection policy BEFORE scoring → dim-wise parallel scoring → compute non-dominated set → present Pareto front (NOT a scalar winner).

## Step 1 — Ensure portfolio exists

If no `portfolio_ref` is given:
- Look up via `mcp__haft__haft_query(action="status")` for active portfolios
- If only one active portfolio matches the problem, ask the operator to confirm

The kernel auto-detects when only one active portfolio exists, but explicit reference is safer.

## Step 2 — Characterize dimensions (if not already declared on the ProblemCard)

If the portfolio's problem has no dimensions, declare them now:

```
mcp__haft__haft_problem(
  action="characterize",
  problem_ref="<prob-...>",
  dimensions=[
    {
      "name": "latency_p95",
      "role": "target",         // constraint | target | observation
      "polarity": "lower_better",
      "scale_type": "ratio",
      "unit": "ms",
      "how_to_measure": "<single sentence>"
    },
    {
      "name": "memory_usage",
      "role": "constraint",     // hard limit — eliminates variant before scoring
      "polarity": "lower_better",
      "scale_type": "ratio",
      "unit": "MB",
      "how_to_measure": "<...>"
    },
    {
      "name": "ops_complexity",
      "role": "observation",    // Anti-Goodhart: watch but don't optimize
      "polarity": "lower_better",
      "scale_type": "ordinal",
      "how_to_measure": "<...>"
    }
  ]
)
```

Per FPF CHR-01: 1-3 targets max, plus constraints (hard limits) and observations (watch but do not optimize — Anti-Goodhart).

## Step 3 — Declare parity_plan (BEFORE scoring per FPF CMP-01)

```
parity_plan = {
  "baseline_set": ["<variant_id_1>", "<variant_id_2>", "<variant_id_3>"],
  "window": "<time/observation window scores are comparable in>",
  "budget": "<resource budget held equal across variants>",
  "missing_data_policy": "explicit_abstain | zero | exclude",
  "pinned_conditions": ["<must-hold condition>", ...]
}
```

For DEEP mode the kernel REQUIRES baseline_set, window, budget, missing_data_policy to be present. Standard mode accepts gaps with warnings.

## Step 4 — Declare selection_policy (BEFORE scoring per FPF CMP-02)

State the rule used to pick from the Pareto front BEFORE you see any scores. This is the Anti-Goodhart enforcement boundary.

Bad (post-hoc): "We picked X because it scored best on the dimensions we cared about."
Good (pre-declared): "Maximize latency_p95 subject to memory_usage < 200MB constraint; tie-break by ops_complexity."

Store the policy string for the kernel call.

## Step 5 — Score variants DIM-WISE in parallel (one Agent per dimension)

For M dimensions and N variants, spawn M Agent subagents IN THE SAME MESSAGE. Each subagent scores ALL variants on ONE dimension. This way the same evaluator applies the same scale, preventing the comparability problem you get if you instead spawned per-variant agents.

```
Agent(
  description="Score all variants on latency_p95",
  prompt="
    You are scoring dimension: latency_p95
    Unit: ms
    Polarity: lower_better
    How to measure: <from characterize step>

    Variants to score:
    1. <variant_id_1>: <description>
    2. <variant_id_2>: <description>
    3. <variant_id_3>: <description>

    Apply the SAME scoring approach to ALL variants. Use parity_plan:
    <parity_plan>

    Return EXACTLY:
    scores:
      <variant_id_1>: <numeric or ordinal value with unit>
      <variant_id_2>: <...>
      <variant_id_3>: <...>
    methodology: <one paragraph: how you measured, what you assumed,
                  any missing data treated per parity_plan policy>
    confidence: low | medium | high
  "
)
```

Spawn M of these in one message. After all return, assemble scores per variant.

## Step 6 — Call kernel with scores + Pareto computation

```
mcp__haft__haft_solution(
  action="compare",
  portfolio_ref="<sol-...>",
  dimensions=["latency_p95", "memory_usage", "ops_complexity"],
  scores={
    "<variant_id_1>": {"latency_p95": "...", "memory_usage": "...", ...},
    "<variant_id_2>": {...},
    "<variant_id_3>": {...}
  },
  parity_plan=<from Step 3>,
  policy_applied="<selection policy declared in Step 4 BEFORE scoring>",
  mode="<inherit from problem>"
)
```

The kernel computes the non-dominated set (Pareto front) from scores. Constraints eliminate variants that violate hard limits BEFORE Pareto computation.

## Step 7 — Present the Pareto front to operator

Surface:
- Non-dominated set (Pareto front) with their score profiles
- Dominated variants with explicit dominance explanation (which variants dominate them, on which dimensions)
- Pareto trade-offs: for non-dominated variants, what they each give up
- Recommendation (advisory only — the operator decides via /h-decide)
- Soft warnings from the kernel (read them — they may flag rigged comparison: missing parity, single-dimension, selected-not-in-non-dominated, etc.)

## Step 8 — Hand off to operator for decision

This skill STOPS at presentation. The binding choice is /h-decide (manual-only per Transformer Mandate). Recommend it as next step.

## What NOT to do

- Do not pre-collapse to a scalar winner. The Pareto front IS the result. The decide step picks from it.
- Do not score per-variant (one agent scores all dimensions of one variant) — different scorers + different scales = uncomparable scores. SCORE DIM-WISE.
- Do not declare selection policy AFTER seeing scores. That's post-hoc rationalization (FPF CMP-02 violation).
- Do not invent dimensions the operator hasn't agreed to.
- Do not skip parity_plan in deep mode — kernel rejects.
- Do not let a variant that violates a constraint dimension survive into the Pareto computation. Constraints eliminate first.
- Do not silently pick a dominated variant as "selected" — the operator must explicitly override with rationale if so.
- Do not commit the decision; /h-decide is the binding step and is manual-only.

## FPF spec references

- B.5.2 — Abductive loop (parent procedure)
- C.18 — NQD-CAL (open-ended search)
- C.18.1 — Scaling Law Lens
- A.17/A.18/A.19 — Characteristic + CSLC + CHR pipeline
- A.19.CN — Comparability/Normalization
- A.19.CPM — Comparison Mechanism (Pareto)
- G.0 — Frame Standard for selection
- G.9 — Parity / Benchmark Harness
- CMP-01 (parity), CMP-02 (selection policy up front), CMP-03 (Pareto front), CMP-06 (CL across options)
- CHR-01 (indicator role taxonomy), CHR-09 (parity plan)

Look up via `mcp__haft__haft_query(action="fpf", query="A.19.CPM")`.
