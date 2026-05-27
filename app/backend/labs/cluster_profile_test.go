package labs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClusterDisableTraefikFromWorkshop(t *testing.T) {
	root := t.TempDir()
	labDir := filepath.Join(root, "02-lab")
	if err := os.MkdirAll(labDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := []byte(`name: Lab
cluster:
  disable_traefik: true
tabs:
  steps:
    - id: q1
      type: question
      title: Q
      answer_type: text
      verify: "true"
`)
	if err := os.WriteFile(filepath.Join(labDir, workshopFile), yaml, 0o644); err != nil {
		t.Fatal(err)
	}
	disable, err := ClusterDisableTraefik(root, "02-lab")
	if err != nil {
		t.Fatal(err)
	}
	if !disable {
		t.Fatal("expected disable_traefik true")
	}
}

func TestWriteClusterProfile(t *testing.T) {
	root := t.TempDir()
	labDir := filepath.Join(root, "01-lab")
	if err := os.MkdirAll(labDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := []byte(`name: Lab
tabs:
  steps:
    - id: q1
      type: question
      title: Q
      answer_type: text
      verify: "true"
`)
	if err := os.WriteFile(filepath.Join(labDir, workshopFile), yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	profilePath := filepath.Join(t.TempDir(), "cluster-profile.env")
	t.Setenv("K3SLAB_CLUSTER_PROFILE", profilePath)

	if err := WriteClusterProfile(root, "01-lab"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	want := FormatClusterProfileLine(false) + "\n"
	if string(data) != want {
		t.Fatalf("profile: got %q want %q", data, want)
	}
}
