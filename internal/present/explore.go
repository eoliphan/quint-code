package present

import (
	"fmt"
	"strings"

	"github.com/m0n0x41d/haft/internal/codeintel"
	"github.com/m0n0x41d/haft/internal/contextgraph"
)

// ExploreResponse renders the capstone single-call view: the fused call-chain
// spine, the blast radius (who breaks + covering decisions), and verbatim seed
// source. The goal is sufficiency — the agent answers "how does this flow and
// what was decided about it" without further reads. Honest: a chain that stops
// at a dispatch boundary says so rather than implying completeness.
func ExploreResponse(res codeintel.ExploreResult, seedName, lang string) string {
	var b strings.Builder

	if len(res.Ambiguous) > 0 {
		fmt.Fprintf(&b, "## Explore — `%s` is ambiguous\n\n", seedName)
		fmt.Fprintf(&b, "%d symbols share this name. Re-query with `file` (and `line`):\n\n", len(res.Ambiguous))
		for _, c := range res.Ambiguous {
			fmt.Fprintf(&b, "- `%s:%d`%s\n", c.FilePath, c.StartLine, receiverSuffix(c))
		}
		return b.String()
	}
	if !res.SeedFound {
		fmt.Fprintf(&b, "## Explore — `%s` not found\n\n", seedName)
		b.WriteString("No symbol with that name is in the code index. Check spelling, or pass `file` to scope it.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "## Explore `%s` — %s:%d\n\n", res.Seed.Name, res.Seed.FilePath, res.Seed.StartLine)

	renderChain(&b, res)
	renderBlastRadius(&b, res.BlastRadius)
	renderSeedSource(&b, res, lang)

	if res.ColdBuilt {
		b.WriteString("_(code index built on first query; subsequent queries are warm.)_\n")
	}
	return b.String()
}

func renderChain(b *strings.Builder, res codeintel.ExploreResult) {
	fmt.Fprintf(b, "### Flow — spine of %d symbol(s)", len(res.Chain))
	if res.BridgesUsed > 0 {
		fmt.Fprintf(b, ", crosses %d interface boundary", res.BridgesUsed)
	}
	b.WriteString("\n")

	for _, step := range res.Chain {
		arrow := "•"
		if step.Distance > 0 {
			arrow = "→"
		}
		via := ""
		if step.Distance > 0 {
			via = fmt.Sprintf(" _(%s", step.ViaKind)
			if step.Bridge() {
				via += " ⚠ heuristic boundary"
			}
			via += ")_"
		}
		recv := ""
		if step.Symbol.Receiver != "" {
			recv = fmt.Sprintf("(%s).", step.Symbol.Receiver)
		}
		fmt.Fprintf(b, "%s **%s%s** `%s:%d`%s\n", arrow, recv, step.Symbol.Name, step.Symbol.FilePath, step.Symbol.StartLine, via)
		renderChainGovernance(b, step.Context)
	}
	if res.UnresolvedEnd {
		b.WriteString("⚠ chain ends at an unresolved dispatch boundary — the flow continues through a dynamic call the static graph cannot resolve. Not shown rather than guessed.\n")
	}
	b.WriteString("\n")
}

// renderChainGovernance is the per-symbol fusion on the spine — the moat: code
// flow interleaved with the reasoning governing each step.
func renderChainGovernance(b *strings.Builder, cc contextgraph.CodeContext) {
	switch {
	case len(cc.Decisions) > 0:
		titles := make([]string, 0, len(cc.Decisions))
		for _, d := range cc.Decisions {
			titles = append(titles, fmt.Sprintf("%s `%s`", d.Meta.Title, d.Meta.ID))
		}
		fmt.Fprintf(b, "    ⮡ governed: %s\n", strings.Join(titles, "; "))
	case len(cc.ModuleDecisions) > 0:
		fmt.Fprintf(b, "    ⮡ module governed by %s\n", moduleDecisionList(cc.ModuleDecisions))
	}
	for _, inv := range cc.Invariants {
		fmt.Fprintf(b, "    ⮡ invariant: %s _(from %s)_\n", inv.Text, inv.DecisionTitle)
	}
}

func renderBlastRadius(b *strings.Builder, callers []codeintel.FusedHop) {
	if len(callers) == 0 {
		b.WriteString("### Blast radius\nNo direct callers in the index — a leaf or entry point.\n\n")
		return
	}
	governed := 0
	for _, h := range callers {
		if h.Governed() {
			governed++
		}
	}
	fmt.Fprintf(b, "### Blast radius — %d direct caller(s), %d governed\n", len(callers), governed)
	for _, h := range callers {
		fmt.Fprintf(b, "- `%s` (%s:%d)", h.Symbol.Name, h.Symbol.FilePath, h.Symbol.StartLine)
		switch {
		case len(h.Context.Decisions) > 0:
			b.WriteString(" — governed: ")
			ids := make([]string, 0, len(h.Context.Decisions))
			for _, d := range h.Context.Decisions {
				ids = append(ids, d.Meta.ID)
			}
			b.WriteString(strings.Join(ids, ", "))
		case len(h.Context.ModuleDecisions) > 0:
			b.WriteString(" — module governed")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func renderSeedSource(b *strings.Builder, res codeintel.ExploreResult, lang string) {
	b.WriteString("### Seed source\n")
	if !res.SeedBodyOK {
		b.WriteString("⚠ source could not be verified byte-exact against disk — not shown to avoid stale source.\n\n")
		return
	}
	fmt.Fprintf(b, "```%s\n%s\n```\n\n", lang, res.SeedBody)
}
