package labs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k3slab/workshop"
)

func clusterProfilePath() string {
	if p := strings.TrimSpace(os.Getenv("K3SLAB_CLUSTER_PROFILE")); p != "" {
		return p
	}
	return "/run/k3slab/cluster-profile.env"
}

// ClusterDisableTraefik reads whether the lab's workshop.yml disables bundled Traefik.
// Missing cluster config defaults to false (Traefik enabled).
func ClusterDisableTraefik(labsRoot, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	labDir, err := LabPath(labsRoot, id)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(filepath.Join(labDir, workshopFile))
	if err != nil {
		return false, fmt.Errorf("read %s: %w", workshopFile, err)
	}
	w, err := workshop.Parse(data)
	if err != nil {
		return false, err
	}
	return w.Cluster.DisableTraefik, nil
}

// WriteClusterProfile writes /run/k3slab/cluster-profile.env for k3s-lifecycle scripts.
func WriteClusterProfile(labsRoot, id string) error {
	disable, err := ClusterDisableTraefik(labsRoot, id)
	if err != nil {
		return err
	}
	val := "false"
	if disable {
		val = "true"
	}
	path := clusterProfilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("K3SLAB_DISABLE_TRAEFIK=%s\n", val)
	return os.WriteFile(path, []byte(content), 0o644)
}

// ClusterProfilePath is the env file path consumed by k3s-lifecycle.sh.
func ClusterProfilePath() string {
	return clusterProfilePath()
}

// FormatClusterProfileLine returns the shell assignment for tests.
func FormatClusterProfileLine(disableTraefik bool) string {
	val := "false"
	if disableTraefik {
		val = "true"
	}
	return strings.TrimSpace("K3SLAB_DISABLE_TRAEFIK=" + val)
}
