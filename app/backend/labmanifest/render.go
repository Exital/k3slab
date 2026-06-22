package labmanifest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func renderScriptPath() string {
	if p := strings.TrimSpace(os.Getenv("K3SLAB_RENDER_MANIFESTS_SCRIPT")); p != "" {
		return p
	}
	return "/usr/local/lib/k3slab/render-lab-manifests.sh"
}

// RenderDir runs the shared render-lab-manifests.sh script for a lab directory.
// Failures are non-fatal (read-only mounts, missing templates).
func RenderDir(labDir string) error {
	labDir = filepath.Clean(labDir)
	if labDir == "" {
		return nil
	}
	info, err := os.Stat(labDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("lab dir is not a directory: %s", labDir)
	}

	script := renderScriptPath()
	if _, err := os.Stat(script); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cmd := exec.Command("bash", script, labDir)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("render-lab-manifests: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
