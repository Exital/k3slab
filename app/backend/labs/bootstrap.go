package labs

import (
	"context"
	"errors"
	"log"
	"time"

	"k3slab/engine"
)

// BootstrapStatus is exposed on GET /api/lab/status.
type BootstrapStatus string

const (
	BootstrapIdle    BootstrapStatus = "idle"
	BootstrapRunning BootstrapStatus = "running"
	BootstrapFailed  BootstrapStatus = "failed"
)

const (
	bootstrapPollInterval = time.Second
	bootstrapTimeout      = 10 * time.Minute
)

var ErrBootstrapSuperseded = errors.New("bootstrap superseded")

// StartEagerBootstrap begins waiting for the cluster and running landing auto-steps.
// Safe to call once from process startup (server path only).
func (m *Manager) StartEagerBootstrap() {
	m.bootMu.Lock()
	if m.bootRootCancel != nil {
		m.bootMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.bootRootCtx = ctx
	m.bootRootCancel = cancel
	m.bootStatus = BootstrapIdle
	m.bootMu.Unlock()

	m.RequestBootstrap()
}

// RequestBootstrap cancels any in-flight bootstrap and starts a new generation.
func (m *Manager) RequestBootstrap() {
	m.bootMu.Lock()
	if m.bootRootCtx == nil {
		// StartEagerBootstrap not called (e.g. tests / lab-test); no-op.
		m.bootMu.Unlock()
		return
	}
	if m.bootCancel != nil {
		m.bootCancel()
	}
	m.bootGen++
	gen := m.bootGen
	ctx, cancel := context.WithCancel(m.bootRootCtx)
	m.bootCancel = cancel
	m.bootMu.Unlock()

	go m.runBootstrap(ctx, gen)
}

// BootstrapStatusValue returns the current eager-bootstrap status and last error message.
func (m *Manager) BootstrapStatusValue() (BootstrapStatus, string) {
	m.bootMu.Lock()
	defer m.bootMu.Unlock()
	return m.bootStatus, m.bootErr
}

// EnsureAutoSteps runs the landing auto chain (task then question setup) with single-flight.
// HTTP handlers and the eager bootstrap goroutine share this path so setup.sh never runs twice.
func (m *Manager) EnsureAutoSteps(ctx context.Context) (logs string, state WorkshopState, err error) {
	m.bootMu.Lock()
	gen := m.bootGen
	m.bootMu.Unlock()
	return m.ensureAutoSteps(ctx, gen)
}

func (m *Manager) runBootstrap(ctx context.Context, gen uint64) {
	if err := m.waitClusterReady(ctx, gen); err != nil {
		return
	}
	if !m.bootstrapGenCurrent(gen) {
		return
	}
	logs, _, err := m.ensureAutoSteps(ctx, gen)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrBootstrapSuperseded) {
		log.Printf("[k3slab] eager bootstrap failed: %v", err)
	} else if err == nil && logs != "" {
		log.Printf("[k3slab] eager bootstrap finished")
	}
}

func (m *Manager) waitClusterReady(ctx context.Context, gen uint64) error {
	for {
		if !m.bootstrapGenCurrent(gen) {
			return ErrBootstrapSuperseded
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !m.cluster.IsResetting() && m.cluster.IsClusterReady(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(bootstrapPollInterval):
		}
	}
}

func (m *Manager) bootstrapGenCurrent(gen uint64) bool {
	m.bootMu.Lock()
	defer m.bootMu.Unlock()
	return gen == m.bootGen
}

func (m *Manager) ensureAutoSteps(ctx context.Context, gen uint64) (logs string, state WorkshopState, err error) {
	m.bootMu.Lock()
	if gen != m.bootGen {
		m.bootMu.Unlock()
		return "", m.WorkshopState(), ErrBootstrapSuperseded
	}
	if m.bootRunning {
		done := m.bootDone
		m.bootMu.Unlock()
		select {
		case <-done:
			m.bootMu.Lock()
			err = m.bootLastErr
			logs = m.bootLastLogs
			m.bootMu.Unlock()
			return logs, m.WorkshopState(), err
		case <-ctx.Done():
			return "", WorkshopState{}, ctx.Err()
		}
	}

	m.bootRunning = true
	m.bootStatus = BootstrapRunning
	m.bootErr = ""
	m.bootLastErr = nil
	m.bootLastLogs = ""
	m.bootDone = make(chan struct{})
	done := m.bootDone
	m.bootMu.Unlock()

	var runErr error
	var runLogs string
	defer func() {
		m.bootMu.Lock()
		m.bootRunning = false
		m.bootLastErr = runErr
		m.bootLastLogs = runLogs
		if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, ErrBootstrapSuperseded) {
			m.bootStatus = BootstrapFailed
			m.bootErr = runErr.Error()
		} else if m.bootStatus == BootstrapRunning {
			m.bootStatus = BootstrapIdle
			m.bootErr = ""
		}
		close(done)
		m.bootMu.Unlock()
	}()

	runCtx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
	defer cancel()

	for {
		if !m.bootstrapGenCurrent(gen) {
			runErr = ErrBootstrapSuperseded
			return runLogs, m.WorkshopState(), runErr
		}
		if err := runCtx.Err(); err != nil {
			runErr = err
			return runLogs, m.WorkshopState(), runErr
		}

		m.mu.Lock()
		eng := m.eng
		m.mu.Unlock()

		action := eng.PeekAutoAction()
		switch action {
		case engine.AutoTask:
			out, err := eng.RunTask(runCtx)
			runLogs = joinLogs(runLogs, out)
			if err != nil {
				runErr = err
				return runLogs, m.WorkshopState(), runErr
			}
			if !m.bootstrapGenCurrent(gen) {
				runErr = ErrBootstrapSuperseded
				return runLogs, m.WorkshopState(), runErr
			}
			continue
		case engine.AutoSetup:
			out, err := eng.RunQuestionSetup(runCtx)
			runLogs = joinLogs(runLogs, out)
			if err != nil {
				runErr = err
				return runLogs, m.WorkshopState(), runErr
			}
			return runLogs, m.WorkshopState(), nil
		default:
			return runLogs, m.WorkshopState(), nil
		}
	}
}

func joinLogs(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n" + b
}
