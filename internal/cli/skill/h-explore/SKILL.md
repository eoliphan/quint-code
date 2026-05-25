---
name: h-explore
description: |
  Generate distinct candidate solution variants with NQD diversity discipline. Use when the operator signals exploration intent — "what are our options", "brainstorm approaches", "how could we", "different ways to do X". Each variant must differ in KIND not degree (FPF EXP-08), carry an explicit weakest_link (FPF EXP-04), and optionally mark stepping-stones (FPF EXP-05) that open future search space.
when_to_use: |
  Operator already has a framed problem (or you'll frame one inline) and wants candidate solutions before choosing. NOT for comparing existing options (that's h-compare). NOT for hypothesis testing on a failure (that's h-diagnose).
argument-hint: "[exploration topic or problem reference]"
allowed-tools: Agent mcp__haft__haft_problem mcp__haft__haft_solution mcp__haft__haft_query
---

# h-explore — Generate distinct variants with NQD discipline

You are running the FPF exploration workflow. The point is rivalrous candidate generation: 3-5 variants that differ in KIND, each with a named weakest_link, and at least one stepping-stone (or an explicit rationale for not having one).

## Step 1 — Ensure the problem is framed

If no `problem_ref` is in the operator's request:
- If they describe a problem inline: call `mcp__haft__haft_problem(action="frame", ...)` first per the h-frame procedure
- If a recent problem exists: ask the operator which one to explore against, or look it up via `mcp__haft__haft_query(action="status")`

Without a problem reference the exploration floats.

## Step 2 — Generate variants in parallel for diversity (optional but recommended)

For genuine diversity force the candidates to come from distinct directions. Spawn 3-5 Agent subagents IN THE SAME MESSAGE, each instructed to produce ONE variant from a different conceptual direction:

```
Agent(
  description="Generate variant #1 (data-flow restructure direction)",
  prompt="
    Problem: <stabilized signal + acceptance from the ProblemCard>

    Your direction (assigned, distinct from siblings): <data flow restructure>
    Other agents are producing variants from other directions
    (caching, batching, infrastructure swap, etc.). Do NOT step on their
    territory — give the operator the variant that comes from YOUR
    direction.

    Return EXACTLY:
    title: <short variant name>
    description: <2-3 sentence sketch of the approach>
    novelty_marker: <what makes this different from typical AI suggestions>
    weakest_link: <what bounds this option's quality if pursued — the
      Achilles' heel, NOT the title repeated>
    stepping_stone: true | false
    stepping_stone_basis: <if true, what future search space this opens>
    risks: [<risk>, ...]
    strengths: [<strength>, ...]
  "
)
```

Parallel directions to consider (pick 3-5 that fit the problem):
- Data-flow restructure (avoid the need for the current operation)
- Algorithmic alternative (same operation, different algorithm)
- Infrastructure swap (different runtime/service/library)
- Caching/batching/queuing (smooth load patterns)
- Architectural extraction (move responsibility to different layer)
- Workflow restructure (change when/how operation is triggered)
- Stepping stone (suboptimal now but opens novel future path)

## Step 3 — Alternative: serial generation (when parallel overkill)

For lightweight exploration (tactical mode, <5 minute task) you can generate variants directly without subagents. Still enforce:
- ≥2 variants (kernel rejects fewer)
- Each variant has weakest_link (kernel rejects empty)
- Each variant has novelty_marker (kernel rejects empty)
- Variants differ in KIND, not degree (your judgment; kernel emits soft warning if titles look similar)

## Step 4 — Call kernel

```
mcp__haft__haft_solution(
  action="explore",
  problem_ref="<prob-...>",
  variants=[
    {
      "title": "<variant 1 name>",
      "description": "<approach>",
      "novelty_marker": "<distinct from siblings>",
      "weakest_link": "<what bounds quality>",
      "stepping_stone": false,
      "risks": ["<risk>"],
      "strengths": ["<strength>"]
    },
    // ... 3-5 variants
  ],
  no_stepping_stone_rationale="<required if no variant has stepping_stone=true>",
  mode="<inherit from problem; tactical OK for low-stakes>"
)
```

The kernel returns the SolutionPortfolio ID. Soft warnings may flag disguised duplicates (titles too similar), missing parity_rules for 3+ variants, or weakest_links that just repeat titles — read and self-correct if needed.

## Step 5 — Present to operator

Surface:
- Each variant with its weakest_link and novelty_marker
- Identify any that look weak on the surface but might be stepping stones
- Recommend next step:
  - `/h-compare` to evaluate variants against declared dimensions and pick a Pareto front
  - More exploration if variants converge too tightly (operator may want a wider net)

## What NOT to do

- Do not produce 2-3 variants of the same approach (cache LRU vs cache LFU vs cache TTL — all caching). Force at least one out-of-kind alternative.
- Do not name weakest_link with the variant title verbatim. The weakest_link is the FAILURE MODE, not the feature description.
- Do not stop at one variant; if the operator can only think of one, prompt them to find a true alternative or document explicitly that no rival exists.
- Do not skip novelty_marker — without it the agent (or future operator) cannot tell why this variant was worth exploring.
- Do not record stepping-stone variants without `stepping_stone_basis` — bare claim is theatre.
- Do not commit to a chosen variant in this skill; `/h-decide` is where commitments are recorded and is manual-only per Transformer Mandate.

## FPF spec references

- B.5.2 — Abductive loop (parent procedure)
- B.5.2.1 — Creative Abduction with NQD (forced diversity)
- C.18 — Open-ended Search Calculus (NQD-CAL)
- EXP-01 (abductive loop), EXP-04 (WLNK per variant), EXP-05 (stepping stones), EXP-07 (Pareto/portfolio thinking), EXP-08 (NQD novelty)

Look up via `mcp__haft__haft_query(action="fpf", query="C.18")`.
