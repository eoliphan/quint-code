package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/artifact"
)

func TestHandleQuintNoteReturnsArtifactID(t *testing.T) {
	store := setupCLIArtifactStore(t)
	ctx := context.Background()
	haftDir := t.TempDir()

	_, ref, err := handleQuintNote(ctx, store, haftDir, map[string]any{
		"title": "Hybrid recall invalidation",
		"observations": []any{
			"Created notes should invalidate semantic recall in the current MCP session.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref == "" {
		t.Fatal("note handler returned empty createdRef; expected canonical note ID")
	}
	if !strings.HasPrefix(ref, "note-") {
		t.Fatalf("createdRef = %q, want note-*", ref)
	}

	note, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatalf("createdRef %q does not resolve to a stored artifact: %v", ref, err)
	}
	if note.Meta.Kind != artifact.KindNote {
		t.Fatalf("createdRef %q resolved to %s, want Note", ref, note.Meta.Kind)
	}
}
