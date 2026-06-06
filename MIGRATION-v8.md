# Migrating from haft v7 → v8

v8 is a governance-substrate pivot. The reasoning kernel, the artifact graph,
the FPF spec retrieval, the WorkCommission lifecycle — **all unchanged**.
What changed is the surface: haft is now consumed through host-AI skills
plus an MCP server, not through a standalone interactive agent.

Full rationale, parity-compared variants, rollback plan, and falsifiable
predictions live in
`.haft/decisions/dec-20260525-v8-architecture-pivot-from-standalone-agent-to-g-bbe45cb7.md`.

## What was dropped

- `haft agent` — the standalone interactive REPL
- TUI surface (`internal/tui`, `tui/` package)
- Desktop wrappers (Tauri / Wails apps in `desktop/`)
- v7 helper commands: `haft login`, `haft models`, `haft setup`
- `/h-reason` umbrella skill — replaced by the 15-skill catalog

If you depended on any of those, **do not upgrade** until you've migrated
to one of the three remaining surfaces (skills/CLI/MCP).

## What replaced what

| v7 you used | v8 equivalent |
|------|------|
| `haft agent` (interactive) | Talk to Claude Code / Codex / OpenCode / Cursor; the v8 skills auto-trigger |
| `/h-reason "..."` (one-shot) | Workflow skills: describe the problem, h-frame fires; explore + compare follow |
| `/h-reason` for explicit reasoning | `/h-frame` → `/h-explore` → `/h-compare` → manual `/h-decide` |
| `haft setup`, `haft login`, `haft models` | Not needed — host AI provides the LLM; haft only manages the artifact graph |
| Desktop dashboard | `haft check`, `/h-status`, `/h-verify` from the host AI; PR creation via `haft run` |
| TUI session view | `/h-status` in the host AI, or `haft_query(action="status")` from MCP |

## Upgrade steps for an existing project

1. **Re-run `haft init`.** This installs the new 15-skill catalog,
   removes the deprecated `h-reason` skill directory, and registers the
   MCP server under the new project config.

   ```bash
   cd /your/project
   haft init --tool claude   # or --tool codex
   ```

2. **Audit references to dropped commands.** Search your project notes
   and CI for `haft agent`, `haft login`, `haft models`, `haft setup`,
   or `/h-reason`. Replace per the table above.

   ```bash
   grep -rn "haft agent\|haft login\|h-reason" .
   ```

3. **Restart your host AI.** Claude Code, Codex, etc. cache skill
   manifests at startup. After `haft init`, restart the host to pick
   up the new catalog.

4. **Run `haft check routing`** (optional, for plugin-mode operators).
   This sanity-checks that the installed skill descriptions still
   route operator-style prompts to the right skill. Threshold is 70%
   per the pivot DRR.

   ```bash
   haft check routing
   ```

5. **Your `.haft/` artifact graph is unchanged.** Decisions, problems,
   evidence, baselines, WorkCommissions — all still load, still verify,
   still surface in `/h-status`. The v7→v8 pivot is a surface change,
   not a schema change.

## Behavioral changes worth knowing

- **h-decide is now manual-only** (`disable-model-invocation: true`).
  The host AI agent will not fire it automatically even on matching
  prompts. You must type `/h-decide` (or its host-specific equivalent).
  Same for `/h-commission`. This enforces the Transformer Mandate:
  binding artifacts come from the human principal, not the agent.

- **Tactical-mode validation has explicit skip.** When recording a
  decision in `tactical` mode, you can pass `_skips` (list of field
  names) plus `_skip_reason` to bypass validation on a specific field
  with reason recorded. The allowlist excludes load-bearing fields
  like `selected_title`. Standard and deep modes cannot skip.

- **MCP returns structured errors as enforcement gates.** Missing
  required DRR fields produce a plain-text error with field hints +
  FPF spec references (CMP-02, DEC-08, X-WLNK, etc.) + a "how to
  proceed" section listing skip semantics. Read it; the message
  contains everything needed to either fill the missing data or
  acknowledge with a skip.

- **Diagnose runs parallel hypotheses.** The new `h-diagnose` skill
  spawns one Agent subagent per hypothesis in the same message,
  preventing the LLM's natural anchoring bias toward the first
  plausible cause. Forces 3+ rivals per FPF CC-B.5.2-2.

- **Compare runs dim-wise parallel scoring.** The new `h-compare`
  skill spawns one Agent subagent per comparison dimension scoring
  all variants — again to prevent anchoring. Parity plan and
  selection policy declared BEFORE scoring (Anti-Goodhart).

## Rollback

If v8 produces regressions in your workflow:

1. Pin to the last v7 release in your install command.
2. Re-run `haft init` on that pinned version to restore the v7 skill
   layout.
3. File a regression issue at https://github.com/m0n0x41d/haft/issues
   with the host AI + version + skill that failed to fire as expected.

The pivot DRR predicts a 70% routing reliability threshold. If your
real-world rate is materially lower (say <50%), that is concrete
evidence the pivot's central claim doesn't hold for your environment
and the operator should reopen the decision.

## Reference

- Full pivot DRR: `.haft/decisions/dec-20260525-v8-architecture-pivot-from-standalone-agent-to-g-bbe45cb7.md`
- Execution plan: `.context/v8_haft_governance_substrate_plan.md`
- Skill catalog: README.md "Fifteen skills installed by `haft init`"
- Routing sanity check: `haft check routing`
