package cli

import (
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/project"
)

// The code-graph doctrine must ship in the MCP instructions even when no
// workflow policy exists — otherwise the agent is never told the graph exists.
func TestComposeServerInstructions_AlwaysIncludesDoctrine(t *testing.T) {
	mustHave := []string{"haft code graph", "explore", "code_context", "impact", "BEFORE"}

	got := composeServerInstructions(nil)
	for _, s := range mustHave {
		if !strings.Contains(got, s) {
			t.Errorf("doctrine (no workflow) missing %q:\n%s", s, got)
		}
	}

	// The mandatory session-start status rule must be present AND first, via the
	// tool (no skill reference — the action returns the governance state directly).
	for _, s := range []string{"MANDATORY FIRST ACTION", `haft_query(action="status")`} {
		if !strings.Contains(got, s) {
			t.Errorf("session-start mandate missing %q:\n%s", s, got)
		}
	}
	if strings.Contains(got, "/h-status") {
		t.Errorf("mandate should reference the tool, not the skill:\n%s", got)
	}
	if strings.Index(got, "MANDATORY FIRST ACTION") > strings.Index(got, "haft code graph") {
		t.Errorf("the session-start mandate must come BEFORE the code-graph doctrine:\n%s", got)
	}

	w := &project.Workflow{Intent: "prefer small reversible changes"}
	withWf := composeServerInstructions(w)
	if !strings.Contains(withWf, "Project Workflow") || !strings.Contains(withWf, "prefer small reversible changes") {
		t.Errorf("workflow policy must be preserved ahead of the doctrine:\n%s", withWf)
	}
	if !strings.Contains(withWf, "haft code graph") {
		t.Errorf("doctrine must still be appended after the workflow policy:\n%s", withWf)
	}
	// Workflow policy comes first, doctrine after.
	if strings.Index(withWf, "Project Workflow") > strings.Index(withWf, "haft code graph") {
		t.Errorf("workflow policy should precede the code-graph doctrine:\n%s", withWf)
	}
}
