package cli

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/m0n0x41d/haft/internal/embedding"
	"github.com/m0n0x41d/haft/internal/fpf"
	"github.com/m0n0x41d/haft/logger"
)

const (
	fpfRRFK             = 60.0
	fpfCandidateFactor  = 4
	fpfMinCandidatePool = 24
	fpfMinSimilarity    = 0.15
	// fpfCardPriorityTopK is how many top card-semantic hits are surfaced ahead of
	// the prose-heavy fusion. Small on purpose: enough that a genuine card query
	// gets its target up top, few enough that a prose query is not pushed out of
	// reach by weakly-matching cards.
	fpfCardPriorityTopK = 5
	// specEmbeddingDim is the MRL-truncated dimension the FPF spec vectors are
	// baked at; the runtime query embedder must match to read them.
	specEmbeddingDim = 256
)

var errFPFSemanticUnavailable = errors.New("fpf semantic index unavailable (no sidecar or no matching baked vectors)")

// fpfHybrid is the process-lived FPF semantic searcher. The FPF index is
// embedded + static, so a single cached vector Index across calls is correct.
// Initialized lazily by CLI/server paths; nil => FPF search stays deterministic FTS.
var (
	fpfHybrid          *FpfHybrid
	fpfHybridMu        sync.Mutex
	buildFPFHybridFunc = buildFPFHybrid
)

func ensureFPFHybrid() *FpfHybrid {
	fpfHybridMu.Lock()
	defer fpfHybridMu.Unlock()

	if fpfHybrid == nil {
		fpfHybrid = buildFPFHybridFunc()
		if fpfHybrid != nil {
			fpfHybrid.Prewarm()
		}
	}
	return fpfHybrid
}

// buildFPFHybrid wires the optional FPF spec semantic layer, forcing the baked
// MRL dimension so the runtime query embeds match the baked vectors. nil when
// embeddings are disabled by config.
func buildFPFHybrid() *FpfHybrid {
	embCfg := embeddingConfigFromFile()
	if strings.EqualFold(strings.TrimSpace(embCfg.Provider), embedding.ProviderNone) {
		return nil
	}
	embCfg.Dim = specEmbeddingDim
	newEmbedder := func() (embedding.Embedder, error) {
		return embedding.New(embCfg)
	}
	return NewFpfHybrid(newEmbedder)
}

// FpfHybrid adds semantic recall to FPF-spec search by fusing the deterministic
// tiers + a section-level keyword arm with baked per-section vectors (cosine via
// RRF), keyed by section_id. The baked-vector Index is loaded ONCE into memory
// (the spec is static; the embedded fpf.db is ephemeral-per-call) and the local
// sidecar embeds only the query. Degrades to SearchSpecWithOptions on any
// semantic-path issue (no sidecar, no/mismatched baked vectors, embed fault).
type FpfHybrid struct {
	newEmbedder func() (embedding.Embedder, error)

	mu                  sync.Mutex
	embedder            embedding.Embedder
	embedderUnavailable bool
	// cardIndex and proseIndex are SEPARATE cosine arms, split by section id at
	// PatternChunkIDBase. Keeping them apart is the whole point: a single pooled
	// index lets the ~5600 prose vectors drown the 66 pattern cards for "how to
	// think" queries (measured card R@10 collapse 0.88 -> 0.24). As distinct RRF
	// input lists, each ranks within its own population, so neither buries the
	// other. A patterns-only bake simply leaves proseIndex empty.
	cardIndex  *embedding.Index
	proseIndex *embedding.Index
	built      bool
	building   bool
}

func NewFpfHybrid(newEmbedder func() (embedding.Embedder, error)) *FpfHybrid {
	return &FpfHybrid{newEmbedder: newEmbedder}
}

// Prewarm kicks the background index load so the first query is fast.
func (h *FpfHybrid) Prewarm() {
	if h.newEmbedder != nil {
		h.ensureWarming()
	}
}

// Warm loads the baked-vector index synchronously. Returns an error when the
// semantic path is unavailable (no sidecar, schema<v3, or no vectors under the
// runtime model contract) — for tests/eval that need readiness before searching.
func (h *FpfHybrid) Warm() error {
	cardIndex, proseIndex := h.buildIndex()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cardIndex, h.proseIndex = cardIndex, proseIndex
	h.built = true
	if cardIndex == nil && proseIndex == nil {
		return errFPFSemanticUnavailable
	}
	return nil
}

// Search fuses keyword + semantic ranking over the FPF spec. It always returns
// valid results, degrading to the deterministic SearchSpecWithOptions whenever
// the semantic path is unavailable. Injected as SpecRetrievalRequest.HybridSearch.
func (h *FpfHybrid) Search(db *sql.DB, query string, limit int) ([]fpf.SpecSearchResult, error) {
	if limit <= 0 {
		limit = fpf.DefaultSpecSearchLimit
	}
	deterministic, err := fpf.SearchSpecWithOptions(db, query, fpf.SpecSearchOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	if h.newEmbedder == nil {
		return deterministic, nil
	}

	h.ensureWarming()
	embedder, cardIndex, proseIndex, ready := h.snapshot()
	if !ready {
		return deterministic, nil
	}

	pool := max(limit*fpfCandidateFactor, fpfMinCandidatePool)
	fts, ftsErr := fpf.SearchFTSSectionIDs(db, query, pool)
	if ftsErr != nil {
		return deterministic, nil
	}

	queryVectors, embedErr := embedder.Embed(context.Background(), embedding.RoleQuery, []string{query})
	if embedErr != nil || len(queryVectors) == 0 {
		if embedErr != nil {
			logger.Warn().Err(embedErr).Msg("fpf query embed failed — returning deterministic FTS results")
		}
		return deterministic, nil
	}

	// Cards and prose are ranked as SEPARATE RRF lists so the larger prose
	// population cannot bury a target card (and vice versa).
	cardSem := semanticArmIDs(cardIndex, queryVectors[0], pool, fpfMinSimilarity)
	proseSem := semanticArmIDs(proseIndex, queryVectors[0], pool, fpfMinSimilarity)
	fused := fuseRRF([][]string{sectionIDStrings(fts), cardSem, proseSem}, fpfRRFK)

	ftsByID := make(map[int]fpf.SpecSearchResult, len(fts))
	for _, r := range fts {
		ftsByID[r.SectionID] = r
	}
	var semOnly []int
	for _, idStr := range fused {
		id, _ := strconv.Atoi(idStr)
		if _, ok := ftsByID[id]; !ok {
			semOnly = append(semOnly, id)
		}
	}
	hydrated, _ := fpf.HydrateSections(db, semOnly)
	resolve := func(idStr string) (fpf.SpecSearchResult, bool) {
		id, _ := strconv.Atoi(idStr)
		if r, ok := ftsByID[id]; ok {
			return r, true
		}
		r, ok := hydrated[id]
		return r, ok
	}

	fusedResults := make([]fpf.SpecSearchResult, 0, len(fused))
	for rank, idStr := range fused {
		r, ok := resolve(idStr)
		if !ok {
			continue
		}
		r.Rank = float64(rank)
		fusedResults = append(fusedResults, r)
	}

	// cardPriority: the top card-semantic hits are the methodology answer surface
	// for "how to think" queries. Surfacing the strongest few ahead of the
	// prose-heavy fusion is what keeps card recall high (0.88) without giving up
	// the prose recall the full corpus unlocks — prose still follows right below.
	cardPriority := make([]fpf.SpecSearchResult, 0, fpfCardPriorityTopK)
	for _, idStr := range topNStrings(cardSem, fpfCardPriorityTopK) {
		if r, ok := resolve(idStr); ok {
			cardPriority = append(cardPriority, r)
		}
	}

	return mergeFPFResults(deterministic, cardPriority, fusedResults, limit), nil
}

// mergeFPFResults keeps the graph primary: exact-pattern (deterministic) hits
// lead, then the strongest card-semantic hits (the "how to think" answer
// surface), then the keyword+semantic fusion, then the remaining deterministic
// tiers as a recall floor — so hybrid recall is never below SearchSpecWithOptions.
func mergeFPFResults(deterministic, cardPriority, fused []fpf.SpecSearchResult, limit int) []fpf.SpecSearchResult {
	seen := make(map[string]bool)
	out := make([]fpf.SpecSearchResult, 0, limit)
	add := func(r fpf.SpecSearchResult) bool {
		if len(out) >= limit {
			return false
		}
		key := r.PatternID + "|" + r.Heading
		if seen[key] {
			return true
		}
		seen[key] = true
		out = append(out, r)
		return len(out) < limit
	}

	for _, r := range deterministic {
		if r.Tier == fpf.SpecSearchTierPattern && !add(r) {
			return out
		}
	}
	for _, r := range cardPriority {
		if !add(r) {
			return out
		}
	}
	for _, r := range fused {
		if !add(r) {
			return out
		}
	}
	for _, r := range deterministic {
		if !add(r) {
			return out
		}
	}
	return out
}

func (h *FpfHybrid) ensureWarming() {
	h.mu.Lock()
	if h.embedderUnavailable || h.building || h.built {
		h.mu.Unlock()
		return
	}
	h.building = true
	h.mu.Unlock()
	go h.warmAsync()
}

func (h *FpfHybrid) warmAsync() {
	cardIndex, proseIndex := h.buildIndex()
	h.mu.Lock()
	h.building = false
	h.cardIndex, h.proseIndex = cardIndex, proseIndex
	h.built = true
	h.mu.Unlock()
}

func (h *FpfHybrid) snapshot() (embedding.Embedder, *embedding.Index, *embedding.Index, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if indexLen(h.cardIndex)+indexLen(h.proseIndex) == 0 {
		return nil, nil, nil, false
	}
	return h.embedder, h.cardIndex, h.proseIndex, true
}

func indexLen(index *embedding.Index) int {
	if index == nil {
		return 0
	}
	return index.Len()
}

// semanticArmIDs runs one cosine arm and returns its section ids above the floor.
// A nil/empty index (e.g. proseIndex for a patterns-only bake) contributes
// nothing — the arm simply drops out of the fusion.
func semanticArmIDs(index *embedding.Index, query []float32, pool int, minScore float64) []string {
	if indexLen(index) == 0 {
		return nil
	}
	return semanticSectionIDs(index.Search(query, pool), minScore)
}

func topNStrings(ids []string, n int) []string {
	if len(ids) < n {
		return ids
	}
	return ids[:n]
}

func (h *FpfHybrid) resolveEmbedder() embedding.Embedder {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.embedderUnavailable || h.newEmbedder == nil {
		h.embedderUnavailable = true
		return nil
	}
	if h.embedder != nil {
		return h.embedder
	}
	embedder, err := h.newEmbedder()
	if err != nil {
		h.embedderUnavailable = true
		logger.Info().Err(err).Msg("embedding sidecar unavailable — FPF spec search degrades to FTS")
		return nil
	}
	h.embedder = embedder
	return embedder
}

// buildIndex loads the baked section vectors and splits them into the card and
// prose cosine arms. Returns (nil, nil) only on a hard failure (no sidecar,
// schema mismatch, or no vectors under the runtime contract); a successful load
// always returns two non-nil indices, either of which may be empty (a
// patterns-only bake yields an empty prose arm).
func (h *FpfHybrid) buildIndex() (cardIndex, proseIndex *embedding.Index) {
	embedder := h.resolveEmbedder()
	if embedder == nil {
		return nil, nil
	}
	descriptor := embedder.Descriptor()

	db, cleanup, err := openFPFDBFunc()
	if err != nil {
		logger.Warn().Err(err).Msg("fpf index open failed — semantic recall disabled this session")
		return nil, nil
	}
	defer cleanup()

	if version, _ := fpf.GetSpecMeta(db, "schema_version"); version != fpf.SpecIndexSchemaVersion {
		return nil, nil
	}
	vectors, err := fpf.LoadSpecEmbeddings(db, descriptor.Provider, descriptor.Model, descriptor.Dimensions)
	if err != nil {
		logger.Warn().Err(err).Msg("fpf load embeddings failed — semantic recall disabled this session")
		return nil, nil
	}
	if len(vectors) == 0 {
		if provider, model, dim, count, _ := fpf.SpecEmbeddingContract(db); count > 0 {
			logger.Warn().
				Str("baked", provider+"/"+model).Int("baked_dim", dim).
				Str("runtime", descriptor.Provider+"/"+descriptor.Model).Int("runtime_dim", descriptor.Dimensions).
				Msg("FPF baked vectors are under a different model contract — semantic recall degraded to FTS")
		}
		return nil, nil
	}

	cardIndex = embedding.NewIndex(0)
	proseIndex = embedding.NewIndex(0)
	for sectionID, vector := range vectors {
		if sectionID >= fpf.PatternChunkIDBase {
			cardIndex.Add(strconv.Itoa(sectionID), vector)
		} else {
			proseIndex.Add(strconv.Itoa(sectionID), vector)
		}
	}
	return cardIndex, proseIndex
}

func sectionIDStrings(results []fpf.SpecSearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = strconv.Itoa(r.SectionID)
	}
	return out
}

func semanticSectionIDs(scored []embedding.Scored, minScore float64) []string {
	out := make([]string, 0, len(scored))
	for _, item := range scored {
		if item.Score >= minScore {
			out = append(out, item.ID)
		}
	}
	return out
}

func fuseRRF(ranked [][]string, k float64) []string {
	score := make(map[string]float64)
	for _, list := range ranked {
		for rank, id := range list {
			score[id] += 1.0 / (k + float64(rank) + 1.0)
		}
	}
	ids := make([]string, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		if score[ids[i]] == score[ids[j]] {
			return ids[i] < ids[j]
		}
		return score[ids[i]] > score[ids[j]]
	})
	return ids
}
