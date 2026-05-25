package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"

	"k3slab/cluster"
	"k3slab/exposure"
	"k3slab/loghub"
	"k3slab/labs"
	"k3slab/server"
)

//go:embed all:dist
var staticDist embed.FS

func init() {
	server.SetDistFS(func() (fs.FS, error) {
		return staticDist, nil
	})
}

func main() {
	hub := loghub.New()
	watcher := exposure.NewWatcher(context.Background())
	clusterMgr := cluster.NewManager()

	labMgr, err := labs.NewManager(hub, clusterMgr, watcher)
	if err != nil {
		log.Fatal(err)
	}

	if root := labMgr.ActiveLabRoot(); root != "" {
		_ = os.Setenv("K3SLAB_TERMINAL_CWD", root)
	}

	srv, err := server.New(labMgr, hub, watcher, clusterMgr)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(srv.Run())
}
