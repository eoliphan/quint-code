package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkillAirUsesProjectSkillsDir(t *testing.T) {
	projectRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	// Multi-skill installer returns the skills ROOT (parent of each skill
	// folder), not a per-skill subdir. Each skill in allSkills lands as
	// `<root>/<skill-name>/SKILL.md`.
	displayPath, err := installSkill("air", false, projectRoot)
	if err != nil {
		t.Fatalf("installSkill returned error: %v", err)
	}

	wantRoot := filepath.Join(projectRoot, "skills")
	if displayPath != wantRoot {
		t.Fatalf("display path = %q, want %q", displayPath, wantRoot)
	}

	// Each governance-substrate skill should land at <root>/<name>/SKILL.md.
	for _, sk := range allSkills {
		skillPath := filepath.Join(wantRoot, sk.Name, "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatalf("failed to read installed skill %q: %v", sk.Name, err)
		}
		if string(content) != string(sk.Content) {
			t.Fatalf("installed skill %q content mismatch", sk.Name)
		}
	}

	// Deprecated h-reason directory MUST NOT exist after install — the
	// migration is part of the install step so re-running haft init
	// always lands a clean post-pivot state.
	if _, err := os.Stat(filepath.Join(wantRoot, "h-reason")); !os.IsNotExist(err) {
		t.Fatalf("h-reason should be removed by deprecation cleanup; got err=%v", err)
	}
}

func TestInstallCodexSkillsWritesExplicitCommandSkills(t *testing.T) {
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	displayPath, count, err := installCodexSkills(projectRoot, false)
	if err != nil {
		t.Fatalf("installCodexSkills returned error: %v", err)
	}

	commandCount := embeddedCommandCount(t)
	// Codex installer writes allSkills (governance substrate skills) +
	// one skill per embedded command. Count = len(allSkills) + commandCount.
	wantCount := len(allSkills) + commandCount
	if count != wantCount {
		t.Fatalf("installed skill count = %d, want %d (allSkills=%d + commands=%d)",
			count, wantCount, len(allSkills), commandCount)
	}
	if displayPath != "~/.agents/skills" {
		t.Fatalf("display path = %q, want %q", displayPath, "~/.agents/skills")
	}

	skillsRoot := filepath.Join(homeDir, ".agents", "skills")
	frameSkillPath := filepath.Join(skillsRoot, "h-frame", "SKILL.md")
	frameSkill, err := os.ReadFile(frameSkillPath)
	if err != nil {
		t.Fatalf("failed to read h-frame skill: %v", err)
	}

	frameContent := string(frameSkill)
	for _, want := range []string{
		"name: h-frame",
		"This skill is explicit-only",
		"Use the user's explicit skill invocation text as the request context.",
		"$h-decide",
	} {
		if !strings.Contains(frameContent, want) {
			t.Fatalf("h-frame skill missing %q:\n%s", want, frameContent)
		}
	}
	for _, banned := range []string{"/h-", "/q-", "$ARGUMENTS", "Quint"} {
		if strings.Contains(frameContent, banned) {
			t.Fatalf("h-frame skill contains stale token %q:\n%s", banned, frameContent)
		}
	}
	skillFiles, err := filepath.Glob(filepath.Join(skillsRoot, "h-*", "SKILL.md"))
	if err != nil {
		t.Fatalf("glob installed skills: %v", err)
	}
	for _, skillFile := range skillFiles {
		content, err := os.ReadFile(skillFile)
		if err != nil {
			t.Fatalf("read installed skill %s: %v", skillFile, err)
		}
		for _, banned := range []string{"/h-", "/q-", "$ARGUMENTS", "Quint"} {
			if strings.Contains(string(content), banned) {
				t.Fatalf("%s contains stale token %q", skillFile, banned)
			}
		}
	}

	explicitPolicyPath := filepath.Join(skillsRoot, "h-frame", "agents", "openai.yaml")
	explicitPolicy, err := os.ReadFile(explicitPolicyPath)
	if err != nil {
		t.Fatalf("failed to read h-frame policy: %v", err)
	}
	if !strings.Contains(string(explicitPolicy), "allow_implicit_invocation: false") {
		t.Fatalf("h-frame should be explicit-only, got:\n%s", string(explicitPolicy))
	}

	// h-fpf is the v8 umbrella replacement for the deprecated h-reason
	// skill. It auto-triggers (narrow fallback) — verify policy reflects.
	fpfPolicyPath := filepath.Join(skillsRoot, "h-fpf", "agents", "openai.yaml")
	fpfPolicy, err := os.ReadFile(fpfPolicyPath)
	if err != nil {
		t.Fatalf("failed to read h-fpf policy: %v", err)
	}
	if !strings.Contains(string(fpfPolicy), "allow_implicit_invocation: true") {
		t.Fatalf("h-fpf should allow implicit invocation, got:\n%s", string(fpfPolicy))
	}

	// h-decide is manual-only (Transformer Mandate via codex policy +
	// disable-model-invocation in claude frontmatter).
	decidePolicyPath := filepath.Join(skillsRoot, "h-decide", "agents", "openai.yaml")
	decidePolicy, err := os.ReadFile(decidePolicyPath)
	if err != nil {
		t.Fatalf("failed to read h-decide policy: %v", err)
	}
	if !strings.Contains(string(decidePolicy), "allow_implicit_invocation: false") {
		t.Fatalf("h-decide must be explicit-only per Transformer Mandate, got:\n%s", string(decidePolicy))
	}

	// Deprecated h-reason directory must be removed (migration step).
	if _, err := os.Stat(filepath.Join(skillsRoot, "h-reason")); !os.IsNotExist(err) {
		t.Fatalf("h-reason must be removed by deprecation cleanup; got err=%v", err)
	}
}

func TestInstallCodexSkillsLocalUsesProjectAgentsDir(t *testing.T) {
	projectRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	displayPath, _, err := installCodexSkills(projectRoot, true)
	if err != nil {
		t.Fatalf("installCodexSkills returned error: %v", err)
	}

	wantPath := filepath.Join(projectRoot, ".agents", "skills")
	if displayPath != wantPath {
		t.Fatalf("display path = %q, want %q", displayPath, wantPath)
	}
}

func TestCleanupCodexPromptCommandsRemovesOnlyHaftPrompts(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	promptDir := filepath.Join(homeDir, ".codex", "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"h-frame.md":  "old h-frame prompt",
		"q-frame.md":  "old q-frame prompt",
		"q-reason.md": "old q-reason prompt",
		"custom.md":   "user prompt",
	}
	for name, content := range files {
		path := filepath.Join(promptDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	displayPath, removed, err := cleanupCodexPromptCommands()
	if err != nil {
		t.Fatalf("cleanupCodexPromptCommands returned error: %v", err)
	}
	if displayPath != "~/.codex/prompts" {
		t.Fatalf("display path = %q, want %q", displayPath, "~/.codex/prompts")
	}
	if removed != 3 {
		t.Fatalf("removed = %d, want 3", removed)
	}

	for _, removedName := range []string{"h-frame.md", "q-frame.md", "q-reason.md"} {
		if _, err := os.Stat(filepath.Join(promptDir, removedName)); !os.IsNotExist(err) {
			t.Fatalf("%s should have been removed", removedName)
		}
	}
	if _, err := os.Stat(filepath.Join(promptDir, "custom.md")); err != nil {
		t.Fatalf("custom prompt should remain: %v", err)
	}
}

func embeddedCommandCount(t *testing.T) int {
	t.Helper()

	entries, err := embeddedCommands.ReadDir("commands")
	if err != nil {
		t.Fatalf("read embedded commands: %v", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		count++
	}
	return count
}

// TestHDecideSkill_IsManualOnlyTransformerMandate verifies that the
// h-decide skill carries the structural Transformer Mandate enforcement
// (disable-model-invocation) so the agent cannot auto-fire a binding
// DecisionRecord write. Per v8 governance substrate pivot.
func TestHDecideSkill_IsManualOnlyTransformerMandate(t *testing.T) {
	content := string(embeddedHDecideSkill)

	required := []string{
		`disable-model-invocation: true`,
		`MANUAL ONLY`,
		`Transformer Mandate`,
	}

	for _, want := range required {
		if !strings.Contains(content, want) {
			t.Fatalf("h-decide skill missing %q — Transformer Mandate enforcement broken", want)
		}
	}
}

// TestHFPFSkill_IsNarrowUmbrella verifies that the h-fpf umbrella does
// not absorb the entire FPF procedural body. It must point to specific
// skills + the spec search rather than recreating h-reason-style
// encyclopedia (per plan §3 and FPF reasoner critique 2026-05-25).
func TestHFPFSkill_IsNarrowUmbrella(t *testing.T) {
	content := string(embeddedHFPFSkill)

	// Must reference the specific skills it routes to.
	for _, sk := range []string{"h-frame", "h-diagnose", "h-explore", "h-compare", "h-decide", "h-verify"} {
		if !strings.Contains(content, sk) {
			t.Fatalf("h-fpf must list %q in its routing table", sk)
		}
	}
	// Must point at the spec-search MCP path so the agent can retrieve
	// pattern text without h-fpf having to inline it.
	if !strings.Contains(content, `haft_query(action="fpf"`) {
		t.Fatal("h-fpf must point at haft_query(action=\"fpf\", ...) for spec lookups")
	}
}

func TestV7EmbeddedCommandPromptsDescribeSpecFirstSurfaceContracts(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		required []string
	}{
		{
			name: "h-onboard",
			path: "commands/h-onboard.md",
			required: []string{
				"TargetSystemSpec",
				"EnablingSystemSpec",
				"TermMap",
				"SpecCoverage",
				"haft spec check",
				"needs_onboard",
				"Claude Code and Codex",
				`haft_spec_section(action="next_step"`,
				`haft_spec_section(action="approve"`,
				`haft_spec_section(action="rebaseline"`,
				`haft_spec_section(action="reopen"`,
				`haft_query(action="check")`,
				"spec_section_needs_baseline",
				"spec_section_drifted",
				"enabling.architecture.draft",
				"enabling.work_methods.draft",
				"enabling.effect_boundaries.draft",
				"enabling.agent_policy.draft",
				"enabling.commission_policy.draft",
				"enabling.runtime_policy.draft",
				"enabling.evidence_policy.draft",
				"haft spec onboard --json",
				`haft_query(action="fpf"`,
				"FRAME-09",
				"CHR-10",
				"CHR-12",
				"X-STATEMENT-TYPE",
				"statement_type",
				"claim_layer",
				"valid_until",
				"target_refs",
				"guard location",
				"never write",
			},
		},
		{
			name: "h-status",
			path: "commands/h-status.md",
			required: []string{
				"needs_onboard",
				"haft spec check",
				"WorkCommissions",
				"stale, blocked, or running-too-long WorkCommissions",
				`haft_commission(action="show"`,
				"do not start Open-Sleigh",
				`haft_query(action="check")`,
			},
		},
		{
			name: "h-verify",
			path: "commands/h-verify.md",
			required: []string{
				`haft_query(action="check")`,
				"spec_section_drifted",
				"spec_section_stale",
				"spec_section_needs_baseline",
				`haft_spec_section(action="rebaseline"`,
				`haft_spec_section(action="reopen"`,
				`haft_spec_section(action="approve"`,
			},
		},
		{
			name: "h-commission",
			path: "commands/h-commission.md",
			required: []string{
				"authorization step only",
				"must not start Open-Sleigh",
				"does not own runtime lifecycle",
				"WorkCommission = bounded permission to execute",
				"Do not requeue a commission whose `valid_until` has expired",
				"Do not physically delete WorkCommissions",
			},
		},
		{
			name: "h-frame",
			path: "commands/h-frame.md",
			required: []string{
				"Project readiness",
				"needs_onboard",
				"/h-onboard",
				"tactical",
				"Investigation-first discipline",
				`haft_query(action="resolve_term"`,
				"bounded context",
			},
		},
		{
			name: "h-decide",
			path: "commands/h-decide.md",
			required: []string{
				"Project readiness",
				"needs_onboard",
				"SpecSection refs",
				"haft_commission(create_from_decision)",
				"/h-onboard",
				"Investigation-first discipline",
				`haft_query(action="resolve_term"`,
			},
		},
		{
			name: "h-note",
			path: "commands/h-note.md",
			required: []string{
				"Investigation-first discipline",
				`haft_query(action="resolve_term"`,
				"bounded context",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contentBytes, err := embeddedCommands.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read command %s: %v", tc.path, err)
			}

			content := string(contentBytes)
			for _, required := range tc.required {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing %q:\n%s", tc.path, required, content)
				}
			}
		})
	}
}
