package labtest

import (
	"fmt"
	"io"
	"os"
)

func logOut() io.Writer {
	return os.Stderr
}

func logf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(logOut(), format+"\n", args...)
}

func logQuestionStart(labID, questionID string) {
	logf("  → %s / %s", labID, questionID)
}

func logResult(r Result) {
	switch r.Status {
	case "pass":
		logf("    PASS")
	case "fail":
		if r.Message != "" {
			logf("    FAIL — %s", r.Message)
		} else {
			logf("    FAIL")
		}
	case "skip":
		logf("    SKIP (no solution_* in workshop.yml)")
	}
}

func logLabHeader(labID string, resetCluster bool) {
	logf("")
	logf("══════════════════════════════════════════════════")
	logf(" Lab: %s", labID)
	if resetCluster {
		logf(" (cluster reset — same as switching labs in the UI)")
	}
	logf("══════════════════════════════════════════════════")
}

func logLabSummary(labID string, results []Result) {
	passed, failed, skipped, tested := 0, 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "pass":
			passed++
			tested++
		case "fail":
			failed++
			tested++
		case "skip":
			skipped++
		}
	}
	logf("──────────────────────────────────────────────────")
	logf(" Lab %s: %d tested, %d pass, %d fail, %d skip", labID, tested, passed, failed, skipped)
	for _, r := range results {
		switch r.Status {
		case "pass":
			logf("   ✓ %s", r.Question)
		case "fail":
			logf("   ✗ %s — %s", r.Question, r.Message)
		case "skip":
			logf("   − %s (skipped)", r.Question)
		}
	}
	logf("──────────────────────────────────────────────────")
}

func printFinalReport(w io.Writer, report *Report) {
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "══════════════════════════════════════════════════\n")
	fmt.Fprintf(w, " k3slab lab-test — final summary\n")
	fmt.Fprintf(w, "══════════════════════════════════════════════════\n")
	fmt.Fprintf(w, " %-22s %-8s\n", "Metric", "Count")
	fmt.Fprintf(w, "──────────────────────────────────────────────────\n")
	fmt.Fprintf(w, " %-22s %-8d\n", "Tested", report.Tested)
	fmt.Fprintf(w, " %-22s %-8d\n", "Passed", report.Passed)
	fmt.Fprintf(w, " %-22s %-8d\n", "Failed", report.Failed)
	fmt.Fprintf(w, " %-22s %-8d\n", "Skipped", report.Skipped)
	fmt.Fprintf(w, "──────────────────────────────────────────────────\n")

	byLab := groupByLab(report.Results)
	for _, labID := range labOrder(byLab) {
		results := byLab[labID]
		p, f, s, t := countResults(results)
		status := "PASS"
		if f > 0 {
			status = "FAIL"
		}
		fmt.Fprintf(w, " %-22s %-8s  (%d pass / %d fail / %d skip)\n", labID, status, p, f, s)
		_ = t
	}
	if report.Failed > 0 {
		fmt.Fprintf(w, "\n Failures:\n")
		for _, r := range report.Results {
			if r.Status == "fail" {
				fmt.Fprintf(w, "   %s / %s: %s\n", r.LabID, r.Question, r.Message)
			}
		}
	}
	fmt.Fprintf(w, "══════════════════════════════════════════════════\n")
	if report.Failed == 0 {
		fmt.Fprintf(w, " TOTAL: PASS\n")
	} else {
		fmt.Fprintf(w, " TOTAL: FAIL\n")
	}
	fmt.Fprintf(w, "══════════════════════════════════════════════════\n")
}

func groupByLab(results []Result) map[string][]Result {
	out := make(map[string][]Result)
	for _, r := range results {
		out[r.LabID] = append(out[r.LabID], r)
	}
	return out
}

func labOrder(byLab map[string][]Result) []string {
	ids := make([]string, 0, len(byLab))
	for id := range byLab {
		ids = append(ids, id)
	}
	// stable sort
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	return ids
}

func countResults(results []Result) (passed, failed, skipped, tested int) {
	for _, r := range results {
		switch r.Status {
		case "pass":
			passed++
			tested++
		case "fail":
			failed++
			tested++
		case "skip":
			skipped++
		}
	}
	return passed, failed, skipped, tested
}
