// Package textsearch is a pure, deterministic leaf for lexical search-term
// preparation shared by both retrieval lanes — the code-symbol store
// (internal/codebase) and the reasoning-artifact store (internal/artifact).
//
// It is the phase-1 core of dec-20260604-3aaad199 (staged retrieval roadmap):
// codegraph-parity lexical heuristics — identifier splitting, suffix stemming,
// stop-word filtering, field-qualified query parsing, and bounded edit distance
// — ported from codegraph and adapted to Go. No embeddings, no second runtime;
// every function here is pure and order-stable so retrieval stays reproducible.
//
// Functional core / imperative shell: this package has zero I/O and zero
// dependency on the stores that consume it — it only transforms strings.
package textsearch

import (
	"regexp"
	"strings"
)

// minTermLen is the shortest token kept as a search term. Mirrors codegraph:
// 1-2 char fragments are noise that match almost everything.
const minTermLen = 3

// Options tunes term extraction. No default-parameter magic — callers pass an
// explicit Options value (the reasoning lane disables Stems because its FTS5
// index is already porter-stemmed; the symbol lane enables them).
type Options struct {
	// Stems adds suffix-stem variants of each token (caching -> cache). Leave
	// false when the downstream index already stems (FTS5 porter tokenizer).
	Stems bool
}

// camelBoundary inserts a split point between a lower/digit and an upper:
// getUser -> get User. acronymBoundary splits an acronym run from a following
// word: HTTPServer -> HTTP Server. Both are package-level so they compile once.
var (
	camelBoundary   = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	acronymBoundary = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	nonAlnum        = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

// SplitIdentifier breaks a single token into its lowercase word parts across
// camelCase, PascalCase, snake_case, SCREAMING_SNAKE, dot.notation and
// kebab-case. It does NOT include the joined compound — that is Terms's job —
// so the result is purely the constituent words, order-stable and lowercased.
//
//	"getUserName"   -> [get user name]
//	"HTTPServer"    -> [http server]
//	"scrape_loop"   -> [scrape loop]
//	"pkg.DoThing"   -> [pkg do thing]
func SplitIdentifier(s string) []string {
	spaced := camelBoundary.ReplaceAllString(s, "$1 $2")
	spaced = acronymBoundary.ReplaceAllString(spaced, "$1 $2")
	parts := nonAlnum.Split(spaced, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, strings.ToLower(p))
	}
	return out
}

// isCompound reports whether a token carries an internal structure worth
// preserving as a joined form — an internal case change, a digit-letter mix, or
// a separator. A plain lowercase word ("authenticate") is not a compound.
func isCompound(token string) bool {
	if strings.ContainsAny(token, "_.-") {
		return true
	}
	// An uppercase letter anywhere after the first character signals a
	// camelCase / PascalCase / acronym boundary (getUser, OrgStore, HTTP).
	for _, r := range token[min(1, len(token)):] {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

// compactCompound is the joined, separator-free, lowercase form of a token:
// "get_user.Name" -> "getusername". Lets a substring index match the whole
// identifier even when the query used separators the symbol does not.
func compactCompound(token string) string {
	var b strings.Builder
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		}
	}
	return b.String()
}

// Terms extracts the deterministic, de-duplicated, order-stable set of lexical
// terms from a raw query. For each whitespace token it: preserves the joined
// compound form (when the token is compound), adds each split word part, drops
// stop-words and sub-minTermLen fragments, and — when opts.Stems — appends
// suffix-stem variants. Insertion order is preserved so callers get stable,
// reproducible expansion (no map iteration leaking into results).
func Terms(query string, opts Options) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 8)

	add := func(tok string) {
		if len(tok) < minTermLen || IsStopWord(tok) {
			return
		}
		if _, dup := seen[tok]; dup {
			return
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}

	for token := range strings.FieldsSeq(query) {
		if isCompound(token) {
			add(compactCompound(token))
		}
		for _, part := range SplitIdentifier(token) {
			add(part)
		}
	}

	if opts.Stems {
		// Snapshot first: stem only the base terms, never stems of stems.
		base := append([]string(nil), out...)
		for _, t := range base {
			for _, v := range StemVariants(t) {
				add(v)
			}
		}
	}

	return out
}
