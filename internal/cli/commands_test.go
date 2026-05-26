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
	displayPath, count, err := installSkill("air", false, projectRoot)
	if err != nil {
		t.Fatalf("installSkill returned error: %v", err)
	}
	if count != len(allSkills) {
		t.Errorf("installSkill installed %d skills, expected %d", count, len(allSkills))
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

	// Codex installer writes exactly allSkills — no embedded commands
	// path. Skills are the primary surface; slash commands are not
	// shipped with haft.
	if count != len(allSkills) {
		t.Fatalf("installed skill count = %d, want %d (len(allSkills))",
			count, len(allSkills))
	}
	if displayPath != "~/.agents/skills" {
		t.Fatalf("display path = %q, want %q", displayPath, "~/.agents/skills")
	}

	skillsRoot := filepath.Join(homeDir, ".agents", "skills")

	// h-frame is an auto-triggering workflow skill — its SKILL.md is the
	// raw skill body (frontmatter has the description for routing), NOT
	// the command-wrapper variant that prefixes "This skill is explicit-
	// only". Slash-command-style $h-X references must still be rewritten
	// from /h-X.
	frameSkillPath := filepath.Join(skillsRoot, "h-frame", "SKILL.md")
	frameSkill, err := os.ReadFile(frameSkillPath)
	if err != nil {
		t.Fatalf("failed to read h-frame skill: %v", err)
	}
	frameContent := string(frameSkill)
	for _, want := range []string{
		"name: h-frame",
		"$h-explore",
	} {
		if !strings.Contains(frameContent, want) {
			t.Fatalf("h-frame skill missing %q:\n%s", want, frameContent)
		}
	}
	for _, banned := range []string{"/h-", "/q-", "Quint"} {
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

	// h-frame is an auto-triggering workflow skill — policy must reflect
	// that. Manual-only skills (h-decide, h-commission) get asserted
	// below.
	framePolicyPath := filepath.Join(skillsRoot, "h-frame", "agents", "openai.yaml")
	framePolicy, err := os.ReadFile(framePolicyPath)
	if err != nil {
		t.Fatalf("failed to read h-frame policy: %v", err)
	}
	if !strings.Contains(string(framePolicy), "allow_implicit_invocation: true") {
		t.Fatalf("h-frame should allow implicit invocation, got:\n%s", string(framePolicy))
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
