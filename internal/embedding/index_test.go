package embedding

import "testing"

func TestIndexSearchRanksByCosine(t *testing.T) {
	index := NewIndex(3)
	// Three orthogonal-ish unit vectors; query aligns with "b".
	index.Add("a", []float32{1, 0, 0})
	index.Add("b", []float32{0, 1, 0})
	index.Add("c", []float32{0, 0, 1})

	results := index.Search([]float32{0.1, 0.9, 0}, 2)
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if results[0].ID != "b" {
		t.Fatalf("want top result b, got %s (%.3f)", results[0].ID, results[0].Score)
	}
	if results[0].Score <= results[1].Score {
		t.Fatalf("results not sorted descending: %+v", results)
	}
}

func TestIndexRejectsDimMismatch(t *testing.T) {
	index := NewIndex(2)
	if !index.Add("a", []float32{1, 0}) {
		t.Fatal("first add should succeed")
	}
	if index.Add("b", []float32{1, 0, 0}) {
		t.Fatal("mismatched-width add should be rejected")
	}
	if index.Len() != 1 {
		t.Fatalf("want 1 row after rejected add, got %d", index.Len())
	}
}

func TestIndexEmptyQueryOrCorpus(t *testing.T) {
	index := NewIndex(0)
	if got := index.Search([]float32{1, 0}, 5); got != nil {
		t.Fatalf("empty corpus should return nil, got %v", got)
	}
	index.Add("a", []float32{1, 0})
	if got := index.Search(nil, 5); got != nil {
		t.Fatalf("empty query should return nil, got %v", got)
	}
}
