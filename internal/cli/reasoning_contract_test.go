package cli

import (
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/agent"
)

func TestReasoningSourcesShareCanonicalInteractionMatrix(t *testing.T) {
	t.Parallel()

	prompt := agent.BuildSystemPrompt(agent.PromptConfig{
		ProjectRoot: "/repo",
		Cwd:         "/repo",
		Lemniscate:  true,
	})
	commandBytes, err := embeddedCommands.ReadFile("commands/h-compare.md")
	if err != nil {
		t.Fatalf("read embedded compare command: %v", err)
	}
	command := string(commandBytes)

	// Drift detector across the two interaction-matrix carriers: the
	// agent system prompt and the canonical /h-compare command body.
	// The previous third source (embedded h-reason skill) was dropped
	// in the v8 governance substrate pivot — h-fpf is narrow umbrella,
	// h-decide carries decision-specific procedure, and per-skill body
	// content diverges by design. The interaction matrix lives in the
	// prompt; commands cite it to stay coherent.
	sources := map[string]string{
		"prompt":    prompt,
		"h-compare": command,
	}

	required := []string{
		`Direct response / direct action`,
		`Research / prepare-and-wait`,
		`Delegated reasoning`,
		`Autonomous execution`,
	}

	for name, content := range sources {
		for _, want := range required {
			if !strings.Contains(content, want) {
				t.Fatalf("%s missing %q", name, want)
			}
		}
	}
}

func TestReasoningSourcesRejectKnownContradictoryPhrases(t *testing.T) {
	t.Parallel()

	prompt := agent.BuildSystemPrompt(agent.PromptConfig{
		ProjectRoot: "/repo",
		Cwd:         "/repo",
		Lemniscate:  true,
	})
	commandBytes, err := embeddedCommands.ReadFile("commands/h-compare.md")
	if err != nil {
		t.Fatalf("read embedded compare command: %v", err)
	}
	command := string(commandBytes)

	// Drift detector across the two interaction-matrix carriers: the
	// agent system prompt and the canonical /h-compare command body.
	// The previous third source (embedded h-reason skill) was dropped
	// in the v8 governance substrate pivot — h-fpf is narrow umbrella,
	// h-decide carries decision-specific procedure, and per-skill body
	// content diverges by design. The interaction matrix lives in the
	// prompt; commands cite it to stay coherent.
	sources := map[string]string{
		"prompt":    prompt,
		"h-compare": command,
	}

	forbidden := []string{
		`Path 3`,
		`Path 4`,
		`Path 5`,
		`"давай" / "do it" / "go ahead" = START WORKING`,
		`After frame and after explore: STOP and present your work. Wait for user.`,
		"`/h-frame` → `/h-decide`",
		"tactical skips exploration",
		"Tactical mode may skip some artifacts",
	}

	for name, content := range sources {
		for _, bad := range forbidden {
			if strings.Contains(content, bad) {
				t.Fatalf("%s still contains %q", name, bad)
			}
		}
	}
}
