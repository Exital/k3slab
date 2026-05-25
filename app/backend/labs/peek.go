package labs

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type peekRaw struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Tabs        *peekTabs `yaml:"tabs"`
	Steps       []peekStep `yaml:"steps"`
}

type peekTabs struct {
	Steps []peekStep `yaml:"steps"`
}

type peekStep struct{}

// peekWorkshop reads catalog fields without full step validation.
func peekWorkshop(data []byte) (name, description string, stepCount int, err error) {
	var raw peekRaw
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return "", "", 0, fmt.Errorf("yaml: %w", err)
	}
	name = strings.TrimSpace(raw.Name)
	if name == "" {
		return "", "", 0, fmt.Errorf("workshop: missing name")
	}
	description = strings.TrimSpace(raw.Description)
	switch {
	case raw.Tabs != nil && len(raw.Tabs.Steps) > 0:
		stepCount = len(raw.Tabs.Steps)
	case len(raw.Steps) > 0:
		stepCount = len(raw.Steps)
	default:
		return "", "", 0, fmt.Errorf("workshop: no steps defined")
	}
	return name, description, stepCount, nil
}
