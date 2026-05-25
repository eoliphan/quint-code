---
name: h-diagnose
description: |
  Diagnose a failure with parallel hypothesis testing. Use when the operator reports something broken and the root cause is unclear — "tests fail", "X doesn't work", "why is Y happening", "investigate this", "what's causing", "the bug is unclear". Implements FPF B.4.1 (Observe → Notice → Stabilize → Route) + B.5.2 (Abductive four-step) with multiple parallel read-only subagents testing distinct hypotheses, then ranks by evidence weight and keeps rivals visible (CC-B.5.2-2).
when_to_use: |
  Operator describes a failure mode with unclear root cause. NOT for feature requests (use h-frame with type=synthesis), NOT for performance optimization with known-bottleneck (use h-frame with type=optimization), NOT for already-formed hypothesis verification (use h-verify on a recorded decision).
argument-hint: "[symptom or failure description]"
allowed-tools: Bash Read Grep Glob Agent mcp__haft__haft_problem mcp__haft__haft_solution mcp__haft__haft_query
---

# h-diagnose — Diagnosis with parallel hypothesis testing

You are running the FPF diagnosis workflow: B.4.1 stabilize → B.5.2 abductive four-step → ranked verdict with rivals visible. Parallel subagents test distinct hypotheses in isolated contexts to prevent anchoring bias.

## Step 1 — Stabilize the signal (B.4.1)

Read the symptom carefully. Do not jump to causes. Compress to one sentence:
- What was observed (not assumed)
- What pattern the failures share
- What stays out of scope for now

Bad: "the cache is broken"
Good: "cache invalidation tests in schema-migration suite fail intermittently since 2026-05-22"

If the signal contains umbrella terms ("slow", "broken", "weird") run inline precision restoration per FPF A.6.P/Q/A before proceeding.

## Step 2 — Frame as diagnosis problem

Call:
```
mcp__haft__haft_problem(
  action="frame",
  problem_type="diagnosis",
  title="<short diagnosis title>",
  signal="<stabilized observation>",
  acceptance="<what would constitute root-cause identified + verified>",
  constraints=["<read-only investigation only — no fixes>"]
)
```

Capture the returned `prob-...` ID for downstream subagent calls.

## Step 3 — Generate ≥3 hypotheses (B.5.2 abductive)

Produce 3-5 candidate root causes. They MUST differ in kind, not degree (FPF EXP-08). If you have three cache hypotheses, force at least one that ISN'T cache-related (data flow, race condition, environmental, etc.).

For each hypothesis state:
- One sentence claim
- Why plausible (one sentence)
- A discriminating probe — what evidence would falsify or confirm

## Step 4 — Test in PARALLEL (one Agent call per hypothesis)

For N hypotheses spawn N Agent subagents IN THE SAME MESSAGE (this is the structural parallelism that prevents anchoring). Each subagent runs in isolated context.

For each hypothesis, spawn:
```
Agent(
  description="Test diagnosis hypothesis #<n>",
  prompt="
    You are testing ONE hypothesis for the diagnosis of:
    <stabilized signal>

    Your hypothesis: <hypothesis claim>
    Why-plausible: <one-line>
    Discriminating probe: <one-line probe>

    Allowed tools: Read, Grep, Glob, Bash (test runners only — no edits).
    NOT allowed: Write, Edit, NotebookEdit. You are READ-ONLY.

    Return EXACTLY this structure:
    evidence_for:
    - <observation supporting hypothesis>
    evidence_against:
    - <observation contradicting hypothesis>
    inconclusive:
    - <observation that neither supports nor refutes>
    verdict: supported | refuted | inconclusive
    confidence: low | medium | high
    notes: <one-paragraph summary>
  "
)
```

All N Agent calls go in the SAME assistant message — Claude Code executes them in parallel. Wait for all results before Step 5.

## Step 5 — Rank by evidence weight (keep rivals visible per CC-B.5.2-2)

After collecting subagent results, rank hypotheses by:
1. Verdict (supported > inconclusive > refuted)
2. Confidence (high > medium > low)
3. Evidence_for count minus evidence_against count

Identify the **prime hypothesis** (top-ranked) but DO NOT discard the others. Per FPF CC-B.5.2-2 rivals stay visible in the artifact graph for replay and fallback.

## Step 6 — Record as SolutionPortfolio variants

Call:
```
mcp__haft__haft_solution(
  action="explore",
  problem_ref="<prob-... from Step 2>",
  variants=[
    {
      "title": "<hypothesis 1 short name>",
      "description": "<claim + evidence summary>",
      "weakest_link": "<what would invalidate this even though current evidence supports>",
      "novelty_marker": "<what makes this hypothesis distinct from the others>",
      "stepping_stone": false
    },
    // ... one entry per hypothesis, with the prime one marked as such in description
  ]
)
```

The kernel returns a SolutionPortfolio ID. Each hypothesis becomes a variant with its evidence trail attached. The operator can verify, supersede, or commission action on any of them.

## Step 7 — Present to operator

Surface to operator:
- Prime hypothesis with evidence summary
- Rivals visible (per CC-B.5.2-2) with their evidence
- Specific next-action recommendation:
  - If prime is `supported high-confidence` → recommend `/h-decide` to commit to the fix path
  - If prime is `supported medium` → recommend more targeted probes before deciding
  - If all hypotheses `inconclusive` → recommend widening exploration (different hypothesis space) via `/h-explore`
  - If all `refuted` → recommend re-framing the problem via `/h-frame` (signal may be misobserved)

Operator decides next step. Do NOT auto-fix; diagnosis stops at root-cause identification.

## What NOT to do

- Do not test hypotheses sequentially in one context — anchoring bias degrades evidence quality.
- Do not collapse to one hypothesis prematurely; keep rivals visible per CC-B.5.2-2.
- Do not let subagents WRITE or EDIT — they are read-only investigators.
- Do not skip the framing step — without a problem record the diagnosis floats.
- Do not run the fix yourself; this skill stops at evidence + recommendation.
- Do not generate hypotheses that differ only in degree (all cache-related with slight variations).
- Do not spawn fewer than 3 hypothesis subagents unless the search space is genuinely binary.

## FPF spec references

- B.4.1 — Observe → Notice → Stabilize → Route (pre-articulation)
- B.5.2 — Abductive four-step micro-cycle
- B.5.2.1 — Creative Abduction with NQD (forced diversity)
- CC-B.5.2-1, CC-B.5.2-2 — Conformance: rival candidates visible
- EXP-01 (abductive loop), EXP-04 (WLNK per variant), EXP-08 (NQD novelty marker)

Look up via `mcp__haft__haft_query(action="fpf", query="B.5.2")`.
