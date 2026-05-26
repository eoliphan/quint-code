---
name: h-onboard
description: |
  First-frame ceremony for a project new to haft. Use when the operator opens a repository without a `.haft/` directory or with empty SpecSections — "onboard this project to FPF", "set up haft here", "first time using haft in this repo". Walks the operator through the minimum FPF carriers (target system, enabling system, term map) without overwhelming first-time UX.
when_to_use: |
  Repo has no `.haft/` directory, or `.haft/` exists but no active SpecSections. For ongoing project work use h-status. For framing a specific problem use h-frame.
argument-hint: "[optional: short project description]"
allowed-tools: Bash Read mcp__haft__haft_problem mcp__haft__haft_query
---

# h-onboard — First-impression FPF setup

You are walking the operator through the minimum first-frame ceremony per FPF reasoner critique (2026-05-25 — current `/h-onboard` ceremony was found too heavy). Five questions; draft carriers only; no auto-activation without operator confirmation.

## Step 1 — Check current state

```
bash: haft doctor
```

Then `mcp__haft__haft_query(action="status")` to see if `.haft/` exists with any SpecSections.

If readiness is `ready` → operator is already onboarded; just run `/h-status` instead.
If readiness is `needs_init` → recommend `haft init` as a CLI command and stop (init is not a skill — it installs configuration).
If readiness is `needs_onboard` → continue with the ceremony below.

## Step 2 — Five minimum-ceremony questions

Ask the operator (sequentially OR collect together if they prefer):

1. **Target system**: What MUST work for this project to be considered successful? (One sentence describing the value-delivering system.)
2. **Environment change**: What changes in the world if it works? (Effect on users / business / system being studied.)
3. **Boundary**: What's in-scope and what's out-of-scope? (One sentence each, hard line.)
4. **First acceptance signal**: What's one observable condition that signals "minimum viable success"?
5. **Agent permission default**: For autonomous agent work in this project, default to: read-only / suggest-only / prepare-only / execute-with-confirmation? (Affects future commission envelopes.)

Don't ask all five rapid-fire if the operator's question was casual. Calibrate ceremony to the operator's signal — if they just said "set up haft" without context, ask the first two; let the rest emerge as work needs them.

## Step 3 — Frame an onboarding ProblemCard (draft)

After collecting answers, frame a first problem record:

```
mcp__haft__haft_problem(
  action="frame",
  problem_type="synthesis",
  title="Project onboarding: <repo name>",
  signal="Project lacks active FPF spec carriers; first principal-led engineering loop needs to be established",
  acceptance="<operator's first acceptance signal>",
  constraints=["<operator's out-of-scope item>"],
  blast_radius="<project scope>",
  reversibility="high",
  mode="tactical"
)
```

This is the bootstrap problem. Tactical mode because onboarding is reversible (delete `.haft/` and start over). The problem becomes the anchor for the first real decision.

## Step 4 — Recommend the operator-side carriers

The kernel's spec carriers (TargetSystemSpec, EnablingSystemSpec, TermMap) need operator authorship — agent should NOT write them autonomously. After the ProblemCard is recorded, surface:

- "Run `/h-onboard` is not enough — you'll want to draft `.haft/specs/target-system.md` (one section per use case), `.haft/specs/enabling-system.md` (the creator/governor side), and `.haft/specs/term-map.md` (one entry per load-bearing project term)."
- "Once those exist, `/h-status` will report readiness=ready."

DO NOT auto-write spec section files. The operator's first authorship pass is part of the onboarding — they need to feel it.

## Step 5 — Set expectations

Make explicit to the operator:

- "Haft is a governance substrate, not a coding agent. Your Claude Code / Codex still does the coding. Haft persists the reasoning that bounds and reviews that coding."
- "You can use /h-frame, /h-explore, /h-compare any time. Binding actions (/h-decide, /h-commission) are manual-only per Transformer Mandate."
- "Run /h-status whenever you want a project FPF state dashboard."

## What NOT to do

- DO NOT ask all five questions rapid-fire if the operator gave a casual signal. Calibrate ceremony to intent strength.
- DO NOT auto-author `.haft/specs/*.md` files. Those are operator authorship.
- DO NOT commit decisions during onboarding — onboarding produces ONE ProblemCard (in tactical mode), not a DecisionRecord.
- DO NOT skip the first acceptance signal — without it the project has no observable success criterion to bind future decisions to.
- DO NOT recommend `haft agent` or `haft desktop` — those surfaces do not exist. The agent layer is the operator's Claude Code / Codex.

## FPF spec references

- FRAME-01 through FRAME-09 — problem framing micro-patterns
- F.17 — Unified Term Sheet (the term-map carrier)
- A.1 — Holonic Foundation (target system vs enabling system distinction)
- E.14 — Human-Centric Working-Model

Look up via `mcp__haft__haft_query(action="fpf", query="A.1")`.
