package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// analysisAPITestFixture bundles a server backed by a real SQL store plus
// helpers for seeding analysis data.
type analysisAPITestFixture struct {
	t      *testing.T
	srv    *Server
	store  *SQLJobStore
	cohort AnalysisCohort
}

func newAnalysisAPITestFixture(t *testing.T) *analysisAPITestFixture {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	configurePool(db, "sqlite")
	t.Cleanup(func() { _ = db.Close() })

	dialect := sqliteDialect{}
	if err := runMigrations(db, dialect); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	store := NewSQLJobStore(db, dialect)
	srv := newServer(DefaultConfig(), store, NewInMemoryQueue())
	cohort, err := store.UpsertAnalysisCohort(AnalysisCohort{
		SourceType:      "tag",
		SourceTag:       "tld",
		Label:           "TLD",
		AnalysisEnabled: true,
		PublicEnabled:   true,
		IsDefault:       true,
	})
	if err != nil {
		t.Fatalf("upsert cohort: %v", err)
	}
	return &analysisAPITestFixture{t: t, srv: srv, store: store, cohort: cohort}
}

// seedDomainSummary writes one analysis_run_domain_summary row and ensures
// the backing domain and run rows exist.
func (f *analysisAPITestFixture) seedDomainSummary(domainName, runID string, finishedAt time.Time, score int, grade, worstLevel string) {
	f.t.Helper()
	domain, err := f.store.GetOrCreateDomain(domainName)
	if err != nil {
		f.t.Fatalf("create domain %q: %v", domainName, err)
	}
	insertTestRun(f.t, f.store, Run{
		ID: runID, DomainID: domain.ID, Domain: domainName,
		Status: JobSucceeded, CreatedAt: finishedAt.Add(-time.Minute),
		StartedAt: finishedAt.Add(-time.Minute), FinishedAt: finishedAt,
		WorstLevel: worstLevel,
	})
	grd := grade
	scr := score
	if err := f.store.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
		CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
		Score: &scr, Grade: &grd, WorstLevel: worstLevel,
		NameserverCount: 2, EndpointCount: 3, ASNCount: 2, PrefixCount: 2,
	}); err != nil {
		f.t.Fatalf("upsert summary: %v", err)
	}
}

// insertTestRun directly inserts a minimal runs row for tests. The analysis
// query methods join on runs.finished_at, so a run record must exist.
func insertTestRun(t *testing.T, store *SQLJobStore, run Run) {
	t.Helper()
	if run.PublicID == "" {
		run.PublicID = GeneratePublicID()
	}
	_, err := store.db.Exec(
		fmt.Sprintf(`INSERT INTO runs (
			id, domain_id, domain, batch_id, status, created_at, started_at, finished_at,
			duration_ms, sev_notice, sev_warning, sev_error, sev_critical, worst_level,
			entry_count, profile, config_json, public_id, priority, profile_name, effective_profile
		) VALUES (%s)`, store.phRange(1, 21)),
		run.ID, run.DomainID, run.Domain, run.BatchID, string(run.Status),
		store.ts(run.CreatedAt), store.ts(run.StartedAt), store.ts(run.FinishedAt),
		run.DurationMs, run.SevNotice, run.SevWarning, run.SevError, run.SevCritical,
		run.WorstLevel, run.EntryCount, run.Profile, "", run.PublicID, run.Priority,
		run.ProfileName, run.EffectiveProfile,
	)
	if err != nil {
		t.Fatalf("insert test run %q: %v", run.ID, err)
	}
}

func decodeDomainList(t *testing.T, body *httptest.ResponseRecorder) PublicAnalysisListResponse[PublicAnalysisDomainView] {
	t.Helper()
	var got PublicAnalysisListResponse[PublicAnalysisDomainView]
	if err := json.NewDecoder(body.Body).Decode(&got); err != nil {
		t.Fatalf("decode domains list: %v", err)
	}
	return got
}

func TestPublicAnalysisDomainsEmpty(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	got := decodeDomainList(t, resp)
	if got.Total != 0 || len(got.Items) != 0 {
		t.Fatalf("expected empty list, got %+v", got)
	}
	if got.Limit != defaultAnalysisListLimit {
		t.Fatalf("expected default limit, got %d", got.Limit)
	}
}

func TestPublicAnalysisDomainsReturnsLatestPerDomain(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	t1 := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedDomainSummary("alpha.example", "run-alpha-1", t1, 60, "C", "ERROR")
	f.seedDomainSummary("alpha.example", "run-alpha-2", t2, 95, "A", "NOTICE")
	f.seedDomainSummary("beta.example", "run-beta-1", t1, 80, "B", "WARNING")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	got := decodeDomainList(t, resp)
	if got.Total != 2 || len(got.Items) != 2 {
		t.Fatalf("expected 2 items (latest per domain), got total=%d len=%d", got.Total, len(got.Items))
	}
	byDomain := map[string]PublicAnalysisDomainView{}
	for _, v := range got.Items {
		byDomain[v.Domain] = v
	}
	if alpha := byDomain["alpha.example"]; alpha.Score == nil || *alpha.Score != 95 || alpha.Grade == nil || *alpha.Grade != "A" {
		t.Fatalf("expected latest alpha score=95 grade=A, got %+v", alpha)
	}
	if beta := byDomain["beta.example"]; beta.WorstLevel != "WARNING" {
		t.Fatalf("expected beta worst_level=WARNING, got %+v", beta)
	}
}

func TestPublicAnalysisDomainsFilterByWorstLevel(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	// Two ERROR domains, one WARNING, one NOTICE, one with an empty
	// worst_level (collapses into the OK bucket).
	f.seedDomainSummary("err1.example", "run-err1", ts, 10, "F", "ERROR")
	f.seedDomainSummary("err2.example", "run-err2", ts, 20, "E", "ERROR")
	f.seedDomainSummary("warn.example", "run-warn", ts, 60, "C", "WARNING")
	f.seedDomainSummary("notice.example", "run-notice", ts, 80, "B", "NOTICE")
	f.seedDomainSummary("ok.example", "run-ok", ts, 95, "A", "")

	cases := []struct {
		bucket  string
		domains []string
	}{
		{"ERROR", []string{"err1.example", "err2.example"}},
		{"WARNING", []string{"warn.example"}},
		{"NOTICE", []string{"notice.example"}},
		{"OK", []string{"ok.example"}},
		{"CRITICAL", nil},
	}
	for _, c := range cases {
		t.Run(c.bucket, func(t *testing.T) {
			resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains?worst_level="+c.bucket)
			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
			}
			got := decodeDomainList(t, resp)
			if got.Total != len(c.domains) {
				t.Fatalf("bucket %q: expected %d, got %d (items=%+v)",
					c.bucket, len(c.domains), got.Total, got.Items)
			}
			seen := map[string]struct{}{}
			for _, v := range got.Items {
				seen[v.Domain] = struct{}{}
			}
			for _, want := range c.domains {
				if _, ok := seen[want]; !ok {
					t.Fatalf("bucket %q: expected %q in result, got %+v", c.bucket, want, got.Items)
				}
			}
		})
	}

	// Lowercase input is accepted and normalized to the same bucket.
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains?worst_level=error")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 on lowercase filter, got %d: %s", resp.Code, resp.Body)
	}
	if got := decodeDomainList(t, resp); got.Total != 2 {
		t.Fatalf("expected lowercase filter to match 2 ERROR domains, got %d", got.Total)
	}
}

func TestPublicAnalysisDomainsRejectsInvalidWorstLevel(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains?worst_level=bogus")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body.String(), "invalid_worst_level") {
		t.Fatalf("expected invalid_worst_level error code, got %s", resp.Body)
	}
}

func TestPublicAnalysisDomainsSearchAndPagination(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	finishedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		f.seedDomainSummary(fmt.Sprintf("alpha-%d.example", i),
			fmt.Sprintf("run-a-%d", i), finishedAt, 90-i, "A", "NOTICE")
		f.seedDomainSummary(fmt.Sprintf("beta-%d.example", i),
			fmt.Sprintf("run-b-%d", i), finishedAt, 50+i, "C", "ERROR")
	}

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains?search=alpha")
	got := decodeDomainList(t, resp)
	if got.Total != 5 {
		t.Fatalf("expected 5 alpha matches, got %d", got.Total)
	}
	for _, v := range got.Items {
		if !strings.Contains(v.Domain, "alpha") {
			t.Fatalf("search leaked non-matching domain: %+v", v)
		}
	}

	resp = getPublic(t, f.srv, "/pub/api/v1/analysis/domains?limit=3&offset=2")
	got = decodeDomainList(t, resp)
	if got.Total != 10 {
		t.Fatalf("expected total=10, got %d", got.Total)
	}
	if len(got.Items) != 3 {
		t.Fatalf("expected 3 items on page, got %d", len(got.Items))
	}
	if got.Offset != 2 || got.Limit != 3 {
		t.Fatalf("unexpected pagination: %+v", got)
	}
}

func TestPublicAnalysisDomainsRejectsInvalidLimit(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains?limit=99999")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestPublicAnalysisDomainsSortScoreDesc(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	finishedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedDomainSummary("low.example", "run-low", finishedAt, 30, "E", "CRITICAL")
	f.seedDomainSummary("high.example", "run-high", finishedAt, 95, "A", "NOTICE")
	f.seedDomainSummary("mid.example", "run-mid", finishedAt, 70, "B", "WARNING")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains?sort=score_desc")
	got := decodeDomainList(t, resp)
	if len(got.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got.Items))
	}
	if got.Items[0].Domain != "high.example" || got.Items[2].Domain != "low.example" {
		t.Fatalf("unexpected score_desc order: %+v", got.Items)
	}
}

func TestPublicAnalysisDomainsRedactsInternalIDs(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	finishedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedDomainSummary("alpha.example", "run-alpha", finishedAt, 95, "A", "NOTICE")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains")
	raw := resp.Body.String()
	for _, needle := range []string{
		`"domain_id"`, `"run_id"`, `"cohort_id"`, `"public_id"`,
	} {
		if strings.Contains(raw, needle) {
			t.Fatalf("domain list should not contain %s: %s", needle, raw)
		}
	}
}

func TestPublicAnalysisDomainsFailsWhenNoCohort(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	configurePool(db, "sqlite")
	t.Cleanup(func() { _ = db.Close() })
	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	store := NewSQLJobStore(db, sqliteDialect{})
	srv := newServer(DefaultConfig(), store, NewInMemoryQueue())

	resp := getPublic(t, srv, "/pub/api/v1/analysis/domains")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no cohorts, got %d: %s", resp.Code, resp.Body)
	}
}

func TestPublicAnalysisDomainsFailsWhenStoreNotReadable(t *testing.T) {
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true, IsDefault: true,
	})

	resp := getPublic(t, srv, "/pub/api/v1/analysis/domains")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when store lacks analysis read surface, got %d: %s", resp.Code, resp.Body)
	}
}
