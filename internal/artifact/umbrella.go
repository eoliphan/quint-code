package artifact

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// UmbrellaTrigger is one entry in the wording-use trigger registry: a vague
// word that pulls weight without precision, the kind of precise recovery it
// demands, and the overread it must NOT silently carry. The registry itself
// lives in umbrella_triggers.json (embedded below) so it can be extended with
// new words or whole languages without touching this logic — and so the
// non-English surface forms are not scanned by the source-level misspell
// linter. Sourced from FPF E.10:0.2b (Wording-Use Trigger Check Registry) and
// the CHR-12 umbrella-word families.
type UmbrellaTrigger struct {
	Family    string   `json:"family"`
	RecoverTo string   `json:"recover_to"`
	MustNot   string   `json:"must_not"`
	Words     []string `json:"words"`
}

// UmbrellaHit is one detected trigger word in framed text.
type UmbrellaHit struct {
	Word      string
	Family    string
	RecoverTo string
	MustNot   string
}

//go:embed umbrella_triggers.json
var umbrellaTriggersJSON []byte

// umbrellaRegistry is the parsed trigger set, loaded once from the embedded
// JSON at process start.
var umbrellaRegistry []UmbrellaTrigger

func init() {
	var doc struct {
		Families []UmbrellaTrigger `json:"families"`
	}
	if err := json.Unmarshal(umbrellaTriggersJSON, &doc); err != nil {
		panic(fmt.Sprintf("artifact: invalid umbrella_triggers.json: %v", err))
	}
	umbrellaRegistry = doc.Families
}

// umbrellaTokens lowercases s and splits it into word tokens on any non-letter
// rune (hyphen kept so "high-quality" stays one token). Unicode-aware so
// Cyrillic and accented Latin forms tokenize correctly.
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

// ScanUmbrellaTerms reports the umbrella/trigger words present across the given
// texts (e.g. a ProblemCard title, signal, acceptance). Pure — no side
// effects. Deduplicated and sorted for deterministic output.
func ScanUmbrellaTerms(texts ...string) []UmbrellaHit {
	tokens := umbrellaTokens(strings.Join(texts, " "))
	seen := map[string]bool{}
	hits := []UmbrellaHit{}
	for _, trig := range umbrellaRegistry {
		for _, w := range trig.Words {
			lw := strings.ToLower(w)
			if tokens[lw] && !seen[lw] {
				seen[lw] = true
				hits = append(hits, UmbrellaHit{
					Word:      lw,
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

// UmbrellaWarning renders a soft, advisory warning for any umbrella words in
// the given texts, or "" when the frame is clean. Advisory only — it never
// blocks (Transformer Mandate: the agent self-corrects, the human stays final
// authority).
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
