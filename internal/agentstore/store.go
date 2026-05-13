package agentstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
)

// Store is the disk-backed session repository. Each session lives in
// <root>/<id>/, with events.jsonl as the authoritative event log and
// meta.json as a denormalised lookup blob (project_id, title, last touch).
// meta.json is rewritten on every Append; it exists purely for fast
// listing without replaying every session.
//
// All Store methods are safe for concurrent use; an internal mutex
// serializes journal writes per session.
type Store struct {
	root string

	mu       sync.Mutex
	open     map[agentcore.SessionID]*Journal
	sessLock map[agentcore.SessionID]*sync.Mutex
}

// SessionMeta is the lightweight summary used by List. It is NOT a
// substitute for a Session value; consumers wanting state must call Load
// to replay the journal.
type SessionMeta struct {
	ID         agentcore.SessionID
	ProjectID  string
	Title      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Archived   bool
	EventCount int
}

// NewStore opens (or creates) a Store rooted at the given directory.
func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create store root: %w", err)
	}
	return &Store{
		root:     root,
		open:     map[agentcore.SessionID]*Journal{},
		sessLock: map[agentcore.SessionID]*sync.Mutex{},
	}, nil
}

// Close flushes all open journals.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for id, j := range s.open {
		if err := j.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(s.open, id)
	}
	return firstErr
}

// Create materializes a new session and writes the SessionCreated journal
// entry. Returns the freshly-built Session value and propagates any disk
// failure.
func (s *Store) Create(id agentcore.SessionID, projectID, title string, model agentcore.ModelChoice, now time.Time) (agentcore.Session, error) {
	if id == "" {
		return agentcore.Session{}, errors.New("agentstore: session id required")
	}
	lock := s.sessionLock(id)
	lock.Lock()
	defer lock.Unlock()
	dir := s.sessionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return agentcore.Session{}, fmt.Errorf("create session dir: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dir, journalFile)); err == nil {
		return agentcore.Session{}, fmt.Errorf("agentstore: session %s already exists", id)
	}

	journal, err := s.openJournal(id)
	if err != nil {
		return agentcore.Session{}, err
	}

	created := agentproto.SessionCreatedEvent{}
	created.SessionID = id
	created.At = now
	created.ProjectID = projectID
	created.Title = title
	created.Model = model

	if _, err := journal.Append(created); err != nil {
		return agentcore.Session{}, err
	}

	session := agentcore.NewSession(id, projectID, title, model, now)
	if err := s.writeMeta(id, session, projectID, false); err != nil {
		return agentcore.Session{}, err
	}
	return session, nil
}

// Load replays a session's journal into its current state. Returns
// ErrSessionNotFound if the session directory does not exist.
func (s *Store) Load(id agentcore.SessionID) (agentcore.Session, error) {
	path := filepath.Join(s.sessionDir(id), journalFile)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return agentcore.Session{}, ErrSessionNotFound
	}
	events, err := ReadAll(path)
	if err != nil {
		return agentcore.Session{}, err
	}
	return Replay(events)
}

// Append writes an event to the session's journal and updates meta.json.
// The session must already exist (call Create first); appending a
// SessionCreated event after creation returns an error.
//
// A per-session mutex is held across Load/Apply/Append so concurrent
// callers cannot validate against a stale snapshot and then write
// conflicting events (e.g. a `model.switched` admitted on top of a
// turn-running state that a sibling goroutine created moments later).
func (s *Store) Append(id agentcore.SessionID, ev agentproto.AgentEvent) error {
	if ev.Kind() == agentproto.EventSessionCreated {
		return fmt.Errorf("agentstore: session.created may only be emitted by Create()")
	}
	if !IsJournalEvent(ev.Kind()) {
		return fmt.Errorf("%w: %s", ErrNotJournalEvent, ev.Kind())
	}
	if ev.Session() != id {
		return fmt.Errorf("agentstore: event session %s does not match journal %s", ev.Session(), id)
	}

	lock := s.sessionLock(id)
	lock.Lock()
	defer lock.Unlock()

	// Require an existing session. openJournal would otherwise materialize
	// an empty events.jsonl under a freshly created directory, leaving an
	// invalid journal (no SessionCreated header) that future Load calls
	// reject and future Create calls refuse because the file exists.
	current, err := s.Load(id)
	if err != nil {
		return err
	}
	// Validate the transition in-memory BEFORE writing. A rejected event
	// (e.g. model.switched while a turn is running) would otherwise be
	// persisted and make the journal unreplayable on the next Load.
	next, err := Apply(current, ev)
	if err != nil {
		return err
	}

	journal, err := s.openJournal(id)
	if err != nil {
		return err
	}
	if _, err := journal.Append(ev); err != nil {
		return err
	}

	meta, err := s.readMeta(id)
	if err != nil {
		return fmt.Errorf("read existing meta: %w", err)
	}
	if ev.Kind() == agentproto.EventSessionArchived {
		meta.Archived = true
	}
	return s.writeMeta(id, next, meta.ProjectID, meta.Archived)
}

// List enumerates session metadata, optionally filtered by project. Empty
// projectID means "all projects". Results are sorted by UpdatedAt
// descending so the TUI sidebar shows the most recent sessions first.
func (s *Store) List(projectID string) ([]SessionMeta, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read store root: %w", err)
	}
	out := make([]SessionMeta, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := agentcore.SessionID(e.Name())
		meta, err := s.readMeta(id)
		if err != nil {
			// Skip half-written sessions rather than failing the whole list.
			continue
		}
		if projectID != "" && meta.ProjectID != projectID {
			continue
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// ErrSessionNotFound is returned by Load when the session directory does
// not exist.
var ErrSessionNotFound = errors.New("agentstore: session not found")

const (
	journalFile = "events.jsonl"
	metaFile    = "meta.json"
)

func (s *Store) sessionDir(id agentcore.SessionID) string {
	return filepath.Join(s.root, string(id))
}

// sessionLock returns the per-session mutex used to serialize the
// Load/Apply/Append sequence in Create and Append. Without it two
// concurrent appends on the same session could validate against the
// same pre-snapshot and persist mutually-inconsistent events.
func (s *Store) sessionLock(id agentcore.SessionID) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.sessLock[id]
	if !ok {
		m = &sync.Mutex{}
		s.sessLock[id] = m
	}
	return m
}

func (s *Store) openJournal(id agentcore.SessionID) (*Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.open[id]; ok {
		return j, nil
	}
	path := filepath.Join(s.sessionDir(id), journalFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("ensure session dir: %w", err)
	}
	j, err := OpenJournal(path)
	if err != nil {
		return nil, err
	}
	s.open[id] = j
	return j, nil
}
