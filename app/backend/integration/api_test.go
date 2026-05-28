//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k3slab/cluster"
	"k3slab/exposure"
	"k3slab/labs"
	"k3slab/loghub"
	"k3slab/server"
	"k3slab/workshop"
)

func TestHealth(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body: %+v", body)
	}
}

func TestLabsCatalog(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/labs", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	var cat labs.Catalog
	if err := json.NewDecoder(rr.Body).Decode(&cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.Labs) == 0 {
		t.Fatal("expected at least one lab")
	}
}

func TestAllBundledLabsParse(t *testing.T) {
	root := labsRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		wp := filepath.Join(root, ent.Name(), "workshop.yml")
		data, err := os.ReadFile(wp)
		if err != nil {
			t.Fatalf("%s: %v", ent.Name(), err)
		}
		if _, err := workshop.Parse(data); err != nil {
			t.Fatalf("%s: %v", ent.Name(), err)
		}
	}
}

func testServer(t *testing.T) *server.Server {
	t.Helper()
	root := labsRoot(t)
	t.Setenv("LABS_ROOT", root)
	t.Setenv("LAB_ID", firstValidLab(t, root))

	hub := loghub.New()
	cm := cluster.NewManager()
	watcher := exposure.NewWatcher(context.Background())
	mgr, err := labs.NewManager(hub, cm, watcher)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := server.New(mgr, hub, watcher, cm)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func labsRoot(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("K3SLAB_INTEGRATION_LABS_ROOT"); r != "" {
		return r
	}
	for _, candidate := range []string{"/src/lab", "../../lab", "../../../lab"} {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	t.Skip("lab directory not found")
	return ""
}

func firstValidLab(t *testing.T, root string) string {
	t.Helper()
	meta, err := labs.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range meta {
		if m.Valid {
			return m.ID
		}
	}
	t.Fatal("no valid lab")
	return ""
}
