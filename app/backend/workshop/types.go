package workshop

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
	Setup             []string   `json:"-"`                           // commands, not exposed as JSON in raw form
	Verify            string     `json:"-"`
	Hints                 []string   `json:"hints,omitempty"`
	PollIntervalSeconds   int        `json:"poll_interval_seconds,omitempty"` // observe only
	Run                   string     `json:"-"`
}

// SidebarTab is reference markdown shown in the UI sidebar (not a progression step).
// Icon is a Material Symbols icon name (e.g. "description", "menu_book"); omit for a UI default.
type SidebarTab struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Icon    string `json:"icon,omitempty"`
}

// Workshop is the parsed workshop.yml root.
type Workshop struct {
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Steps        []Step       `json:"steps"`
	SidebarTabs  []SidebarTab `json:"sidebarTabs"`
}
