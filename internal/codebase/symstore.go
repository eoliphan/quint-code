package codebase

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/m0n0x41d/haft/internal/textsearch"
)

// CodeSymbol is a persisted symbol node. Identity is (FilePath, Name, StartLine)
// — so two same-name methods on different receivers (different start lines) are
// two distinct nodes, never one. ID is the deterministic surrogate derived from
// that identity, used as the stable handle that code_edges reference. Immutable
// value; the store is the only shell.
type CodeSymbol struct {
	ID        string
	FilePath  string
	Name      string
	Kind      string
	Receiver  string
	StartLine int
	EndLine   int
	StartByte int
	EndByte   int
	Hash      string
	Exported  bool
	Lang      string
}

// NodeID is the deterministic identity hash for a symbol node — same identity
// (file, name, start line) always yields the same id across re-indexing, so
// edges stay valid through an idempotent rebuild of unchanged symbols.
func NodeID(filePath, name string, startLine int) string {
	return fmt.Sprintf("%s#%s#%d", filePath, name, startLine)
}

const codeSymbolsSchema = `
CREATE TABLE IF NOT EXISTS code_symbols (
  id         TEXT PRIMARY KEY,
  file_path  TEXT NOT NULL,
  name       TEXT NOT NULL,
  kind       TEXT,
  receiver   TEXT,
  start_line INTEGER NOT NULL,
  end_line   INTEGER,
  start_byte INTEGER,
  end_byte   INTEGER,
  hash       TEXT,
  exported   INTEGER DEFAULT 0,
  lang       TEXT,
  UNIQUE (file_path, name, start_line)
);
CREATE INDEX IF NOT EXISTS idx_code_symbols_name ON code_symbols(name);
CREATE INDEX IF NOT EXISTS idx_code_symbols_file ON code_symbols(file_path);`

// SymbolStore persists code symbols (the node layer of the code graph). It does
// not own the DB connection — the caller manages lifecycle.
type SymbolStore struct {
	db *sql.DB
}

// NewSymbolStore creates a symbol store over an existing DB connection.
func NewSymbolStore(db *sql.DB) *SymbolStore { return &SymbolStore{db: db} }

// EnsureSchema creates the code_symbols table + indexes if absent (idempotent).
func (s *SymbolStore) EnsureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, codeSymbolsSchema); err != nil {
		return fmt.Errorf("ensure code_symbols schema: %w", err)
	}
	return nil
}

// codeSymbolFromSnapshot is the pure mapping extraction snapshot → persisted node.
func codeSymbolFromSnapshot(snap SymbolSnapshot, lang string) CodeSymbol {
	return CodeSymbol{
		ID:        NodeID(snap.FilePath, snap.SymbolName, snap.Line),
		FilePath:  snap.FilePath,
		Name:      snap.SymbolName,
		Kind:      snap.SymbolKind,
		Receiver:  snap.Receiver,
		StartLine: snap.Line,
		EndLine:   snap.EndLine,
		StartByte: snap.StartByte,
		EndByte:   snap.EndByte,
		Hash:      snap.Hash,
		Exported:  snap.Exported,
		Lang:      lang,
	}
}

// langNameForExt resolves a file extension to its extractor language name.
func langNameForExt(ext string) string {
	if li, ok := languages[ext]; ok {
		return li.name
	}
	return ""
}

// ReplaceFileSymbols idempotently rebuilds one file's symbol rows in a single
// transaction (delete-then-insert), so re-indexing a file is exact, not additive.
func (s *SymbolStore) ReplaceFileSymbols(ctx context.Context, filePath string, syms []CodeSymbol) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM code_symbols WHERE file_path = ?`, filePath); err != nil {
		return err
	}
	for _, sym := range syms {
		id := sym.ID
		if id == "" {
			id = NodeID(sym.FilePath, sym.Name, sym.StartLine)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO code_symbols
			 (id, file_path, name, kind, receiver, start_line, end_line, start_byte, end_byte, hash, exported, lang)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, sym.FilePath, sym.Name, sym.Kind, sym.Receiver, sym.StartLine, sym.EndLine,
			sym.StartByte, sym.EndByte, sym.Hash, boolToInt(sym.Exported), sym.Lang,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IndexFileSymbols extracts a file and replaces its symbol rows. The rebuild-on-
// demand half of the freshness primitive — calling it makes the store match disk.
func (s *SymbolStore) IndexFileSymbols(ctx context.Context, projectRoot, relPath string) error {
	snaps, err := ExtractSymbolSnapshots(projectRoot, relPath)
	if err != nil {
		return err
	}
	lang := langNameForExt(filepath.Ext(relPath))
	syms := make([]CodeSymbol, 0, len(snaps))
	for _, snap := range snaps {
		syms = append(syms, codeSymbolFromSnapshot(snap, lang))
	}
	return s.ReplaceFileSymbols(ctx, relPath, syms)
}

// GetByFile returns all symbols stored for a file, ordered by start line.
func (s *SymbolStore) GetByFile(ctx context.Context, filePath string) ([]CodeSymbol, error) {
	rows, err := s.db.QueryContext(ctx, codeSymbolSelect+` WHERE file_path = ? ORDER BY start_line`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodeSymbols(rows)
}

// GetByDir returns symbols whose file lives DIRECTLY in dir (a Go package =
// the files in one directory, not nested). Used to scope impl resolution to a
// single package so bare receiver names don't collide across packages.
func (s *SymbolStore) GetByDir(ctx context.Context, dir string) ([]CodeSymbol, error) {
	dir = filepath.Clean(dir)
	pattern := dir + "/%"
	if dir == "." {
		pattern = "%"
	}

	rows, err := s.db.QueryContext(ctx, codeSymbolSelect+` WHERE file_path LIKE ? ORDER BY file_path, start_line`, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all, err := scanCodeSymbols(rows)
	if err != nil {
		return nil, err
	}

	out := make([]CodeSymbol, 0, len(all))
	for _, sym := range all {
		if filepath.Dir(sym.FilePath) == dir {
			out = append(out, sym)
		}
	}
	return out, nil
}

// GetByName returns all symbols with the given name across files (overloads incl.).
func (s *SymbolStore) GetByName(ctx context.Context, name string) ([]CodeSymbol, error) {
	rows, err := s.db.QueryContext(ctx, codeSymbolSelect+` WHERE name = ? ORDER BY file_path, start_line`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodeSymbols(rows)
}

// SymbolRef is a symbol's stable id paired with its file — the minimum the
// fused-graph ranker needs to bridge a symbol node to its file connector node
// (graphrank, dec-20260604-3aaad199 phase 2) without loading full symbol rows.
type SymbolRef struct {
	ID       string
	FilePath string
}

// AllSymbolRefs enumerates every symbol's (id, file) in one pass, stably ordered.
func (s *SymbolStore) AllSymbolRefs(ctx context.Context) ([]SymbolRef, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, file_path FROM code_symbols ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SymbolRef
	for rows.Next() {
		var r SymbolRef
		if err := rows.Scan(&r.ID, &r.FilePath); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// editScanCap bounds how many names the edit-distance fallback scans, so a
// typo query stays cheap even on a large index. Names beyond the cap are not
// considered — surfaced as a silent ceiling, never as a wrong match.
const editScanCap = 20000

// SearchSymbols returns symbols whose name CONTAINS q (case-insensitive),
// deterministically ranked — exact, prefix, substring, then fuzzy — and capped.
// The seed-resolution fallback when the exact name is not known. When substring
// finds nothing it falls back to a bounded edit-distance scan so a TYPO still
// resolves (autenticate -> authenticate), the tier the store previously lacked.
// Deterministic (LIKE / edit distance + a fixed Go sort); no embeddings, no
// second runtime. An empty/whitespace query matches nothing (never "everything").
func (s *SymbolStore) SearchSymbols(ctx context.Context, q string, limit int) ([]CodeSymbol, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 25
	}

	// Field-qualified parse (dec-20260604-3aaad199): kind:/lang:/path:/name:
	// narrow the match; the remainder is the free-text symbol term. A bare name
	// has no fields, so existing seed-resolution callers are unaffected.
	pq := textsearch.ParseQuery(q)
	term := pq.Text
	if term == "" && len(pq.NameFilters) > 0 {
		term = pq.NameFilters[0]
	}

	var all []CodeSymbol
	var err error
	if term != "" {
		all, err = s.searchByName(ctx, term, limit*4) // over-fetch, rank + trim in Go
	} else {
		// Filter-only query (e.g. "kind:function"): scan, then filter in Go.
		all, err = s.scanAllSymbols(ctx, editScanCap)
	}
	if err != nil {
		return nil, err
	}

	all = applySymbolFilters(all, pq)
	rankSymbolMatches(all, term)
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// searchByName resolves a free-text symbol term: case-insensitive substring on
// name, then a bounded edit-distance scan (typo tolerance) ONLY when substring
// matched nothing — so the exact behavior is never regressed. Returns ranked
// candidates; the caller lists them and never auto-picks (no-wrong-edge).
func (s *SymbolStore) searchByName(ctx context.Context, term string, fetch int) ([]CodeSymbol, error) {
	rows, err := s.db.QueryContext(ctx,
		codeSymbolSelect+` WHERE instr(lower(name), lower(?)) > 0 ORDER BY name, file_path, start_line LIMIT ?`,
		term, fetch)
	if err != nil {
		return nil, err
	}
	all, err := scanCodeSymbols(rows)
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return s.searchByEditDistance(ctx, term, maxEditDist(term))
	}
	return all, nil
}

// scanAllSymbols loads up to maxScan symbols for a filter-only query.
func (s *SymbolStore) scanAllSymbols(ctx context.Context, maxScan int) ([]CodeSymbol, error) {
	rows, err := s.db.QueryContext(ctx,
		codeSymbolSelect+` ORDER BY name, file_path, start_line LIMIT ?`, maxScan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodeSymbols(rows)
}

// applySymbolFilters keeps only symbols satisfying every field-qualified filter
// present in pq. Within a category the filters are OR'd (kind:a kind:b → a or b);
// categories AND together. An absent category imposes no constraint.
func applySymbolFilters(syms []CodeSymbol, pq textsearch.ParsedQuery) []CodeSymbol {
	if len(pq.Kinds) == 0 && len(pq.Langs) == 0 && len(pq.PathFilters) == 0 && len(pq.NameFilters) == 0 {
		return syms
	}
	out := make([]CodeSymbol, 0, len(syms))
	for _, c := range syms {
		if anyEqualFold(pq.Kinds, c.Kind) &&
			anyEqualFold(pq.Langs, c.Lang) &&
			anySubstringFold(pq.PathFilters, c.FilePath) &&
			anySubstringFold(pq.NameFilters, c.Name) {
			out = append(out, c)
		}
	}
	return out
}

// anyEqualFold reports whether c case-insensitively equals any of xs. An empty
// xs is "no constraint" (true).
func anyEqualFold(xs []string, c string) bool {
	if len(xs) == 0 {
		return true
	}
	for _, x := range xs {
		if strings.EqualFold(x, c) {
			return true
		}
	}
	return false
}

// anySubstringFold reports whether c case-insensitively contains any of xs. An
// empty xs is "no constraint" (true).
func anySubstringFold(xs []string, c string) bool {
	if len(xs) == 0 {
		return true
	}
	lc := strings.ToLower(c)
	for _, x := range xs {
		if strings.Contains(lc, strings.ToLower(x)) {
			return true
		}
	}
	return false
}

// maxEditDist scales typo tolerance with query length: a short token can only
// afford 1 edit before it spans unrelated names; longer tokens allow 2.
func maxEditDist(q string) int {
	if len(q) <= 4 {
		return 1
	}
	return 2
}

// searchByEditDistance scans up to editScanCap names and keeps those within
// maxDist Levenshtein edits of q (case-folded). Bounded edit distance early-exits
// per name, so the scan stays cheap.
func (s *SymbolStore) searchByEditDistance(ctx context.Context, q string, maxDist int) ([]CodeSymbol, error) {
	rows, err := s.db.QueryContext(ctx,
		codeSymbolSelect+` ORDER BY name, file_path, start_line LIMIT ?`, editScanCap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanCodeSymbols(rows)
	if err != nil {
		return nil, err
	}
	lq := strings.ToLower(q)
	hits := make([]CodeSymbol, 0, 8)
	for _, c := range all {
		if textsearch.BoundedEditDistance(strings.ToLower(c.Name), lq, maxDist) <= maxDist {
			hits = append(hits, c)
		}
	}
	return hits, nil
}

// rankSymbolMatches orders matches by closeness to the query: exact name, then
// prefix, then substring, then fuzzy; within a tier, exported symbols before
// unexported, callable/type kinds before fields/vars, then shorter names, then
// name/file for stable, deterministic output.
func rankSymbolMatches(syms []CodeSymbol, q string) {
	lq := strings.ToLower(q)
	tier := func(c CodeSymbol) int {
		ln := strings.ToLower(c.Name)
		switch {
		case ln == lq:
			return 0
		case strings.HasPrefix(ln, lq):
			return 1
		case strings.Contains(ln, lq):
			return 2
		default:
			return 3 // fuzzy / edit-distance match
		}
	}
	sort.SliceStable(syms, func(i, j int) bool {
		if ti, tj := tier(syms[i]), tier(syms[j]); ti != tj {
			return ti < tj
		}
		// A generated stub (protobuf, mock, codegen) shares names with the
		// hand-written code it wraps — surface the real implementation first.
		if gi, gj := textsearch.IsGeneratedPath(syms[i].FilePath), textsearch.IsGeneratedPath(syms[j].FilePath); gi != gj {
			return !gi
		}
		if syms[i].Exported != syms[j].Exported {
			return syms[i].Exported // exported API before internals
		}
		if ki, kj := kindRank(syms[i].Kind), kindRank(syms[j].Kind); ki != kj {
			return ki < kj
		}
		if len(syms[i].Name) != len(syms[j].Name) {
			return len(syms[i].Name) < len(syms[j].Name)
		}
		if syms[i].Name != syms[j].Name {
			return syms[i].Name < syms[j].Name
		}
		return syms[i].FilePath < syms[j].FilePath
	})
}

// kindRank orders symbol kinds by typical search relevance (lower = better):
// callables and type definitions above fields/vars/consts; unknown kinds last.
func kindRank(kind string) int {
	switch kind {
	case "func", "function", "method":
		return 0
	case "interface", "type", "struct", "class", "trait", "enum":
		return 1
	case "const", "constant", "var", "variable", "field", "property":
		return 2
	default:
		return 3
	}
}

// GetByID returns the single node with the given surrogate id, if present. The
// inverse of NodeID — resolves a traversal hop back to its symbol for display.
func (s *SymbolStore) GetByID(ctx context.Context, id string) (CodeSymbol, bool, error) {
	rows, err := s.db.QueryContext(ctx, codeSymbolSelect+` WHERE id = ?`, id)
	if err != nil {
		return CodeSymbol{}, false, err
	}
	defer rows.Close()
	syms, err := scanCodeSymbols(rows)
	if err != nil || len(syms) == 0 {
		return CodeSymbol{}, false, err
	}
	return syms[0], true, nil
}

// GetByIdentity returns the single node at (file, name, start_line), if present.
func (s *SymbolStore) GetByIdentity(ctx context.Context, file, name string, startLine int) (CodeSymbol, bool, error) {
	rows, err := s.db.QueryContext(ctx, codeSymbolSelect+` WHERE file_path = ? AND name = ? AND start_line = ?`, file, name, startLine)
	if err != nil {
		return CodeSymbol{}, false, err
	}
	defer rows.Close()
	syms, err := scanCodeSymbols(rows)
	if err != nil || len(syms) == 0 {
		return CodeSymbol{}, false, err
	}
	return syms[0], true, nil
}

// FileSymbolsStale reports whether the file on disk no longer matches the stored
// symbols — by node identity + body hash. The is-stale half of the freshness
// primitive; pair with IndexFileSymbols to rebuild on demand before slicing.
func (s *SymbolStore) FileSymbolsStale(ctx context.Context, projectRoot, relPath string) (bool, error) {
	current, err := ExtractSymbolSnapshots(projectRoot, relPath)
	if err != nil {
		return true, err
	}
	stored, err := s.GetByFile(ctx, relPath)
	if err != nil {
		return true, err
	}
	if len(current) != len(stored) {
		return true, nil
	}
	storedKey := make(map[string]string, len(stored)) // (name|startLine) -> hash
	for _, sym := range stored {
		storedKey[fmt.Sprintf("%s|%d", sym.Name, sym.StartLine)] = sym.Hash
	}
	for _, snap := range current {
		h, ok := storedKey[fmt.Sprintf("%s|%d", snap.SymbolName, snap.Line)]
		if !ok || h != snap.Hash {
			return true, nil
		}
	}
	return false, nil
}

// SliceBody returns the byte-exact body of a symbol from file content. Pure;
// returns ok=false if the offsets don't fit the content (stale — caller must
// re-index before trusting source).
func SliceBody(content []byte, sym CodeSymbol) ([]byte, bool) {
	if sym.StartByte < 0 || sym.EndByte <= sym.StartByte || sym.EndByte > len(content) {
		return nil, false
	}
	return content[sym.StartByte:sym.EndByte], true
}

// VerifyBody slices the symbol's body from freshly-read content AND re-hashes
// it against the stored hash — the freshness guarantee for P3. fresh=true means
// the stored byte range still points at the exact same source on disk; false
// means the file was edited (offsets stale or content changed) and the caller
// must re-index before trusting the slice. Pure: the hash comparison, not a
// stored-hash-vs-stored-hash check, is what catches an actively-edited file.
func VerifyBody(content []byte, sym CodeSymbol) (body []byte, fresh bool) {
	slice, ok := SliceBody(content, sym)
	if !ok {
		return nil, false
	}
	if sym.Hash == "" {
		return slice, false // no baseline hash to verify against — treat as unverified
	}
	sum := sha256.Sum256(slice)
	return slice, hex.EncodeToString(sum[:]) == sym.Hash
}

const codeSymbolSelect = `SELECT id, file_path, name, kind, receiver, start_line, end_line, start_byte, end_byte, hash, exported, lang FROM code_symbols`

func scanCodeSymbols(rows *sql.Rows) ([]CodeSymbol, error) {
	var out []CodeSymbol
	for rows.Next() {
		var c CodeSymbol
		var exported int
		var receiver, kind, hash, lang sql.NullString
		if err := rows.Scan(&c.ID, &c.FilePath, &c.Name, &kind, &receiver, &c.StartLine, &c.EndLine, &c.StartByte, &c.EndByte, &hash, &exported, &lang); err != nil {
			return nil, err
		}
		c.Kind = kind.String
		c.Receiver = receiver.String
		c.Hash = hash.String
		c.Lang = lang.String
		c.Exported = exported != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
