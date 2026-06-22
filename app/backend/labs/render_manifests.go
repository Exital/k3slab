package labs

import (
	"k3slab/labmanifest"
)

// RenderLabManifests renders templated manifests for a lab under labsRoot.
func RenderLabManifests(labsRoot, id string) error {
	if id == "" {
		return nil
	}
	labDir, err := LabPath(labsRoot, id)
	if err != nil {
		return err
	}
	return labmanifest.RenderDir(labDir)
}
