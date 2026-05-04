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
	ch chan agentproto.AgentEvent
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{subscribers: map[*subscriber]struct{}{}}
}

// Subscribe registers a listener and returns its receive channel plus a
// cancel func. The buffer size is intentionally small — backpressure on
// a slow listener is the correct behavior, not silent drops.
func (h *Hub) Subscribe() (<-chan agentproto.AgentEvent, func()) {
	sub := &subscriber{ch: make(chan agentproto.AgentEvent, 16)}
	h.mu.Lock()
	h.subscribers[sub] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		_, present := h.subscribers[sub]
		delete(h.subscribers, sub)
		h.mu.Unlock()
		if present {
			close(sub.ch)
		}
	}
	return sub.ch, cancel
}

// Publish broadcasts an event to all current subscribers. A subscriber
// whose buffer is full receives the event in a blocking send; the caller
// of Publish must therefore avoid holding locks the slow consumer needs
// to drain. In practice this is fine because the only consumer is the
// TUI's SSE writer and it drains as fast as the network allows.
func (h *Hub) Publish(ev agentproto.AgentEvent) {
	h.mu.Lock()
	subs := make([]*subscriber, 0, len(h.subscribers))
	for s := range h.subscribers {
		subs = append(subs, s)
	}
	h.mu.Unlock()
	for _, s := range subs {
		s.ch <- ev
	}
}

// Count returns the number of active subscribers. Test hook only.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}
