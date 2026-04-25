package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestSingleflightCoalescesConcurrentMaterializations proves that concurrent
// requests for the same snapshot share one compute pass via singleflight,
// rather than each running their own under a global mutex.
func TestSingleflightCoalescesConcurrentMaterializations(t *testing.T) {
	srv := newSingleflightTestServer(t)
	cohort, snap := singleflightSeedSnapshot(t, srv.store)

	var computeCount atomic.Int64
	store := &countingMatStore{
		AnalysisReadStore: srv.store,
		hook: func() {
			computeCount.Add(1)
			time.Sleep(50 * time.Millisecond)
		},
	}
	srv.store = nil
	matSrv := &Server{
		store:            countingJobStoreShim{SQLJobStore: srvStore(t, srv), AnalysisReadStore: store},
		analysisMatCache: map[int64]analysisMatCacheEntry{},
	}

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			matSrv.latestMaterializationForSnapshot(cohort, snap)
		}()
	}
	wg.Wait()

	got := computeCount.Load()
	if got > 2 {
		t.Fatalf("singleflight broken: %d concurrent requests triggered %d computes (want 1, allow up to 2 for racy lookup before group.Do)", concurrency, got)
	}
}

// countingJobStoreShim wraps a JobStore but routes the AnalysisReadStore
// surface through a counting hook. The compute path uses cohortMaterializationLookup
// methods (GetRun, GetDomainNamesByIDs, ListRuns) directly off the store, so
// we must keep all of those passing through to the real store.
type countingJobStoreShim struct {
	*SQLJobStore
	AnalysisReadStore
}

func (s countingJobStoreShim) GetRun(id string) (Run, bool) {
	return s.SQLJobStore.GetRun(id)
}

func (s countingJobStoreShim) GetDomainNamesByIDs(ids []int64) map[int64]string {
	return s.SQLJobStore.GetDomainNamesByIDs(ids)
}

func (s countingJobStoreShim) ListRuns(filter RunFilter) RunList {
	return s.SQLJobStore.ListRuns(filter)
}

// countingMatStore wraps an AnalysisReadStore so the compute hook fires once
// per legacy materialization compute.
type countingMatStore struct {
	AnalysisReadStore
	hook func()
}

func (c *countingMatStore) ListAnalysisRunDomainSummariesByCohort(cohortID int64) []AnalysisRunDomainSummary {
	c.hook()
	return c.AnalysisReadStore.ListAnalysisRunDomainSummariesByCohort(cohortID)
}

// singleflightTestFixture is a shrunken copy of analysisAPITestFixture
// scoped to this file's needs.
type singleflightTestFixture struct {
	store *SQLJobStore
}

func newSingleflightTestServer(t *testing.T) *singleflightTestFixture {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	configurePool(db, "sqlite")
	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return &singleflightTestFixture{store: NewSQLJobStore(db, sqliteDialect{})}
}

func srvStore(t *testing.T, f *singleflightTestFixture) *SQLJobStore {
	t.Helper()
	return f.store
}

func singleflightSeedSnapshot(t *testing.T, store *SQLJobStore) (AnalysisCohort, AnalysisCohortSnapshot) {
	t.Helper()
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	cohort, err := store.UpsertAnalysisCohort(AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD", AnalysisEnabled: true,
	})
	if err != nil {
		t.Fatalf("seed cohort: %v", err)
	}
	if err := store.CreateBatch(Batch{
		ID: "batch-sf", Tag: "tld", CreatedAt: now, SnapshotIntent: true,
	}); err != nil {
		t.Fatalf("create batch: %v", err)
	}
	snap, err := store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
		CohortID: cohort.ID, BatchID: "batch-sf", Slug: "sf",
		Status: AnalysisSnapshotStatusCaptured, IsPublic: true,
		CapturedAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	return cohort, snap
}

// TestAnalysisTimeoutMiddlewareReturnsTimeout verifies that a slow handler
// behind /pub/api/v1/analysis/ is cut off and the client receives 503.
func TestAnalysisTimeoutMiddlewareReturnsTimeout(t *testing.T) {
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			return
		}
	})
	wrapped := analysisTimeoutMiddleware(50*time.Millisecond, slow)

	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/analysis/nameservers", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "request_timeout") {
		t.Errorf("body = %q, want request_timeout", rr.Body.String())
	}
}

// TestAnalysisTimeoutMiddlewareLetsFastRequestsThrough verifies that a
// fast handler is unaffected by the wrapping.
func TestAnalysisTimeoutMiddlewareLetsFastRequestsThrough(t *testing.T) {
	fast := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	wrapped := analysisTimeoutMiddleware(time.Second, fast)

	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/analysis/nameservers", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

// TestAnalysisTimeoutMiddlewareSkipsNonAnalysisPaths verifies that
// non-analysis paths bypass the timeout wrap entirely.
func TestAnalysisTimeoutMiddlewareSkipsNonAnalysisPaths(t *testing.T) {
	hits := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		// Sleep longer than the wrap's deadline; if the wrap applied,
		// this would 503. /jobs is not under /analysis/, so it must not.
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	wrapped := analysisTimeoutMiddleware(20*time.Millisecond, next)

	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/abc", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (timeout must not apply outside /analysis/)", rr.Code)
	}
	if hits != 1 {
		t.Fatalf("next handler hits = %d, want 1", hits)
	}
}

// TestAnalysisTimeoutMiddlewareDisabledOnZeroDuration verifies the
// passthrough path when the operator turns the timeout off.
func TestAnalysisTimeoutMiddlewareDisabledOnZeroDuration(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := analysisTimeoutMiddleware(0, next)
	if wrapped == nil {
		t.Fatal("middleware must not return nil on zero duration")
	}
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/analysis/nameservers", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

// TestConfigurePoolWithOverrides exercises the operator-tunable pool
// numbers: zeros fall back to defaults, non-zeros override.
func TestConfigurePoolWithOverrides(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	configurePoolWith(db, "sqlite", DatabaseConfig{MaxOpenConns: 4})
	stats := db.Stats()
	if stats.MaxOpenConnections != 4 {
		t.Errorf("sqlite override: MaxOpenConnections = %d, want 4", stats.MaxOpenConnections)
	}

	configurePoolWith(db, "sqlite", DatabaseConfig{})
	stats = db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("sqlite default: MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}

	// Postgres-style defaults: 25 / 5 / 5min when unspecified, and
	// overrides when supplied.
	configurePoolWith(db, "postgres", DatabaseConfig{})
	stats = db.Stats()
	if stats.MaxOpenConnections != 25 {
		t.Errorf("postgres default: MaxOpenConnections = %d, want 25", stats.MaxOpenConnections)
	}

	configurePoolWith(db, "postgres", DatabaseConfig{
		MaxOpenConns: 50, MaxIdleConns: 10, ConnMaxLifetimeSeconds: 600,
	})
	stats = db.Stats()
	if stats.MaxOpenConnections != 50 {
		t.Errorf("postgres override: MaxOpenConnections = %d, want 50", stats.MaxOpenConnections)
	}
}
