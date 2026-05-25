package labs

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultLabsRoot   = "/lab"
	activeLabFile     = "/var/lib/k3slab/active-lab"
	workshopFile      = "workshop.yml"
)

// ResolveConfig returns LABS_ROOT and the active lab id (may be empty if none selected).
func ResolveConfig() (labsRoot, activeID string) {
	labsRoot = strings.TrimSpace(os.Getenv("LABS_ROOT"))
	if labsRoot == "" {
		labsRoot = defaultLabsRoot
	}
	labsRoot = filepath.Clean(labsRoot)

	activeID = strings.TrimSpace(os.Getenv("LAB_ID"))
	if activeID != "" {
		if err := ValidateID(activeID); err != nil {
			activeID = ""
		}
		return labsRoot, activeID
	}

	if b, err := os.ReadFile(activeLabFile); err == nil {
		id := strings.TrimSpace(string(b))
		if id != "" && ValidateID(id) == nil {
			return labsRoot, id
		}
	}

	meta, err := Discover(labsRoot)
	if err != nil {
		return labsRoot, ""
	}
	var valid []string
	for _, m := range meta {
		if m.Valid {
			valid = append(valid, m.ID)
		}
	}
	if len(valid) == 1 {
		return labsRoot, valid[0]
	}
	return labsRoot, ""
}

// ValidateID rejects path traversal in lab ids.
func ValidateID(id string) error {
	if id == "" {
		return ErrInvalidID
	}
	if id == "." || id == ".." {
		return ErrInvalidID
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return ErrInvalidID
	}
	if strings.HasPrefix(id, ".") {
		return ErrInvalidID
	}
	return nil
}

// LabPath returns the absolute directory for a lab id under labsRoot.
func LabPath(labsRoot, id string) (string, error) {
	if err := ValidateID(id); err != nil {
		return "", err
	}
	root := filepath.Clean(labsRoot)
	p := filepath.Join(root, id)
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrInvalidID
	}
	return p, nil
}

// PersistActiveLab writes the active lab id for the next process start.
func PersistActiveLab(id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(activeLabFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(activeLabFile, []byte(id), 0o644)
}
