package labs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderLabManifestsFromTemplate(t *testing.T) {
	root := t.TempDir()
	labDir := filepath.Join(root, "02-lab")
	manifestsDir := filepath.Join(labDir, "manifests")
	if err := os.MkdirAll(manifestsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(t.TempDir(), "render-lab-manifests.sh")
	scriptBody := `#!/usr/bin/env bash
set -euo pipefail
lab_dir="$1"
export K3SLAB_INGRESS_HOST="${K3SLAB_INGRESS_HOST:-localhost}"
vars=$(env | awk -F= '/^K3SLAB_/ {printf "${%s} ", $1}')
for tpl in "${lab_dir}"/manifests/*.yml.template; do
  [[ -f "$tpl" ]] || continue
  out="${tpl%.template}"
  envsubst "$vars" < "$tpl" > "$out"
done
`
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("K3SLAB_RENDER_MANIFESTS_SCRIPT", script)
	t.Setenv("K3SLAB_INGRESS_HOST", "k3slab-vm")

	tpl := []byte(`apiVersion: networking.k8s.io/v1
kind: Ingress
spec:
  rules:
    - host: ${K3SLAB_INGRESS_HOST}
`)
	if err := os.WriteFile(filepath.Join(manifestsDir, "ingress.yml.template"), tpl, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RenderLabManifests(root, "02-lab"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(manifestsDir, "ingress.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "host: k3slab-vm") {
		t.Fatalf("rendered ingress: got %q", got)
	}
}

func TestRenderLabManifestsEmptyID(t *testing.T) {
	if err := RenderLabManifests("/lab", ""); err != nil {
		t.Fatal(err)
	}
}
