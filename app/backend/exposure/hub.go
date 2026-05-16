package exposure

import "sync"

// Hub fans out exposure snapshots to SSE subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Snapshot]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan Snapshot]struct{})}
}

const subBuf = 8

func (h *Hub) Subscribe() chan Snapshot {
	ch := make(chan Snapshot, subBuf)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Snapshot) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *Hub) Broadcast(snap Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- snap:
		default:
		}
	}
}
