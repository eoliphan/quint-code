---
name: h-abduct
description: |
  INTERNAL SUBROUTINE — used by h-diagnose for parallel rival-hypothesis generation. Manual invocation possible but the right user-facing entry point is almost always h-diagnose (which uses h-abduct internally with parallel testing). Generates ≥3 typed rival explanations for an observed signal per FPF B.5.2 abductive cycle. Do not auto-select this skill — when failure investigation is needed, select h-diagnose; when problem framing is needed, select h-frame.
when_to_use: |
  Internal subroutine only. Use h-diagnose for failures; use h-frame for problem framing.
argument-hint: "[abductive prompt: signal or observation requiring explanation]"
disable-model-invocation: true
allowed-tools: mcp__haft__haft_problem mcp__haft__haft_solution mcp__haft__haft_query
---

# h-abduct — Pure abductive four-step (subroutine)

You are running the B.5.2 abductive four-step micro-cycle: frame the prompt → generate candidates → apply plausibility filters → select prime hypothesis. Per CC-B.5.2-2 rivals stay visible; per CC-B.5.2-1 every run starts from a declared typed AbductivePrompt.

Explicit-only — `disable-model-invocation: true`. The skill is invoked manually when the operator wants the discipline without the parallel-test overhead of h-diagnose.

## Step 1 — Frame the typed AbductivePrompt

State the initiating prompt precisely. Per B.5.2.0 the typed species are:
- **AnomalyStatement** — "<observed pattern> doesn't fit existing explanation"
- **ProblemCuePrompt** — "<sketch of a problem we don't yet know how to solve>"
- **OpportunityCuePrompt** — "<sketch of an opportunity worth exploring>"
- **ProbeCuePrompt** — "<we want to probe X to learn Y>"

Capture the prompt in writing before generating candidates — otherwise the abduction floats.

## Step 2 — Generate ≥3 candidate hypotheses

Per CC-B.5.2-2 at least one rival required. Aim for 3-5 candidates that differ in kind, not degree. Each candidate states:
- Claim (one sentence)
- Why-plausible (one sentence)
- Discriminating probe — what would falsify or confirm

If all candidates trend in one direction (all blame the same component), force a rival from a structurally different direction (data flow, race condition, environmental, configuration, infrastructure, etc.).

## Step 3 — Apply plausibility filters (≥2 declared)

For each candidate, evaluate against explicit filters. Typical:
- **Parsimony** — does the candidate introduce only the additional structure the prompt requires?
- **Explanatory reach** — how much of the prompt does the candidate actually account for?
- **Consistency** — does the candidate avoid collision with already-trusted pillars / mechanisms / scope declarations?
- **Falsifiability / probeability** — does the candidate create a path for deduction, testing, contrast, or evidence acquisition?
- **Scope fit** — is the candidate framed for the declared prompt scope rather than for an inflated or shifted target?

Pick at least two filters and apply them. Record the rationale.

## Step 4 — Select prime hypothesis + publish

Pick one candidate for downstream work. Per CC-B.5.2-1 publish:
- The selected prime hypothesis as a new `U.Episteme` at `AssuranceLevel:L0`
- Linked back to the original AbductivePrompt
- With selection rationale referencing the applied filters

In haft kernel this translates to either:

```
mcp__haft__haft_solution(
  action="explore",
  problem_ref="<prob-... if existing or framed>",
  variants=[
    // 3-5 variants, prime hypothesis marked in description
  ],
  no_stepping_stone_rationale="<if no stepping stone among candidates>"
)
```

OR (lighter — just record the abductive cycle without portfolio):

```
mcp__haft__haft_problem(
  action="frame",
  signal="<prime hypothesis statement>",
  problem_type="diagnosis",
  acceptance="prime hypothesis confirmed or falsified by <discriminating probe>",
  mode="tactical"
)
```

The lighter path is for exploratory abduction where the operator doesn't yet want full portfolio comparison. The heavier path is when subsequent /h-compare is expected.

## Step 5 — Hand off

Surface to operator:
- Prime hypothesis
- Rivals (per CC-B.5.2-2 stay visible)
- Filter rationale
- Recommended next step:
  - `/h-verify` if the prime hypothesis is verifiable against existing evidence
  - `/h-explore` if rivals deserve deeper variant exploration before commitment
  - `/h-decide` (manual-only) if the prime is binding and ready to commit

## What NOT to do

- DO NOT generate only one candidate — rivals are required (CC-B.5.2-2).
- DO NOT skip plausibility filters — at least two declared, per CC-B.5.2.
- DO NOT collapse to a "best guess" without documenting why other candidates were rejected.
- DO NOT publish prime as L1+ — abductive output is L0 (unsubstantiated until tested).
- DO NOT auto-invoke from anywhere except explicit operator command or h-diagnose subagent spawn.

## FPF spec references

- B.5.2 — Abductive Loop (parent procedure)
- B.5.2.0 — U.AbductivePrompt (typed entry surfaces)
- B.5.2.1 — Creative Abduction with NQD
- CC-B.5.2-1, CC-B.5.2-2 — Conformance: typed prompt + visible rivals
- EXP-01 (abductive loop), EXP-04 (WLNK per variant)

Look up via `mcp__haft__haft_query(action="fpf", query="B.5.2")`.
