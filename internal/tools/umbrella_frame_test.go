package tools

import (
	"context"
	"strings"
	"testing"
)

// TestHaftProblemTool_FrameSurfacesUmbrellaWarning is the end-to-end
// assertion for FPF E.10 wording-use precision: framing a problem whose
// signal carries umbrella words must surface the advisory warning in the
// tool's actual response (not just in the unit-tested scanner).
func TestHaftProblemTool_FrameSurfacesUmbrellaWarning(t *testing.T) {
	store := setupHaftToolStore(t)
	haftDir := t.TempDir()
	problemTool := NewHaftProblemTool(store, haftDir)

	result, err := problemTool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action": "frame",
		"title":  "Upload hardening",
		"signal": "Make the upload flow more robust and scalable",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.DisplayText, "Umbrella words") {
		t.Fatalf("expected umbrella warning in frame output, got:\n%s", result.DisplayText)
	}
	for _, want := range []string{"robust", "scalable", "resolve_term"} {
		if !strings.Contains(result.DisplayText, want) {
			t.Fatalf("frame output missing %q:\n%s", want, result.DisplayText)
		}
	}
}

// TestHaftProblemTool_FrameCleanNoUmbrellaWarning guards the false-positive
// boundary: a metric-only, already-precise frame must NOT trip the warning.
func TestHaftProblemTool_FrameCleanNoUmbrellaWarning(t *testing.T) {
	store := setupHaftToolStore(t)
	haftDir := t.TempDir()
	problemTool := NewHaftProblemTool(store, haftDir)

	result, err := problemTool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":     "frame",
		"title":      "Upload throughput target",
		"signal":     "Uploads finish within p95 200ms at 1000 req/s",
		"acceptance": "p95 under 200ms sustained for 14 days in production",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.DisplayText, "Umbrella words") {
		t.Fatalf("did not expect umbrella warning for metric-only frame, got:\n%s", result.DisplayText)
	}
}
