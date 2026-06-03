package artifact

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// UmbrellaTrigger is one entry in the wording-use trigger registry: a
// vague word that pulls weight without precision, the kind of precise
// recovery it demands, and the overread it must NOT silently carry.
// Sourced from FPF E.10:0.2b (Wording-Use Trigger Check Registry) and
// the CHR-12 umbrella-word families. The list is intentionally curated
// for HIGH PRECISION — a noisy scanner trains agents to ignore it, so
// only genuinely load-bearing-but-vague engineering words are included.
type UmbrellaTrigger struct {
	Family    string
	RecoverTo string
	MustNot   string
	Words     []string // lowercase single tokens (hyphen allowed, no spaces)
}

// UmbrellaHit is one detected trigger word in framed text.
type UmbrellaHit struct {
	Word      string
	Family    string
	RecoverTo string
	MustNot   string
}

// umbrellaRegistry is the seeded trigger set (EN + RU surface forms).
// Each family collapses several kinds; the agent must split them per
// FPF CHR-11/CHR-12 before the frame is verifiable.
var umbrellaRegistry = []UmbrellaTrigger{
	{
		Family:    "quality",
		RecoverTo: "a named Characteristic + who evaluates + scale + evidence",
		MustNot:   "vague praise or proof-of-success",
		Words:     []string{"quality", "high-quality", "качество", "качественно", "качественный"},
	},
	{
		Family:    "reliability",
		RecoverTo: "a failure mode + a measurable threshold (survives X, p99 under Y)",
		MustNot:   "generally sturdy",
		Words:     []string{"robust", "robustness", "reliable", "надёжно", "надежно", "надёжный", "надежный"},
	},
	{
		Family:    "scalability",
		RecoverTo: "a scale variable + window + target (req/s, data size, concurrency)",
		MustNot:   "handles more, somehow",
		Words:     []string{"scalable", "scalability", "масштабируемый", "масштабируемость"},
	},
	{
		Family:    "performance",
		RecoverTo: "a metric + a number (latency p95 ms, throughput/s)",
		MustNot:   "feels fast",
		Words:     []string{"fast", "faster", "performant", "performance", "быстро", "быстрый", "производительность"},
	},
	{
		Family:    "simplicity",
		RecoverTo: "a concrete property (fewer deps, lower cyclomatic, fewer steps)",
		MustNot:   "aesthetically nicer",
		Words:     []string{"clean", "cleaner", "simple", "simpler", "elegant", "чисто", "просто", "проще"},
	},
	{
		Family:    "maintainability",
		RecoverTo: "which change must become cheap + how that is measured",
		MustNot:   "future-proof in general",
		Words:     []string{"maintainable", "flexible", "extensible", "гибкий", "поддерживаемый", "расширяемый"},
	},
	{
		Family:    "readiness",
		RecoverTo: "an observable acceptance condition checkable now",
		MustNot:   "feels finished",
		Words:     []string{"ready", "done", "готово", "готов", "готовый"},
	},
	{
		Family:    "improvement",
		RecoverTo: "better on WHICH dimension, versus what baseline",
		MustNot:   "generally better",
		Words:     []string{"better", "improve", "improved", "optimize", "лучше", "улучшить", "оптимизировать"},
	},
	{
		Family:    "security",
		RecoverTo: "a threat model + a specific property (authN, injection, at-rest encryption)",
		MustNot:   "hardened in general",
		Words:     []string{"secure", "security", "безопасно", "безопасный", "безопасность"},
	},
	{
		Family:    "modernity",
		RecoverTo: "the concrete change (which library version, which pattern)",
		MustNot:   "newer is better",
		Words:     []string{"modern", "modernize", "современный"},
	},
}

// umbrellaTokens lowercases s and splits it into word tokens on any
// non-letter rune (hyphen kept so "high-quality" stays one token).
// Unicode-aware so Cyrillic forms tokenize correctly.
func umbrellaTokens(s string) map[string]bool {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && r != '-'
	})
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		set[f] = true
	}
	return set
}

// ScanUmbrellaTerms reports the umbrella/trigger words present across the
// given texts (e.g. a ProblemCard title, signal, acceptance). Pure — no
// side effects. Deduplicated and sorted for deterministic output.
func ScanUmbrellaTerms(texts ...string) []UmbrellaHit {
	tokens := umbrellaTokens(strings.Join(texts, " "))
	seen := map[string]bool{}
	hits := []UmbrellaHit{}
	for _, trig := range umbrellaRegistry {
		for _, w := range trig.Words {
			if tokens[w] && !seen[w] {
				seen[w] = true
				hits = append(hits, UmbrellaHit{
					Word:      w,
					Family:    trig.Family,
					RecoverTo: trig.RecoverTo,
					MustNot:   trig.MustNot,
				})
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Word < hits[j].Word })
	return hits
}

// UmbrellaWarning renders a soft, advisory warning for any umbrella words
// in the given texts, or "" when the frame is clean. Advisory only — it
// never blocks (Transformer Mandate: the agent self-corrects, the human
// stays final authority).
func UmbrellaWarning(texts ...string) string {
	hits := ScanUmbrellaTerms(texts...)
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("⚠ Umbrella words in this frame (FPF E.10 wording-use precision):\n")
	for _, h := range hits {
		b.WriteString(fmt.Sprintf("  • %q — recover to %s; must not mean %s\n", h.Word, h.RecoverTo, h.MustNot))
	}
	b.WriteString("Ground each in a measurable term before exploring, or haft_query(action=\"resolve_term\", term=\"...\").\n")
	return b.String()
}
