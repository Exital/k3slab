package workshop

import (
	"fmt"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type rawWorkshop struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Cluster     *rawCluster `yaml:"cluster"`
	Tabs        *rawTabs    `yaml:"tabs"`
	Steps       []rawStep   `yaml:"steps"` // legacy: top-level steps when tabs absent
}

type rawCluster struct {
	DisableTraefik *bool `yaml:"disable_traefik"`
}

type rawTabs struct {
	Steps     []rawStep     `yaml:"steps"`
	Markdowns []rawMarkdown `yaml:"markdowns"`
}

type rawMarkdown struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Content string `yaml:"content"`
	Icon    string `yaml:"icon"`
}

type rawStep struct {
	ID               string      `yaml:"id"`
	Type             string      `yaml:"type"`
	Title            string      `yaml:"title"`
	Description      string      `yaml:"description"`
	AnswerType       string      `yaml:"answer_type"`
	Options          []string    `yaml:"options"`
	IncorrectMessage string      `yaml:"incorrect_message"`
	CorrectMessage   string      `yaml:"correct_message"`
	Setup            interface{} `yaml:"setup"`
	Verify           string      `yaml:"verify"`
	Hints                []string `yaml:"hints"`
	PollIntervalSeconds  int      `yaml:"poll_interval_seconds"`
	Run                  string   `yaml:"run"`
}

// Parse loads workshop YAML into a Workshop.
func Parse(data []byte) (*Workshop, error) {
	var raw rawWorkshop
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	if strings.TrimSpace(raw.Name) == "" {
		return nil, fmt.Errorf("workshop: missing name")
	}

	var stepSource []rawStep
	switch {
	case raw.Tabs != nil && len(raw.Tabs.Steps) > 0:
		stepSource = raw.Tabs.Steps
		if len(raw.Steps) > 0 {
			return nil, fmt.Errorf("workshop: use either tabs.steps or legacy top-level steps, not both")
		}
	case len(raw.Steps) > 0:
		stepSource = raw.Steps
	default:
		return nil, fmt.Errorf("workshop: tabs.steps (or legacy steps) must contain at least one step")
	}

	out := &Workshop{
		Name:        strings.TrimSpace(raw.Name),
		Description: strings.TrimSpace(raw.Description),
		Cluster:     normalizeCluster(raw.Cluster),
	}
	for i, rs := range stepSource {
		st, err := normalizeStep(rs, i)
		if err != nil {
			return nil, err
		}
		out.Steps = append(out.Steps, st)
	}

	if raw.Tabs != nil {
		for i, rm := range raw.Tabs.Markdowns {
			tab, err := normalizeMarkdown(rm, i)
			if err != nil {
				return nil, fmt.Errorf("tabs.markdowns[%d]: %w", i, err)
			}
			out.SidebarTabs = append(out.SidebarTabs, tab)
		}
	}

	return out, nil
}

func normalizeCluster(rc *rawCluster) ClusterConfig {
	if rc == nil || rc.DisableTraefik == nil {
		return ClusterConfig{}
	}
	return ClusterConfig{DisableTraefik: *rc.DisableTraefik}
}

func normalizeMarkdown(rm rawMarkdown, idx int) (SidebarTab, error) {
	title := strings.TrimSpace(rm.Title)
	content := strings.TrimSpace(rm.Content)
	if title == "" {
		return SidebarTab{}, fmt.Errorf("missing title")
	}
	if content == "" {
		return SidebarTab{}, fmt.Errorf("missing content")
	}
	id := strings.TrimSpace(rm.ID)
	if id == "" {
		id = syntheticMarkdownID(title, idx)
	}
	return SidebarTab{
		ID:      id,
		Title:   title,
		Content: rm.Content,
		Icon:    strings.TrimSpace(rm.Icon),
	}, nil
}

func syntheticMarkdownID(title string, idx int) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if b.Len() > 0 && b.String()[b.Len()-1] != '-' {
				b.WriteByte('-')
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return fmt.Sprintf("markdown-%d", idx)
	}
	return fmt.Sprintf("%s-%d", s, idx)
}

func normalizeStep(rs rawStep, idx int) (Step, error) {
	id := strings.TrimSpace(rs.ID)
	if id == "" {
		return Step{}, fmt.Errorf("step %d: missing id", idx)
	}
	switch strings.TrimSpace(rs.Type) {
	case "question":
		return normalizeQuestion(rs, id)
	case "task":
		return normalizeTask(rs, id)
	default:
		return Step{}, fmt.Errorf("step %s: unknown type %q", id, rs.Type)
	}
}

func normalizeTask(rs rawStep, id string) (Step, error) {
	if strings.TrimSpace(rs.Run) == "" {
		return Step{}, fmt.Errorf("task %s: missing run", id)
	}
	return Step{
		ID:    id,
		Type:  StepTask,
		Title: strings.TrimSpace(rs.Title),
		Run:   strings.TrimSpace(rs.Run),
	}, nil
}

func normalizeQuestion(rs rawStep, id string) (Step, error) {
	at := AnswerType(strings.TrimSpace(rs.AnswerType))
	switch at {
	case AnswerText, AnswerSingleChoice, AnswerObserve:
	default:
		return Step{}, fmt.Errorf("question %s: invalid answer_type %q", id, rs.AnswerType)
	}
	if at == AnswerSingleChoice && len(rs.Options) == 0 {
		return Step{}, fmt.Errorf("question %s: single_choice requires options", id)
	}
	if at == AnswerObserve {
		if len(rs.Options) > 0 {
			return Step{}, fmt.Errorf("question %s: observe must not have options", id)
		}
		if rs.PollIntervalSeconds < PollIntervalMin || rs.PollIntervalSeconds > PollIntervalMax {
			return Step{}, fmt.Errorf("question %s: poll_interval_seconds must be between %d and %d", id, PollIntervalMin, PollIntervalMax)
		}
	} else if rs.PollIntervalSeconds != 0 {
		return Step{}, fmt.Errorf("question %s: poll_interval_seconds is only valid for answer_type observe", id)
	}
	setup, err := parseSetup(rs.Setup)
	if err != nil {
		return Step{}, fmt.Errorf("question %s: setup: %w", id, err)
	}
	if strings.TrimSpace(rs.Verify) == "" {
		return Step{}, fmt.Errorf("question %s: missing verify", id)
	}
	st := Step{
		ID:               id,
		Type:             StepQuestion,
		Title:            strings.TrimSpace(rs.Title),
		Description:      strings.TrimSpace(rs.Description),
		AnswerType:       at,
		Options:          rs.Options,
		IncorrectMessage: strings.TrimSpace(rs.IncorrectMessage),
		CorrectMessage:   strings.TrimSpace(rs.CorrectMessage),
		Setup:            setup,
		Verify:           strings.TrimSpace(rs.Verify),
		Hints:            rs.Hints,
	}
	if at == AnswerObserve {
		st.PollIntervalSeconds = rs.PollIntervalSeconds
	}
	return st, nil
}

func parseSetup(v interface{}) ([]SetupCommand, error) {
	if v == nil {
		return nil, nil
	}
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil, nil
		}
		return []SetupCommand{{Run: s}}, nil
	case map[string]interface{}:
		cmd, err := parseSetupMap(t)
		if err != nil {
			return nil, err
		}
		return []SetupCommand{cmd}, nil
	case []interface{}:
		var out []SetupCommand
		for i, item := range t {
			switch entry := item.(type) {
			case string:
				s := strings.TrimSpace(entry)
				if s != "" {
					out = append(out, SetupCommand{Run: s})
				}
			case map[string]interface{}:
				cmd, err := parseSetupMap(entry)
				if err != nil {
					return nil, fmt.Errorf("entry %d: %w", i, err)
				}
				out = append(out, cmd)
			default:
				return nil, fmt.Errorf("entry %d: expected string or map", i)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported setup shape %T", v)
	}
}

func parseSetupMap(m map[string]interface{}) (SetupCommand, error) {
	runRaw, ok := m["run"]
	if !ok {
		return SetupCommand{}, fmt.Errorf("missing run")
	}
	runStr, ok := runRaw.(string)
	if !ok {
		return SetupCommand{}, fmt.Errorf("run must be a string")
	}
	runStr = strings.TrimSpace(runStr)
	if runStr == "" {
		return SetupCommand{}, fmt.Errorf("run must not be empty")
	}

	cmd := SetupCommand{Run: runStr}
	if bgRaw, ok := m["background"]; ok {
		bg, ok := bgRaw.(bool)
		if !ok {
			return SetupCommand{}, fmt.Errorf("background must be a boolean")
		}
		cmd.Background = bg
	}
	return cmd, nil
}
