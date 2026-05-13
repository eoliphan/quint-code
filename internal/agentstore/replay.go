package agentstore

import (
	"errors"
	"fmt"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
)

// Replay reconstructs an agentcore.Session by applying a sequence of
// journal events to the zero Session. Pure: no IO, no globals.
//
// The first event must be SessionCreated; any other first event yields
// ErrJournalMissingHeader. Internal apply errors are wrapped with the
// 1-based event index so the caller can pinpoint which event broke the
// chain.
func Replay(events []agentproto.AgentEvent) (agentcore.Session, error) {
	if len(events) == 0 {
		return agentcore.Session{}, ErrEmptyJournal
	}
	if events[0].Kind() != agentproto.EventSessionCreated {
		return agentcore.Session{}, ErrJournalMissingHeader
	}
	var s agentcore.Session
	for i, ev := range events {
		if !IsJournalEvent(ev.Kind()) {
			return agentcore.Session{}, fmt.Errorf("event %d: non-journal event %s in journal", i+1, ev.Kind())
		}
		next, err := Apply(s, ev)
		if err != nil {
			return agentcore.Session{}, fmt.Errorf("event %d (%s): %w", i+1, ev.Kind(), err)
		}
		s = next
	}
	return s, nil
}

// Sentinel errors for replay failure modes that callers may want to
// match with errors.Is.
var (
	ErrEmptyJournal         = errors.New("agentstore: empty journal")
	ErrJournalMissingHeader = errors.New("agentstore: first event must be session.created")
)
