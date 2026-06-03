package present

import (
	"fmt"
	"strings"

	"github.com/m0n0x41d/haft/internal/artifact"
	"github.com/m0n0x41d/haft/internal/contextgraph"
)

// CodeContextResponse renders the fused "what haft knows about this code" view
// for an agent exploring a file or symbol — decisions, problems, solution
// variants, notes, and invariants that touch it, plus module coverage. Pure
// formatting; no IO.
func CodeContextResponse(cc contextgraph.CodeContext) string {
	var b strings.Builder

	target := cc.Target.File
	if cc.Target.Symbol != "" {
		target = fmt.Sprintf("%s :: %s", cc.Target.File, cc.Target.Symbol)
	}
	fmt.Fprintf(&b, "## Code context — %s\n\n", target)

	if cc.Module != "" {
		status := "blind — no decisions govern this module"
		if cc.Governed {
			status = "governed"
		}
		fmt.Fprintf(&b, "Module `%s`: %s\n\n", cc.Module, status)
	}

	if cc.Target.Symbol != "" && cc.SymbolGranularity != "" {
		fmt.Fprintf(&b, "Symbol governance granularity: %s\n\n", cc.SymbolGranularity)
	}

	if cc.Empty() {
		b.WriteString("No recorded reasoning touches this code yet — nothing decided, framed, or noted here.\n")
		b.WriteString("About to make a non-trivial change? Consider /h-frame or /h-note so the next reader sees why.\n")
		return b.String()
	}

	renderContextArtifacts(&b, "Decisions governing this code", cc.Decisions)
	renderContextArtifacts(&b, "Problems framed around it", cc.Problems)
	renderContextArtifacts(&b, "Solution variants explored", cc.Portfolios)
	renderContextArtifacts(&b, "Notes", cc.Notes)

	if len(cc.Invariants) > 0 {
		b.WriteString("### Invariants that must hold here\n")
		for _, inv := range cc.Invariants {
			fmt.Fprintf(&b, "- %s _(from %s)_\n", inv.Text, inv.DecisionTitle)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderContextArtifacts lists a kind-group, pairing every artifact ID with
// its human-readable title (FPF A.7 re-grounding) and flagging non-active
// status.
func renderContextArtifacts(b *strings.Builder, heading string, items []*artifact.Artifact) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "### %s\n", heading)
	for _, a := range items {
		suffix := ""
		if a.Meta.Status != artifact.StatusActive {
			suffix = fmt.Sprintf(" [%s]", a.Meta.Status)
		}
		fmt.Fprintf(b, "- **%s** `%s`%s\n", a.Meta.Title, a.Meta.ID, suffix)
	}
	b.WriteString("\n")
}
