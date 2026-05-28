package workshop

import "strings"

// StepType identifies workshop step kinds.
type StepType string

const (
	StepQuestion StepType = "question"
	StepTask     StepType = "task"
)

// AnswerType for question steps.
type AnswerType string

const (
	AnswerText         AnswerType = "text"
	AnswerSingleChoice AnswerType = "single_choice"
	AnswerObserve      AnswerType = "observe"
)

// Poll interval bounds for observe questions (seconds).
const (
	PollIntervalMin = 3
	PollIntervalMax = 120
)

// Step is a normalized workshop step (question or task).
type Step struct {
	ID                string     `json:"id"`
	Type              StepType   `json:"type"`
	Title             string     `json:"title"`
	Description       string     `json:"description,omitempty"`
	AnswerType        AnswerType `json:"answer_type,omitempty"`
	Options           []string   `json:"options,omitempty"`
	IncorrectMessage  string     `json:"incorrect_message,omitempty"` // question: shown when verify fails; optional
	CorrectMessage    string     `json:"correct_message,omitempty"`   // question: shown after verify succeeds; optional
	Setup             []SetupCommand `json:"-"`                       // commands, not exposed as JSON in raw form
	Verify            string     `json:"-"`
	Hints                 []string   `json:"hints,omitempty"`
	PollIntervalSeconds   int        `json:"poll_interval_seconds,omitempty"` // observe only
	Run                   string     `json:"-"`
	// Author/CI only — not exposed via learner API.
	SolutionAnswer       string `json:"-"`
	SolutionScript       string `json:"-"` // setup actions; stdout is not used as the answer
	SolutionAnswerScript string `json:"-"` // prints the answer on stdout (lab-test only)
}

// HasSolution reports whether lab-test should exercise this question.
func (s Step) HasSolution() bool {
	return strings.TrimSpace(s.SolutionAnswer) != "" ||
		strings.TrimSpace(s.SolutionScript) != "" ||
		strings.TrimSpace(s.SolutionAnswerScript) != ""
}

// SetupCommand is one question setup command entry.
type SetupCommand struct {
	Run        string
	Background bool
}

// SidebarTab is reference markdown shown in the UI sidebar (not a progression step).
// Icon is a Material Symbols icon name (e.g. "description", "menu_book"); omit for a UI default.
type SidebarTab struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Icon    string `json:"icon,omitempty"`
}

// ClusterConfig controls K3s server flags for a lab (applied on cluster start/reset).
type ClusterConfig struct {
	// DisableTraefik when true starts K3s with --disable=traefik (Traefik off).
	// When false or unset, K3s keeps the bundled Traefik ingress controller enabled.
	DisableTraefik bool `json:"disable_traefik,omitempty"`
}

// Workshop is the parsed workshop.yml root.
type Workshop struct {
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Cluster      ClusterConfig `json:"cluster,omitempty"`
	Steps        []Step       `json:"steps"`
	SidebarTabs  []SidebarTab `json:"sidebarTabs"`
}
