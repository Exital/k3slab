package loghub

import (
	"encoding/json"
	"sync"
)

// Hub fans out log lines to SSE subscribers (MVP: single-user, best-effort).
type Hub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func New() *Hub {
	return &Hub{subs: make(map[chan string]struct{})}
}

const subBuf = 256

// Subscribe returns a channel of log lines; caller must Unsubscribe when done.
func (h *Hub) Subscribe() chan string {
	ch := make(chan string, subBuf)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

// Broadcast sends a line to all subscribers; drops if buffer full.
func (h *Hub) Broadcast(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- line:
		default:
			// slow consumer: drop to avoid blocking setup
		}
	}
}

// SSEData returns a JSON object safe for one SSE "data:" line.
func SSEData(line string) []byte {
	b, _ := json.Marshal(map[string]string{"line": line})
	return b
}
