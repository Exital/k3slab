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

func TestParseLabIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		lab, labs, env string
		want           []string
	}{
		{"01-kubectl-basics", "", "", []string{"01-kubectl-basics"}},
		{"", "01-kubectl-basics,02-deployment-basics", "", []string{"01-kubectl-basics", "02-deployment-basics"}},
		{"ignored", "01-kubectl-basics, 02-deployment-basics", "", []string{"01-kubectl-basics", "02-deployment-basics"}},
		{"", "", "01-kubectl-basics,02-deployment-basics", []string{"01-kubectl-basics", "02-deployment-basics"}},
		{"", "01-kubectl-basics,01-kubectl-basics", "", []string{"01-kubectl-basics"}},
		{"", "", "", nil},
	}
	for _, tc := range tests {
		got := parseLabIDs(tc.lab, tc.labs, tc.env)
		if len(got) != len(tc.want) {
			t.Fatalf("parseLabIDs(%q,%q,%q) len=%d want %d (%v)", tc.lab, tc.labs, tc.env, len(got), len(tc.want), got)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parseLabIDs(%q,%q,%q)[%d]=%q want %q", tc.lab, tc.labs, tc.env, i, got[i], tc.want[i])
			}
		}
	}
}
