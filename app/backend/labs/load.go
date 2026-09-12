package labs

import (
	"fmt"
	"os"
	"path/filepath"

	"k3slab/engine"
	"k3slab/loghub"
	"k3slab/progress"
	"k3slab/workshop"
)

// LoadEngine builds an engine for the given lab id under labsRoot.
func LoadEngine(labsRoot, id string, hub *loghub.Hub, progressHub *progress.Hub) (*engine.Engine, error) {
	if id == "" {
		return engine.NewLoadError(ErrNoLabSelected, labsRoot, hub, progressHub), nil
	}
	labDir, err := LabPath(labsRoot, id)
	if err != nil {
		return engine.NewLoadError(err, labsRoot, hub, progressHub), nil
	}
	wp := filepath.Join(labDir, workshopFile)
	data, err := os.ReadFile(wp)
	if err != nil {
		return engine.NewLoadError(fmt.Errorf("read %s: %w", workshopFile, err), labDir, hub, progressHub), nil
	}
	w, err := workshop.Parse(data)
	if err != nil {
		return engine.NewLoadError(err, labDir, hub, progressHub), nil
	}
	return engine.New(w, labDir, hub, progressHub), nil
}
