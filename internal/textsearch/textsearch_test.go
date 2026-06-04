package textsearch

import (
	"reflect"
	"slices"
	"testing"
)

func TestSplitIdentifier(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"getUserName", []string{"get", "user", "name"}},
		{"UserService", []string{"user", "service"}},
		{"HTTPServer", []string{"http", "server"}},
		{"scrape_loop", []string{"scrape", "loop"}},
		{"SCREAMING_SNAKE", []string{"screaming", "snake"}},
		{"pkg.DoThing", []string{"pkg", "do", "thing"}},
		{"kebab-case-name", []string{"kebab", "case", "name"}},
		{"authenticate", []string{"authenticate"}},
		{"parseHTTP2Request", []string{"parse", "http2", "request"}},
		{"", nil},
	}
	for _, c := range cases {
		got := SplitIdentifier(c.in)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitIdentifier(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTermsCompoundPreservedAndSplit(t *testing.T) {
	// A compound query token keeps its joined form AND its parts, so a substring
	// index can match the whole identifier and individual words alike.
	got := Terms("getUserName", Options{})
	want := []string{"getusername", "get", "user", "name"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Terms(getUserName) = %v, want %v", got, want)
	}
}

func TestTermsDropsStopWordsAndShort(t *testing.T) {
	got := Terms("how do I fix the cache", Options{})
	// how/do/the/fix are stop-words; "I" is sub-minTermLen; only "cache" stays.
	want := []string{"cache"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Terms = %v, want %v", got, want)
	}
}

func TestTermsStemsToggle(t *testing.T) {
	without := Terms("caching", Options{Stems: false})
	if !reflect.DeepEqual(without, []string{"caching"}) {
		t.Fatalf("no-stems = %v, want [caching]", without)
	}
	with := Terms("caching", Options{Stems: true})
	if !contains(with, "cache") || !contains(with, "cach") {
		t.Fatalf("stems = %v, want to contain cache and cach", with)
	}
	if with[0] != "caching" {
		t.Fatalf("stems must keep base term first, got %v", with)
	}
}

func TestTermsOrderStableAndDeduped(t *testing.T) {
	// Run repeatedly: order must be identical every time (no map iteration leak).
	first := Terms("UserStore userStore user_store", Options{})
	for range 20 {
		again := Terms("UserStore userStore user_store", Options{})
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("Terms not order-stable: %v vs %v", first, again)
		}
	}
	// "user" and "store" each appear once despite three compound spellings.
	if countOf(first, "user") != 1 || countOf(first, "store") != 1 {
		t.Fatalf("Terms not deduped: %v", first)
	}
}

func TestStemVariants(t *testing.T) {
	cases := []struct {
		in       string
		contains []string
		excludes []string
	}{
		{"caching", []string{"cach", "cache"}, []string{"caching"}},
		{"eviction", []string{"evict"}, nil},
		{"management", []string{"manage"}, nil},
		{"entries", []string{"entry"}, nil},
		{"processes", []string{"process"}, nil},
		{"errors", []string{"error"}, nil},
		{"handled", []string{"handle", "handl"}, nil},
		{"carried", []string{"carry"}, nil},
		{"builder", []string{"build", "builde"}, nil},
		{"running", []string{"run", "runn"}, nil},
		{"class", nil, []string{"clas"}}, // -ss guarded, no bogus "clas"
	}
	for _, c := range cases {
		got := StemVariants(c.in)
		for _, want := range c.contains {
			if !contains(got, want) {
				t.Errorf("StemVariants(%q) = %v, want to contain %q", c.in, got, want)
			}
		}
		for _, bad := range c.excludes {
			if contains(got, bad) {
				t.Errorf("StemVariants(%q) = %v, must NOT contain %q", c.in, got, bad)
			}
		}
		if contains(got, c.in) {
			t.Errorf("StemVariants(%q) must not contain the original term", c.in)
		}
	}
}

func TestBoundedEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		max  int
		want int
	}{
		{"authenticate", "authenticate", 2, 0},
		{"autenticate", "authenticate", 2, 1}, // one deletion (missing h)
		{"kitten", "sitting", 3, 3},
		{"flaw", "lawn", 2, 2},
		{"abcdef", "uvwxyz", 2, 3},    // far apart -> early exit returns max+1
		{"short", "muchlonger", 2, 3}, // length gap alone exceeds max
		{"", "abc", 5, 3},
	}
	for _, c := range cases {
		if got := BoundedEditDistance(c.a, c.b, c.max); got != c.want {
			t.Errorf("BoundedEditDistance(%q,%q,%d) = %d, want %d", c.a, c.b, c.max, got, c.want)
		}
	}
}

func TestParseQuery(t *testing.T) {
	q := ParseQuery(`kind:function name:auth path:internal/cli authenticate`)
	if !reflect.DeepEqual(q.Kinds, []string{"function"}) {
		t.Errorf("Kinds = %v", q.Kinds)
	}
	if !reflect.DeepEqual(q.NameFilters, []string{"auth"}) {
		t.Errorf("NameFilters = %v", q.NameFilters)
	}
	if !reflect.DeepEqual(q.PathFilters, []string{"internal/cli"}) {
		t.Errorf("PathFilters = %v", q.PathFilters)
	}
	if q.Text != "authenticate" {
		t.Errorf("Text = %q, want authenticate", q.Text)
	}
}

func TestParseQueryQuotedPathAndLangAlias(t *testing.T) {
	q := ParseQuery(`language:Go path:"some dir/with spaces" findThing`)
	if !reflect.DeepEqual(q.Langs, []string{"go"}) {
		t.Errorf("Langs = %v, want [go]", q.Langs)
	}
	if !reflect.DeepEqual(q.PathFilters, []string{"some dir/with spaces"}) {
		t.Errorf("PathFilters = %v", q.PathFilters)
	}
	if q.Text != "findThing" {
		t.Errorf("Text = %q", q.Text)
	}
}

func TestParseQueryUnknownFieldAndColonPassThrough(t *testing.T) {
	q := ParseQuery(`TODO: foo:bar plainword`)
	// "TODO:" has an empty value -> text; "foo:bar" unknown field -> text.
	if q.Text != "TODO: foo:bar plainword" {
		t.Errorf("Text = %q", q.Text)
	}
	if len(q.Kinds)+len(q.Langs)+len(q.PathFilters)+len(q.NameFilters) != 0 {
		t.Errorf("expected no filters, got %+v", q)
	}
}

func TestParseQueryUnterminatedQuote(t *testing.T) {
	q := ParseQuery(`name:"unclosed value here`)
	// Forgiving: swallow the rest as one value rather than erroring.
	if len(q.NameFilters) != 1 {
		t.Fatalf("NameFilters = %v, want one entry", q.NameFilters)
	}
}

func contains(xs []string, want string) bool {
	return slices.Contains(xs, want)
}

func countOf(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}
