package codeintel

import (
	"testing"

	"github.com/m0n0x41d/haft/internal/codebase"
)

func TestSymbolCoveringLine_InnermostWins(t *testing.T) {
	syms := []codebase.CodeSymbol{
		{Name: "Outer", StartLine: 10, EndLine: 100},
		{Name: "Inner", StartLine: 40, EndLine: 60}, // nested — should win for line 50
		{Name: "Other", StartLine: 200, EndLine: 250},
	}

	got, ok := symbolCoveringLine(syms, 50)
	if !ok || got.Name != "Inner" {
		t.Fatalf("line 50 should resolve to innermost Inner, got %q (ok=%v)", got.Name, ok)
	}
	got, ok = symbolCoveringLine(syms, 15)
	if !ok || got.Name != "Outer" {
		t.Fatalf("line 15 should resolve to Outer, got %q (ok=%v)", got.Name, ok)
	}
	if _, ok := symbolCoveringLine(syms, 500); ok {
		t.Fatalf("line 500 covers nothing, expected ok=false")
	}
}

func TestFilterByFile(t *testing.T) {
	syms := []codebase.CodeSymbol{
		{Name: "A", FilePath: "x.go"},
		{Name: "B", FilePath: "y.go"},
		{Name: "C", FilePath: "x.go"},
	}
	got := filterByFile(syms, "x.go")
	if len(got) != 2 || got[0].Name != "A" || got[1].Name != "C" {
		t.Fatalf("filterByFile(x.go) = {A,C}, got %+v", got)
	}
	if len(filterByFile(syms, "z.go")) != 0 {
		t.Fatalf("filterByFile(z.go) should be empty")
	}
}
