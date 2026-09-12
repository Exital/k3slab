package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k3slab/loghub"
	"k3slab/progress"
	"k3slab/workshop"
)

func TestRunQuestionSetupBackgroundCommand(t *testing.T) {
	labRoot := t.TempDir()
	outFile := filepath.Join(labRoot, "bg-ready.txt")

	w := &workshop.Workshop{
		Name: "test",
		Steps: []workshop.Step{
			{
				ID:         "q1",
				Type:       workshop.StepQuestion,
				Title:      "Question",
				AnswerType: workshop.AnswerText,
				Verify:     "true",
				Setup: []workshop.SetupCommand{
					{Run: "sleep 1; echo ok > bg-ready.txt", Background: true},
				},
			},
		},
	}
	eng := New(w, labRoot, loghub.New(), nil)

	start := time.Now()
	logs, err := eng.RunQuestionSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("setup waited for background command")
	}
	if logs == "" {
		t.Fatalf("expected setup logs")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(outFile); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("background setup command did not complete")
}

func TestPeekAutoAction(t *testing.T) {
	labRoot := t.TempDir()
	w := &workshop.Workshop{
		Name: "test",
		Steps: []workshop.Step{
			{ID: "t1", Type: workshop.StepTask, Title: "T", Run: "true"},
			{ID: "q1", Type: workshop.StepQuestion, Title: "Q", AnswerType: workshop.AnswerText, Verify: "true"},
		},
	}
	eng := New(w, labRoot, loghub.New(), nil)
	if got := eng.PeekAutoAction(); got != AutoTask {
		t.Fatalf("got %q want task", got)
	}
	if _, err := eng.RunTask(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := eng.PeekAutoAction(); got != AutoSetup {
		t.Fatalf("got %q want setup", got)
	}
	if _, err := eng.RunQuestionSetup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := eng.PeekAutoAction(); got != AutoNone {
		t.Fatalf("got %q want none", got)
	}
}

func TestRunTaskProgressMarkers(t *testing.T) {
	labRoot := t.TempDir()
	w := &workshop.Workshop{
		Name: "test",
		Steps: []workshop.Step{
			{
				ID:    "t1",
				Type:  workshop.StepTask,
				Title: "T",
				Run:   `echo hello; printf '::k3slab-progress::40::Halfway\n'; echo world; printf '::k3slab-progress::90::Almost\n'`,
			},
		},
	}
	ph := progress.New()
	eng := New(w, labRoot, loghub.New(), ph)

	ch := ph.Subscribe()
	defer ph.Unsubscribe(ch)

	logs, err := eng.RunTask(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs, "::k3slab-progress::") {
		t.Fatalf("progress markers leaked into logs: %q", logs)
	}
	if !strings.Contains(logs, "hello") || !strings.Contains(logs, "world") {
		t.Fatalf("expected normal log lines, got %q", logs)
	}

	snap := ph.Snapshot()
	if snap.Active || snap.Pct != 100 {
		t.Fatalf("expected Complete snapshot, got %+v", snap)
	}

	// Drain events; last non-complete progress should have reached 90.
	saw90 := false
	for {
		select {
		case s := <-ch:
			if s.Pct == 90 && s.Message == "Almost" && s.Active {
				saw90 = true
			}
		default:
			goto done
		}
	}
done:
	if !saw90 {
		t.Fatal("expected progress event at 90%")
	}
}
