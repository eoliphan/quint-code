package recall

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/m0n0x41d/haft/internal/embedding"
)

// vectorCache persists artifact embeddings keyed by the model contract
// (provider/model/dim). content_hash lets the corpus builder skip re-embedding
// artifacts whose text is unchanged. Dropping the table just forces a recompute
// — never a migration to reverse (decision dec-20260605-fe77b358).
type vectorCache struct {
	db *sql.DB
}

type cachedVector struct {
	Vector      []float32
	ContentHash string
}

// load returns every cached vector for the given model contract, keyed by
// artifact id. Foreign/corrupt blobs (wrong width) are skipped, not fatal.
func (c vectorCache) load(ctx context.Context, descriptor embedding.Descriptor) (map[string]cachedVector, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT artifact_id, content_hash, vector FROM artifact_embeddings WHERE provider=? AND model=? AND dim=?`,
		descriptor.Provider, descriptor.Model, descriptor.Dimensions)
	if err != nil {
		return nil, fmt.Errorf("load embeddings: %w", err)
	}
	defer rows.Close()

	cached := make(map[string]cachedVector)
	for rows.Next() {
		var id, hash string
		var blob []byte
		if err := rows.Scan(&id, &hash, &blob); err != nil {
			return nil, fmt.Errorf("scan embedding: %w", err)
		}
		vector := embedding.DecodeVector(blob)
		if vector == nil {
			continue
		}
		cached[id] = cachedVector{Vector: vector, ContentHash: hash}
	}
	return cached, rows.Err()
}

// store upserts one artifact's vector for the model contract.
func (c vectorCache) store(
	ctx context.Context,
	descriptor embedding.Descriptor,
	artifactID string,
	contentHash string,
	vector []float32,
) error {
	_, err := c.db.ExecContext(ctx,
		`INSERT INTO artifact_embeddings (artifact_id, provider, model, dim, content_hash, vector, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(artifact_id, provider, model, dim)
		 DO UPDATE SET content_hash=excluded.content_hash, vector=excluded.vector, updated_at=CURRENT_TIMESTAMP`,
		artifactID, descriptor.Provider, descriptor.Model, descriptor.Dimensions,
		contentHash, embedding.EncodeVector(vector))
	if err != nil {
		return fmt.Errorf("store embedding %s: %w", artifactID, err)
	}
	return nil
}
