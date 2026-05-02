package server

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/scoring"
	serveranalysisui "codeberg.org/pawal/gonemaster/server/analysisui"
	serverpublic "codeberg.org/pawal/gonemaster/server/public"
	serverui "codeberg.org/pawal/gonemaster/server/ui"
)

// Server holds the HTTP API and supporting services.
type Server struct {
	cfg                      Config
	mux                      *http.ServeMux
	store                    JobStore
	analysis                 AnalysisController
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
	trustedProxies           []netip.Prefix
	hotCache                 *nameserverHotCache
	delegationLookup         func(context.Context, string) DelegationInfo
	configSources            map[string]SettingSource
	retentionDays            atomic.Int64
	// cohortRebuildsInFlight guards against overlapping rebuilds of the
	// same cohort. Async-dispatched rebuilds record their cohort ID here;
	// a second request for the same cohort is refused until the first
	// clears the set.
	cohortRebuildsMu       sync.Mutex
	cohortRebuildsInFlight map[int64]struct{}
}

// setScoringConfig loads an optional scoring config file and sets it on the
// store. Only *InMemoryJobStore and *SQLJobStore are supported; other
// implementations are left unchanged.
func setScoringConfig(store JobStore, path string) error {
	if path == "" {
		return nil
	}
	cfg, err := scoring.LoadConfig(path)
	if err != nil {
		return err
	}
	switch s := store.(type) {
	case *InMemoryJobStore:
		s.SetScoringConfig(cfg)
	case *SQLJobStore:
		s.SetScoringConfig(cfg)
	}
	return nil
}

// applyAnalysisConfig pushes capture-time analysis settings onto the store.
func applyAnalysisConfig(store JobStore, cfg AnalysisConfig) {
	if sql, ok := store.(*SQLJobStore); ok && cfg.TagViewMinLevel != "" {
		sql.SetTagViewMinLevel(cfg.TagViewMinLevel)
	}
}

// New builds a server with in-memory components.
func New(cfg Config) *Server {
	if cfg.ListenAddr == "" {
		cfg = DefaultConfig()
	}
	store := NewInMemoryJobStore()
	// Ignore scoring config load errors in the simple constructor.
	_ = setScoringConfig(store, cfg.ScoringConfigPath)
	applyAnalysisConfig(store, cfg.Analysis)
	return newServer(cfg, store, NewInMemoryQueue())
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
		db, err := openSQLDBWith(cfg.Database.Driver, cfg.Database.DSN, cfg.Database)
		if err != nil {
			return nil, err
		}
		if err := runMigrations(db, dialect); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("run migrations: %w", err)
		}
		store = NewSQLJobStore(db, dialect)
	}
	if err := setScoringConfig(store, cfg.ScoringConfigPath); err != nil {
		if sql, ok := store.(*SQLJobStore); ok {
			_ = sql.db.Close()
		}
		return nil, fmt.Errorf("load scoring config: %w", err)
	}
	applyAnalysisConfig(store, cfg.Analysis)

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
		cohortRebuildsInFlight:   map[int64]struct{}{},
		engineRunner:             engine.Run,
		engineLimiter:            newEngineLimiter(cfg.MaxConcurrentJobs),
		cancels:                  map[string]context.CancelFunc{},
		delegationLookup:         lookupDelegation,
	}
	if cfg.PublicAPI.RateLimitEnabled {
		s.rateLimiter = NewRateLimiter(cfg.PublicAPI.RateLimitMax, cfg.PublicAPI.RateLimitWindow.Duration)
	}
	s.trustedProxies = parseTrustedProxies(cfg.TrustedProxyCIDRs)
	if cfg.CrossJobHotCache {
		s.hotCache = newNameserverHotCache(0, cfg.EffectiveCrossJobHotCacheTTL())
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

// Store exposes the configured job store for optional integration layers.
func (s *Server) Store() JobStore {
	return s.store
}

// SHA-256 of SvelteKit's #svelte-announcer inline style; verified against
// the embedded bundle by TestAnalysisAnnouncerHashMatchesDist.
const announcerStyleHash = "'sha256-S8qMpvofolR8Mpjy4kQvEm7m1q8clzU4dfDH0AmvZjo='"

// securityHeadersMiddleware sets defensive HTTP security headers on every
// response. API paths get a restrictive CSP; UI/static paths get one that
// allows same-origin scripts, styles, and data URIs.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	const apiCSP = "default-src 'none'"
	const uiCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'"
	// script-src 'unsafe-inline': SvelteKit index.html bootstrap <script>.
	const analysisCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-hashes' " + announcerStyleHash + "; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), bluetooth=(), serial=(), midi=(), hid=(), accelerometer=(), gyroscope=(), magnetometer=(), fullscreen=(), display-capture=(), idle-detection=(), screen-wake-lock=(), xr-spatial-tracking=(), clipboard-read=()")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/pub/api/"):
			h.Set("Content-Security-Policy", apiCSP)
		case strings.HasPrefix(r.URL.Path, "/analysis/") || r.URL.Path == "/analysis":
			h.Set("Content-Security-Policy", analysisCSP)
		default:
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
	apiMux.HandleFunc("GET /batches/{id}/delete-preview", s.handleBatchDeletePreview)
	apiMux.HandleFunc("DELETE /batches/{id}", s.handleDeleteBatch)
	apiMux.HandleFunc("GET /batches/{id}", s.handleBatchByID)

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
	apiMux.HandleFunc("GET /tags/{name}/batches", s.handleTagBatches)
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

	apiMux.HandleFunc("POST /analysis/cohorts/{id}/rebuild", s.handleAnalysisCohortRebuild)
	apiMux.HandleFunc("POST /analysis/cohorts/{id}/clear", s.handleAnalysisCohortClear)
	apiMux.HandleFunc("POST /analysis/cohorts/{id}/snapshots/{slug}/rematerialize", s.handleAnalysisCohortSnapshotRematerialize)
	apiMux.HandleFunc("/analysis/cohorts/{id}/snapshots/{slug}", s.handleAnalysisCohortSnapshotByID)
	apiMux.HandleFunc("GET /analysis/cohorts/{id}/snapshots", s.handleAnalysisCohortSnapshots)
	apiMux.HandleFunc("/analysis/cohorts/{id}", s.handleAnalysisCohortByID)
	apiMux.HandleFunc("/analysis/cohorts", s.handleAnalysisCohorts)
	apiMux.HandleFunc("/analysis/status", s.handleAnalysisStatus)

	apiMux.HandleFunc("GET /features", s.handleFeatures)
	apiMux.HandleFunc("/settings", s.handleSettings)
	apiMux.HandleFunc("GET /scoring-config/defaults", s.handleScoringConfigDefaults)
	apiMux.HandleFunc("/scoring-config", s.handleScoringConfig)
	apiMux.HandleFunc("/locales", s.handleLocales)
	apiMux.HandleFunc("/metrics", s.handleMetrics)
	apiMux.HandleFunc("/healthz", s.handleHealth)

	s.mux.Handle("/api/v1/", s.recoverMiddleware(s.apiMetricsMiddleware(http.StripPrefix("/api/v1", apiMux))))
	s.mux.Handle("/api/v1", s.recoverMiddleware(s.apiMetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/", http.StatusMovedPermanently)
	}))))

	pubMux := http.NewServeMux()
	pubMux.HandleFunc("POST /jobs", s.handlePublicCreateJob)
	pubMux.HandleFunc("GET /profiles", s.handlePublicProfiles)
	pubMux.HandleFunc("GET /jobs/{publicID}/result", s.handlePublicGetResult)
	pubMux.HandleFunc("GET /jobs/{publicID}", s.handlePublicGetJob)
	pubMux.HandleFunc("GET /locales", s.handleLocales)
	pubMux.HandleFunc("GET /lookup/{domain}", s.handlePublicLookupDomain)
	pubMux.HandleFunc("GET /version", s.handlePublicVersion)
	pubMux.HandleFunc("GET /info", s.handlePublicInfo)
	pubMux.HandleFunc("GET /analysis/catalog", s.handlePublicAnalysisCatalog)
	pubMux.HandleFunc("GET /analysis/cohorts", s.handlePublicAnalysisCohorts)
	pubMux.HandleFunc("GET /analysis/version", s.handlePublicVersion)

	// Path-segmented snapshot reads. Each one resolves the cohort + slug
	// from the path; immutable cache headers fire because ?snapshot= is
	// effectively explicit.
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/overview",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisOverview))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/domains",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisDomains))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/domains/{domain}",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisDomainDetail))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/nameservers",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisNameservers))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/nameservers/{name}",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisNameserverDetail))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/endpoints",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisEndpoints))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/endpoints/{address}",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisEndpointDetail))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/asns",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisASNs))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/asns/{asn}",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisASNDetail))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/prefixes",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisPrefixes))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/prefix",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisPrefixDetail))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/tags",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisTags))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/tags/{tag}",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisTagDetail))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/testcases",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisTestcases))
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}/testcase",
		pubAnalysisSnapshotPath(s.handlePublicAnalysisTestcaseDetail))

	// Cohort/snapshot-list/diff/trends are inherently multi-snapshot or
	// auto-latest by URL design, so they stay as-is.
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots", s.handlePublicAnalysisSnapshots)
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/snapshots/{slug}", s.handlePublicAnalysisSnapshotDetail)
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/trends", s.handlePublicAnalysisTrends)
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/diff", s.handlePublicAnalysisDiff)
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}", s.handlePublicAnalysisCohortDetail)
	var pubHandler http.Handler = http.StripPrefix("/pub/api/v1", pubMux)
	if d := s.cfg.PublicAPI.AnalysisRequestTimeout.Duration; d > 0 {
		pubHandler = analysisTimeoutMiddleware(d, pubHandler)
	}
	if s.rateLimiter != nil {
		pubHandler = rateLimitMiddleware(s.rateLimiter, s.trustedProxies, pubHandler)
	}
	pubHandler = s.recoverMiddleware(pubHandler)
	s.mux.Handle("/pub/api/v1/", pubHandler)

	s.mux.HandleFunc("GET /robots.txt", s.handleRobotsTxt)
	s.mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	s.mux.Handle("/public/", http.StripPrefix("/public", serverpublic.Handler(s.cfg.PublicURL)))
	s.mux.Handle("/analysis/", http.StripPrefix("/analysis", serveranalysisui.Handler()))
	s.mux.HandleFunc("/analysis", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/analysis/", http.StatusMovedPermanently)
	})
	s.mux.Handle("/", serverui.Handler())
}
