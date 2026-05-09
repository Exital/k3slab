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

	"k3slab/engine"
	"k3slab/loghub"
)

// Server wires HTTP handlers, static SPA, SSE, and terminal WS.
type Server struct {
	mux     *http.ServeMux
	eng     *engine.Engine
	hub     *loghub.Hub
	labRoot string
	static  fs.FS
}

// New constructs the HTTP server with routes registered.
func New(eng *engine.Engine, hub *loghub.Hub, labRoot string) (*Server, error) {
	staticFS, err := staticFileSystem()
	if err != nil {
		log.Printf("static files: %v", err)
	}
	s := &Server{
		mux:     http.NewServeMux(),
		eng:     eng,
		hub:     hub,
		labRoot: labRoot,
		static:  staticFS,
	}
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/workshop", s.handleWorkshop)
	s.mux.HandleFunc("POST /api/workshop/restart", s.handleWorkshopRestart)
	s.mux.HandleFunc("POST /api/task/run", s.handleTaskRun)
	s.mux.HandleFunc("POST /api/question/setup", s.handleQuestionSetup)
	s.mux.HandleFunc("POST /api/question/submit", s.handleQuestionSubmit)
	s.mux.HandleFunc("GET /api/stream/logs", s.handleLogStream)
	s.mux.HandleFunc("GET /api/ws/terminal", s.handleTerminalWS)

	if s.static != nil {
		// One wildcard only: registering both "GET /" and "GET /{path...}" panics on Go 1.22+ (overlapping patterns).
		// This pattern matches "/" as well (empty remainder).
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

// spaFileServer serves the Vite build. Missing non-asset paths fall back to index.html (client router).
// We use ServeFileFS instead of FileServer+fallback Open to avoid "attempting to traverse a non-directory".
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
		// Bundled assets must be real files
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

func (s *Server) handleWorkshop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.eng.Snapshot())
}

func (s *Server) handleWorkshopRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.eng.Restart(); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, s.eng.Snapshot())
}

func (s *Server) handleTaskRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logs, err := s.eng.RunTask(ctx)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "logs": logs, "state": s.eng.Snapshot()})
}

func (s *Server) handleQuestionSetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logs, err := s.eng.RunQuestionSetup(ctx)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "logs": logs, "state": s.eng.Snapshot()})
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
	ctx := r.Context()
	ok, logs, err := s.eng.SubmitAnswer(ctx, body.Answer)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": ok, "logs": logs, "state": s.eng.Snapshot()})
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

// SetDistFS registers the embed FS factory (called from main via static.go).
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
