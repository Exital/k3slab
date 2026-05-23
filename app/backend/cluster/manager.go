package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k3slab/kube"
)

const resetScript = "/usr/local/bin/k3slab-cluster-reset"

var (
	ErrAlreadyResetting = errors.New("cluster is resetting")
	ErrResetDisabled    = errors.New("cluster reset is disabled")
	ErrResetFailed      = errors.New("lab restart failed; stop and recreate the container to recover")
	ErrClusterUnavailable = errors.New("cluster is unavailable")
)

// Manager orchestrates in-container K3s cluster reset.
type Manager struct {
	mu        sync.Mutex
	resetting bool
}

// NewManager returns a cluster lifecycle manager.
func NewManager() *Manager {
	return &Manager{}
}

// IsResetting reports whether a reset is in progress.
func (m *Manager) IsResetting() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resetting
}

// IsClusterReady reports whether kubectl can reach a Ready node.
func (m *Manager) IsClusterReady(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "get", "nodes", "--no-headers")
	cmd.Env = kube.Env()
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Ready")
}

// Status returns cluster state for the lab status API.
func (m *Manager) Status(ctx context.Context) string {
	if m.IsResetting() {
		return "resetting"
	}
	if m.IsClusterReady(ctx) {
		return "ready"
	}
	return "unavailable"
}

// Reset stops K3s, wipes data, restarts, and waits until the node is Ready.
func (m *Manager) Reset(ctx context.Context) error {
	if !resetAllowed() {
		return ErrResetDisabled
	}

	m.mu.Lock()
	if m.resetting {
		m.mu.Unlock()
		return ErrAlreadyResetting
	}
	m.resetting = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.resetting = false
		m.mu.Unlock()
	}()

	if _, err := os.Stat(resetScript); err != nil {
		return fmt.Errorf("%w: reset script missing (%v)", ErrResetFailed, err)
	}

	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, resetScript)
	cmd.Env = kube.Env()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	go drainResetLogs(stdout, "cluster-reset")
	go drainResetLogs(stderr, "cluster-reset")

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%w (%v)", ErrResetFailed, err)
	}

	if !m.IsClusterReady(ctx) {
		return ErrResetFailed
	}
	return nil
}

func resetAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("K3SLAB_ALLOW_CLUSTER_RESET"))) {
	case "", "true", "1", "yes":
		return true
	default:
		return false
	}
}

func drainResetLogs(r io.Reader, prefix string) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			log.Printf("%s: %s", prefix, strings.TrimSpace(string(buf[:n])))
		}
		if err != nil {
			return
		}
	}
}
