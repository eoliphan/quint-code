package present

import (
	"fmt"
	"strings"

	"github.com/m0n0x41d/haft/internal/codeintel"
	"github.com/m0n0x41d/haft/internal/contextgraph"
)

// NodeResponse renders haft_node: every overload of a symbol name, each with its
// freshness-revalidated body, the reasoning fused onto exactly it, an immediate
// caller/callee trail, and (for container types) a member outline. The body is
// only shown when verified byte-exact against disk — a failed verification says
// so rather than printing possibly-stale source.
func NodeResponse(view codeintel.NodeView, lang string) string {
	var b strings.Builder

	if !view.Found {
		fmt.Fprintf(&b, "## Node — `%s` not found\n\n", view.Name)
		b.WriteString("No symbol with that name is in the code index. Check spelling, or pass `file` to scope it.\n")
		return b.String()
	}

	plural := ""
	if len(view.Overloads) != 1 {
		plural = "s"
	}
	fmt.Fprintf(&b, "## Node `%s` — %d definition%s\n\n", view.Name, len(view.Overloads), plural)

	for i := range view.Overloads {
		renderOverload(&b, view.Overloads[i], lang)
	}
	if view.ColdBuilt {
		b.WriteString("_(code index built on first query; subsequent queries are warm.)_\n")
	}
	return b.String()
}

func renderOverload(b *strings.Builder, ol codeintel.NodeOverload, lang string) {
	sym := ol.Symbol
	recv := ""
	if sym.Receiver != "" {
		recv = fmt.Sprintf(" (%s)", sym.Receiver)
	}
	fmt.Fprintf(b, "### %s%s — `%s:%d-%d` _(%s)_\n", sym.Name, recv, sym.FilePath, sym.StartLine, sym.EndLine, sym.Kind)
	if ol.Reindexed {
		b.WriteString("_file was edited — re-indexed for fresh byte offsets before slicing_\n")
	}

	renderHopGovernance(b, ol.Context)
	renderTrail(b, "Called by", ol.Callers)
	renderTrail(b, "Calls", ol.Callees)

	if len(ol.Members) > 0 {
		b.WriteString("\nMembers:\n")
		for _, m := range ol.Members {
			fmt.Fprintf(b, "- %s `%s:%d`\n", m.Name, m.FilePath, m.StartLine)
		}
	}

	b.WriteString("\n")
	if !ol.BodyOK {
		b.WriteString("⚠ source could not be verified byte-exact against disk (file changing, or symbol removed) — not shown to avoid stale source. Re-run.\n\n")
		return
	}
	fmt.Fprintf(b, "```%s\n%s\n```\n\n", lang, ol.Body)
}

// renderHopGovernance prints the fusion for one overload, same discipline as the
// flow view: symbol-level decisions, else module-level, else explicitly none.
func renderHopGovernance(b *strings.Builder, cc contextgraph.CodeContext) {
	switch {
	case len(cc.Decisions) > 0:
		b.WriteString("Governed by:\n")
		for _, d := range cc.Decisions {
			fmt.Fprintf(b, "- **%s** `%s`\n", d.Meta.Title, d.Meta.ID)
		}
		if cc.SymbolGranularity != "" && cc.SymbolGranularity != "line-precise" {
			fmt.Fprintf(b, "_granularity: %s_\n", cc.SymbolGranularity)
		}
	case len(cc.ModuleDecisions) > 0:
		fmt.Fprintf(b, "Module governed by %s _(no symbol-level link)_\n", moduleDecisionList(cc.ModuleDecisions))
	default:
		b.WriteString("No recorded reasoning touches this definition.\n")
	}
}

func renderTrail(b *strings.Builder, label string, hops []codeintel.FusedHop) {
	if len(hops) == 0 {
		return
	}
	parts := make([]string, 0, len(hops))
	for _, h := range hops {
		via := ""
		if h.ViaKind == "interface_dispatch" {
			via = " via dispatch"
		}
		parts = append(parts, fmt.Sprintf("`%s` (%s:%d%s)", h.Symbol.Name, h.Symbol.FilePath, h.Symbol.StartLine, via))
	}
	fmt.Fprintf(b, "%s: %s\n", label, strings.Join(parts, ", "))
}
