package cli

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

//go:embed testdata/routing-prompts.yaml
var embeddedRoutingPrompts []byte

// routingTestCase is one golden prompt + the skill it should route to
// per the v8 governance substrate skill descriptions.
type routingTestCase struct {
	Prompt    string `yaml:"prompt"`
	Expected  string `yaml:"expected"`
	Rationale string `yaml:"rationale,omitempty"`
}

type routingTestSuite struct {
	Cases []routingTestCase `yaml:"cases"`
}

var checkRoutingCmd = &cobra.Command{
	Use:   "routing",
	Short: "Run golden routing tests against the installed skill descriptions",
	Long: `Sanity-check that the v8 skill descriptions route operator-style
prompts to the expected skill via a heuristic keyword matcher.

This is NOT the real Claude Code / Codex matcher (those defer routing
to the LLM at runtime). It is a CI-friendly drift detector: when a
golden prompt no longer matches its expected skill via the keyword
heuristic, the skill description has probably drifted in a way that
will also weaken real auto-trigger reliability.

Treat failures as "this skill description may need sharper trigger
keywords", not "broken installation".`,
	RunE: runCheckRouting,
}

func init() {
	checkCmd.AddCommand(checkRoutingCmd)
}

func runCheckRouting(_ *cobra.Command, _ []string) error {
	var suite routingTestSuite
	if err := yaml.Unmarshal(embeddedRoutingPrompts, &suite); err != nil {
		return fmt.Errorf("parse golden prompts: %w", err)
	}

	// Build per-skill keyword set from embedded skill descriptions.
	keywordsBySkill := make(map[string][]string, len(allSkills))
	for _, sk := range allSkills {
		keywordsBySkill[sk.Name] = extractSkillTriggerKeywords(sk.Content)
	}

	passes := 0
	failures := []routingFailure{}

	for _, tc := range suite.Cases {
		predicted := predictSkillForPrompt(tc.Prompt, keywordsBySkill)
		if predicted == tc.Expected {
			passes++
			continue
		}
		failures = append(failures, routingFailure{
			Prompt:    tc.Prompt,
			Expected:  tc.Expected,
			Predicted: predicted,
			Rationale: tc.Rationale,
		})
	}

	total := len(suite.Cases)
	passRate := 0.0
	if total > 0 {
		passRate = float64(passes) / float64(total) * 100
	}

	fmt.Printf("haft check routing — %d/%d cases pass (%.1f%%)\n", passes, total, passRate)
	if len(failures) == 0 {
		return nil
	}

	fmt.Println()
	fmt.Println("Failures (heuristic matcher picked different skill than expected):")
	for _, f := range failures {
		fmt.Printf("  prompt:    %q\n", f.Prompt)
		fmt.Printf("  expected:  %s\n", f.Expected)
		fmt.Printf("  predicted: %s\n", f.Predicted)
		if f.Rationale != "" {
			fmt.Printf("  rationale: %s\n", f.Rationale)
		}
		fmt.Println()
	}

	// Threshold per dec-20260525-v8-architecture-pivot prediction
	// "routing reliability >= 70%". Exit non-zero when below.
	if passRate < 70.0 {
		return fmt.Errorf("routing pass rate %.1f%% is below 70%% threshold from pivot DRR", passRate)
	}
	return nil
}

type routingFailure struct {
	Prompt    string
	Expected  string
	Predicted string
	Rationale string
}

// extractSkillTriggerKeywords builds the keyword vocabulary the
// heuristic matcher uses for a skill. Sources, in priority order:
//  1. Comma/space-delimited tokens inside "TRIGGER" / "TRIGGER on:" lines
//     (skills like h-diagnose carry explicit trigger blocks)
//  2. The description frontmatter block (first ~200 chars)
//  3. The skill name itself (h-frame → "frame")
//
// All tokens lowercased and de-stopworded for noise control.
func extractSkillTriggerKeywords(content []byte) []string {
	text := string(content)
	keywords := map[string]struct{}{}

	addToken := func(t string) {
		t = strings.ToLower(strings.TrimSpace(t))
		t = strings.Trim(t, ".,!?;:()[]{}\"'`")
		if len(t) < 3 || isStopword(t) {
			return
		}
		keywords[t] = struct{}{}
	}

	// Pass 1: TRIGGER lines (explicit operator phrases).
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "trigger") && !strings.Contains(lower, "use when") {
			continue
		}
		// Heuristic: pull quoted phrases first; if none, take whole-line tokens.
		if strings.Contains(line, "\"") {
			for _, segment := range strings.Split(line, "\"") {
				if strings.TrimSpace(segment) == "" {
					continue
				}
				for _, tok := range strings.Fields(segment) {
					addToken(tok)
				}
			}
			continue
		}
		for _, tok := range strings.Fields(line) {
			addToken(tok)
		}
	}

	// Pass 2: description frontmatter (between --- markers). YAML
	// multiline values for description / when_to_use continue past
	// the first line, so once we enter such a block, keep consuming
	// indented continuation lines until we hit the next key or the
	// closing fence.
	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end > 0 {
			frontmatter := text[4 : 4+end]
			inDescBlock := false
			for _, line := range strings.Split(frontmatter, "\n") {
				lower := strings.ToLower(line)
				trimmed := strings.TrimSpace(lower)
				if strings.HasPrefix(trimmed, "description") ||
					strings.HasPrefix(trimmed, "when_to_use") {
					inDescBlock = true
					for _, tok := range strings.Fields(line) {
						addToken(tok)
					}
					continue
				}
				if inDescBlock {
					// Continuation lines of a YAML | or > block start
					// with whitespace. Lines that start a new key
					// (no leading space) close the block.
					if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
						for _, tok := range strings.Fields(line) {
							addToken(tok)
						}
						continue
					}
					inDescBlock = false
				}
			}
		}
	}

	// Pass 3 fallback: if the first two passes produced nothing, the
	// skill description likely doesn't use TRIGGER/use-when markers and
	// the frontmatter parser missed continuation lines. Walk the body
	// h2 sections (## headers) and treat their text as the routing
	// surface — coarser but never empty for a published skill.
	if len(keywords) == 0 {
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") {
				continue
			}
			for _, tok := range strings.Fields(line) {
				addToken(tok)
			}
		}
	}

	out := make([]string, 0, len(keywords))
	for k := range keywords {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// predictSkillForPrompt scores each skill by keyword overlap with the
// prompt and returns the best-match skill name. Ties broken by skill
// declared order in allSkills (workflow skills before subroutines so
// generic prompts route to the primary surface).
func predictSkillForPrompt(prompt string, keywordsBySkill map[string][]string) string {
	promptTokens := map[string]struct{}{}
	for _, tok := range strings.Fields(strings.ToLower(prompt)) {
		tok = strings.Trim(tok, ".,!?;:()[]{}\"'`")
		if len(tok) >= 3 && !isStopword(tok) {
			promptTokens[tok] = struct{}{}
		}
	}

	bestSkill := ""
	bestScore := -1
	// Iterate allSkills in declared order for deterministic tie-breaking.
	for _, sk := range allSkills {
		keywords := keywordsBySkill[sk.Name]
		score := 0
		for _, kw := range keywords {
			if _, ok := promptTokens[kw]; ok {
				score++
			}
		}
		// Skill-name token always counts heavily — h-diagnose vs prompt
		// "investigate" needs the name itself to anchor.
		nameRoot := strings.TrimPrefix(sk.Name, "h-")
		if nameRoot != "" {
			if _, ok := promptTokens[nameRoot]; ok {
				score += 3
			}
		}
		if score > bestScore {
			bestScore = score
			bestSkill = sk.Name
		}
	}
	return bestSkill
}

// isStopword filters out generic English words that don't carry
// routing signal. Conservative list — favor missing-stopword over
// stripping a real keyword.
func isStopword(t string) bool {
	switch t {
	case "the", "and", "for", "with", "this", "that", "have", "use", "you",
		"are", "from", "not", "but", "what", "when", "how", "why", "via",
		"description", "when_to_use", "trigger", "name", "skill", "skills",
		"haft", "use:", "should", "could", "would", "their", "they", "them",
		"into", "your", "our", "may", "can", "must", "will", "want", "need",
		"any", "all", "each", "one", "two", "etc", "via:",
		"argument-hint:", "allowed-tools:", "disable-model-invocation:":
		return true
	}
	return false
}
