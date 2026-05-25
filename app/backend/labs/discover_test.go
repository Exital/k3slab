package labs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAndValidateID(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha", workshopFile), []byte(`
name: Alpha Lab
description: First lab
tabs:
  steps:
    - id: t1
      type: task
      title: T
      run: echo ok
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken", workshopFile), []byte("not: valid"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta) != 2 {
		t.Fatalf("got %d labs, want 2", len(meta))
	}
	if meta[0].ID != "alpha" || !meta[0].Valid || meta[0].Name != "Alpha Lab" {
		t.Fatalf("alpha: %+v", meta[0])
	}
	if meta[1].ID != "broken" || meta[1].Valid {
		t.Fatalf("broken: %+v", meta[1])
	}

	for _, bad := range []string{"", "..", "a/b", ".hidden"} {
		if ValidateID(bad) == nil {
			t.Fatalf("expected invalid id %q", bad)
		}
	}
}
