package artifact

import (
	"strings"
	"testing"
)

func TestScanReputationSignals_DetectsAndDedupes(t *testing.T) {
	hits := ScanReputationSignals(
		"gRPC is the industry standard",
		"everyone uses gRPC; it is the industry standard after all",
	)
	joined := strings.Join(hits, "|")
	if !strings.Contains(joined, "industry standard") {
		t.Fatalf("expected 'industry standard' detected, got %v", hits)
	}
	if !strings.Contains(joined, "everyone uses") {
		t.Fatalf("expected 'everyone uses' detected, got %v", hits)
	}
	count := 0
	for _, h := range hits {
		if h == "industry standard" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 'industry standard' deduped to 1, got %d", count)
	}
}

func TestScanReputationSignals_Cyrillic(t *testing.T) {
	hits := ScanReputationSignals("это самый популярный выбор и стандарт индустрии")
	if len(hits) < 2 {
		t.Fatalf("expected популярн + стандарт индустрии, got %v", hits)
	}
}

func TestScanReputationSignals_ContentReasonClean(t *testing.T) {
	hits := ScanReputationSignals(
		"gRPC gives strict schemas and bidirectional streaming our ordering guarantee needs",
	)
	if len(hits) != 0 {
		t.Fatalf("expected a content reason to be clean, got %v", hits)
	}
}

func TestReputationWarning_FormatAndEmpty(t *testing.T) {
	if w := ReputationWarning("chosen for its strict schema and streaming"); w != "" {
		t.Fatalf("expected empty warning for content reason, got %q", w)
	}
	w := ReputationWarning("it is a best practice and widely used")
	if !strings.Contains(w, "best practice") || !strings.Contains(w, "widely used") {
		t.Fatalf("warning missing detected signals: %q", w)
	}
	if !strings.Contains(w, "CONTENT reason") {
		t.Fatalf("warning should ask for the content reason: %q", w)
	}
}
