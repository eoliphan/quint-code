---
name: h-onboard
description: |
  First-setup ceremony for a project that does not yet use haft — the agent reads the repository, drafts the minimum FPF carriers (target system, enabling system, term map) from observed code/docs, and presents them to the operator for review. The operator is NOT asked to author spec files from scratch — that defeats the value of having an AI agent. Make sure to use this skill whenever the repository has no `.haft/` directory yet, when the user says "set up haft here", "onboard this project", "initialize FPF", "first time using haft in this repo", "let's add haft to this project", "scaffold haft for this codebase" — or whenever they want to start recording decisions but the artifact graph is not scaffolded. NOT for ongoing work in a project that already has `.haft/` (use h-status). NOT for framing one specific problem (use h-frame).
when_to_use: |
  Repo has no `.haft/` directory, OR `.haft/` exists but has no active SpecSections. For existing haft projects use h-status.
argument-hint: "[optional: short project description]"
allowed-tools: Bash Read Grep Glob Write mcp__haft__haft_problem mcp__haft__haft_query
---

# h-onboard — First-impression FPF setup (agent-drafts, operator-reviews)

You are onboarding a project to haft. **The agent does the work; the operator reviews.** Spec files are descriptive observation, not binding decisions — Transformer Mandate does NOT prohibit you from authoring them. Your job is to read the repository, infer the project's structure, and produce initial spec drafts. The operator's job is to correct what you got wrong.

## Step 1 — Check current state

Run `mcp__haft__haft_query(action="status")` to confirm `.haft/` exists but has no active SpecSections.

- `ready` → already onboarded; offer `/h-status` instead and stop.
- `needs_init` → operator must run `haft init` from the CLI first (it installs MCP config + skills, this skill cannot do that). Stop and tell them.
- `needs_onboard` → continue.

## Step 2 — Discover the project (agent reads repo)

Read these in parallel before asking the operator anything:

- `README.md`, `README.rst` if present
- Project-config files: `package.json`, `pyproject.toml`, `Cargo.toml`, `go.mod`, `Gemfile`, `composer.json`, etc.
- Top-level: `LICENSE`, `CHANGELOG.md`, `CONTRIBUTING.md`
- Source entry points: `src/`, `cmd/`, `lib/`, or whatever the project layout uses
- Existing docs: `docs/`, `ADR/`, `decisions/`
- Test layout: `tests/`, `__tests__/`, `*_test.go`, etc.
- CI: `.github/workflows/`, `.gitlab-ci.yml`, `Taskfile.yaml`, `Makefile`

From this you should be able to answer most of these on your own:

- **What is the project?** (library / CLI / service / infra / docs site / something else)
- **Primary tech stack** (language, key frameworks)
- **Who uses it** (other developers / end-users / internal team) — from README
- **How it's released** (PyPI / npm / Docker / binary / git-only) — from config + CI
- **What use cases it serves** — from README intro + examples
- **Load-bearing project-specific terms** — recurring nouns in README and docs that aren't generic engineering vocab

If after this pass you have ambiguity that genuinely blocks drafting (e.g., README is just a logo, or the project does multiple unrelated things), prepare 1-3 short clarifying questions for Step 4. Otherwise skip clarifications and go straight to drafting.

## Step 3 — Draft the three spec files

Write directly to disk using the `Write` tool. Each file starts with a `<!-- DRAFT — onboarding by haft agent on YYYY-MM-DD; operator must review and edit -->` HTML comment so the operator sees it's a starting point, not authoritative.

### `.haft/specs/target-system.md`

The system that delivers value to its users. One section per identifiable use case. Template:

```markdown
<!-- DRAFT — onboarding by haft agent on 2026-XX-XX; operator must review and edit -->

# Target System: <project name>

## Use case: <name from README/docs>

**Who:** <user role, inferred>
**What they do:** <action / outcome, inferred from README>
**Why it matters:** <value statement, inferred>
**Acceptance signal:** <how you can tell it's working — observable, from tests or examples>

## Use case: <next one>
...
```

Aim for 1-4 use cases. If only one is clear, write only one — the operator can add more later. Don't invent use cases that aren't visible in the codebase.

### `.haft/specs/enabling-system.md`

The system that creates and maintains the target system — i.e., the team + their tooling + their process. Inferable from CI files, contributors, code conventions. Template:

```markdown
<!-- DRAFT — onboarding by haft agent on 2026-XX-XX; operator must review and edit -->

# Enabling System

## Team
<inferred from git log authors count, README "Team" / "Contributors" / "Maintainers" section>

## Tooling
- Build: <e.g., `task install` from Taskfile.yaml, `go build`, `npm run build`>
- Test: <e.g., `pytest`, `go test ./...`, `npm test`>
- Lint: <e.g., `ruff`, `golangci-lint`, `eslint`>
- Release: <e.g., GitHub release + PyPI publish, manual, `goreleaser`>

## Process
- CI: <inferred from `.github/workflows/`>
- Branching: <inferred from default branch and any contributing docs>
- Review: <if CODEOWNERS / branch protection inferable>
```

### `.haft/specs/term-map.md`

Project-specific load-bearing terms — vocabulary an outsider would misread. Template:

```markdown
<!-- DRAFT — onboarding by haft agent on 2026-XX-XX; operator must review and edit -->

# Term Map

## <Term 1>
<concise definition, in the project's actual meaning>

## <Term 2>
...
```

Extract these from README headings, docstrings, identifier patterns. Aim for 3-8 entries. Skip generic engineering terms (function, class, test, etc.) — focus on what's *project-specific*.

## Step 4 — Ask only what you couldn't infer

After Step 3, surface to the operator:

```
I drafted three spec files based on reading the repo:
  - .haft/specs/target-system.md
  - .haft/specs/enabling-system.md
  - .haft/specs/term-map.md

I marked them DRAFT. Review and edit where I got things wrong.

[N clarifying questions only if needed]
1. [question 1]
2. [question 2]
```

Clarifying questions are **only** for things you couldn't infer with reasonable confidence. Typical examples:

- "I couldn't determine primary use case — is this a library for application developers, or an end-user CLI?"
- "Observability/monitoring isn't visible in CI — is it in-scope or out-of-scope?"
- "I see two distinct subprojects; should they be one target system or two?"

**Do NOT ask:**

- "What's your target system?" — if you could infer it, just write it.
- "What's the team?" — git log + README give you this.
- "What's in scope?" — out-of-scope is harder, but in-scope is usually obvious.
- "What's your agent autonomy default?" — that's a commission-time decision, not onboarding.

## Step 5 — Frame an onboarding ProblemCard

After drafts are written, record a bootstrap ProblemCard:

```
mcp__haft__haft_problem(
  action="frame",
  problem_type="synthesis",
  title="Project onboarding: <repo name>",
  signal="Project lacks active FPF spec carriers; agent has drafted initial spec files for operator review",
  acceptance="Operator has reviewed and edited the three drafted spec files; readiness=ready on /h-status",
  blast_radius="<project scope>",
  reversibility="high",
  mode="tactical"
)
```

Tactical mode — onboarding is reversible (delete `.haft/specs/` and try again).

## Step 6 — Set expectations

Tell the operator:

- Three spec files are DRAFTED on disk, marked DRAFT. Edit them with your judgment — agent inferred from observable code, you know context the code doesn't carry.
- haft is a governance substrate, not a coding agent. Your Claude Code / Codex still does the coding. haft persists the reasoning around it.
- After you accept the spec drafts, /h-status will show `ready`.
- /h-frame, /h-explore, /h-compare auto-fire on signal. /h-decide and /h-commission are manual-only (Transformer Mandate). /h-status is your cheap dashboard.

## What NOT to do

- DO NOT ask the operator to write spec files themselves — that defeats the point of an AI agent. You read the repo; you draft; they review.
- DO NOT ask about autonomy defaults / commission permissions / agent-budget — those are per-`/h-commission` decisions, not onboarding scope.
- DO NOT bind any DecisionRecord during onboarding — only one ProblemCard (tactical).
- DO NOT ask 5 rapid-fire questions if you can infer 4 of them from the repo. Calibrate clarifications to genuine ambiguity, not ceremony.
- DO NOT recommend `haft agent` or `haft desktop` — those surfaces do not exist in v8. The agent layer is the operator's Claude Code / Codex / Cursor / OpenCode.

## Edge cases

- **Empty repo / single README**: draft minimal specs from whatever's there (even one paragraph) plus operator clarifications. Don't refuse.
- **Monorepo with many subprojects**: ask one clarifying question — "treat as one project or scaffold per-subproject?" — then proceed.
- **Heavily docs-driven project (e.g., spec repo with no code)**: target-system is the spec itself; enabling-system is the maintainers + their authoring tooling; that's fine.

## FPF spec references (if operator asks why)

- A.1 — Holonic Foundation (target system vs enabling system distinction)
- F.17 — Unified Term Sheet (the term-map carrier)
- E.14 — Human-Centric Working-Model

Look up via `mcp__haft__haft_query(action="fpf", query="A.1")`.
