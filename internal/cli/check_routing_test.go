package cli

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestRoutingPromptsYAMLParses guards the embedded golden suite from
// accidental YAML corruption. Without this, a bad commit could leave
// the runner reporting 0/0 pass and silently lose drift coverage.
func TestRoutingPromptsYAMLParses(t *testing.T) {
	var suite routingTestSuite
	if err := yaml.Unmarshal(embeddedRoutingPrompts, &suite); err != nil {
		t.Fatalf("embedded routing-prompts.yaml does not parse: %v", err)
	}
	if len(suite.Cases) == 0 {
		t.Fatal("routing-prompts.yaml has zero cases — golden suite is empty")
	}
	if len(suite.Cases) < 20 {
		t.Errorf("routing-prompts.yaml has only %d cases; pivot DRR predicts the suite grows to 30+",
			len(suite.Cases))
	}
}

// TestRoutingHeuristicMeetsThreshold runs the golden suite against the
// embedded skill descriptions and verifies the pass rate meets the
// threshold declared in dec-20260525-v8-architecture-pivot (>=70%).
// Failure means either:
//   - a skill description has drifted in a way that weakens auto-trigger
//   - the golden case set has grown stale relative to current skills
//   - the heuristic matcher has a bug
//
// Whichever — operator investigates via `haft check routing`.
func TestRoutingHeuristicMeetsThreshold(t *testing.T) {
	var suite routingTestSuite
	if err := yaml.Unmarshal(embeddedRoutingPrompts, &suite); err != nil {
		t.Fatalf("parse golden prompts: %v", err)
	}

	keywordsBySkill := make(map[string][]string, len(allSkills))
	for _, sk := range allSkills {
		keywordsBySkill[sk.Name] = extractSkillTriggerKeywords(sk.Content)
	}

	passes := 0
	for _, tc := range suite.Cases {
		if predictSkillForPrompt(tc.Prompt, keywordsBySkill) == tc.Expected {
			passes++
		}
	}

	total := len(suite.Cases)
	passRate := float64(passes) / float64(total) * 100

	const threshold = 70.0
	if passRate < threshold {
		t.Fatalf("routing heuristic pass rate %.1f%% (%d/%d) is below %.1f%% threshold from pivot DRR",
			passRate, passes, total, threshold)
	}
	t.Logf("routing heuristic: %d/%d cases pass (%.1f%% — threshold %.1f%%)",
		passes, total, passRate, threshold)
}

// TestExtractSkillTriggerKeywordsNonEmpty guards that every embedded
// skill produces at least one keyword. A skill that yields zero
// keywords cannot route under the heuristic — its description is
// either malformed or too vague to be auto-triggerable.
func TestExtractSkillTriggerKeywordsNonEmpty(t *testing.T) {
	for _, sk := range allSkills {
		kw := extractSkillTriggerKeywords(sk.Content)
		if len(kw) == 0 {
			t.Errorf("skill %q produced zero trigger keywords — description too vague?", sk.Name)
		}
	}
}
