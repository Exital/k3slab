package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k3slab/loghub"
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
	eng := New(w, labRoot, loghub.New())

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
