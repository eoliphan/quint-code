package artifact

import (
	"fmt"
	"sort"
	"strings"
)

// reputationSignals are social-proof phrases that, per FPF E.9.DA:4.4b,
// are NOT decision-adequacy evidence: popularity, adoption, standard
// status, star counts, and "best practice" appeals do not justify a
// choice by themselves — only the content property that makes the option
// right for THIS problem does. Substring-matched (case-insensitive) so
// multi-word phrases and RU morphological stems both catch. Curated for
// HIGH PRECISION — a noisy flag trains agents to ignore it.
var reputationSignals = []string{
	// EN
	"popular",
	"industry standard",
	"industry-standard",
	"de facto standard",
	"de-facto standard",
	"everyone uses",
	"everybody uses",
	"widely used",
	"widely adopted",
	"most-starred",
	"most stars",
	"best practice",
	// RU (stems for morphology)
	"популярн",
	"общепринят",
	"широко использ",
	"стандарт индустрии",
	"индустриальный стандарт",
	"все используют",
	"все пользуются",
	"де-факто",
	"лучшая практика",
	"лучшие практики",
}

// ScanReputationSignals reports the reputation/social-proof phrases present
// across the given rationale texts. Pure, deduplicated, sorted.
func ScanReputationSignals(texts ...string) []string {
	hay := strings.ToLower(strings.Join(texts, " \n "))
	seen := map[string]bool{}
	out := []string{}
	for _, sig := range reputationSignals {
		if strings.Contains(hay, sig) && !seen[sig] {
			seen[sig] = true
			out = append(out, sig)
		}
	}
	sort.Strings(out)
	return out
}

// ReputationWarning renders a soft, advisory warning when a decision's
// rationale leans on reputation instead of content, or "" when clean.
// Advisory only — it never blocks (Transformer Mandate).
func ReputationWarning(texts ...string) string {
	hits := ScanReputationSignals(texts...)
	if len(hits) == 0 {
		return ""
	}
	quoted := make([]string, len(hits))
	for i, h := range hits {
		quoted[i] = fmt.Sprintf("%q", h)
	}
	var b strings.Builder
	b.WriteString("⚠ Reputation signals in this rationale (FPF E.9.DA:4.4b — popularity is not decision evidence):\n")
	b.WriteString(fmt.Sprintf("  • detected: %s\n", strings.Join(quoted, ", ")))
	b.WriteString("  Adoption / standard-status / popularity does not justify a choice by itself. State the CONTENT reason — the property of this option that makes it right for THIS problem (throughput need, ordering guarantee, ops familiarity, constraint fit).\n")
	return b.String()
}
