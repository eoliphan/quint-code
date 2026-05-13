package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestEmitLegacyTUIBanner_PrintsOnce verifies that two back-to-back
// invocations in the same process produce the banner only once,
// matching the DR-v8-sunset invariant ("printed once at launch via
// sync.Once gated").
func TestEmitLegacyTUIBanner_PrintsOnce(t *testing.T) {
	// Reset the sync.Once so this test is reproducible regardless of
	// other tests in the package having tripped it.
	legacyTUIBannerOnce = sync.Once{}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })

	emitLegacyTUIBanner()
	emitLegacyTUIBanner()

	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read: %v", err)
	}

	got := buf.String()
	occurrences := strings.Count(got, "[DEPRECATED]")
	if occurrences != 1 {
		t.Fatalf("banner [DEPRECATED] markers in stderr = %d, want 1\noutput:\n%s", occurrences, got)
	}
	if !strings.Contains(got, "tui-react/") {
		t.Fatalf("banner missing tui-react/ marker: %q", got)
	}
	if !strings.Contains(got, "haft agent --v8") {
		t.Fatalf("banner missing v8 opt-in instruction: %q", got)
	}
	if !strings.Contains(got, "DR-v8-sunset") {
		t.Fatalf("banner missing DR citation: %q", got)
	}
}
