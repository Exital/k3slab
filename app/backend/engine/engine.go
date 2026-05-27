package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k3slab/kube"
	"k3slab/loghub"
	"k3slab/workshop"
)

const (
	taskTimeout   = 10 * time.Minute
	setupTimeout  = 10 * time.Minute
	verifyTimeout = 5 * time.Minute
)

// Engine holds workshop progression (single learner, in-memory).
type Engine struct {
	mu sync.Mutex

	loadErr error
	w       *workshop.Workshop

	current int
	// Per-step-index: only meaningful for questions (setup) and completion.
	setupDone   map[int]bool
	completed   map[int]bool
	lastSetup   strings.Builder
	lastVerify  strings.Builder
	lastTaskOut strings.Builder

	labRoot string
	hub     *loghub.Hub
}

// New builds an engine from a parsed workshop.
func New(w *workshop.Workshop, labRoot string, hub *loghub.Hub) *Engine {
	return &Engine{
		w:         w,
		labRoot:   labRoot,
		hub:       hub,
		current:   0,
		setupDone: make(map[int]bool),
		completed: make(map[int]bool),
	}
}

// NewLoadError returns an engine that only reports a load/parse error.
func NewLoadError(err error, labRoot string, hub *loghub.Hub) *Engine {
	return &Engine{loadErr: err, labRoot: labRoot, hub: hub}
}

// LabRoot is the directory used as cwd for workshop shell commands.
func (e *Engine) LabRoot() string {
	return e.labRoot
}

func (e *Engine) kubeEnv() []string {
	return kube.Env()
}

// Snapshot is API-safe workshop state.
type Snapshot struct {
	Name                  string       `json:"name"`
	Error                 string       `json:"error,omitempty"`
	TotalSteps            int          `json:"totalSteps"`
	CurrentStepIndex      int          `json:"currentStepIndex"`
	TotalQuestions        int          `json:"totalQuestions"`
	CurrentQuestionNumber int          `json:"currentQuestionNumber"` // 0 on task or when done; 1-based on active question
	Done                  bool         `json:"done"`
	Current               *CurrentStep `json:"current,omitempty"`
	LastSetupLogs         string               `json:"lastSetupLogs,omitempty"`
	LastVerifyLogs        string               `json:"lastVerifyLogs,omitempty"`
	LastTaskLogs          string               `json:"lastTaskLogs,omitempty"`
	SidebarTabs           []workshop.SidebarTab `json:"sidebarTabs,omitempty"`
}

type CurrentStep struct {
	ID                 string               `json:"id"`
	Type               workshop.StepType    `json:"type"`
	Title              string               `json:"title"`
	Description        string               `json:"description,omitempty"`
	AnswerType         workshop.AnswerType  `json:"answer_type,omitempty"`
	Options            []string             `json:"options,omitempty"`
	IncorrectMessage   string               `json:"incorrect_message,omitempty"`
	CorrectMessage     string               `json:"correct_message,omitempty"`
	Hints                 []string             `json:"hints,omitempty"`
	PollIntervalSeconds   int                  `json:"poll_interval_seconds,omitempty"`
	SetupDone             bool                 `json:"setupDone"`
	Completed             bool                 `json:"completed"`
}

func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loadErr != nil {
		return Snapshot{Error: e.loadErr.Error()}
	}
	if e.w == nil {
		return Snapshot{Error: "no workshop loaded"}
	}
	total := len(e.w.Steps)
	tq := countQuestionSteps(e.w)
	done := e.current >= total
	cqn := questionOrdinal(e.w, e.current, done)
	snap := Snapshot{
		Name:                  e.w.Name,
		TotalSteps:            total,
		CurrentStepIndex:      e.current,
		TotalQuestions:        tq,
		CurrentQuestionNumber: cqn,
		Done:                  done,
		LastSetupLogs:         e.lastSetup.String(),
		LastVerifyLogs:        e.lastVerify.String(),
		LastTaskLogs:          e.lastTaskOut.String(),
		SidebarTabs:           append([]workshop.SidebarTab(nil), e.w.SidebarTabs...),
	}
	if done || e.current < 0 || e.current >= total {
		return snap
	}
	st := e.w.Steps[e.current]
	snap.Current = &CurrentStep{
		ID:                st.ID,
		Type:              st.Type,
		Title:             st.Title,
		Description:       st.Description,
		AnswerType:        st.AnswerType,
		Options:           append([]string(nil), st.Options...),
		IncorrectMessage:  st.IncorrectMessage,
		CorrectMessage:    st.CorrectMessage,
		Hints:               append([]string(nil), st.Hints...),
		PollIntervalSeconds: st.PollIntervalSeconds,
		SetupDone:           e.setupDone[e.current],
		Completed:           e.completed[e.current],
	}
	return snap
}

func countQuestionSteps(w *workshop.Workshop) int {
	n := 0
	for _, st := range w.Steps {
		if st.Type == workshop.StepQuestion {
			n++
		}
	}
	return n
}

// questionOrdinal returns 1-based index of the current question among all questions,
// or 0 when the workshop is done or the active step is a task.
func questionOrdinal(w *workshop.Workshop, current int, done bool) int {
	if done || w == nil || current < 0 || current >= len(w.Steps) {
		return 0
	}
	if w.Steps[current].Type != workshop.StepQuestion {
		return 0
	}
	n := 0
	for i := 0; i <= current; i++ {
		if w.Steps[i].Type == workshop.StepQuestion {
			n++
		}
	}
	return n
}

// Restart resets progression to the beginning (in-memory state only).
func (e *Engine) Restart() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loadErr != nil {
		return fmt.Errorf("workshop not loaded: %w", e.loadErr)
	}
	if e.w == nil {
		return errors.New("no workshop loaded")
	}
	e.current = 0
	e.setupDone = make(map[int]bool)
	e.completed = make(map[int]bool)
	e.lastSetup.Reset()
	e.lastVerify.Reset()
	e.lastTaskOut.Reset()
	return nil
}

func (e *Engine) currentStep() (*workshop.Step, error) {
	if e.loadErr != nil {
		return nil, e.loadErr
	}
	if e.w == nil {
		return nil, errors.New("no workshop")
	}
	if e.current >= len(e.w.Steps) {
		return nil, errors.New("workshop complete")
	}
	st := e.w.Steps[e.current]
	return &st, nil
}

// RunTask runs the current task step shell command; advances on success.
func (e *Engine) RunTask(ctx context.Context) (logs string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := e.currentStep()
	if err != nil {
		return "", err
	}
	if st.Type != workshop.StepTask {
		return "", fmt.Errorf("current step is not a task")
	}
	if e.completed[e.current] {
		return e.lastTaskOut.String(), nil
	}
	e.lastTaskOut.Reset()
	ctx, cancel := context.WithTimeout(ctx, taskTimeout)
	defer cancel()
	code, err := e.runShell(ctx, st.Run, &e.lastTaskOut, e.hub)
	if err != nil {
		return e.lastTaskOut.String(), err
	}
	if code != 0 {
		return e.lastTaskOut.String(), fmt.Errorf("task exited with code %d", code)
	}
	e.completed[e.current] = true
	e.current++
	e.lastSetup.Reset()
	e.lastVerify.Reset()
	return e.lastTaskOut.String(), nil
}

// RunQuestionSetup runs setup commands for the current question once.
func (e *Engine) RunQuestionSetup(ctx context.Context) (logs string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := e.currentStep()
	if err != nil {
		return "", err
	}
	if st.Type != workshop.StepQuestion {
		return "", fmt.Errorf("current step is not a question")
	}
	if e.setupDone[e.current] {
		return e.lastSetup.String(), nil
	}
	e.lastSetup.Reset()
	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()
	if len(st.Setup) == 0 {
		e.setupDone[e.current] = true
		return "", nil
	}
	for _, cmdline := range st.Setup {
		e.appendLine(&e.lastSetup, "$ "+cmdline)
		e.hub.Broadcast("$ " + cmdline)
		code, err := e.runShell(ctx, cmdline, &e.lastSetup, e.hub)
		if err != nil {
			return e.lastSetup.String(), err
		}
		if code != 0 {
			err := fmt.Errorf("setup command failed with exit %d", code)
			return e.lastSetup.String(), err
		}
	}
	e.setupDone[e.current] = true
	return e.lastSetup.String(), nil
}

// SubmitAnswer runs verify with ANSWER set; marks the question complete on exit 0 (does not advance).
func (e *Engine) SubmitAnswer(ctx context.Context, answer string) (ok bool, logs string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := e.currentStep()
	if err != nil {
		return false, "", err
	}
	if st.Type != workshop.StepQuestion {
		return false, "", fmt.Errorf("current step is not a question")
	}
	if st.AnswerType == workshop.AnswerObserve {
		return false, "", fmt.Errorf("observe questions use automatic checking, not submit")
	}
	if !e.setupDone[e.current] {
		return false, "", fmt.Errorf("setup not completed for this question")
	}
	if e.completed[e.current] {
		return true, e.lastVerify.String(), nil
	}
	e.lastVerify.Reset()
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	answer = strings.TrimSpace(answer)
	env := append(e.kubeEnv(), "ANSWER="+answer)
	code, err := e.runVerifyShell(ctx, st.Verify, &e.lastVerify, e.hub, env)
	if err != nil {
		return false, e.lastVerify.String(), err
	}
	ok = code == 0
	if ok {
		e.completed[e.current] = true
	}
	return ok, e.lastVerify.String(), nil
}

// CheckQuestion runs verify for observe questions (no ANSWER); marks complete on exit 0.
// Verify output is not broadcast to the log hub (quiet polls).
func (e *Engine) CheckQuestion(ctx context.Context) (ok bool, logs string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := e.currentStep()
	if err != nil {
		return false, "", err
	}
	if st.Type != workshop.StepQuestion {
		return false, "", fmt.Errorf("current step is not a question")
	}
	if st.AnswerType != workshop.AnswerObserve {
		return false, "", fmt.Errorf("current question is not observe type")
	}
	if !e.setupDone[e.current] {
		return false, "", fmt.Errorf("setup not completed for this question")
	}
	if e.completed[e.current] {
		return true, e.lastVerify.String(), nil
	}
	e.lastVerify.Reset()
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	code, err := e.runVerifyShell(ctx, st.Verify, &e.lastVerify, nil, e.kubeEnv())
	if err != nil {
		return false, e.lastVerify.String(), err
	}
	ok = code == 0
	if ok {
		e.completed[e.current] = true
	}
	return ok, e.lastVerify.String(), nil
}

// AdvanceQuestion moves to the next step after the current question was verified correct.
func (e *Engine) AdvanceQuestion() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := e.currentStep()
	if err != nil {
		return err
	}
	if st.Type != workshop.StepQuestion {
		return fmt.Errorf("current step is not a question")
	}
	if !e.completed[e.current] {
		return fmt.Errorf("question not completed")
	}
	e.current++
	e.lastVerify.Reset()
	return nil
}

func (e *Engine) appendLine(buf *strings.Builder, line string) {
	if buf.Len() > 0 {
		buf.WriteByte('\n')
	}
	buf.WriteString(line)
}

func (e *Engine) runShell(ctx context.Context, script string, accum *strings.Builder, hub *loghub.Hub) (int, error) {
	return e.runShellWithEnv(ctx, script, accum, hub, e.kubeEnv())
}

func (e *Engine) runShellWithEnv(ctx context.Context, script string, accum *strings.Builder, hub *loghub.Hub, env []string) (int, error) {
	return e.runShellWithEnvOpts(ctx, script, accum, hub, env, false)
}

func (e *Engine) runVerifyShell(ctx context.Context, script string, accum *strings.Builder, hub *loghub.Hub, env []string) (int, error) {
	return e.runShellWithEnvOpts(ctx, script, accum, hub, env, true)
}

func (e *Engine) runShellWithEnvOpts(ctx context.Context, script string, accum *strings.Builder, hub *loghub.Hub, env []string, errexit bool) (int, error) {
	if errexit {
		script = "set -e\n" + script
	}
	cmd := exec.CommandContext(ctx, "bash", "-lc", script)
	cmd.Dir = e.labRoot
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, err
	}
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		e.drainPipe(stdout, accum, hub, "")
	}()
	go func() {
		defer wg.Done()
		e.drainPipe(stderr, accum, hub, "stderr: ")
	}()
	wg.Wait()
	err = cmd.Wait()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

func (e *Engine) drainPipe(r io.Reader, accum *strings.Builder, hub *loghub.Hub, prefix string) {
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			for {
				line, rest, found := bytes.Cut(buf.Bytes(), []byte("\n"))
				if !found {
					break
				}
				s := string(line)
				buf.Reset()
				buf.Write(rest)
				out := prefix + s
				if accum != nil {
					if accum.Len() > 0 {
						accum.WriteByte('\n')
					}
					accum.WriteString(out)
				}
				if hub != nil {
					hub.Broadcast(out)
				}
			}
		}
		if err != nil {
			if buf.Len() > 0 {
				s := prefix + buf.String()
				if accum != nil {
					if accum.Len() > 0 {
						accum.WriteByte('\n')
					}
					accum.WriteString(s)
				}
				if hub != nil {
					hub.Broadcast(s)
				}
			}
			return
		}
	}
}
