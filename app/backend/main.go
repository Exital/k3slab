package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"k3slab/engine"
	"k3slab/exposure"
	"k3slab/loghub"
	"k3slab/server"
	"k3slab/workshop"
)

//go:embed all:dist
var staticDist embed.FS

func init() {
	server.SetDistFS(func() (fs.FS, error) {
		return staticDist, nil
	})
}

func main() {
	lab := os.Getenv("LAB_ROOT")
	if lab == "" {
		lab = "/lab/k3s"
	}
	hub := loghub.New()
	wp := filepath.Join(lab, "workshop.yml")
	data, err := os.ReadFile(wp)
	var eng *engine.Engine
	switch {
	case err != nil:
		eng = engine.NewLoadError(err, lab, hub)
	default:
		w, err2 := workshop.Parse(data)
		if err2 != nil {
			eng = engine.NewLoadError(err2, lab, hub)
		} else {
			eng = engine.New(w, lab, hub)
		}
	}
	watcher := exposure.NewWatcher(context.Background())
	srv, err := server.New(eng, hub, lab, watcher)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(srv.Run())
}
