package labs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"k3slab/cluster"
	"k3slab/engine"
	"k3slab/loghub"
	"k3slab/workshop"
)

func testManager(t *testing.T, w *workshop.Workshop, labRoot string) *Manager {
	t.Helper()
	hub := loghub.New()
	eng := engine.New(w, labRoot, hub)
	return &Manager{
		labsRoot:   labRoot,
		activeID:   "test-lab",
		eng:        eng,
		cluster:    cluster.NewManager(),
		hub:        hub,
		bootStatus: BootstrapIdle,
		bootGen:    1,
	}
}

func TestEnsureAutoStepsAdvancesTaskAndQuestionSetup(t *testing.T) {
	labRoot := t.TempDir()
	counter := filepath.Join(labRoot, "counter.txt")

	w := &workshop.Workshop{
		Name: "eager",
		Steps: []workshop.Step{
			{
				ID:    "prepare",
				Type:  workshop.StepTask,
				Title: "Prepare",
				Run:   "echo task >> counter.txt",
			},
			{
				ID:         "q1",
				Type:       workshop.StepQuestion,
				Title:      "Q1",
				AnswerType: workshop.AnswerText,
				Verify:     "true",
				Setup: []workshop.SetupCommand{
					{Run: "echo setup >> counter.txt"},
				},
			},
		},
	}
	m := testManager(t, w, labRoot)

	logs, state, err := m.ensureAutoSteps(context.Background(), 1)
	if err != nil {
		t.Fatalf("ensureAutoSteps: %v", err)
	}
	if logs == "" {
		t.Fatal("expected logs")
	}
	if state.Current == nil || state.Current.Type != workshop.StepQuestion {
		t.Fatalf("expected question current, got %+v", state.Current)
	}
	if !state.Current.SetupDone {
		t.Fatal("expected setupDone")
	}
	if state.CurrentStepIndex != 1 {
		t.Fatalf("currentStepIndex=%d", state.CurrentStepIndex)
	}

	body, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(body))
	if got != "task\nsetup" {
		t.Fatalf("counter=%q", got)
	}
}

func TestEnsureAutoStepsIdempotent(t *testing.T) {
	labRoot := t.TempDir()

	w := &workshop.Workshop{
		Name: "eager",
		Steps: []workshop.Step{
			{
				ID:    "prepare",
				Type:  workshop.StepTask,
				Title: "Prepare",
				Run:   "echo task >> counter.txt",
			},
			{
				ID:         "q1",
				Type:       workshop.StepQuestion,
				Title:      "Q1",
				AnswerType: workshop.AnswerText,
				Verify:     "true",
				Setup: []workshop.SetupCommand{
					{Run: "echo setup >> counter.txt"},
				},
			},
		},
	}
	m := testManager(t, w, labRoot)

	if _, _, err := m.ensureAutoSteps(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.ensureAutoSteps(context.Background(), 1); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(labRoot, "counter.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(body))
	if got != "task\nsetup" {
		t.Fatalf("expected single run, counter=%q", got)
	}
}

func TestEnsureAutoStepsSingleFlight(t *testing.T) {
	labRoot := t.TempDir()

	w := &workshop.Workshop{
		Name: "eager",
		Steps: []workshop.Step{
			{
				ID:    "prepare",
				Type:  workshop.StepTask,
				Title: "Prepare",
				Run:   "sleep 0.4; echo task >> counter.txt",
			},
			{
				ID:         "q1",
				Type:       workshop.StepQuestion,
				Title:      "Q1",
				AnswerType: workshop.AnswerText,
				Verify:     "true",
			},
		},
	}
	m := testManager(t, w, labRoot)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := m.ensureAutoSteps(context.Background(), 1)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	body, err := os.ReadFile(filepath.Join(labRoot, "counter.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "task") != 1 {
		t.Fatalf("expected one task run, got %q", body)
	}
}

func TestEnsureAutoStepsSupersededSkipsQuestionSetup(t *testing.T) {
	labRoot := t.TempDir()

	w := &workshop.Workshop{
		Name: "eager",
		Steps: []workshop.Step{
			{
				ID:    "prepare",
				Type:  workshop.StepTask,
				Title: "Prepare",
				Run:   "sleep 0.3; echo task >> counter.txt",
			},
			{
				ID:         "q1",
				Type:       workshop.StepQuestion,
				Title:      "Q1",
				AnswerType: workshop.AnswerText,
				Verify:     "true",
				Setup: []workshop.SetupCommand{
					{Run: "echo setup >> counter.txt"},
				},
			},
		},
	}
	m := testManager(t, w, labRoot)

	errCh := make(chan error, 1)
	go func() {
		_, _, err := m.ensureAutoSteps(context.Background(), 1)
		errCh <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		m.bootMu.Lock()
		running := m.bootRunning
		m.bootMu.Unlock()
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bootstrap did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	m.bootMu.Lock()
	m.bootGen = 2
	m.bootMu.Unlock()

	err := <-errCh
	if !errors.Is(err, ErrBootstrapSuperseded) {
		t.Fatalf("expected superseded, got %v", err)
	}

	body, err := os.ReadFile(filepath.Join(labRoot, "counter.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(body))
	if got != "task" {
		t.Fatalf("expected task only before supersede, got %q", got)
	}

	// Restart workshop progress and run the new generation to completion.
	if err := m.eng.Restart(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.ensureAutoSteps(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(filepath.Join(labRoot, "counter.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got = strings.TrimSpace(string(body))
	if got != "task\ntask\nsetup" {
		t.Fatalf("after new gen, counter=%q", got)
	}
}

func TestEnsureAutoStepsQuestionOnly(t *testing.T) {
	labRoot := t.TempDir()

	w := &workshop.Workshop{
		Name: "eager",
		Steps: []workshop.Step{
			{
				ID:         "q1",
				Type:       workshop.StepQuestion,
				Title:      "Q1",
				AnswerType: workshop.AnswerText,
				Verify:     "true",
				Setup: []workshop.SetupCommand{
					{Run: "echo setup > ready.txt"},
				},
			},
		},
	}
	m := testManager(t, w, labRoot)

	_, state, err := m.ensureAutoSteps(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current == nil || !state.Current.SetupDone {
		t.Fatalf("expected setupDone, got %+v", state.Current)
	}
	if _, err := os.Stat(filepath.Join(labRoot, "ready.txt")); err != nil {
		t.Fatal(err)
	}
}
