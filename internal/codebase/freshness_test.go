package codebase

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceFingerprint_StableAndChangeSensitive(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package a\nfunc A() {}\n")
	sc := NewScanner(nil) // SourceFingerprint is stat-only; no DB needed

	fp1, err := sc.SourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	fp2, err := sc.SourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("unchanged tree must reproduce the same fingerprint: %s vs %s", fp1, fp2)
	}

	// A content/size change flips it.
	write("a.go", "package a\nfunc A() {}\nfunc B() {}\n")
	fp3, _ := sc.SourceFingerprint(root)
	if fp3 == fp1 {
		t.Fatalf("a modified file must change the fingerprint")
	}

	// A new file flips it.
	write("b.go", "package a\nfunc C() {}\n")
	fp4, _ := sc.SourceFingerprint(root)
	if fp4 == fp3 {
		t.Fatalf("an added file must change the fingerprint")
	}

	// A non-source file must NOT affect it.
	write("notes.txt", "hello")
	fp5, _ := sc.SourceFingerprint(root)
	if fp5 != fp4 {
		t.Fatalf("a non-indexed file must not change the fingerprint")
	}
}
