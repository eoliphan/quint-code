package agentdriver

import (
	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/agentstore"
)

// EventSink is what the driver emits events to. Implementations decide
// whether to journal, broadcast, or both. The driver does not care.
//
// Sink errors are surfaced as turn failures: a journal write that fails
// is treated as if the provider errored, because the session can't
// reliably continue without persisting state.
type EventSink interface {
	Publish(ev agentproto.AgentEvent) error
}

// CombinedSink is the production EventSink: it journals state-mutating
// events through the Store and broadcasts every event to a Hub-like
// publisher. Streaming deltas are broadcast only.
type CombinedSink struct {
	Store     *agentstore.Store
	Broadcast func(agentproto.AgentEvent)
}

// Publish journals if the event is a state-mutating one and always
// broadcasts. Order: journal first, then broadcast. If journal fails, the
// event is NOT broadcast — consumers must not see state the server hasn't
// committed.
func (s *CombinedSink) Publish(ev agentproto.AgentEvent) error {
	if agentstore.IsJournalEvent(ev.Kind()) && ev.Kind() != agentproto.EventSessionCreated {
		if err := s.Store.Append(ev.Session(), ev); err != nil {
			return err
		}
	}
	if s.Broadcast != nil {
		s.Broadcast(ev)
	}
	return nil
}

// CollectingSink stores every published event in memory. Used by tests.
type CollectingSink struct {
	Events []agentproto.AgentEvent
	Err    error // set to fail Publish for failure-mode tests
}

// Publish appends the event and optionally returns the configured error.
func (s *CollectingSink) Publish(ev agentproto.AgentEvent) error {
	if s.Err != nil {
		return s.Err
	}
	s.Events = append(s.Events, ev)
	return nil
}
