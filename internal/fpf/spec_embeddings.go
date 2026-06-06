package fpf

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

// specEmbeddingBatch bounds how many section texts are embedded per sidecar
// round-trip during the bake pass. Kept SMALL on purpose: transformer attention
// is O(batch · seq²), so a large batch of long sections builds a multi-GB
// activation tensor (256 × ~680² × heads × 4B ≈ several GB) and the sidecar OOMs.
// 16 keeps peak activation memory in the low hundreds of MB; the extra
// round-trips are negligible against inference cost.
const specEmbeddingBatch = 16

// BodyEmbeddingPrefixLen caps how much of a section body feeds the embedding,
// matching the bake/runtime contract.
const BodyEmbeddingPrefixLen = 1200

// specSection is one indexed section's identity + the canonical text used for
// BOTH its content hash and its document embedding (single source of truth).
type specSection struct {
	id   int
	text string
}

// specEmbeddingText assembles the one canonical text for a section. It must be
// identical at bake time and (if ever recomputed) at runtime so content_hash and
// the vector always agree.
func specEmbeddingText(heading, summary, bodyPreview, keywordsJSON, queriesJSON, aliasesJSON string) string {
	parts := []string{
		strings.TrimSpace(heading),
		strings.TrimSpace(summary),
		strings.TrimSpace(bodyPreview),
		jsonArrayToText(aliasesJSON),
		jsonArrayToText(keywordsJSON),
		jsonArrayToText(queriesJSON),
	}
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func jsonArrayToText(jsonStr string) string {
	var arr []string
	if json.Unmarshal([]byte(jsonStr), &arr) != nil {
		return ""
	}
	return strings.Join(arr, " ")
}

func specContentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// encodeSpecVector / decodeSpecVector serialize float32 vectors as little-endian
// bytes — the same layout internal/embedding uses, so the runtime can decode
// either way.
func encodeSpecVector(vector []float32) []byte {
	buffer := make([]byte, len(vector)*4)
	for index, value := range vector {
		binary.LittleEndian.PutUint32(buffer[index*4:], math.Float32bits(value))
	}
	return buffer
}

func decodeSpecVector(buffer []byte) []float32 {
	if len(buffer) == 0 || len(buffer)%4 != 0 {
		return nil
	}
	vector := make([]float32, len(buffer)/4)
	for index := range vector {
		vector[index] = math.Float32frombits(binary.LittleEndian.Uint32(buffer[index*4:]))
	}
	return vector
}

func normalizeSpecVector(vector []float32) []float32 {
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return vector
	}
	out := make([]float32, len(vector))
	for index, value := range vector {
		out[index] = float32(float64(value) / norm)
	}
	return out
}

// SpecEmbeddingScope selects which sections get baked vectors. The bake cost is
// dominated by CPU inference, so the scope is the one real speed/quality lever:
// the full corpus maximizes section-level recall but adds prose competitors that
// can outrank a target card; the pattern-card scope bakes only the 66 compiled
// thinking-pattern cards (seconds, no prose noise) — the right cut when the
// retrieval target is "which pattern answers this", not "which prose paragraph".
type SpecEmbeddingScope int

const (
	// ScopeAllSections bakes every indexed section (spec prose + pattern cards).
	ScopeAllSections SpecEmbeddingScope = iota
	// ScopePatternCards bakes only the compiled pattern cards (id >= PatternChunkIDBase).
	ScopePatternCards
)

func (s SpecEmbeddingScope) idFloor() int {
	if s == ScopePatternCards {
		return PatternChunkIDBase
	}
	return 0
}

// loadSpecSections reads each in-scope section's canonical embedding text,
// ordered by id for a deterministic bake.
func loadSpecSections(db *sql.DB, scope SpecEmbeddingScope) ([]specSection, error) {
	rows, err := db.Query(`
		SELECT id, heading, summary, substr(body, 1, ?), keywords_json, queries_json, aliases_json
		FROM sections WHERE id >= ? ORDER BY id`, BodyEmbeddingPrefixLen, scope.idFloor())
	if err != nil {
		return nil, fmt.Errorf("load sections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []specSection
	for rows.Next() {
		var id int
		var heading, summary, body, keywords, queries, aliases string
		if err := rows.Scan(&id, &heading, &summary, &body, &keywords, &queries, &aliases); err != nil {
			return nil, err
		}
		out = append(out, specSection{id: id, text: specEmbeddingText(heading, summary, body, keywords, queries, aliases)})
	}
	return out, rows.Err()
}

// BakeSpecEmbeddings embeds every indexed section (document role) via the given
// embedder and stores L2-normalized vectors in fpf_embeddings, keyed by
// section_id + the embedder's model contract. Deterministic: sections are
// embedded in id order and the model/dim are fixed, so a given spec + model
// yields byte-reproducible vectors. Takes the provider-free SemanticEmbedder
// port so internal/fpf never imports a provider.
func BakeSpecEmbeddings(ctx context.Context, dbPath string, embedder SemanticEmbedder, scope SpecEmbeddingScope) (int, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, fmt.Errorf("open index for bake: %w", err)
	}
	defer func() { _ = db.Close() }()

	sections, err := loadSpecSections(db, scope)
	if err != nil {
		return 0, err
	}
	if len(sections) == 0 {
		return 0, nil
	}

	descriptor := embedder.Descriptor()
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin bake tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	insert, err := tx.Prepare(`INSERT INTO fpf_embeddings (section_id, provider, model, dim, content_hash, vector) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare bake insert: %w", err)
	}
	defer func() { _ = insert.Close() }()

	for start := 0; start < len(sections); start += specEmbeddingBatch {
		end := min(start+specEmbeddingBatch, len(sections))
		batch := sections[start:end]
		texts := make([]string, len(batch))
		for i, section := range batch {
			texts[i] = section.text
		}
		vectors, err := embedder.EmbedTexts(ctx, texts)
		if err != nil {
			return 0, fmt.Errorf("embed sections [%d:%d]: %w", start, end, err)
		}
		fmt.Fprintf(os.Stderr, "  baking FPF vectors: %d/%d\n", end, len(sections))
		if len(vectors) != len(batch) {
			return 0, fmt.Errorf("embed sections: got %d vectors for %d texts", len(vectors), len(batch))
		}
		for i, section := range batch {
			normalized := normalizeSpecVector(vectors[i])
			if _, err := insert.Exec(section.id, descriptor.Provider, descriptor.Model, descriptor.Dimensions,
				specContentHash(section.text), encodeSpecVector(normalized)); err != nil {
				return 0, fmt.Errorf("store section %d vector: %w", section.id, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(sections), nil
}

// LoadSpecEmbeddings returns the baked section vectors that match the given
// model contract, keyed by section_id. A zero-length result means the index has
// no vectors for this contract (sidecar-less build or model mismatch) — the
// caller degrades to FTS.
func LoadSpecEmbeddings(db *sql.DB, provider, model string, dim int) (map[int][]float32, error) {
	rows, err := db.Query(`SELECT section_id, vector FROM fpf_embeddings WHERE provider=? AND model=? AND dim=?`,
		provider, model, dim)
	if err != nil {
		return nil, fmt.Errorf("load spec embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int][]float32)
	for rows.Next() {
		var sectionID int
		var blob []byte
		if err := rows.Scan(&sectionID, &blob); err != nil {
			return nil, err
		}
		if vector := decodeSpecVector(blob); vector != nil {
			out[sectionID] = vector
		}
	}
	return out, rows.Err()
}

// SpecEmbeddingContract reports whether the index carries baked vectors and
// under which model contract (for the runtime degrade/mismatch decision).
func SpecEmbeddingContract(db *sql.DB) (provider, model string, dim, count int, err error) {
	row := db.QueryRow(`SELECT provider, model, dim, COUNT(*) FROM fpf_embeddings GROUP BY provider, model, dim ORDER BY COUNT(*) DESC LIMIT 1`)
	err = row.Scan(&provider, &model, &dim, &count)
	if err == sql.ErrNoRows {
		return "", "", 0, 0, nil
	}
	return provider, model, dim, count, err
}

// SearchFTSSectionIDs is the section-level keyword arm for the hybrid: unlike
// runFTSQuery it carries SectionID and does NOT filter to pattern sections, so
// the ~5630 non-pattern sections compete on keyword too. It builds a safe FTS5
// match from the raw query (AND-first, OR-fallback), mirroring searchFTS.
func SearchFTSSectionIDs(db *sql.DB, query string, limit int) ([]SpecSearchResult, error) {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil, nil
	}
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = fmt.Sprintf(`"%s"*`, strings.ReplaceAll(t, `"`, `""`))
	}

	results, err := runSectionFTS(db, strings.Join(quoted, " "), limit)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 && len(quoted) > 1 {
		return runSectionFTS(db, strings.Join(quoted, " OR "), limit)
	}
	return results, nil
}

func runSectionFTS(db *sql.DB, matchExpr string, limit int) ([]SpecSearchResult, error) {
	rows, err := db.Query(`
		SELECT s.id, s.pattern_id, s.heading, s.summary, rank
		FROM fpf_fts
		JOIN sections s ON s.id = fpf_fts.section_id
		WHERE fpf_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, matchExpr, limit)
	if err != nil {
		return nil, fmt.Errorf("section fts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []SpecSearchResult
	for rows.Next() {
		var r SpecSearchResult
		var patternID sql.NullString
		if err := rows.Scan(&r.SectionID, &patternID, &r.Heading, &r.Summary, &r.Rank); err != nil {
			return nil, err
		}
		r.PatternID = patternID.String
		r.Tier = SpecSearchTierFTS
		r.Reason = "keyword match"
		results = append(results, r)
	}
	return results, rows.Err()
}

// HydrateSections returns presentation-ready results for section ids surfaced by
// the semantic arm but absent from the keyword pool.
func HydrateSections(db *sql.DB, ids []int) (map[int]SpecSearchResult, error) {
	out := make(map[int]SpecSearchResult, len(ids))
	for _, id := range ids {
		var r SpecSearchResult
		var patternID sql.NullString
		err := db.QueryRow(`SELECT pattern_id, heading, summary FROM sections WHERE id=?`, id).
			Scan(&patternID, &r.Heading, &r.Summary)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		r.SectionID = id
		r.PatternID = patternID.String
		r.Tier = "semantic"
		r.Reason = "semantic match"
		out[id] = r
	}
	return out, nil
}
