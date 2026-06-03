package present

import (
	"fmt"
	"strings"

	"github.com/m0n0x41d/haft/internal/codebase"
	"github.com/m0n0x41d/haft/internal/codeintel"
)

// FlowResponse renders a callers / callees / impact traversal fused with the
// reasoning graph. action is "callers" | "callees" | "impact". Every reached
// symbol shows BOTH its code relationship (call vs interface_dispatch, distance,
// heuristic provenance) AND its governance — symbol-level decisions, or the
// honest module-level fallback so a governed node never renders blank.
func FlowResponse(res codeintel.FlowResult, action, seedName string) string {
	var b strings.Builder

	// Ambiguous seed: surface candidates, never silently pick (keystone discipline).
	if len(res.Ambiguous) > 0 {
		fmt.Fprintf(&b, "## %s — `%s` is ambiguous\n\n", titleFor(action), seedName)
		fmt.Fprintf(&b, "%d symbols share this name. Re-query with `file` (and `line`) to disambiguate:\n\n", len(res.Ambiguous))
		for _, c := range res.Ambiguous {
			fmt.Fprintf(&b, "- `%s:%d`%s\n", c.FilePath, c.StartLine, receiverSuffix(c))
		}
		return b.String()
	}

	if !res.SeedFound {
		fmt.Fprintf(&b, "## %s — `%s` not found\n\n", titleFor(action), seedName)
		b.WriteString("No symbol with that name is in the code index. Check spelling, or pass `file` to scope it.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "## %s of `%s` — %s:%d (depth %d, %s)\n\n",
		titleFor(action), res.Seed.Name, res.Seed.FilePath, res.Seed.StartLine, res.Depth, directionWord(res.Direction))

	governed := 0
	for _, h := range res.Hops {
		if h.Governed() {
			governed++
		}
	}
	fmt.Fprintf(&b, "%d reached • %d carry recorded reasoning\n\n", len(res.Hops), governed)

	if len(res.Hops) == 0 {
		b.WriteString("Nothing reached at this depth — a leaf in this direction.\n")
		return b.String()
	}

	for _, h := range res.Hops {
		renderFusedHop(&b, h)
	}

	if res.ColdBuilt {
		b.WriteString("\n_(code index built on first query; subsequent queries are warm.)_\n")
	}
	return b.String()
}

// renderFusedHop prints one reached symbol: its code relationship, then its
// governance (symbol-level decisions, else module-level, else explicitly none).
func renderFusedHop(b *strings.Builder, h codeintel.FusedHop) {
	via := string(h.ViaKind)
	if h.Provenance == codebase.ProvenanceHeuristic {
		via += " ⚠ heuristic"
	}
	fmt.Fprintf(b, "- **%s** `%s:%d` _(%s, d%d)_\n", h.Symbol.Name, h.Symbol.FilePath, h.Symbol.StartLine, via, h.Distance)

	cc := h.Context
	switch {
	case len(cc.Decisions) > 0:
		for _, d := range cc.Decisions {
			fmt.Fprintf(b, "    governs: **%s** `%s`\n", d.Meta.Title, d.Meta.ID)
		}
		if cc.SymbolGranularity != "" && cc.SymbolGranularity != "line-precise" {
			fmt.Fprintf(b, "    _granularity: %s_\n", cc.SymbolGranularity)
		}
	case len(cc.ModuleDecisions) > 0:
		fmt.Fprintf(b, "    module governed by %s _(no symbol-level link)_\n", moduleDecisionList(cc.ModuleDecisions))
	default:
		b.WriteString("    — no recorded reasoning\n")
	}
}

func titleFor(action string) string {
	switch action {
	case "impact":
		return "Impact"
	case "callers":
		return "Callers"
	default:
		return "Callees"
	}
}

func directionWord(dir codeintel.Direction) string {
	if dir == codeintel.Callers {
		return "inbound"
	}
	return "outbound"
}

func receiverSuffix(c codebase.CodeSymbol) string {
	if c.Receiver != "" {
		return fmt.Sprintf(" — receiver `%s`", c.Receiver)
	}
	return ""
}
