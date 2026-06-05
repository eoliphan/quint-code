package embedding

import "sort"

// Scored pairs a corpus id with its cosine similarity to a query vector.
type Scored struct {
	ID    string
	Score float64
}

// Index is a brute-force in-memory cosine index over L2-normalized vectors.
// haft's corpora are small (tens of decisions, thousands of symbols), so no
// vector DB / ANN / HNSW is warranted — the decision invariant is an explicit
// float32 matrix scanned linearly (dec-20260605-fe77b358). Vectors are assumed
// already L2-normalized (the sidecar normalizes), so cosine is a plain dot
// product; NormalizeInPlace guards inputs from other adapters.
type Index struct {
	ids  []string
	rows [][]float32
	dim  int
}

// NewIndex returns an empty index sized for an expected row count.
func NewIndex(capacity int) *Index {
	return &Index{
		ids:  make([]string, 0, capacity),
		rows: make([][]float32, 0, capacity),
	}
}

// Add appends one id/vector row. The first row fixes the index dimension;
// rows whose width disagrees are rejected (returns false) rather than corrupt
// the cosine scan.
func (ix *Index) Add(id string, vector []float32) bool {
	if len(vector) == 0 {
		return false
	}
	if ix.dim == 0 {
		ix.dim = len(vector)
	}
	if len(vector) != ix.dim {
		return false
	}
	ix.ids = append(ix.ids, id)
	ix.rows = append(ix.rows, vector)
	return true
}

// Len reports the number of indexed rows.
func (ix *Index) Len() int {
	return len(ix.rows)
}

// Search returns the topK rows by cosine similarity to query, highest first.
// A non-positive topK returns all rows ranked. Rows of mismatched width score 0.
func (ix *Index) Search(query []float32, topK int) []Scored {
	if len(query) == 0 || len(ix.rows) == 0 {
		return nil
	}

	scored := make([]Scored, 0, len(ix.rows))
	for row, vector := range ix.rows {
		scored = append(scored, Scored{ID: ix.ids[row], Score: dot(query, vector)})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].ID < scored[j].ID
		}
		return scored[i].Score > scored[j].Score
	})

	if topK > 0 && len(scored) > topK {
		scored = scored[:topK]
	}
	return scored
}

func dot(left, right []float32) float64 {
	if len(left) != len(right) {
		return 0
	}
	sum := 0.0
	for i := range left {
		sum += float64(left[i]) * float64(right[i])
	}
	return sum
}
