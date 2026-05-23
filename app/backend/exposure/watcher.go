package exposure

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k3slab/kube"
)

func debugf(format string, args ...any) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("K3SLAB_DEBUG"))) {
	case "1", "true", "yes":
		log.Printf(format, args...)
	}
}

type watchEvent struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

type objectMeta struct {
	Metadata meta `json:"metadata"`
}

// Watcher tracks Services and Ingresses via kubectl watch and exposes browser URLs.
type Watcher struct {
	ctx    context.Context
	hub    *Hub
	mu     sync.RWMutex
	svc    map[string]json.RawMessage
	ing    map[string]json.RawMessage
	snap   Snapshot
}

// NewWatcher starts background kubectl watch loops. ctx cancels them on shutdown.
func NewWatcher(ctx context.Context) *Watcher {
	w := &Watcher{
		ctx: ctx,
		hub: NewHub(),
		svc: make(map[string]json.RawMessage),
		ing: make(map[string]json.RawMessage),
	}
	go w.run()
	return w
}

// Snapshot returns the latest endpoint list.
func (w *Watcher) Snapshot() Snapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.snap
}

// Hub returns the broadcast hub for SSE subscribers.
func (w *Watcher) Hub() *Hub {
	return w.hub
}

func (w *Watcher) run() {
	for {
		if w.ctx.Err() != nil {
			return
		}
		if w.syncList() {
			break
		}
		select {
		case <-w.ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		w.watchLoop("svc", "svc", "get", "svc", "-A", "-w", "--output-watch-events", "-o", "json")
	}()
	go func() {
		defer wg.Done()
		w.watchLoop("ing", "ing", "get", "ingress", "-A", "-w", "--output-watch-events", "-o", "json")
	}()
	go func() {
		defer wg.Done()
		w.resyncLoop()
	}()
	wg.Wait()
}

func (w *Watcher) resyncLoop() {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-tick.C:
			if w.syncList() {
				debugf("exposure: periodic resync (%d endpoints)", len(w.snap.Endpoints))
			}
		}
	}
}

func (w *Watcher) syncList() bool {
	ok := true
	if raw, err := kubectlJSON(w.ctx, "get", "svc", "-A", "-o", "json"); err == nil {
		w.replaceList(raw, "svc", w.svc)
	} else {
		log.Printf("exposure: list svc: %v", err)
		ok = false
	}
	if raw, err := kubectlJSON(w.ctx, "get", "ingress", "-A", "-o", "json"); err == nil {
		w.replaceList(raw, "ing", w.ing)
	} else {
		log.Printf("exposure: list ingress: %v", err)
		ok = false
	}
	if ok {
		w.rebuild()
		debugf("exposure: synced %d service(s), %d ingress(es), %d endpoint(s)",
			len(w.svc), len(w.ing), len(w.Snapshot().Endpoints))
	}
	return ok
}

func (w *Watcher) replaceList(raw []byte, kind string, dest map[string]json.RawMessage) {
	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return
	}
	clear(dest)
	for _, item := range list.Items {
		var om objectMeta
		if json.Unmarshal(item, &om) != nil {
			continue
		}
		key := kind + "/" + om.Metadata.Namespace + "/" + om.Metadata.Name
		dest[key] = append(json.RawMessage(nil), item...)
	}
}

func (w *Watcher) watchLoop(name, kind, getVerb, resource string, getArgs ...string) {
	for w.ctx.Err() == nil {
		args := append([]string{getVerb, resource}, getArgs...)
		cmd := exec.CommandContext(w.ctx, "kubectl", args...)
		cmd.Env = kube.Env()
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("exposure: %s watch pipe: %v", name, err)
			w.sleepRetry()
			continue
		}
		if err := cmd.Start(); err != nil {
			log.Printf("exposure: %s watch start: %v", name, err)
			w.sleepRetry()
			continue
		}
		w.consumeWatch(stdout, kind, name == "svc")
		_ = cmd.Wait()
		if w.ctx.Err() != nil {
			return
		}
		log.Printf("exposure: %s watch ended, restarting", name)
		w.sleepRetry()
	}
}

func (w *Watcher) consumeWatch(r io.Reader, kind string, isSvc bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if w.ctx.Err() != nil {
			return
		}
		line := sc.Bytes()
		var ev watchEvent
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "ADDED", "MODIFIED":
			var om objectMeta
			if json.Unmarshal(ev.Object, &om) != nil {
				continue
			}
			key := kind + "/" + om.Metadata.Namespace + "/" + om.Metadata.Name
			w.mu.Lock()
			if isSvc {
				w.svc[key] = append(json.RawMessage(nil), ev.Object...)
			} else {
				w.ing[key] = append(json.RawMessage(nil), ev.Object...)
			}
			w.mu.Unlock()
			w.rebuild()
		case "DELETED":
			var om objectMeta
			if json.Unmarshal(ev.Object, &om) != nil {
				continue
			}
			key := kind + "/" + om.Metadata.Namespace + "/" + om.Metadata.Name
			w.mu.Lock()
			if isSvc {
				delete(w.svc, key)
			} else {
				delete(w.ing, key)
			}
			w.mu.Unlock()
			w.rebuild()
		case "ERROR":
			log.Printf("exposure: watch error event: %s", string(ev.Object))
		}
	}
}

func (w *Watcher) rebuild() {
	w.mu.Lock()
	snap := Snapshot{Endpoints: buildEndpoints(w.svc, w.ing)}
	w.snap = snap
	w.mu.Unlock()
	w.hub.Broadcast(snap)
}

// Clear drops cached Services/Ingresses and broadcasts an empty endpoint list.
func (w *Watcher) Clear() {
	w.mu.Lock()
	clear(w.svc)
	clear(w.ing)
	snap := Snapshot{Endpoints: []Endpoint{}}
	w.snap = snap
	w.mu.Unlock()
	w.hub.Broadcast(snap)
}

// Sync refreshes the endpoint list from the current cluster state.
func (w *Watcher) Sync() {
	w.syncList()
}

func (w *Watcher) sleepRetry() {
	select {
	case <-w.ctx.Done():
	case <-time.After(2 * time.Second):
	}
}

func kubectlJSON(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Env = kube.Env()
	return cmd.Output()
}
