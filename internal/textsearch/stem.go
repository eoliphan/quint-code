package textsearch

import "strings"

// stopWords are dropped from queries before matching: generic English plus
// code-noise that would otherwise match almost everything. Ported from
// codegraph and trimmed to avoid filtering common symbol verbs (get/set/add/
// build/find/list stay searchable). Lower-case keys only.
var stopWords = func() map[string]struct{} {
	words := []string{
		// English
		"the", "a", "an", "and", "or", "but", "in", "on", "at", "to", "for",
		"of", "with", "by", "from", "is", "it", "that", "this", "are", "was",
		"be", "has", "had", "have", "do", "does", "did", "will", "would", "could",
		"should", "may", "might", "can", "shall", "not", "no", "all", "each",
		"every", "how", "what", "where", "when", "who", "which", "why",
		"me", "my", "we", "our", "you", "your", "he", "she", "they",
		"show", "give", "tell",
		"been", "done", "made", "used", "using", "work", "works", "found",
		"also", "into", "then", "than", "just", "more", "some", "such",
		"over", "only", "out", "its", "so", "up", "as", "if",
		"look", "need", "needs", "want", "happen", "happens",
		"affect", "affected", "break", "breaks", "failing",
		"implemented", "implement",
		// Code-specific noise. Deliberately excludes get/set/add/build/find/
		// list — those are real symbol names.
		"code", "file", "files", "function", "method", "class", "type",
		"fix", "bug", "called",
	}
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		set[w] = struct{}{}
	}
	return set
}()

// IsStopWord reports whether a lower-cased token is search noise.
func IsStopWord(token string) bool {
	_, ok := stopWords[token]
	return ok
}

// StemVariants returns suffix-stripped variants of a term for FTS prefix
// expansion: "caching" -> [cach cache], "eviction" -> [evict]. Stems are used
// as PREFIX matches downstream, so they need not be real dictionary words. The
// result excludes the original term, sub-minTermLen fragments, and stop-words,
// and is order-stable (no map iteration). Ported from codegraph getStemVariants.
func StemVariants(term string) []string {
	t := strings.ToLower(term)
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	add := func(v string) {
		if len(v) < minTermLen || v == t || IsStopWord(v) {
			return
		}
		if _, dup := seen[v]; dup {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	// -ing: caching -> cach/cache, running -> run
	if strings.HasSuffix(t, "ing") && len(t) > 5 {
		base := t[:len(t)-3]
		add(base)
		add(base + "e")
		if n := len(base); n >= 2 && base[n-1] == base[n-2] {
			add(base[:n-1]) // running -> run (drop doubled consonant)
		}
	}

	// -tion / -sion: eviction -> evict, expression -> express
	if (strings.HasSuffix(t, "tion") || strings.HasSuffix(t, "sion")) && len(t) > 5 {
		add(t[:len(t)-3])
	}

	// -ment: management -> manage
	if strings.HasSuffix(t, "ment") && len(t) > 6 {
		add(t[:len(t)-4])
	}

	// plural family: entries -> entry, processes -> process, errors -> error
	switch {
	case strings.HasSuffix(t, "ies") && len(t) > 4:
		add(t[:len(t)-3] + "y")
	case strings.HasSuffix(t, "es") && len(t) > 4:
		add(t[:len(t)-2])
	case strings.HasSuffix(t, "s") && !strings.HasSuffix(t, "ss") && len(t) > 4:
		add(t[:len(t)-1])
	}

	// -ed: handled -> handle, carried -> carry
	if strings.HasSuffix(t, "ed") && !strings.HasSuffix(t, "eed") && len(t) > 4 {
		add(t[:len(t)-1])
		add(t[:len(t)-2])
		if strings.HasSuffix(t, "ied") && len(t) > 5 {
			add(t[:len(t)-3] + "y")
		}
	}

	// -er: builder -> build/builde, getter -> get
	if strings.HasSuffix(t, "er") && len(t) > 4 {
		base := t[:len(t)-2]
		add(base)
		add(base + "e")
		if n := len(base); n >= 2 && base[n-1] == base[n-2] {
			add(base[:n-1])
		}
	}

	return out
}
