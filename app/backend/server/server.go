package server

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"k3slab/cluster"
	"k3slab/engine"
	"k3slab/exposure"
	"k3slab/labs"
	"k3slab/loghub"
)

// Server wires HTTP handlers, static SPA, SSE, and terminal WS.
type Server struct {
	mux      *http.ServeMux
	labMgr   *labs.Manager
	hub      *loghub.Hub
	exposure *exposure.Watcher
	cluster  *cluster.Manager
	static   fs.FS
}

// New constructs the HTTP server with routes registered.
func New(labMgr *labs.Manager, hub *loghub.Hub, watcher *exposure.Watcher, clusterMgr *cluster.Manager) (*Server, error) {
	staticFS, err := staticFileSystem()
	if err != nil {
		log.Printf("static files: %v", err)
	}
	s := &Server{
		mux:      http.NewServeMux(),
		labMgr:   labMgr,
		hub:      hub,
		exposure: watcher,
		cluster:  clusterMgr,
		static:   staticFS,
	}
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/lab/status", s.handleLabStatus)
	s.mux.HandleFunc("POST /api/lab/restart", s.handleLabRestart)
	s.mux.HandleFunc("GET /api/labs", s.handleLabs)
	s.mux.HandleFunc("POST /api/labs/select", s.handleLabsSelect)
	s.mux.HandleFunc("GET /api/workshop", s.handleWorkshop)
	s.mux.HandleFunc("POST /api/workshop/restart", s.handleWorkshopRestart)
	s.mux.HandleFunc("POST /api/task/run", s.handleTaskRun)
	s.mux.HandleFunc("POST /api/question/setup", s.handleQuestionSetup)
	s.mux.HandleFunc("POST /api/question/submit", s.handleQuestionSubmit)
	s.mux.HandleFunc("POST /api/question/check", s.handleQuestionCheck)
	s.mux.HandleFunc("POST /api/question/next", s.handleQuestionNext)
	s.mux.HandleFunc("GET /api/stream/logs", s.handleLogStream)
	s.mux.HandleFunc("GET /api/exposed", s.handleExposed)
	s.mux.HandleFunc("GET /api/stream/exposed", s.handleExposedStream)
	s.mux.HandleFunc("GET /api/ws/terminal", s.handleTerminalWS)

	if s.static != nil {
		s.mux.Handle("GET /{path...}", s.spaFileServer())
	}
	return s, nil
}

// Run starts the HTTP server.
func (s *Server) Run() error {
	addr := strings.TrimSpace(os.Getenv("K3SLAB_LISTEN"))
	if addr == "" {
		addr = "0.0.0.0:3010"
	} else if strings.HasPrefix(addr, ":") {
		addr = "0.0.0.0" + addr
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           withLogging(s.mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s", addr)
	return srv.ListenAndServe()
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) spaFileServer() http.Handler {
	root := s.static
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tail := strings.TrimPrefix(r.URL.Path, "/")
		if strings.Contains(tail, "..") {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		if strings.HasPrefix(tail, "api/") {
			http.NotFound(w, r)
			return
		}

		name := tail
		if name == "" {
			name = "index.html"
		}
		name = path.Clean("/" + name)
		name = strings.TrimPrefix(name, "/")

		if fileExists(root, name) {
			http.ServeFileFS(w, r, root, name)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, root, "index.html")
	})
}

func fileExists(root fs.FS, name string) bool {
	if name == "" || name == "." {
		return false
	}
	f, err := root.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleLabStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"cluster": s.cluster.Status(r.Context())})
}

func (s *Server) handleLabs(w http.ResponseWriter, r *http.Request) {
	cat, err := s.labMgr.Catalog()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, cat)
}

type selectLabBody struct {
	ID string `json:"id"`
}

func (s *Server) handleLabsSelect(w http.ResponseWriter, r *http.Request) {
	var body selectLabBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("missing id"))
		return
	}

	state, err := s.labMgr.SelectLab(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, cluster.ErrResetDisabled):
			writeErr(w, http.StatusForbidden, err)
		case errors.Is(err, cluster.ErrAlreadyResetting):
			writeErr(w, http.StatusConflict, err)
		case errors.Is(err, labs.ErrLabNotFound), errors.Is(err, labs.ErrLabInvalid), errors.Is(err, labs.ErrInvalidID):
			writeErr(w, http.StatusBadRequest, err)
		default:
			writeErr(w, http.StatusInternalServerError, err)
		}
		return
	}
	writeJSON(w, map[string]any{"state": state})
}

func (s *Server) handleLabRestart(w http.ResponseWriter, r *http.Request) {
	if s.cluster.IsResetting() {
		writeErr(w, http.StatusConflict, cluster.ErrAlreadyResetting)
		return
	}

	state, err := s.labMgr.RestartLab(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, cluster.ErrResetDisabled):
			writeErr(w, http.StatusForbidden, err)
		case errors.Is(err, cluster.ErrAlreadyResetting):
			writeErr(w, http.StatusConflict, err)
		default:
			writeErr(w, http.StatusInternalServerError, cluster.ErrResetFailed)
		}
		return
	}
	writeJSON(w, map[string]any{"state": state})
}

func (s *Server) requireClusterReady(w http.ResponseWriter) bool {
	if s.cluster.IsResetting() {
		writeErr(w, http.StatusConflict, cluster.ErrAlreadyResetting)
		return false
	}
	return true
}

func (s *Server) handleWorkshop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.labMgr.WorkshopState())
}

func (s *Server) handleWorkshopRestart(w http.ResponseWriter, r *http.Request) {
	state, err := s.labMgr.RestartWorkshop()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, state)
}

func (s *Server) handleTaskRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireClusterReady(w) {
		return
	}
	ctx := r.Context()
	var logs string
	var err error
	var snap labs.WorkshopState
	s.labMgr.WithEngine(func(eng *engine.Engine, labID, labsRoot string) {
		logs, err = eng.RunTask(ctx)
		if err == nil {
			snap = s.labMgr.WorkshopSnap(eng, labID, labsRoot)
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "logs": logs, "state": snap})
}

func (s *Server) handleQuestionSetup(w http.ResponseWriter, r *http.Request) {
	if !s.requireClusterReady(w) {
		return
	}
	ctx := r.Context()
	var logs string
	var err error
	var snap labs.WorkshopState
	s.labMgr.WithEngine(func(eng *engine.Engine, labID, labsRoot string) {
		logs, err = eng.RunQuestionSetup(ctx)
		if err == nil {
			snap = s.labMgr.WorkshopSnap(eng, labID, labsRoot)
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "logs": logs, "state": snap})
}

type submitBody struct {
	Answer string `json:"answer"`
}

func (s *Server) handleQuestionSubmit(w http.ResponseWriter, r *http.Request) {
	var body submitBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !s.requireClusterReady(w) {
		return
	}
	ctx := r.Context()
	var ok bool
	var logs string
	var err error
	var snap labs.WorkshopState
	s.labMgr.WithEngine(func(eng *engine.Engine, labID, labsRoot string) {
		ok, logs, err = eng.SubmitAnswer(ctx, body.Answer)
		if err == nil {
			snap = s.labMgr.WorkshopSnap(eng, labID, labsRoot)
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": ok, "logs": logs, "state": snap})
}

func (s *Server) handleQuestionCheck(w http.ResponseWriter, r *http.Request) {
	if !s.requireClusterReady(w) {
		return
	}
	ctx := r.Context()
	var ok bool
	var logs string
	var err error
	var snap labs.WorkshopState
	s.labMgr.WithEngine(func(eng *engine.Engine, labID, labsRoot string) {
		ok, logs, err = eng.CheckQuestion(ctx)
		if err == nil {
			snap = s.labMgr.WorkshopSnap(eng, labID, labsRoot)
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": ok, "logs": logs, "state": snap})
}

func (s *Server) handleQuestionNext(w http.ResponseWriter, r *http.Request) {
	if !s.requireClusterReady(w) {
		return
	}
	var err error
	var snap labs.WorkshopState
	s.labMgr.WithEngine(func(eng *engine.Engine, labID, labsRoot string) {
		err = eng.AdvanceQuestion()
		if err == nil {
			snap = s.labMgr.WorkshopSnap(eng, labID, labsRoot)
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"state": snap})
}

func (s *Server) handleExposed(w http.ResponseWriter, r *http.Request) {
	snap := s.exposure.Snapshot()
	if snap.Endpoints == nil {
		snap.Endpoints = []exposure.Endpoint{}
	}
	writeJSON(w, snap)
}

func (s *Server) handleExposedStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	snap := s.exposure.Snapshot()
	if snap.Endpoints == nil {
		snap.Endpoints = []exposure.Endpoint{}
	}
	writeExposedSSE(w, flusher, snap)
	ch := s.exposure.Hub().Subscribe()
	defer s.exposure.Hub().Unsubscribe(ch)
	for {
		select {
		case snap, open := <-ch:
			if !open {
				return
			}
			writeExposedSSE(w, flusher, snap)
		case <-r.Context().Done():
			return
		}
	}
}

func writeExposedSSE(w http.ResponseWriter, flusher http.Flusher, snap exposure.Snapshot) {
	b, _ := json.Marshal(snap)
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
	flusher.Flush()
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)
	_, _ = w.Write([]byte(":ok\n\n"))
	flusher.Flush()
	for {
		select {
		case line, open := <-ch:
			if !open {
				return
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(loghub.SSEData(line))
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

var distFS func() (fs.FS, error) = func() (fs.FS, error) {
	return nil, errors.New("no embedded dist")
}

func SetDistFS(f func() (fs.FS, error)) {
	distFS = f
}

func staticFileSystem() (fs.FS, error) {
	if p := strings.TrimSpace(os.Getenv("K3SLAB_STATIC_DIR")); p != "" {
		if _, err := os.Stat(filepath.Join(p, "index.html")); err == nil {
			return os.DirFS(p), nil
		}
	}
	return distFS()
}
