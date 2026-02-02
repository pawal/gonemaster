package server

import (
	"context"
	"net/http"
	"sync"

	"codeberg.org/pawal/gonemaster/engine"
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
	s.mux.HandleFunc("/jobs/batch", s.handleJobsBatch)
	s.mux.HandleFunc("/jobs/", s.handleJobByID)
	s.mux.HandleFunc("/jobs", s.handleJobs)
	s.mux.HandleFunc("/batches/", s.handleBatchByID)

	s.mux.HandleFunc("/queue/pause", s.handleQueuePause)
	s.mux.HandleFunc("/queue/resume", s.handleQueueResume)
	s.mux.HandleFunc("/queue/reorder", s.handleQueueReorder)

	s.mux.HandleFunc("/metrics", s.handleMetrics)
	s.mux.HandleFunc("/healthz", s.handleHealth)
}
