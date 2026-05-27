package workshop

import (
	"strings"
	"testing"
)

func TestParseObserveQuestion(t *testing.T) {
	yaml := `
name: Test
tabs:
  steps:
    - id: q1
      type: question
      title: Wait
      answer_type: observe
      poll_interval_seconds: 10
      verify: "true"
`
	w, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Steps) != 1 {
		t.Fatalf("steps: got %d", len(w.Steps))
	}
	st := w.Steps[0]
	if st.AnswerType != AnswerObserve {
		t.Fatalf("answer_type: got %q", st.AnswerType)
	}
	if st.PollIntervalSeconds != 10 {
		t.Fatalf("poll_interval_seconds: got %d", st.PollIntervalSeconds)
	}
}

func TestParseObserveMissingInterval(t *testing.T) {
	yaml := `
name: Test
tabs:
  steps:
    - id: q1
      type: question
      title: Wait
      answer_type: observe
      verify: "true"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "poll_interval_seconds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseObserveInvalidInterval(t *testing.T) {
	yaml := `
name: Test
tabs:
  steps:
    - id: q1
      type: question
      title: Wait
      answer_type: observe
      poll_interval_seconds: 2
      verify: "true"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseObserveWithOptions(t *testing.T) {
	yaml := `
name: Test
tabs:
  steps:
    - id: q1
      type: question
      title: Wait
      answer_type: observe
      poll_interval_seconds: 5
      options: [a]
      verify: "true"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "options") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTextWithPollInterval(t *testing.T) {
	yaml := `
name: Test
tabs:
  steps:
    - id: q1
      type: question
      title: Q
      answer_type: text
      poll_interval_seconds: 5
      verify: 'test "$ANSWER" = "x"'
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "poll_interval_seconds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSetupBackgroundCommand(t *testing.T) {
	yaml := `
name: Test
tabs:
  steps:
    - id: q1
      type: question
      title: Q
      answer_type: text
      setup:
        - run: "echo warmup"
          background: true
      verify: 'test "$ANSWER" = "x"'
`
	w, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Steps[0].Setup) != 1 {
		t.Fatalf("setup entries: got %d", len(w.Steps[0].Setup))
	}
	cmd := w.Steps[0].Setup[0]
	if cmd.Run != "echo warmup" {
		t.Fatalf("run: got %q", cmd.Run)
	}
	if !cmd.Background {
		t.Fatalf("background: got false")
	}
}

func TestParseSetupMixedEntries(t *testing.T) {
	yaml := `
name: Test
tabs:
  steps:
    - id: q1
      type: question
      title: Q
      answer_type: text
      setup:
        - "echo first"
        - run: "echo second"
          background: false
      verify: 'test "$ANSWER" = "x"'
`
	w, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Steps[0].Setup) != 2 {
		t.Fatalf("setup entries: got %d", len(w.Steps[0].Setup))
	}
	if w.Steps[0].Setup[0].Run != "echo first" || w.Steps[0].Setup[0].Background {
		t.Fatalf("unexpected first setup entry: %+v", w.Steps[0].Setup[0])
	}
	if w.Steps[0].Setup[1].Run != "echo second" || w.Steps[0].Setup[1].Background {
		t.Fatalf("unexpected second setup entry: %+v", w.Steps[0].Setup[1])
	}
}

func TestParseSetupBackgroundInvalidType(t *testing.T) {
	yaml := `
name: Test
tabs:
  steps:
    - id: q1
      type: question
      title: Q
      answer_type: text
      setup:
        - run: "echo warmup"
          background: "yes"
      verify: 'test "$ANSWER" = "x"'
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "background must be a boolean") {
		t.Fatalf("unexpected error: %v", err)
	}
}
