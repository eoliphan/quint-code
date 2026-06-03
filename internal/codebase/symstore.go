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
	rows, err := s.db.QueryContext(ctx, codeSymbolSelect+` WHERE file_path LIKE ? ORDER BY file_path, start_line`, dir+"/%")
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

// SearchSymbols returns symbols whose name CONTAINS q (case-insensitive),
// deterministically ranked — exact name, then prefix, then shorter names — and
// capped. The fuzzy fallback for seed resolution when the exact name is not
// known. Deterministic (LIKE + a fixed Go sort); no embeddings, no second
// runtime. An empty/whitespace query matches nothing (never "everything").
func (s *SymbolStore) SearchSymbols(ctx context.Context, q string, limit int) ([]CodeSymbol, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx,
		codeSymbolSelect+` WHERE instr(lower(name), lower(?)) > 0 ORDER BY name, file_path, start_line LIMIT ?`,
		q, limit*4) // over-fetch, then rank + trim in Go
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanCodeSymbols(rows)
	if err != nil {
		return nil, err
	}
	rankSymbolMatches(all, q)
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// rankSymbolMatches orders matches by closeness to the query: exact name first,
// then prefix matches, then shorter names, then name/file for stable output.
func rankSymbolMatches(syms []CodeSymbol, q string) {
	lq := strings.ToLower(q)
	rank := func(c CodeSymbol) int {
		ln := strings.ToLower(c.Name)
		switch {
		case ln == lq:
			return 0
		case strings.HasPrefix(ln, lq):
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(syms, func(i, j int) bool {
		ri, rj := rank(syms[i]), rank(syms[j])
		if ri != rj {
			return ri < rj
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
