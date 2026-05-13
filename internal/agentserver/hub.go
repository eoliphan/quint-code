package agentserver

import (
	"sync"

	"github.com/m0n0x41d/haft/internal/agentproto"
)

// Hub is the SSE fan-out: every accepted AgentEvent is broadcast to every
// currently-subscribed listener. It is unbounded by design — the server
// is process-local and the only consumer is the TUI process. If the TUI
// stops reading, its receive channel buffer fills and Publish blocks for
// that subscriber but not for others.
//
// Hub is safe for concurrent use. Subscribe returns a channel and a
// cancel func; cancel detaches the subscriber and drains the channel.
type Hub struct {
	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
}

type subscriber struct {
	ch   chan agentproto.AgentEvent
	done chan struct{}
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{subscribers: map[*subscriber]struct{}{}}
}

// Subscribe registers a listener and returns its receive channel plus a
// cancel func. The buffer size is intentionally small — backpressure on
// a slow listener is the correct behavior, not silent drops.
//
// The channel is intentionally never closed: a Publish goroutine may
// hold a snapshot of the subscriber after cancel() has detached it from
// the map, and closing the channel while that send is pending would
// panic the server. The cancel func instead signals shutdown via a
// per-subscriber done channel; Publish selects on done so it abandons
// any send to a detached subscriber. Consumers should drive their
// own exit (e.g. ctx.Done()) rather than rely on a channel close.
func (h *Hub) Subscribe() (<-chan agentproto.AgentEvent, func()) {
	sub := &subscriber{
		ch:   make(chan agentproto.AgentEvent, 16),
		done: make(chan struct{}),
	}
	h.mu.Lock()
	h.subscribers[sub] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers, sub)
			h.mu.Unlock()
			close(sub.done)
		})
	}
	return sub.ch, cancel
}

// Publish broadcasts an event to all current subscribers. A subscriber
// whose buffer is full receives the event in a blocking send; the caller
// of Publish must therefore avoid holding locks the slow consumer needs
// to drain. In practice this is fine because the only consumer is the
// TUI's SSE writer and it drains as fast as the network allows.
//
// If a subscriber cancels between the snapshot and the send, the select
// on done lets Publish drop the event for that subscriber instead of
// blocking forever (or panicking on a closed channel).
func (h *Hub) Publish(ev agentproto.AgentEvent) {
	h.mu.Lock()
	subs := make([]*subscriber, 0, len(h.subscribers))
	for s := range h.subscribers {
		subs = append(subs, s)
	}
	h.mu.Unlock()
	for _, s := range subs {
		select {
		case s.ch <- ev:
		case <-s.done:
		}
	}
}

// Count returns the number of active subscribers. Test hook only.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}
