package textsearch

import "strings"

// ParsedQuery is a raw query split into field-qualified filters plus the
// remaining free text. Filters narrow a result set; Text is what feeds the
// underlying index (FTS5 / substring).
//
//	kind:function name:auth path:internal/cli authenticate
//	  -> Kinds=[function] NameFilters=[auth] PathFilters=[internal/cli] Text="authenticate"
type ParsedQuery struct {
	Text        string   // free-text portion (may be empty)
	Kinds       []string // kind: filters, OR'd, verbatim (caller validates vocabulary)
	Langs       []string // lang:/language: filters, OR'd, lower-cased
	PathFilters []string // path: case-insensitive substring of file path, OR'd
	NameFilters []string // name: case-insensitive substring of symbol name, OR'd
}

// ParseQuery splits a raw query into structured filters + remaining text. It
// never fails: an unknown field prefix (foo:bar) or an empty value passes
// through to Text untouched, so a search for "TODO:" yields a result rather
// than a parse error. Field VALUES are not validated against any vocabulary —
// the leaf stays vocabulary-free; the consuming lane decides whether a Kind or
// Lang it does not recognize should be dropped or folded back into text.
//
// Quoting: a double-quoted span keeps whitespace inside a value
// (path:"some dir/with spaces"); an unterminated quote swallows the rest of the
// input as one token rather than erroring. Ported from codegraph parseQuery.
func ParseQuery(raw string) ParsedQuery {
	out := ParsedQuery{}
	var textParts []string

	for _, tok := range tokenizeRespectingQuotes(raw) {
		key, value, ok := splitField(tok)
		if !ok {
			textParts = append(textParts, tok)
			continue
		}
		switch key {
		case "kind":
			out.Kinds = append(out.Kinds, value)
		case "lang", "language":
			out.Langs = append(out.Langs, strings.ToLower(value))
		case "path":
			out.PathFilters = append(out.PathFilters, value)
		case "name":
			out.NameFilters = append(out.NameFilters, value)
		default:
			textParts = append(textParts, tok)
		}
	}

	out.Text = strings.TrimSpace(strings.Join(textParts, " "))
	return out
}

// splitField splits "key:value" on the FIRST colon. Returns ok=false when there
// is no usable field shape (no colon, leading colon, or empty value) so the
// caller routes the whole token to free text.
func splitField(tok string) (key, value string, ok bool) {
	colon := strings.IndexByte(tok, ':')
	if colon <= 0 || colon == len(tok)-1 {
		return "", "", false
	}
	value = unquote(tok[colon+1:])
	if value == "" {
		return "", "", false
	}
	return strings.ToLower(tok[:colon]), value, true
}

// tokenizeRespectingQuotes splits on whitespace but keeps a double-quoted span
// — whether leading ("…") or mid-token (path:"…") — as part of one token.
func tokenizeRespectingQuotes(raw string) []string {
	var tokens []string
	i := 0
	for i < len(raw) {
		for i < len(raw) && isSpace(raw[i]) {
			i++
		}
		if i >= len(raw) {
			break
		}
		start := i
		for i < len(raw) && !isSpace(raw[i]) {
			if raw[i] == '"' {
				end := strings.IndexByte(raw[i+1:], '"')
				if end == -1 {
					i = len(raw) // unterminated — swallow the rest, forgiving
					break
				}
				i += end + 2 // skip to just past the closing quote
				continue
			}
			i++
		}
		tokens = append(tokens, raw[start:i])
	}
	return tokens
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}
