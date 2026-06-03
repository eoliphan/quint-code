package codebase

import (
	"context"
	"database/sql"
	"fmt"
)

// EdgeKind enumerates the code→code relationships walked by traversal. Reference
// edges are deliberately NOT here — they are far more numerous and resolve far
// weaker, so they never enter the traversal set (per the parity decision).
type EdgeKind string

const (
	EdgeCall              EdgeKind = "call"
	EdgeInterfaceDispatch EdgeKind = "interface_dispatch"
)

// Provenance records how an edge was established. static = resolved directly
// from the AST + symbol table; heuristic = synthesized (e.g. structural
// interface→impl matching), which the agent can treat with appropriate caution.
type Provenance string

const (
	ProvenanceStatic    Provenance = "static"
	ProvenanceHeuristic Provenance = "heuristic"
)

// CodeEdge is a resolved directed relationship between two symbol nodes. It
// exists ONLY when both endpoints resolved — an unresolved call is an absent
// edge, never a nullable one ("partial coverage worse than none", in the type).
type CodeEdge struct {
	SrcID      string
	DstID      string
	Kind       EdgeKind
	FilePath   string
	Line       int
	Provenance Provenance
}

const codeEdgesSchema = `
CREATE TABLE IF NOT EXISTS code_edges (
  src_id     TEXT NOT NULL,
  dst_id     TEXT NOT NULL,
  kind       TEXT NOT NULL,
  file_path  TEXT NOT NULL,
  line       INTEGER NOT NULL DEFAULT 0,
  provenance TEXT NOT NULL,
  PRIMARY KEY (src_id, dst_id, kind, file_path, line)
);
CREATE INDEX IF NOT EXISTS idx_code_edges_src ON code_edges(src_id);
CREATE INDEX IF NOT EXISTS idx_code_edges_dst ON code_edges(dst_id);`

// EdgeStore persists code edges. Shell over *sql.DB; the caller owns lifecycle.
type EdgeStore struct {
	db *sql.DB
}

// NewEdgeStore creates an edge store over an existing DB connection.
func NewEdgeStore(db *sql.DB) *EdgeStore { return &EdgeStore{db: db} }

// EnsureSchema creates the code_edges table + indexes if absent (idempotent).
func (e *EdgeStore) EnsureSchema(ctx context.Context) error {
	if _, err := e.db.ExecContext(ctx, codeEdgesSchema); err != nil {
		return fmt.Errorf("ensure code_edges schema: %w", err)
	}
	return nil
}

// ReplaceFileEdges idempotently rebuilds the edges originating in one file
// (delete-by-file then insert) so re-indexing a file is exact, not additive.
func (e *EdgeStore) ReplaceFileEdges(ctx context.Context, filePath string, edges []CodeEdge) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM code_edges WHERE file_path = ?`, filePath); err != nil {
		return err
	}
	for _, ed := range edges {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO code_edges (src_id, dst_id, kind, file_path, line, provenance)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			ed.SrcID, ed.DstID, string(ed.Kind), ed.FilePath, ed.Line, string(ed.Provenance),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const (
	outEdgesQuery = `SELECT src_id, dst_id, kind, file_path, line, provenance FROM code_edges WHERE src_id = ? ORDER BY dst_id, src_id`
	inEdgesQuery  = `SELECT src_id, dst_id, kind, file_path, line, provenance FROM code_edges WHERE dst_id = ? ORDER BY dst_id, src_id`
)

// OutEdges returns edges where srcID is the source (its callees / dispatch targets).
func (e *EdgeStore) OutEdges(ctx context.Context, srcID string) ([]CodeEdge, error) {
	rows, err := e.db.QueryContext(ctx, outEdgesQuery, srcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// InEdges returns edges where dstID is the target (its callers / dispatchers).
func (e *EdgeStore) InEdges(ctx context.Context, dstID string) ([]CodeEdge, error) {
	rows, err := e.db.QueryContext(ctx, inEdgesQuery, dstID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

func scanEdges(rows *sql.Rows) ([]CodeEdge, error) {
	var out []CodeEdge
	for rows.Next() {
		var ed CodeEdge
		var kind, prov string
		if err := rows.Scan(&ed.SrcID, &ed.DstID, &kind, &ed.FilePath, &ed.Line, &prov); err != nil {
			return nil, err
		}
		ed.Kind = EdgeKind(kind)
		ed.Provenance = Provenance(prov)
		out = append(out, ed)
	}
	return out, rows.Err()
}
