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
	cfg, err := readClusterConfig(labsRoot, id)
	if err != nil {
		return false, err
	}
	return cfg.DisableTraefik, nil
}

// ClusterEnableNetworkPolicy reads whether the lab enables K3s NetworkPolicy enforcement.
// Missing cluster config defaults to false (network policy disabled via --disable-network-policy).
func ClusterEnableNetworkPolicy(labsRoot, id string) (bool, error) {
	cfg, err := readClusterConfig(labsRoot, id)
	if err != nil {
		return false, err
	}
	return cfg.EnableNetworkPolicy, nil
}

func readClusterConfig(labsRoot, id string) (workshop.ClusterConfig, error) {
	if id == "" {
		return workshop.ClusterConfig{}, nil
	}
	labDir, err := LabPath(labsRoot, id)
	if err != nil {
		return workshop.ClusterConfig{}, err
	}
	data, err := os.ReadFile(filepath.Join(labDir, workshopFile))
	if err != nil {
		return workshop.ClusterConfig{}, fmt.Errorf("read %s: %w", workshopFile, err)
	}
	w, err := workshop.Parse(data)
	if err != nil {
		return workshop.ClusterConfig{}, err
	}
	return w.Cluster, nil
}

// WriteClusterProfile writes /run/k3slab/cluster-profile.env for k3s-lifecycle scripts.
func WriteClusterProfile(labsRoot, id string) error {
	cfg, err := readClusterConfig(labsRoot, id)
	if err != nil {
		return err
	}
	path := clusterProfilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := FormatClusterProfile(cfg.DisableTraefik, cfg.EnableNetworkPolicy) + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

// ClusterProfilePath is the env file path consumed by k3s-lifecycle.sh.
func ClusterProfilePath() string {
	return clusterProfilePath()
}

// FormatClusterProfile returns the shell assignments for the cluster profile env file.
func FormatClusterProfile(disableTraefik, enableNetworkPolicy bool) string {
	traefik := "false"
	if disableTraefik {
		traefik = "true"
	}
	netpol := "false"
	if enableNetworkPolicy {
		netpol = "true"
	}
	return strings.TrimSpace(fmt.Sprintf(
		"K3SLAB_DISABLE_TRAEFIK=%s\nK3SLAB_ENABLE_NETWORK_POLICY=%s",
		traefik, netpol,
	))
}

// FormatClusterProfileLine returns the Traefik assignment (kept for older tests/callers).
func FormatClusterProfileLine(disableTraefik bool) string {
	return FormatClusterProfile(disableTraefik, false)
}
