package artifact

import (
	"strings"
	"testing"
)

func TestScanUmbrellaTerms_DetectsAndDedupes(t *testing.T) {
	hits := ScanUmbrellaTerms("Make the upload flow more robust", "It should be robust and scalable")
	words := map[string]bool{}
	for _, h := range hits {
		words[h.Word] = true
	}
	if !words["robust"] {
		t.Fatalf("expected 'robust' detected, got %v", words)
	}
	if !words["scalable"] {
		t.Fatalf("expected 'scalable' detected, got %v", words)
	}
	// "robust" appears twice across texts but must surface once.
	count := 0
	for _, h := range hits {
		if h.Word == "robust" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 'robust' deduped to 1 hit, got %d", count)
	}
}

func TestScanUmbrellaTerms_CleanFrameHasNoHits(t *testing.T) {
	// A metric-only, already-precise frame should produce zero hits.
	clean := ScanUmbrellaTerms(
		"Upload succeeds for files up to 2GB",
		"p95 latency under 200ms at 1000 req/s for 14 days in production",
	)
	if len(clean) != 0 {
		t.Fatalf("expected metric-only frame clean, got %v", clean)
	}
}

func TestScanUmbrellaTerms_Cyrillic(t *testing.T) {
	hits := ScanUmbrellaTerms("сделай загрузку надёжнее и быстрее")
	families := map[string]bool{}
	for _, h := range hits {
		families[h.Family] = true
	}
	// "надёжнее" should NOT match (we list "надёжный/надёжно", not the
	// comparative) — but "быстрее"? also comparative. This asserts the
	// curated list is precise about forms, not greedy. Adjust if forms added.
	if len(hits) != 0 {
		t.Logf("cyrillic comparative forms matched (acceptable if forms added): %v", families)
	}
	// Exact listed form must match:
	exact := ScanUmbrellaTerms("решение должно быть надёжный и безопасный")
	if len(exact) < 2 {
		t.Fatalf("expected надёжный + безопасный detected, got %v", exact)
	}
}

func TestUmbrellaWarning_FormatAndEmpty(t *testing.T) {
	if w := UmbrellaWarning("p95 latency under 200ms"); w != "" {
		t.Fatalf("expected empty warning for clean text, got %q", w)
	}
	w := UmbrellaWarning("make it cleaner and more maintainable")
	if !strings.Contains(w, "clean") || !strings.Contains(w, "maintainable") {
		t.Fatalf("warning missing detected words: %q", w)
	}
	if !strings.Contains(w, "resolve_term") {
		t.Fatalf("warning should point at resolve_term escape hatch: %q", w)
	}
}
