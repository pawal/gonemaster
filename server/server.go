package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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
	configSources            map[string]SettingSource
	retentionDays            atomic.Int64
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

	srv := newServer(cfg, store, queue)
	srv.ApplyDatabaseSettings()
	return srv, nil
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
	s.retentionDays.Store(int64(cfg.Database.RetentionDays))
	s.routes()
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	if s.cfg.Debug {
		return securityHeadersMiddleware(debugMiddleware(s.mux))
	}
	return securityHeadersMiddleware(s.mux)
}

// securityHeadersMiddleware sets defensive HTTP security headers on every
// response. API paths get a restrictive CSP; UI/static paths get one that
// allows same-origin scripts, styles, and data URIs.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	const apiCSP = "default-src 'none'"
	const uiCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/pub/api/") {
			h.Set("Content-Security-Policy", apiCSP)
		} else {
			h.Set("Content-Security-Policy", uiCSP)
		}
		next.ServeHTTP(w, r)
	})
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
	apiMux.HandleFunc("/tags/{name}/profile", s.handleTagProfile)
	apiMux.HandleFunc("/tags/{name}/domains", s.handleTagDomains)
	apiMux.HandleFunc("/tags/{name}", s.handleTagByName)
	apiMux.HandleFunc("/tags", s.handleTags)
	apiMux.HandleFunc("GET /profiles/default", s.handleDefaultProfile)
	apiMux.HandleFunc("GET /profiles/defaults", s.handleProfileDefaults)
	apiMux.HandleFunc("GET /profiles/compatibility", s.handleProfilesCompatibility)
	apiMux.HandleFunc("POST /profiles/mark-all-reviewed", s.handleMarkAllProfilesReviewed)
	apiMux.HandleFunc("GET /profiles/{id}/compatibility", s.handleProfileCompatibility)
	apiMux.HandleFunc("PATCH /profiles/{id}", s.handlePatchProfile)
	apiMux.HandleFunc("/profiles/{id}", s.handleProfileByID)
	apiMux.HandleFunc("/profiles", s.handleProfiles)

	apiMux.HandleFunc("/settings", s.handleSettings)
	apiMux.HandleFunc("/locales", s.handleLocales)
	apiMux.HandleFunc("/metrics", s.handleMetrics)
	apiMux.HandleFunc("/healthz", s.handleHealth)

	s.mux.Handle("/api/v1/", s.apiMetricsMiddleware(http.StripPrefix("/api/v1", apiMux)))
	s.mux.Handle("/api/v1", s.apiMetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/", http.StatusMovedPermanently)
	})))

	pubMux := http.NewServeMux()
	pubMux.HandleFunc("POST /jobs", s.handlePublicCreateJob)
	pubMux.HandleFunc("GET /profiles", s.handlePublicProfiles)
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

	s.mux.HandleFunc("GET /robots.txt", s.handleRobotsTxt)
	s.mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	s.mux.Handle("/public/", http.StripPrefix("/public", serverpublic.Handler(s.cfg.PublicURL)))
	s.mux.Handle("/", serverui.Handler())
}
