---
name: h-boundary-unpack
description: |
  Decompose a boundary statement into L/A/D/E quadrants per FPF A.6.B (Boundary Norm Square). Subroutine skill — invoked when the operator works with API/contract/service statements that mix Laws (definitions), Admissibility (gates), Deontics (commitments), and Evidence (claims/observations) in one prose blob. Splits the statement into typed atomic claims with quadrant labels so each part can be reviewed and routed appropriately.
when_to_use: |
  Operator writes or reviews an API contract / service definition / policy statement / compliance text and the prose mixes statement types. NOT for general code review (use Claude Code review). NOT for problem framing (use h-frame).
argument-hint: "[boundary statement text or path to spec section]"
disable-model-invocation: true
allowed-tools: Read mcp__haft__haft_query
---

# h-boundary-unpack — A.6.B L/A/D/E routing (subroutine)

You are decomposing a boundary statement into FPF A.6.B's four quadrants. Per A.6.B, every load-bearing statement at a boundary belongs to exactly one of:

- **L — Laws / Definitions** — what IS / what THIS MEANS. Definitional. Not actionable on its own.
- **A — Admissibility / Gates** — what is ALLOWED / REJECTED under what conditions. Gate semantics.
- **D — Deontics / Commitments** — what AGENTS PROMISE / OBLIGE / ARE PERMITTED to do. Speech-act semantics.
- **E — Work-Effects / Evidence** — what HAPPENED / WAS OBSERVED. Run-time evidence; not a contract.

Mixing quadrants in one sentence is the root of contract-soup bugs. The unpack discipline forces atomic claims with explicit quadrant tags.

Explicit-only — `disable-model-invocation: true`. Operator invokes when working specifically on boundary semantics.

## Step 1 — Read the boundary statement

Either operator pastes it inline, or they give a path to a spec section. Use `Read` to fetch if path-based.

## Step 2 — Atomize

Break the statement into atomic claims — one assertion per claim. A claim is atomic when it can't be split further without losing meaning.

## Step 3 — Tag each atomic claim with its quadrant

For each atomic claim:

- Ask: does this DEFINE something, or use a definition? → likely **L**
- Ask: does this state ALLOW / REJECT / DENY conditions for an action? → likely **A**
- Ask: does this OBLIGE / PERMIT / FORBID an agent? → likely **D**
- Ask: does this state SOMETHING WAS DONE / OBSERVED / MEASURED? → likely **E**

A claim that resists categorization usually mixes quadrants and needs further splitting.

## Step 4 — Cross-quadrant dependencies

Per A.6.B, the four quadrants have explicit dependency discipline:
- L statements depend on nothing (root vocabulary)
- A statements may reference L for term grounding
- D statements may reference L + A (you can't oblige what isn't permitted by A)
- E statements may reference any L / A / D for context but stand as observation

Note any cross-references between your atomic claims. If a D claim depends on an A claim, write the dependency edge explicitly.

## Step 5 — Render the routed claim set

Output as a Claim Register:

```
L-001: <law statement> — defines: <term>
L-002: <law statement> — defines: <term>

A-001: <admissibility rule> — references L-001, L-002
A-002: <admissibility rule>

D-001: <deontic commitment> — references A-001
D-002: <deontic commitment>

E-001: <evidence claim> — references L-001 + D-001
```

Each row has stable ID and explicit dependencies. The original prose becomes auditable.

## Step 6 — Recommend per-quadrant follow-up

After routing:
- L quadrant items → terms map (recommend adding to project term-map per FPF F.17)
- A quadrant items → admissibility tests (recommend deriving CI checks if applicable)
- D quadrant items → commitment ledger (recommend tracking promise satisfaction)
- E quadrant items → evidence (recommend attaching via `mcp__haft__haft_decision(action="evidence", ...)` to relevant decision)

## Step 7 — Surface to operator

Present:
- Original boundary statement
- Routed Claim Register with quadrant tags
- Dependencies between claims
- Per-quadrant follow-up recommendations

The operator decides what to act on. This skill stops at decomposition + recommendation.

## What NOT to do

- DO NOT tag a claim with two quadrants. If it doesn't fit one, split further.
- DO NOT skip dependency tracking — without it the routed claims still mix.
- DO NOT auto-create term-map / decision / evidence artifacts from this skill — those are downstream operator decisions.
- DO NOT use this skill for general code review or feature design — it's specifically for boundary statement decomposition.

## FPF spec references

- A.6 — Signature Stack & Boundary Discipline
- A.6.B — Boundary Norm Square (THE pattern this skill implements)
- A.6.C — Contract Unpacking for Boundaries
- A.6.0 — U.Signature
- F.18 — Local-First Unification Naming Protocol
- X-STATEMENT-TYPE — every load-bearing sentence = rule / promise / explanation / gate / evidence

Look up via `mcp__haft__haft_query(action="fpf", query="A.6.B")`.
