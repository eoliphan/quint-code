package agentstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// metaPayload is the on-disk shape of meta.json. It mirrors SessionMeta
// but uses time.Time directly; encoding/json formats it consistently.
type metaPayload struct {
	ID         agentcore.SessionID `json:"id"`
	ProjectID  string              `json:"project_id"`
	Title      string              `json:"title"`
	CreatedAt  string              `json:"created_at"`
	UpdatedAt  string              `json:"updated_at"`
	Archived   bool                `json:"archived"`
	EventCount int                 `json:"event_count"`
}

func (s *Store) writeMeta(id agentcore.SessionID, session agentcore.Session, projectID string, archived bool) error {
	dir := s.sessionDir(id)
	count, err := countLines(filepath.Join(dir, journalFile))
	if err != nil {
		return err
	}
	payload := metaPayload{
		ID:         session.ID,
		ProjectID:  projectID,
		Title:      session.Title,
		CreatedAt:  session.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		UpdatedAt:  session.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		Archived:   archived,
		EventCount: count,
	}
	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	tmp := filepath.Join(dir, metaFile+".tmp")
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, metaFile)); err != nil {
		return fmt.Errorf("commit meta: %w", err)
	}
	return nil
}

func (s *Store) readMeta(id agentcore.SessionID) (SessionMeta, error) {
	path := filepath.Join(s.sessionDir(id), metaFile)
	bytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionMeta{}, ErrSessionNotFound
		}
		return SessionMeta{}, fmt.Errorf("read meta: %w", err)
	}
	var payload metaPayload
	if err := json.Unmarshal(bytes, &payload); err != nil {
		return SessionMeta{}, fmt.Errorf("parse meta: %w", err)
	}
	createdAt, _ := parseISO(payload.CreatedAt)
	updatedAt, _ := parseISO(payload.UpdatedAt)
	return SessionMeta{
		ID:         payload.ID,
		ProjectID:  payload.ProjectID,
		Title:      payload.Title,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		Archived:   payload.Archived,
		EventCount: payload.EventCount,
	}, nil
}
