package agentstore

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/m0n0x41d/haft/internal/agentproto"
)

// Journal is a per-session append-only event log persisted as JSONL. One
// event per line; lines are encoded by [agentproto.EncodeEvent]. Append is
// fsync'd by default — sessions survive a kernel panic mid-stream.
//
// A Journal owns a single open file descriptor. Concurrent Append calls
// are serialized via the embedded mutex; concurrent reads via Iterate run
// against a separate read-only file handle and do not block writes.
type Journal struct {
	path string
	w    *os.File
}

// OpenJournal opens (or creates) the journal file at path. The directory
// must exist; the store layer is responsible for directory layout.
func OpenJournal(path string) (*Journal, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open journal %s: %w", path, err)
	}
	return &Journal{path: path, w: f}, nil
}

// Append writes one event to the journal and fsyncs. Returns the wire
// bytes the event was encoded to so callers can broadcast the same bytes
// over SSE without re-encoding.
func (j *Journal) Append(ev agentproto.AgentEvent) ([]byte, error) {
	if !IsJournalEvent(ev.Kind()) {
		return nil, fmt.Errorf("%w: %s", ErrNotJournalEvent, ev.Kind())
	}
	bytes, err := agentproto.EncodeEvent(ev)
	if err != nil {
		return nil, fmt.Errorf("encode event: %w", err)
	}
	if _, err := j.w.Write(append(bytes, '\n')); err != nil {
		return nil, fmt.Errorf("write event: %w", err)
	}
	if err := j.w.Sync(); err != nil {
		return nil, fmt.Errorf("fsync journal: %w", err)
	}
	return bytes, nil
}

// Close releases the writer. Safe to call multiple times.
func (j *Journal) Close() error {
	if j.w == nil {
		return nil
	}
	err := j.w.Close()
	j.w = nil
	return err
}

// ReadAll reads the journal start-to-end and returns all events. Used by
// the store on Load to replay a session into its current state.
func ReadAll(path string) ([]agentproto.AgentEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	defer f.Close()

	var events []agentproto.AgentEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		ev, err := agentproto.DecodeEvent(line)
		if err != nil {
			return nil, fmt.Errorf("decode line %d: %w", lineNum, err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		if err != io.EOF {
			return nil, fmt.Errorf("scan journal: %w", err)
		}
	}
	return events, nil
}
