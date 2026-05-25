---
name: h-note
description: |
  Record a micro-decision with rationale. Use when the operator signals lightweight persistence intent — "remember that", "FYI for later", "side note", "let's note that we chose X", "remember we ruled out Y". The kernel validates the rationale (rejects content-free notes) and persists into the artifact graph so future sessions and conflict-detection can surface the note.
when_to_use: |
  Operator wants to persist a small fact, choice, or observation that doesn't deserve a full DecisionRecord ceremony. For binding choices use h-decide (manual-only). For observations during diagnosis use h-diagnose. For framing problems use h-frame.
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
