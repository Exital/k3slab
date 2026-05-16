package kube

import (
	"os"
	"strings"
)

// Env returns environment variables for kubectl and cluster shell commands.
// extra entries are appended after the standard KUBECONFIG and HOME values.
func Env(extra ...string) []string {
	kc := strings.TrimSpace(os.Getenv("KUBECONFIG"))
	if kc == "" {
		kc = "/etc/rancher/k3s/k3s.yaml"
	}
	base := append(os.Environ(),
		"KUBECONFIG="+kc,
		"HOME=/root",
	)
	return append(base, extra...)
}
