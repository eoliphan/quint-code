---
name: h-fpf
description: |
  Use as fallback ONLY when the operator references FPF in general terms ("let's think FPF-style", "what does FPF say about X", "which haft skill should I use", "explore the FPF spec"). Do NOT use for concrete tasks where a specific haft skill applies — those go to h-frame, h-diagnose, h-explore, h-compare, h-decide, h-verify, h-status, h-onboard, h-spec-cover, h-note, h-abduct, h-boundary-unpack, h-semio-review, or h-commission.
when_to_use: |
  Fallback for FPF-meta queries. Specific haft skills always take precedence.
argument-hint: "[FPF topic, pattern id, or 'which haft skill for X?']"
---

# h-fpf — FPF umbrella (narrow fallback)

This skill exists for operator FPF-meta queries. The concrete FPF workflows
live in specific haft skills — invoke them first when the operator's intent
matches.

## Specific haft skills (prefer these over h-fpf)

| Trigger | Skill |
|---|---|
| "frame this", "what's the problem", "before we solve" | `h-frame` |
| "X doesn't work", "tests fail", "investigate" | `h-diagnose` |
| "what are our options", "brainstorm approaches" | `h-explore` |
| "compare A and B", "trade-off between" | `h-compare` |
| "let's go with X", "decision time" (manual only) | `h-decide` |
| "did it work", "is dec-X still valid" | `h-verify` |
| "what's stale", "where are we" | `h-status` |
| working in repo without `.haft/` | `h-onboard` |
| "is X documented", "spec coverage" | `h-spec-cover` |
| "remember that", "FYI" | `h-note` |
| "let's commission" (manual only) | `h-commission` |
| "what could explain", "generate hypotheses" | `h-abduct` |
| "API surface", "contract for X" | `h-boundary-unpack` |
| "rename X to Y", "audit spec consistency" | `h-semio-review` |

## When this skill IS the right one

- Operator asks "what is FPF" / "tell me about FPF generally"
- Operator wants to explore the FPF specification: `mcp__haft__haft_query(action="fpf", query="<topic or pattern-id>")`
- Operator says "use FPF in your thinking" without a specific workflow
- No specific haft skill above matches the request

## Procedure

1. If a specific haft skill above matches the operator's intent — recommend that skill instead of proceeding.
2. For FPF spec lookups use `mcp__haft__haft_query(action="fpf", query="<topic>")` (add `full=true` for complete pattern text, `explain=true` for guided explanation).
3. For general FPF reasoning: apply FPF discipline (Strict Distinction A.7, WLNK per claim, parity before compare, evidence anchored) but persist load-bearing decisions via the specific haft skills, not free-form.

## Do not

- Do not auto-fire on "compare", "decide", "frame", "diagnose" — those route to specific skills.
- Do not become a procedure carrier; this is a routing surface + spec-search pointer.
- Do not exceed ~50 lines of body content. If you're explaining FPF principles, link to the spec via `haft_query(action="fpf", ...)` instead.
