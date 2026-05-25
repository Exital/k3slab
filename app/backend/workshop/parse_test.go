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
