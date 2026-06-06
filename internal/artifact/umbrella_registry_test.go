package artifact

import "testing"

// TestUmbrellaRegistry_RichAndMultilingual guards against the registry
// regressing back to a token-sized, two-language list. It must stay large and
// cover multiple languages.
func TestUmbrellaRegistry_RichAndMultilingual(t *testing.T) {
	if len(umbrellaRegistry) < 12 {
		t.Fatalf("expected a rich registry (>=12 families), got %d", len(umbrellaRegistry))
	}
	total := 0
	for _, fam := range umbrellaRegistry {
		if fam.Family == "" || fam.RecoverTo == "" || fam.MustNot == "" || len(fam.Words) == 0 {
			t.Fatalf("family %q is missing required fields", fam.Family)
		}
		total += len(fam.Words)
	}
	if total < 150 {
		t.Fatalf("expected >=150 trigger words across languages, got %d", total)
	}

	// Same vague concept must fire across several languages, proving the
	// registry is genuinely multilingual, not EN+RU only.
	multilingualQuality := []string{
		"quality",   // EN
		"качество",  // RU
		"якість",    // UK
		"qualität",  // DE
		"qualité",   // FR
		"calidad",   // ES
		"qualidade", // PT
		"qualità",   // IT
	}
	for _, w := range multilingualQuality {
		if len(ScanUmbrellaTerms(w)) == 0 {
			t.Fatalf("expected multilingual quality word %q to be detected", w)
		}
	}
}
