---
name: h-note
description: |
  Records a micro-decision with rationale into the haft artifact graph — lighter than a full DecisionRecord but persisted so future sessions and conflict detection can surface it. Make sure to use this skill whenever the user says "remember that", "FYI for later", "note that we chose X", "side note", "let's record we ruled out Y", "remember we decided X", "for the record", "worth noting", "TIL", "important caveat", "save this thought" — or whenever a small choice with stated rationale belongs in project memory but does not justify the full DRR ceremony. The kernel rejects content-free notes — rationale is required. For binding choices use h-decide (manual-only). For framing problems use h-frame.
when_to_use: |
  Small choice or observation with rationale, worth persisting across sessions, but lighter than a binding DecisionRecord.
argument-hint: "[note text — what + why]"
allowed-tools: mcp__haft__haft_problem mcp__haft__haft_query
---

# h-note — Record a micro-decision

You are recording a Note via the kernel. Notes are lightweight artifacts in the graph — they don't have the DRR ceremony but they do require rationale per FPF DEC-01 (no rationale-free decision capture).

The Note tool lives under `haft_problem` action `select` does not write notes; the note-write surface is exposed through dedicated handlers in the MCP server. Check the actual tool name available — at present notes are written via the haft note CLI subcommand and via certain MCP integrations.

## Step 1 — Confirm intent

The operator said something note-worthy. Before persisting:
- Verify the note has substantive content: a fact OR a choice OR an observation with rationale
- Reject: "FYI" alone, "we should remember this" with no payload
- Accept: "We chose X over Y because Z" / "Observation: tests run 30s slower on M1 baseline since dependency update"

## Step 2 — Capture rationale explicitly

Every Note must answer:
- WHAT was decided / observed
- WHY (the rationale)
- WHEN (optional — kernel adds timestamp automatically; only add a domain timestamp if relevant)

## Step 3 — Persist

Use the appropriate note-write tool. Currently:

```
mcp__haft__haft_problem(
  action="frame",
  problem_type="optimization",
  title="<short title>",
  signal="<the observation or choice>",
  acceptance="N/A — recorded as note for future recall",
  mode="tactical"
)
```

(Notes use the lightweight tactical-mode problem record OR a dedicated note write surface depending on what the kernel exposes at runtime. The kernel rejects empty rationale.)

For richer note support, prefer the `haft note` CLI command when available.

## Step 4 — Confirm to operator

Surface:
- The note ID (the recorded artifact)
- A reminder that future `/h-status` or related-query lookups will surface this note when relevant context arises

## What NOT to do

- DO NOT persist notes that lack rationale. Force the operator to articulate WHY, or refuse and ask for the rationale.
- DO NOT use h-note for binding choices — those go through `/h-decide` (manual-only).
- DO NOT use h-note for full problem framing — that's `/h-frame`.
- DO NOT silently expand a note into a DecisionRecord. If the operator's intent is bigger than a note, recommend `/h-decide` and let them invoke it explicitly.
- DO NOT capture meta-notes about agent behavior ("agent helped me with X") — those are session telemetry, not project knowledge.

## FPF spec references

- DEC-01 — Decision record structure (notes are the lightweight cousin — same problem-frame + decision + rationale + consequences minimum, just compressed)
- E.9 — Design-Rationale Record (full DRR; notes are sub-DRR but still rationale-bearing)

Look up via `mcp__haft__haft_query(action="fpf", query="DEC-01")`.
