package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	serverpublic "codeberg.org/pawal/gonemaster/server/public"
	serverui "codeberg.org/pawal/gonemaster/server/ui"
)

// Server holds the HTTP API and supporting services.
type Server struct {
	cfg                      Config
	mux                      *http.ServeMux
	store                    JobStore
	queue                    Queue
	workers                  workerPool
	metrics                  *MetricsCollector
	metricsCache             map[string]metricsCacheEntry
	metricsCacheMu           sync.Mutex
	progressWriteMu          sync.Mutex
	progressWrites           map[string]progressWriteState
	progressWriteMinStep     int
	progressWriteMinInterval time.Duration
	engineRunner             func(engine.RunRequest) ([]engine.LogEntry, error)
	engineLimiter            *engineLimiter
	cancelMu                 sync.Mutex
	cancels                  map[string]context.CancelFunc
	rateLimiter              *RateLimiter
}

// New builds a server with in-memory components.
func New(cfg Config) *Server {
	if cfg.ListenAddr == "" {
		cfg = DefaultConfig()
	}
	return newServer(cfg, NewInMemoryJobStore(), NewInMemoryQueue())
}

// NewWithOptions builds a server using the configured storage backend.
// When cfg.Database.Driver is set, a SQL store is opened, schema migrations
// are run, and any jobs from an unclean shutdown are recovered. An error is
// returned if the database cannot be opened or migrated.
func NewWithOptions(cfg Config) (*Server, error) {
	if cfg.ListenAddr == "" {
		cfg = DefaultConfig()
	}

	var store JobStore
	if cfg.Database.Driver == "" {
		store = NewInMemoryJobStore()
	} else {
		dialect, err := dialectFor(cfg.Database.Driver)
		if err != nil {
			return nil, err
		}
		db, err := openSQLDB(cfg.Database.Driver, cfg.Database.DSN)
		if err != nil {
			return nil, err
		}
		if err := runMigrations(db, dialect); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("run migrations: %w", err)
		}
		store = NewSQLJobStore(db, dialect)
	}

	queue := NewInMemoryQueue()
	if err := RecoverJobs(store, queue); err != nil {
		if sql, ok := store.(*SQLJobStore); ok {
			_ = sql.db.Close()
		}
		return nil, fmt.Errorf("recover jobs: %w", err)
	}

	return newServer(cfg, store, queue), nil
}

// newServer constructs a Server with the given store and queue.
func newServer(cfg Config, store JobStore, queue Queue) *Server {
	s := &Server{
		cfg:                      cfg,
		mux:                      http.NewServeMux(),
		store:                    store,
		queue:                    queue,
		metrics:                  NewMetricsCollector(cfg),
		metricsCache:             map[string]metricsCacheEntry{},
		progressWrites:           map[string]progressWriteState{},
		progressWriteMinStep:     defaultProgressWriteMinStep,
		progressWriteMinInterval: defaultProgressWriteMinInterval,
		engineRunner:             engine.Run,
		engineLimiter:            newEngineLimiter(cfg.MaxConcurrentJobs),
		cancels:                  map[string]context.CancelFunc{},
	}
	if cfg.PublicAPI.RateLimitEnabled {
		s.rateLimiter = NewRateLimiter(cfg.PublicAPI.RateLimitMax, cfg.PublicAPI.RateLimitWindow.Duration)
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
	apiMux.HandleFunc("/jobs/purge", s.handleJobsPurge)
	apiMux.HandleFunc("/jobs/", s.handleJobByID)
	apiMux.HandleFunc("/jobs", s.handleJobs)
	apiMux.HandleFunc("/batches/", s.handleBatchByID)

	apiMux.HandleFunc("/queue/pause", s.handleQueuePause)
	apiMux.HandleFunc("/queue/resume", s.handleQueueResume)
	apiMux.HandleFunc("/queue/reorder", s.handleQueueReorder)
	apiMux.HandleFunc("/queue/remove", s.handleQueueRemove)

	apiMux.HandleFunc("GET /domains/{id}/runs", s.handleGetDomainRuns)
	apiMux.HandleFunc("GET /domains/{id}", s.handleGetDomain)
	apiMux.HandleFunc("GET /domains", s.handleListDomains)

	apiMux.HandleFunc("GET /entries", s.handleListEntries)

	apiMux.HandleFunc("GET /runs/{id}/result", s.handleGetRunResult)
	apiMux.HandleFunc("GET /runs/{id}", s.handleGetRun)
	apiMux.HandleFunc("GET /runs", s.handleListRuns)

	apiMux.HandleFunc("GET /tags/{name}/summary", s.handleTagSummary)
	apiMux.HandleFunc("/tags/{name}/domains", s.handleTagDomains)
	apiMux.HandleFunc("/tags/{name}", s.handleTagByName)
	apiMux.HandleFunc("/tags", s.handleTags)

	apiMux.HandleFunc("/locales", s.handleLocales)
	apiMux.HandleFunc("/metrics", s.handleMetrics)
	apiMux.HandleFunc("/healthz", s.handleHealth)

	s.mux.Handle("/api/v1/", s.apiMetricsMiddleware(http.StripPrefix("/api/v1", apiMux)))
	s.mux.Handle("/api/v1", s.apiMetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/", http.StatusMovedPermanently)
	})))

	pubMux := http.NewServeMux()
	pubMux.HandleFunc("POST /jobs", s.handlePublicCreateJob)
	pubMux.HandleFunc("GET /jobs/{publicID}/result", s.handlePublicGetResult)
	pubMux.HandleFunc("GET /jobs/{publicID}", s.handlePublicGetJob)
	pubMux.HandleFunc("GET /locales", s.handleLocales)
	pubMux.HandleFunc("GET /lookup/{domain}", s.handlePublicLookupDomain)
	pubMux.HandleFunc("GET /version", s.handlePublicVersion)
	var pubHandler http.Handler = http.StripPrefix("/pub/api/v1", pubMux)
	if s.rateLimiter != nil {
		pubHandler = rateLimitMiddleware(s.rateLimiter, pubHandler)
	}
	s.mux.Handle("/pub/api/v1/", pubHandler)

	s.mux.Handle("/public/", http.StripPrefix("/public", serverpublic.Handler()))
	s.mux.Handle("/", serverui.Handler())
}
