package labs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Meta describes one lab directory for the catalog API.
type Meta struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	StepCount   int    `json:"stepCount,omitempty"`
	Valid       bool   `json:"valid"`
	Error       string `json:"error,omitempty"`
}

// Discover scans immediate subdirectories of labsRoot for workshop.yml.
func Discover(labsRoot string) ([]Meta, error) {
	root := filepath.Clean(labsRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Meta
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		id := ent.Name()
		if strings.HasPrefix(id, ".") {
			continue
		}
		m := inspectLab(root, id)
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func inspectLab(labsRoot, id string) Meta {
	m := Meta{ID: id}
	labDir, err := LabPath(labsRoot, id)
	if err != nil {
		m.Error = err.Error()
		return m
	}
	wp := filepath.Join(labDir, workshopFile)
	data, err := os.ReadFile(wp)
	if err != nil {
		m.Error = fmt.Sprintf("read %s: %v", workshopFile, err)
		return m
	}
	name, desc, steps, err := peekWorkshop(data)
	if err != nil {
		m.Error = err.Error()
		return m
	}
	m.Name = name
	m.Description = desc
	m.StepCount = steps
	m.Valid = true
	return m
}
