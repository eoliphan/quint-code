package codebase

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// the registry kernel-dispatch pattern, plus shapes that must resolve and shapes
// that must be dropped (conservative).
const facadeGo = `package p

type Registry struct{}
type Scanner struct{ registry *Registry }
type EdgeResolver interface{ Resolve() }

func (r *Registry) ResolverForFile(path string) EdgeResolver { return nil }

func mk() EdgeResolver { return nil }

func (s *Scanner) Scan() {
	resolver := s.registry.ResolverForFile("x") // two-hop: field then method return
	direct := mk()                               // package-level func return
	var explicit EdgeResolver                    // explicit var type
	cross := other.New()                         // unknown package func -> DROP
	blank := 1 + 2                               // non-type expr -> DROP
	_, _, _, _, _ = resolver, direct, explicit, cross, blank
}
`

func findFunc(f *ast.File, name string) *ast.FuncDecl {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == name {
			return fd
		}
	}
	return nil
}

func TestInferLocalVarTypes_RegistryPatternAndDrops(t *testing.T) {
	root := t.TempDir()
	rel := "p.go"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(facadeGo), 0o644); err != nil {
		t.Fatal(err)
	}

	facts, err := ExtractGoTypeFacts(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	// Facts sanity: the two-hop chain's pieces are recorded.
	if facts.StructFields["Scanner"]["registry"] != "Registry" {
		t.Fatalf("struct field not recorded: %+v", facts.StructFields)
	}
	if got := facts.MethodReturns["Registry\x00ResolverForFile"]; len(got) != 1 || got[0] != "EdgeResolver" {
		t.Fatalf("method return not recorded: %+v", facts.MethodReturns)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	scan := findFunc(f, "Scan")
	base := map[string]string{"s": "Scanner"} // the receiver, as ExtractGoSignatures provides
	vars := InferLocalVarTypes(scan, base, facts)

	if vars["resolver"] != "EdgeResolver" {
		t.Errorf("two-hop `s.registry.ResolverForFile()` should infer EdgeResolver, got %q", vars["resolver"])
	}
	if vars["direct"] != "EdgeResolver" {
		t.Errorf("package-func return should infer EdgeResolver, got %q", vars["direct"])
	}
	if vars["explicit"] != "EdgeResolver" {
		t.Errorf("explicit var type should be recorded, got %q", vars["explicit"])
	}
	// Conservative drops — a wrong inference here is exactly what produces a false
	// dispatch edge.
	if _, ok := vars["cross"]; ok {
		t.Errorf("cross-package call must NOT be inferred, got %q", vars["cross"])
	}
	if _, ok := vars["blank"]; ok {
		t.Errorf("non-type expression must NOT be inferred, got %q", vars["blank"])
	}
}

// The soundness guard: a name bound to two DIFFERENT types (inner-block shadow)
// must be dropped, never reported as either — a flat per-function table cannot
// say which holds at a given line, and a wrong report becomes a wrong edge.
func TestInferLocalVarTypes_ShadowDroppedNotMisbound(t *testing.T) {
	root := t.TempDir()
	rel := "s.go"
	src := `package s
type A interface{ M() }
type B interface{ M() }
func mkA() A { return nil }
func mkB() B { return nil }
func f() {
	x := mkA()
	{
		x := mkB() // shadows outer x with a DIFFERENT interface
		_ = x
	}
	_ = x
}
func keep() {
	y := mkA()
	z := mkA() // distinct names, both A — must survive
	_, _ = y, z
}
`
	if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	facts, _ := ExtractGoTypeFacts(root, rel)
	fset := token.NewFileSet()
	af, _ := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)

	fv := InferLocalVarTypes(findFunc(af, "f"), map[string]string{}, facts)
	if got, ok := fv["x"]; ok {
		t.Errorf("shadowed x (A then B) must be dropped, got %q", got)
	}
	kv := InferLocalVarTypes(findFunc(af, "keep"), map[string]string{}, facts)
	if kv["y"] != "A" || kv["z"] != "A" {
		t.Errorf("distinct same-type vars must survive, got y=%q z=%q", kv["y"], kv["z"])
	}
}

// A method-return key that does not exist in package facts must not resolve —
// e.g. calling a method on a var whose type we never recorded.
func TestInferLocalVarTypes_UnknownMethodDropped(t *testing.T) {
	root := t.TempDir()
	rel := "q.go"
	src := `package q
type T struct{}
func (t *T) Do() {
	x := t.unknownField.Method()
	_ = x
}
`
	if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	facts, _ := ExtractGoTypeFacts(root, rel)
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
	vars := InferLocalVarTypes(findFunc(f, "Do"), map[string]string{"t": "T"}, facts)
	if _, ok := vars["x"]; ok {
		t.Errorf("unknown field/method chain must drop, got %q", vars["x"])
	}
}
