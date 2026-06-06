package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// VerifyBody must catch an edited file: re-hashing a freshly-read slice against
// the stored hash, not comparing stored-hash-to-stored-hash, is what detects
// that a byte range now points at different source.
func TestVerifyBody_DetectsEditedSource(t *testing.T) {
	st, root := newSymbolStore(t)
	ctx := context.Background()
	rel := "v.go"
	if err := os.WriteFile(filepath.Join(root, rel), []byte("package v\n\nfunc Do() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexFileSymbols(ctx, root, rel); err != nil {
		t.Fatal(err)
	}
	syms, err := st.GetByName(ctx, "Do")
	if err != nil || len(syms) != 1 {
		t.Fatalf("expected 1 Do, got %d (err=%v)", len(syms), err)
	}
	sym := syms[0]

	// Fresh content matches the stored hash → verified, byte-exact.
	fresh, _ := os.ReadFile(filepath.Join(root, rel))
	body, ok := VerifyBody(fresh, sym)
	if !ok {
		t.Fatalf("VerifyBody on unchanged content should pass")
	}
	if string(body) != "func Do() int { return 1 }" {
		t.Fatalf("body not byte-exact: %q", body)
	}

	// Edit the file: same byte length region now holds different source. The
	// stored offsets still "fit", so SliceBody alone would return wrong bytes —
	// only the re-hash catches it.
	edited := []byte("package v\n\nfunc Do() int { return 9 }\n")
	if _, stale := VerifyBody(edited, sym); stale {
		t.Fatalf("VerifyBody must report stale (false) when content changed under the offsets")
	}
}

func TestVerifyBody_BadOffsetsAndMissingHash(t *testing.T) {
	content := []byte("0123456789")
	if _, ok := VerifyBody(content, CodeSymbol{StartByte: 0, EndByte: 100, Hash: "x"}); ok {
		t.Fatalf("out-of-range offsets must not verify")
	}
	if _, ok := VerifyBody(content, CodeSymbol{StartByte: 0, EndByte: 4, Hash: ""}); ok {
		t.Fatalf("empty stored hash must not verify (nothing to check against)")
	}
}
