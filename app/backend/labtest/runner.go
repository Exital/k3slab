package labtest

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"k3slab/engine"
	"k3slab/labs"
	"k3slab/loghub"
	"k3slab/workshop"
)

// Result is one question outcome.
type Result struct {
	LabID    string `json:"labId"`
	Question string `json:"questionId"`
	Status   string `json:"status"` // pass, fail, skip
	Message  string `json:"message,omitempty"`
}

// Report is the full lab-test output.
type Report struct {
	LabsRoot string   `json:"labsRoot"`
	Results  []Result `json:"results"`
	Tested   int      `json:"tested"`
	Passed   int      `json:"passed"`
	Failed   int      `json:"failed"`
	Skipped  int      `json:"skipped"`
}

// Config drives lab-test execution.
type Config struct {
	LabsRoot string
	LabID    string
	Question string
	JSON     bool
}

// Main runs lab-test from args (after the subcommand name). Returns exit code.
func Main(args []string) int {
	fs := flag.NewFlagSet("lab-test", flag.ExitOnError)
	labsRoot := fs.String("labs-root", "", "parent directory of lab folders")
	lab := fs.String("lab", "", "run only this lab id")
	question := fs.String("question", "", "run only this question id")
	jsonOut := fs.Bool("json", false, "write JSON report to stdout")
	_ = fs.Parse(args)

	root := strings.TrimSpace(*labsRoot)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("LABS_ROOT"))
	}
	if root == "" {
		root = "/lab"
	}

	cfg := Config{
		LabsRoot: filepath.Clean(root),
		LabID:    strings.TrimSpace(*lab),
		Question: strings.TrimSpace(*question),
		JSON:     *jsonOut,
	}

	report, err := Run(context.Background(), cfg)
	printFinalReport(logOut(), report)
	if cfg.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	}
	if err != nil {
		logf("lab-test error: %v", err)
		return 1
	}
	if report.Failed > 0 {
		return 1
	}
	return 0
}

// Run executes lab tests and returns a report. Err is set on operational failures.
func Run(ctx context.Context, cfg Config) (*Report, error) {
	meta, err := labs.Discover(cfg.LabsRoot)
	if err != nil {
		return nil, err
	}
	report := &Report{LabsRoot: cfg.LabsRoot}

	var targets []labs.Meta
	for _, m := range meta {
		if !m.Valid {
			continue
		}
		if cfg.LabID != "" && m.ID != cfg.LabID {
			continue
		}
		targets = append(targets, m)
	}
	if cfg.LabID != "" && len(targets) == 0 {
		return nil, fmt.Errorf("lab %q not found or invalid under %s", cfg.LabID, cfg.LabsRoot)
	}
	if len(targets) == 0 {
		return report, nil
	}

	for i, m := range targets {
		// Like switching labs in the UI: reset cluster between labs, not before the first one.
		doReset := i > 0
		logLabHeader(m.ID, doReset)
		// Important: write the next lab's cluster profile before reset/start.
		// K3s start flags (for example --disable=traefik) are read during startup.
		if err := applyClusterProfile(cfg.LabsRoot, m.ID); err != nil {
			return report, err
		}
		if doReset {
			logf(" Resetting cluster...")
			if err := resetCluster(ctx); err != nil {
				return report, fmt.Errorf("cluster reset before lab %s: %w", m.ID, err)
			}
		}
		labResults, err := runLab(ctx, cfg, m.ID)
		if err != nil {
			return report, fmt.Errorf("lab %s: %w", m.ID, err)
		}
		logLabSummary(m.ID, labResults)
		report.Results = append(report.Results, labResults...)
	}

	for _, r := range report.Results {
		switch r.Status {
		case "pass":
			report.Passed++
			report.Tested++
		case "fail":
			report.Failed++
			report.Tested++
		case "skip":
			report.Skipped++
		}
	}
	return report, nil
}

func runLab(ctx context.Context, cfg Config, labID string) ([]Result, error) {
	w, labDir, err := loadWorkshop(cfg.LabsRoot, labID)
	if err != nil {
		return nil, err
	}

	hub := loghub.New()
	eng, err := labs.LoadEngine(cfg.LabsRoot, labID, hub)
	if err != nil {
		return nil, err
	}
	if snap := eng.Snapshot(); snap.Error != "" {
		return nil, fmt.Errorf("load workshop: %s", snap.Error)
	}
	_ = labDir

	var results []Result
	for {
		snap := eng.Snapshot()
		if snap.Done || snap.Current == nil {
			break
		}
		cur := snap.Current
		if cfg.Question != "" && cur.ID != cfg.Question {
			if err := advanceStep(ctx, eng, cur.Type); err != nil {
				return results, err
			}
			continue
		}

		step := stepByID(w, cur.ID)
		if step == nil {
			return results, fmt.Errorf("step %q not in workshop", cur.ID)
		}

		switch step.Type {
		case workshop.StepTask:
			logQuestionStart(labID, step.ID+" (task)")
			if _, err := eng.RunTask(ctx); err != nil {
				res := Result{
					LabID: labID, Question: step.ID, Status: "fail",
					Message: fmt.Sprintf("task: %v", err),
				}
				logResult(res)
				results = append(results, res)
				return results, nil
			}
			logf("    PASS")
		case workshop.StepQuestion:
			logQuestionStart(labID, step.ID)
			res := runQuestion(ctx, eng, step)
			res.LabID = labID
			logResult(res)
			results = append(results, res)
			if res.Status == "fail" {
				return results, nil
			}
			if err := advanceAfterQuestion(ctx, eng, res); err != nil {
				return results, err
			}
		}
	}
	return results, nil
}

func runQuestion(ctx context.Context, eng *engine.Engine, step *workshop.Step) Result {
	res := Result{Question: step.ID}

	if !step.HasSolution() {
		if _, err := eng.RunQuestionSetup(ctx); err != nil {
			res.Status = "fail"
			res.Message = fmt.Sprintf("setup: %v", err)
			return res
		}
		res.Status = "skip"
		return res
	}

	if _, err := eng.RunQuestionSetup(ctx); err != nil {
		res.Status = "fail"
		res.Message = fmt.Sprintf("setup: %v", err)
		return res
	}

	if script := strings.TrimSpace(step.SolutionScript); script != "" {
		scriptLogs, err := eng.RunSolutionScript(ctx, script)
		if err != nil {
			res.Status = "fail"
			res.Message = fmt.Sprintf("solution_script: %v", err)
			if strings.TrimSpace(scriptLogs) != "" {
				res.Message += "\n" + strings.TrimSpace(scriptLogs)
			}
			return res
		}
	}

	answer := strings.TrimSpace(step.SolutionAnswer)
	if answer == "" && step.AnswerType != workshop.AnswerObserve {
		script := strings.TrimSpace(step.SolutionAnswerScript)
		if script == "" {
			res.Status = "fail"
			res.Message = "text question needs solution_answer or solution_answer_script for lab-test"
			return res
		}
		answerLogs, err := eng.RunSolutionScript(ctx, wrapAnswerScript(script))
		if err != nil {
			res.Status = "fail"
			res.Message = fmt.Sprintf("solution_answer_script: %v", err)
			if strings.TrimSpace(answerLogs) != "" {
				res.Message += "\n" + strings.TrimSpace(answerLogs)
			}
			return res
		}
		answer = answerFromAnswerScript(answerLogs)
		if answer == "" {
			res.Status = "fail"
			res.Message = "solution_answer_script produced no answer on stdout"
			if strings.TrimSpace(answerLogs) != "" {
				res.Message += "\n" + strings.TrimSpace(answerLogs)
			}
			return res
		}
	}
	if step.AnswerType == workshop.AnswerObserve {
		ok, msg, err := pollObserve(ctx, eng, step)
		if err != nil {
			res.Status = "fail"
			res.Message = fmt.Sprintf("check: %v", err)
			return res
		}
		if !ok {
			res.Status = "fail"
			res.Message = msg
			return res
		}
	} else {
		ok, verifyLogs, err := eng.SubmitAnswer(ctx, answer)
		if err != nil {
			res.Status = "fail"
			res.Message = fmt.Sprintf("submit: %v", err)
			return res
		}
		if !ok {
			res.Status = "fail"
			res.Message = "verify failed"
			if strings.TrimSpace(verifyLogs) != "" {
				res.Message += ": " + strings.TrimSpace(verifyLogs)
			}
			return res
		}
	}

	res.Status = "pass"
	return res
}

func wrapAnswerScript(script string) string {
	return "set -euo pipefail\n" + strings.TrimSpace(script) + "\n"
}

// answerFromAnswerScript returns stdout from solution_answer_script (stderr lines are prefixed by the engine).
func answerFromAnswerScript(logs string) string {
	var stdout []string
	for _, line := range strings.Split(logs, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "stderr: ") {
			continue
		}
		stdout = append(stdout, line)
	}
	return strings.TrimSpace(strings.Join(stdout, "\n"))
}

func advanceAfterQuestion(ctx context.Context, eng *engine.Engine, res Result) error {
	if res.Status == "pass" {
		return eng.AdvanceQuestion()
	}
	return eng.SkipCurrentStep()
}

func advanceStep(ctx context.Context, eng *engine.Engine, typ workshop.StepType) error {
	snap := eng.Snapshot()
	if snap.Done || snap.Current == nil {
		return nil
	}
	switch typ {
	case workshop.StepTask:
		_, err := eng.RunTask(ctx)
		return err
	case workshop.StepQuestion:
		return eng.SkipCurrentStep()
	default:
		return eng.SkipCurrentStep()
	}
}

func loadWorkshop(labsRoot, labID string) (*workshop.Workshop, string, error) {
	labDir, err := labs.LabPath(labsRoot, labID)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(filepath.Join(labDir, "workshop.yml"))
	if err != nil {
		return nil, "", err
	}
	w, err := workshop.Parse(data)
	if err != nil {
		return nil, "", err
	}
	return w, labDir, nil
}

func stepByID(w *workshop.Workshop, id string) *workshop.Step {
	for i := range w.Steps {
		if w.Steps[i].ID == id {
			return &w.Steps[i]
		}
	}
	return nil
}

// pollObserve re-checks until verify passes, like the UI observe poll loop.
func pollObserve(ctx context.Context, eng *engine.Engine, step *workshop.Step) (ok bool, failMsg string, err error) {
	interval := time.Duration(step.PollIntervalSeconds) * time.Second
	if interval < 3*time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(5 * time.Minute)
	var lastLogs string
	attempt := 0
	for {
		attempt++
		var checkErr error
		ok, lastLogs, checkErr = eng.CheckQuestion(ctx)
		if checkErr != nil {
			return false, "", checkErr
		}
		if ok {
			if attempt > 1 {
				logf("    observe verify passed (after %d checks)", attempt)
			}
			return true, "", nil
		}
		if attempt == 1 || attempt%3 == 0 {
			logf("    observe: waiting for verify (%d)…", attempt)
		}
		if time.Now().After(deadline) {
			msg := "verify failed (observe, timed out after 5m)"
			if strings.TrimSpace(lastLogs) != "" {
				msg += ": " + strings.TrimSpace(lastLogs)
			}
			return false, msg, nil
		}
		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		case <-time.After(interval):
		}
	}
}

func resetCluster(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/local/bin/k3slab-cluster-reset")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func k3slabBin() string {
	if p := strings.TrimSpace(os.Getenv("K3SLAB_BIN")); p != "" {
		return p
	}
	if _, err := os.Stat("/app/k3slab"); err == nil {
		return "/app/k3slab"
	}
	exe, err := os.Executable()
	if err != nil {
		return "k3slab"
	}
	return exe
}

func applyClusterProfile(labsRoot, labID string) error {
	cmd := exec.Command(k3slabBin(), "apply-cluster-profile", labsRoot, labID)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

