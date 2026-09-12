package progress

import (
	"sync"
)

// Snapshot is the API/SSE payload for setup/task progress.
type Snapshot struct {
	Pct     int    `json:"pct"`
	Message string `json:"message"`
	Active  bool   `json:"active"`
}

// Hub fans out progress snapshots to SSE subscribers (single-user, best-effort).
type Hub struct {
	mu   sync.Mutex
	cur  Snapshot
	subs map[chan Snapshot]struct{}
}

func New() *Hub {
	return &Hub{subs: make(map[chan Snapshot]struct{})}
}

const subBuf = 8

// Snapshot returns the latest progress state.
func (h *Hub) Snapshot() Snapshot {
	if h == nil {
		return Snapshot{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cur
}

// Reset marks a setup/task run as active at 0%.
func (h *Hub) Reset() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.cur = Snapshot{Pct: 0, Message: "", Active: true}
	snap := h.cur
	h.mu.Unlock()
	h.broadcast(snap)
}

// Set updates percent (clamped 0–100) and optional message.
// Percent is non-decreasing while active so late/out-of-order markers cannot go backwards.
func (h *Hub) Set(pct int, message string) {
	if h == nil {
		return
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	h.mu.Lock()
	if !h.cur.Active {
		h.mu.Unlock()
		return
	}
	if pct < h.cur.Pct {
		pct = h.cur.Pct
	}
	h.cur.Pct = pct
	h.cur.Message = message
	snap := h.cur
	h.mu.Unlock()
	h.broadcast(snap)
}

// Complete marks a successful run finished at 100%.
func (h *Hub) Complete() {
	h.Finish(true)
}

// Fail marks the run finished without forcing 100% (keeps last published pct).
func (h *Hub) Fail() {
	h.Finish(false)
}

// Finish ends the active run. On success pct becomes 100; on failure the last pct is kept.
func (h *Hub) Finish(success bool) {
	if h == nil {
		return
	}
	h.mu.Lock()
	pct := h.cur.Pct
	msg := h.cur.Message
	if success {
		pct = 100
	}
	h.cur = Snapshot{Pct: pct, Message: msg, Active: false}
	snap := h.cur
	h.mu.Unlock()
	h.broadcast(snap)
}

// Subscribe returns a channel of snapshots. Caller must Unsubscribe when done.
// The current snapshot is not sent automatically; use Snapshot() for that.
func (h *Hub) Subscribe() chan Snapshot {
	ch := make(chan Snapshot, subBuf)
	if h == nil {
		return ch
	}
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Snapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *Hub) broadcast(snap Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- snap:
		default:
			// slow consumer: drop to avoid blocking setup
		}
	}
}
