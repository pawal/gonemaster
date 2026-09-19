package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/scoring"
	serveranalysisui "codeberg.org/pawal/gonemaster/server/analysisui"
	"codeberg.org/pawal/gonemaster/server/extdata"
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
	lookup                   lookupResolvers
	engineLimiter            *engineLimiter
	cancelMu                 sync.Mutex
	cancels                  map[string]context.CancelFunc
	rateLimiter              *RateLimiter
	trustedProxies           []netip.Prefix
	hotCache                 *nameserverHotCache
	delegationLookup         func(context.Context, string) DelegationInfo
	configSources            map[string]SettingSource
	retentionDays            atomic.Int64
	purgeIntervalSec         atomic.Int64
	// cohortRebuildsInFlight guards against overlapping rebuilds of the
	// same cohort. Async-dispatched rebuilds record their cohort ID here;
	// a second request for the same cohort is refused until the first
	// clears the set.
	cohortRebuildsMu       sync.Mutex
	cohortRebuildsInFlight map[int64]struct{}
	// snapshotRematerializeInFlight guards against overlapping rematerializes
	// of the same snapshot, keyed by snapshot ID.
	snapshotRematerializeMu       sync.Mutex
	snapshotRematerializeInFlight map[int64]struct{}
	// snapshotSweepsInFlight guards one cohort-wide snapshot sweep per
	// cohort, keyed by cohort ID.
	snapshotSweepsMu       sync.Mutex
	snapshotSweepsInFlight map[int64]struct{}
	// reportCache holds rendered cohort reports per snapshot pair.
	reportCache *analysisReportCache
	adminTokens atomic.Pointer[tokenSet]
	extData     *extdata.Provider
	registry    registryLookup
	refLists    referenceListLookup
	logger      *slog.Logger
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

// Unset or "memory" selects the in-memory store.
func memoryDriver(driver string) bool {
	return driver == "" || driver == "memory"
}

// NewWithOptions builds a server using the configured storage backend.
// A SQL driver opens a store, runs migrations, and recovers jobs from an
// unclean shutdown.
func NewWithOptions(cfg Config) (*Server, error) {
	if cfg.ListenAddr == "" {
		cfg = DefaultConfig()
	}

	var store JobStore
	if memoryDriver(cfg.Database.Driver) {
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
	// --debug implies at least debug level so body capture and verbose lines show.
	level := cfg.LogLevel
	if cfg.Debug {
		level = "debug"
	}
	logger := newLogger(cfg.LogFormat, level, os.Stderr)
	slog.SetDefault(logger)
	s := &Server{
		cfg:                           cfg,
		mux:                           http.NewServeMux(),
		store:                         store,
		queue:                         queue,
		logger:                        logger,
		metrics:                       NewMetricsCollector(cfg),
		metricsCache:                  map[string]metricsCacheEntry{},
		progressWrites:                map[string]progressWriteState{},
		progressWriteMinStep:          defaultProgressWriteMinStep,
		progressWriteMinInterval:      defaultProgressWriteMinInterval,
		cohortRebuildsInFlight:        map[int64]struct{}{},
		snapshotRematerializeInFlight: map[int64]struct{}{},
		snapshotSweepsInFlight:        map[int64]struct{}{},
		reportCache:                   newAnalysisReportCache(),
		engineRunner:                  engine.Run,
		engineLimiter:                 newEngineLimiter(cfg.MaxConcurrentJobs),
		cancels:                       map[string]context.CancelFunc{},
	}
	// Bound as a method value so it reads s.lookup at call time.
	s.delegationLookup = s.lookupDelegation
	if cfg.PublicAPI.RateLimitEnabled {
		s.rateLimiter = NewRateLimiter(cfg.PublicAPI.RateLimitMax, cfg.PublicAPI.RateLimitWindow.Duration)
		s.metrics.SetRateLimitKeysSource(s.rateLimiter.Keys)
	}
	s.trustedProxies = parseTrustedProxies(cfg.TrustedProxyCIDRs)
	if cfg.CrossJobHotCache {
		s.hotCache = newNameserverHotCache(0, cfg.EffectiveCrossJobHotCacheTTL())
	}
	s.retentionDays.Store(int64(cfg.Database.RetentionDays))
	s.purgeIntervalSec.Store(int64(cfg.EffectivePurgeInterval() / time.Second))
	ts, err := newTokenSet(cfg.Auth)
	if err != nil {
		s.logger.Warn("invalid admin_tokens, running in open mode", "err", err)
		ts = &tokenSet{}
	}
	s.adminTokens.Store(ts)
	s.extData = newExternalDataProvider(cfg, logger)
	if s.extData != nil {
		s.registry = s.extData
		s.refLists = s.extData
	}
	s.routes()
	return s
}

// authTokens returns the current admin token set (never nil).
func (s *Server) authTokens() *tokenSet {
	if ts := s.adminTokens.Load(); ts != nil {
		return ts
	}
	return &tokenSet{}
}

// authMode reports "open" or "token" and the configured token count.
func (s *Server) authMode() (string, int) {
	ts := s.authTokens()
	if !ts.enabled {
		return "open", 0
	}
	return "token", len(ts.tokens)
}

// ReloadAuth rebuilds and atomically swaps the admin token set. On error the
// previous set is left in place.
func (s *Server) ReloadAuth(cfg AuthConfig) error {
	ts, err := newTokenSet(cfg)
	if err != nil {
		return err
	}
	s.adminTokens.Store(ts)
	return nil
}

// Handler returns the root HTTP handler. Request-ID and access-log middleware
// are applied per API surface in routes(), not here.
func (s *Server) Handler() http.Handler {
	return s.stripUntrustedForwardedHeaders(
		securityHeadersMiddleware(gzipMiddleware(s.mux)))
}

// Store exposes the configured job store for optional integration layers.
func (s *Server) Store() JobStore {
	return s.store
}

// Logger returns the server's structured logger so the binary can route its
// lifecycle messages through the same format and level.
func (s *Server) Logger() *slog.Logger {
	return s.logger
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

// apiChain wraps an API mount with request ID, access log, panic recovery, and
// request metrics. fallbackRoute labels requests no route matched.
func (s *Server) apiChain(fallbackRoute string, next http.Handler) http.Handler {
	return s.requestIDMiddleware(s.accessLogMiddleware(fallbackRoute,
		s.recoverMiddleware(s.apiMetricsMiddleware(fallbackRoute, next))))
}

// pageChain wraps a human-facing mount with request ID, access log, and panic
// recovery. Static assets are served without a log line.
func (s *Server) pageChain(route string, next http.Handler) http.Handler {
	recovered := s.recoverMiddleware(next)
	logged := s.accessLogMiddleware(route, recovered)
	return s.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStaticAsset(r.URL.Path) {
			recovered.ServeHTTP(w, r)
			return
		}
		logged.ServeHTTP(w, r)
	}))
}

func (s *Server) routes() {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/jobs/batch", s.handleJobsBatch)
	apiMux.HandleFunc("/jobs/purge", s.handleJobsPurge)
	apiMux.HandleFunc("/jobs/", s.handleJobByID)
	apiMux.HandleFunc("/jobs", s.handleJobs)
	apiMux.HandleFunc("GET /batches", s.handleListBatches)
	apiMux.HandleFunc("GET /batches/{id}/delete-preview", s.handleBatchDeletePreview)
	apiMux.HandleFunc("GET /batches/{id}/tag-values", s.handleBatchTagValues)
	apiMux.HandleFunc("DELETE /batches/{id}", s.handleDeleteBatch)
	apiMux.HandleFunc("PATCH /batches/{id}", s.handlePatchBatch)
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
	apiMux.HandleFunc("GET /runs/{id}/dnssec-chain", s.handleGetRunDNSSECChain)
	apiMux.HandleFunc("GET /runs/{id}", s.handleGetRun)
	apiMux.HandleFunc("GET /runs", s.handleListRuns)

	apiMux.HandleFunc("GET /spec/testcases/{id}", s.handleSpecTestcase)
	apiMux.HandleFunc("GET /spec/testcases", s.handleSpecTestcases)

	apiMux.HandleFunc("GET /tags/{name}/summary", s.handleTagSummary)
	apiMux.HandleFunc("POST /tags/{name}/purge", s.handleTagPurge)
	apiMux.HandleFunc("GET /tags/{name}/batches", s.handleTagBatches)
	apiMux.HandleFunc("/tags/{name}/profile", s.handleTagProfile)
	apiMux.HandleFunc("/tags/{name}/domains", s.handleTagDomains)
	apiMux.HandleFunc("/tags/{name}", s.handleTagByName)
	apiMux.HandleFunc("/tags", s.handleTags)
	apiMux.HandleFunc("GET /profiles/default", s.handleDefaultProfile)
	apiMux.HandleFunc("GET /profiles/defaults", s.handleProfileDefaults)
	apiMux.HandleFunc("GET /profiles/compatibility", s.handleProfilesCompatibility)
	apiMux.HandleFunc("POST /profiles/mark-all-reviewed", s.handleMarkAllProfilesReviewed)
	apiMux.HandleFunc("POST /profiles/diff", s.handleProfileDiff)
	apiMux.HandleFunc("GET /profiles/{id}/compatibility", s.handleProfileCompatibility)
	apiMux.HandleFunc("PATCH /profiles/{id}", s.handlePatchProfile)
	apiMux.HandleFunc("/profiles/{id}", s.handleProfileByID)
	apiMux.HandleFunc("/profiles", s.handleProfiles)

	apiMux.HandleFunc("POST /analysis/cohorts/{id}/rebuild", s.handleAnalysisCohortRebuild)
	apiMux.HandleFunc("POST /analysis/cohorts/{id}/clear", s.handleAnalysisCohortClear)
	apiMux.HandleFunc("POST /analysis/cohorts/{id}/snapshots/rematerialize", s.handleAnalysisCohortSnapshotsRematerialize)
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
	apiMux.HandleFunc("GET /whoami", s.handleWhoami)
	apiMux.HandleFunc("/session", s.handleSession)

	s.mux.Handle("/api/v1/", s.apiChain("/api/v1/unknown",
		s.authMiddleware(http.StripPrefix("/api/v1", captureRoute("/api/v1", apiMux)))))
	// One fixed path with no router to match, so it labels itself.
	s.mux.Handle("/api/v1", s.apiChain("/api/v1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/", http.StatusMovedPermanently)
	})))

	pubMux := http.NewServeMux()
	pubMux.HandleFunc("POST /jobs", s.handlePublicCreateJob)
	pubMux.HandleFunc("GET /profiles", s.handlePublicProfiles)
	pubMux.HandleFunc("GET /jobs/{publicID}/result", s.handlePublicGetResult)
	pubMux.HandleFunc("GET /jobs/{publicID}/dnssec-chain", s.handlePublicGetDNSSECChain)
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
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/report", s.handlePublicAnalysisReport)
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}/history", s.handlePublicAnalysisEntityHistory)
	pubMux.HandleFunc("GET /analysis/cohorts/{dataset_tag}", s.handlePublicAnalysisCohortDetail)
	var pubHandler http.Handler = http.StripPrefix("/pub/api/v1", captureRoute("/pub/api/v1", pubMux))
	if d := s.cfg.PublicAPI.AnalysisRequestTimeout.Duration; d > 0 {
		pubHandler = analysisTimeoutMiddleware(d, pubHandler)
	}
	if s.rateLimiter != nil {
		pubHandler = rateLimitMiddleware(s.rateLimiter, s.trustedProxies, pubHandler)
	}
	// Outside the rate limiter, so a 429 is counted rather than invisible.
	pubHandler = s.apiChain("/pub/api/v1/unknown", pubHandler)
	s.mux.Handle("/pub/api/v1/", pubHandler)

	s.mux.Handle("GET /robots.txt", s.pageChain("/robots.txt", http.HandlerFunc(s.handleRobotsTxt)))
	s.mux.Handle("GET /sitemap.xml", s.pageChain("/sitemap.xml", http.HandlerFunc(s.handleSitemap)))
	s.mux.Handle("/public/", s.pageChain("/public/",
		http.StripPrefix("/public", serverpublic.Handler(s.cfg.PublicURL, s.cfg.PublicUIPath, s.publicResultLookup()))))
	s.mux.Handle("/analysis/", s.pageChain("/analysis/", legacyTagRedirect(
		http.StripPrefix("/analysis", serveranalysisui.Handler(s.cfg.PublicURL)),
	)))
	s.mux.Handle("/analysis", s.pageChain("/analysis", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/analysis/", http.StatusMovedPermanently)
	})))
	s.mux.Handle("/", s.pageChain("/", serverui.Handler()))
}
