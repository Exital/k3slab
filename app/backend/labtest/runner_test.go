package labtest

import (
	"testing"

	"k3slab/workshop"
)

func TestAnswerFromAnswerScript(t *testing.T) {
	t.Parallel()
	tests := []struct {
		logs string
		want string
	}{
		{"CTF{abc}\n", "CTF{abc}"},
		{"stderr: noise\nCTF{xyz}\n", "CTF{xyz}"},
		{"line1\nline2\n", "line1\nline2"},
		{"stderr: only\n", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := answerFromAnswerScript(tc.logs); got != tc.want {
			t.Errorf("answerFromAnswerScript(%q) = %q, want %q", tc.logs, got, tc.want)
		}
	}
}

func TestHasSolutionIncludesAnswerScript(t *testing.T) {
	t.Parallel()
	flag := workshop.Step{SolutionAnswerScript: "curl | jq"}
	if !flag.HasSolution() {
		t.Fatal("expected HasSolution with solution_answer_script only")
	}
}
