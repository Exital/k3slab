package labs

import (
	"fmt"
	"os"
	"path/filepath"

	"k3slab/engine"
	"k3slab/loghub"
	"k3slab/workshop"
)

// LoadEngine builds an engine for the given lab id under labsRoot.
func LoadEngine(labsRoot, id string, hub *loghub.Hub) (*engine.Engine, error) {
	if id == "" {
		return engine.NewLoadError(ErrNoLabSelected, labsRoot, hub), nil
	}
	labDir, err := LabPath(labsRoot, id)
	if err != nil {
		return engine.NewLoadError(err, labsRoot, hub), nil
	}
	wp := filepath.Join(labDir, workshopFile)
	data, err := os.ReadFile(wp)
	if err != nil {
		return engine.NewLoadError(fmt.Errorf("read %s: %w", workshopFile, err), labDir, hub), nil
	}
	w, err := workshop.Parse(data)
	if err != nil {
		return engine.NewLoadError(err, labDir, hub), nil
	}
	return engine.New(w, labDir, hub), nil
}
