package codebase

import "testing"

func TestResolveQualifierType_DottedSimpleAndDrop(t *testing.T) {
	vars := map[string]string{"s": "Service", "store": "Store"}
	facts := NewTypeFacts()
	facts.StructFields["Service"] = map[string]string{"scanner": "Scanner"}

	if got := resolveQualifierType("s.scanner", vars, facts); got != "Scanner" {
		t.Errorf("dotted s.scanner should resolve to Scanner, got %q", got)
	}
	if got := resolveQualifierType("store", vars, facts); got != "Store" {
		t.Errorf("simple store should resolve to Store, got %q", got)
	}
	if got := resolveQualifierType("pkg", vars, facts); got != "" {
		t.Errorf("unknown base (package alias) must drop, got %q", got)
	}
	if got := resolveQualifierType("s.missing", vars, facts); got != "" {
		t.Errorf("unknown field must drop, got %q", got)
	}
}

func TestResolveConcreteMethodCallEdges_CrossPackageAndCollisionDrop(t *testing.T) {
	caller := CodeSymbol{ID: "caller", Name: "EnsureIndex", Kind: "method", Receiver: "Service", FilePath: "internal/codeintel/flow.go", StartLine: 10, EndLine: 20}
	fileSyms := []CodeSymbol{caller}
	sites := []CallSite{{Callee: "ScanEdges", Qualifier: "s.scanner", Line: 15}}
	sigs := map[int]map[string]string{10: {"s": "Service"}}
	facts := NewTypeFacts()
	facts.StructFields["Service"] = map[string]string{"scanner": "Scanner"}

	// The method lives in ANOTHER package (cross-package resolution).
	target := CodeSymbol{ID: "scanedges", Name: "ScanEdges", Kind: "method", Receiver: "Scanner", FilePath: "internal/codebase/walker.go", StartLine: 232}

	// Unique (receiver, method) → one static call edge, cross-package.
	got := ResolveConcreteMethodCallEdges("internal/codeintel/flow.go", fileSyms, sites, sigs, facts,
		func(name string) []CodeSymbol { return []CodeSymbol{target} })
	if len(got) != 1 || got[0].SrcID != "caller" || got[0].DstID != "scanedges" || got[0].Kind != EdgeCall {
		t.Fatalf("expected one EnsureIndex->ScanEdges call edge, got %+v", got)
	}

	// Collision: two "Scanner.ScanEdges" across packages → DROP (never fan a wrong edge).
	other := CodeSymbol{ID: "other", Name: "ScanEdges", Kind: "method", Receiver: "Scanner", FilePath: "elsewhere/x.go", StartLine: 5}
	drop := ResolveConcreteMethodCallEdges("internal/codeintel/flow.go", fileSyms, sites, sigs, facts,
		func(name string) []CodeSymbol { return []CodeSymbol{target, other} })
	if len(drop) != 0 {
		t.Fatalf("ambiguous (receiver,method) must drop, got %+v", drop)
	}

	// Unknown qualifier type (package alias, not a var) → drop, left to cross-file P1b.
	pkgSites := []CallSite{{Callee: "NewStore", Qualifier: "db", Line: 15}}
	none := ResolveConcreteMethodCallEdges("internal/codeintel/flow.go", fileSyms, pkgSites, sigs, facts,
		func(name string) []CodeSymbol { return []CodeSymbol{target} })
	if len(none) != 0 {
		t.Fatalf("package-alias qualifier must not resolve as a concrete method, got %+v", none)
	}
}
