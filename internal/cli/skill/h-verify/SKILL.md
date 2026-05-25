---
name: h-verify
description: |
  Verify a recorded decision still holds: baseline → measure → evidence loop with drift detection. Use when the operator signals verification intent — "did it work", "is dec-X still valid", "check if Y holds", "measure that decision", "did the prediction come true". Implements FPF B.3 (Trust & Assurance) + B.3.4 (Evidence Decay) + the canonical measurement protocol per /h-decide's predictions.
when_to_use: |
  A DecisionRecord exists and needs post-implementation verification. NOT for ad-hoc sanity checks (just run the tests yourself). NOT for re-framing problems (that's h-frame).
argument-hint: "[decision-ref or 'what's stale' for full project verification]"
allowed-tools: Bash Read Grep Glob mcp__haft__haft_decision mcp__haft__haft_query mcp__haft__haft_refresh
---

# h-verify — Verify a decision still holds

You are running the FPF verification loop: baseline → measure → evidence → record. Drift detection compares current state against baselined affected_files; evidence decay reports surface when valid_until passes; measure verdict is recorded for the predictions the decide step declared.

## Step 1 — Identify the decision

If `decision_ref` is given, use it. Otherwise:
- `mcp__haft__haft_query(action="status")` — surfaces stale/refresh-due decisions
- `mcp__haft__haft_query(action="list", kind="DecisionRecord")` — full list
- Ask the operator which decision to verify

## Step 2 — Read the decision's predictions

`mcp__haft__haft_query(action="search", query="<decision_ref>")` returns the DecisionRecord including its `predictions` field. Each prediction has:
- `claim` — the falsifiable statement
- `observable` — what to measure
- `threshold` — pass/fail boundary
- `verify_after` — when async evidence should be available (if any)

If predictions are empty (the decision was recorded tactical with `_skips: ["predictions"]`), there's nothing to measure — report that to operator and recommend either:
- `/h-refresh` action=reopen to add predictions and re-decide properly
- Just attach evidence directly via `haft_decision(action="evidence", ...)`

## Step 3 — Baseline (if drift detection wanted)

If the decision has `affected_files` and you want drift comparison:

```
mcp__haft__haft_decision(
  action="baseline",
  decision_ref="<dec-...>"
  // affected_files optional — kernel uses the decision's list
)
```

The kernel snapshots file content hashes. Subsequent comparisons detect drift. Call once after each commit cycle if you want continuous drift signal.

## Step 4 — Gather evidence per prediction

For each prediction:
- Run the observable (test, metric query, log scan, code grep)
- Compare to threshold
- Capture the actual measurement value

Tools available depending on the observable:
- `Bash` for test runners, metric queries, log scans
- `Read` / `Grep` / `Glob` for code-level invariant checks
- For external metrics: kernel has no special integration; agent describes the source

## Step 5 — Attach evidence to the artifact

For each material evidence item:

```
mcp__haft__haft_decision(
  action="evidence",
  artifact_ref="<dec-...>",
  evidence_type="measurement | test | research | benchmark | audit",
  evidence_content="<what you observed, with concrete numbers>",
  evidence_verdict="supports | weakens | refutes",
  carrier_ref="<file path or URL where the evidence lives>",
  claim_refs=["<prediction id or scope label>"],
  congruence_level=3,  // 3=same context, 2=similar, 1=different, 0=opposed
  valid_until="<RFC3339 or YYYY-MM-DD — when this evidence expires>",
  causal_support_basis="observational | interventional | realized_counterfactual | identified_estimate | simulation_only"
)
```

**congruence_level** (CL) defaults per FPF B.3.5:
- 3: same-context evidence (own production system, own tests)
- 2: similar-context (related project, similar load)
- 1: different-context (external docs, vendor benchmarks)
- 0: opposed-context (rare; conflicting framework)

CL impacts R_eff per FPF B.3:3 — never average across CL.

## Step 6 — Record the measurement verdict

After all evidence is attached, record the overall verdict:

```
mcp__haft__haft_decision(
  action="measure",
  decision_ref="<dec-...>",
  verdict="accepted | partial | failed",
  findings="<what actually happened compared to predictions>",
  measurements=["p99 latency: 42ms (predicted <50ms — accepted)", "..."],
  criteria_met=["<criterion that was met>"],
  criteria_not_met=["<criterion that was NOT met>"]
)
```

Kernel ties the verdict back to the predictions and surfaces:
- Accepted → decision health remains good
- Partial → some predictions held, some didn't → consider reopen or supersede
- Failed → decision invalidated → consider supersede or rollback per the decision's rollback spec

## Step 7 — Handle stale or drifted decisions

If verification reveals:
- **Evidence decayed** (valid_until passed): `mcp__haft__haft_refresh(action="waive", artifact_ref=..., evidence="<new evidence>", new_valid_until="...")` to extend validity, OR `action=reopen` to start a new problem cycle
- **Drift detected** (affected_files changed since baseline): classify drift as cosmetic / incidental / material via `haft_query(action="status")` and decide whether to re-baseline or reopen
- **Verdict failed**: `mcp__haft__haft_refresh(action="supersede", artifact_ref=<old>, new_artifact_ref=<replacement>)` after recording the replacement decision via `/h-decide`

## Step 8 — Present to operator

Surface:
- Predictions vs actual measurements
- Verdict (accepted / partial / failed)
- Evidence attached with CL
- Drift status if baseline existed
- Recommended next action (waive / reopen / supersede / nothing — decision still good)

## What NOT to do

- Do NOT call `action="measure"` without first gathering evidence — kernel rejects measure-from-memory (the protocol requires evidence before verdict).
- Do NOT use congruence_level=3 for evidence that came from a different context (vendor benchmark, external doc). Misclassifying CL inflates R_eff and corrupts trust signal.
- Do NOT skip baseline if the decision has affected_files and drift matters — without baseline, drift is invisible.
- Do NOT silently change a decision's verify_after to "kick the can". Use `action="waive"` with `evidence` parameter so the extension is auditable.
- Do NOT mark verdict=accepted when predictions were skipped — verify what was declared, not what was hoped.

## FPF spec references

- B.3 — Trust & Assurance Calculus (F-G-R + CL)
- B.3.3 — Assurance Subtypes & Levels (L0/L1/L2 + TA/VA/LA)
- B.3.4 — Evidence Decay & Epistemic Debt
- A.10 — Evidence Graph Referring
- VER-01 (evidence graph), VER-02 (decay), VER-03 (R_eff), VER-07 (refresh triggers)
- C.27 — Temporal Claim Adequacy (state reading vs trend vs intervention claim)

Look up via `mcp__haft__haft_query(action="fpf", query="B.3")`.
