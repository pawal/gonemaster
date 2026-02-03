package server

import (
	"context"
	"net/http"
	"sync"

	"codeberg.org/pawal/gonemaster/engine"
	serverui "codeberg.org/pawal/gonemaster/server/ui"
)

// Server holds the HTTP API and supporting services.
type Server struct {
	cfg          Config
	mux          *http.ServeMux
	store        JobStore
	queue        Queue
	workers      workerPool
	engineMu     sync.Mutex
	engineRunner func(engine.RunRequest) ([]engine.LogEntry, error)
	cancelMu     sync.Mutex
	cancels      map[string]context.CancelFunc
}

// New builds a server with in-memory components.
func New(cfg Config) *Server {
	if cfg.ListenAddr == "" {
		cfg = DefaultConfig()
	}
	s := &Server{
		cfg:          cfg,
		mux:          http.NewServeMux(),
		store:        NewInMemoryJobStore(),
		queue:        NewInMemoryQueue(),
		engineRunner: engine.Run,
		cancels:      map[string]context.CancelFunc{},
	}
	s.routes()
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	if s.cfg.Debug {
		return debugMiddleware(s.mux)
	}
	return s.mux
}

func (s *Server) routes() {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/jobs/batch", s.handleJobsBatch)
	apiMux.HandleFunc("/jobs/", s.handleJobByID)
	apiMux.HandleFunc("/jobs", s.handleJobs)
	apiMux.HandleFunc("/batches/", s.handleBatchByID)

	apiMux.HandleFunc("/queue/pause", s.handleQueuePause)
	apiMux.HandleFunc("/queue/resume", s.handleQueueResume)
	apiMux.HandleFunc("/queue/reorder", s.handleQueueReorder)

	apiMux.HandleFunc("/metrics", s.handleMetrics)
	apiMux.HandleFunc("/healthz", s.handleHealth)

	s.mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiMux))
	s.mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/", http.StatusMovedPermanently)
	})
	s.mux.Handle("/", serverui.Handler())
}
