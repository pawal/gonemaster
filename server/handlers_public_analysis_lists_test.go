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
	t        *testing.T
	srv      *Server
	store    *SQLJobStore
	cohort   AnalysisCohort
	batchID  string
	snapshot AnalysisCohortSnapshot
}

// testAnalysisFixtureBatchID is the snapshot-intent batch every fixture
// seeds on construction. All graduated runs through the fixture attach to
// this batch so the snapshot-scoped read path returns the seeded data
// under the default auto-latest snapshot.
const testAnalysisFixtureBatchID = "batch-fixture"

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
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	if err := store.CreateBatch(Batch{
		ID:             testAnalysisFixtureBatchID,
		Tag:            "tld",
		CreatedAt:      now,
		SnapshotIntent: true,
	}); err != nil {
		t.Fatalf("create fixture batch: %v", err)
	}
	snap, err := store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
		CohortID:    cohort.ID,
		BatchID:     testAnalysisFixtureBatchID,
		Slug:        "2026-04-20-fixture",
		Label:       "Fixture",
		CapturedAt:  now,
		FirstRunAt:  now,
		LastRunAt:   now,
		Status:      AnalysisSnapshotStatusCaptured,
		IsPublic:    true,
		RunCount:    0,
		DomainCount: 0,
	})
	if err != nil {
		t.Fatalf("upsert fixture snapshot: %v", err)
	}
	return &analysisAPITestFixture{
		t: t, srv: srv, store: store, cohort: cohort,
		batchID:  testAnalysisFixtureBatchID,
		snapshot: snap,
	}
}

// publicURL builds a path-segmented public read URL anchored to the
// fixture's auto-latest snapshot. sub is the analysis sub-path (e.g.
// "domains", "nameservers/ns1.example", or "prefix?prefix=...").
func (f *analysisAPITestFixture) publicURL(sub string) string {
	return f.publicURLForSnapshot(f.snapshot.Slug, sub)
}

// publicURLForSnapshot is publicURL with an explicit snapshot slug.
func (f *analysisAPITestFixture) publicURLForSnapshot(slug, sub string) string {
	return "/pub/api/v1/analysis/cohorts/" + f.cohort.SourceTag +
		"/snapshots/" + slug + "/" + sub
}

// seedAlternateSnapshot creates a second snapshot-intent batch plus a
// captured public snapshot with a capturedAt older than the fixture
// default, so the fixture snapshot stays auto-latest. Tests use it to
// scope old vs. new runs into separate snapshots and verify that
// ?snapshot= and auto-latest both pin to a specific materialization.
func (f *analysisAPITestFixture) seedAlternateSnapshot(batchID, slug string, capturedAt time.Time) AnalysisCohortSnapshot {
	f.t.Helper()
	if err := f.store.CreateBatch(Batch{
		ID:             batchID,
		Tag:            "tld",
		CreatedAt:      capturedAt,
		SnapshotIntent: true,
	}); err != nil {
		f.t.Fatalf("create alternate batch: %v", err)
	}
	snap, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
		CohortID:   f.cohort.ID,
		BatchID:    batchID,
		Slug:       slug,
		Label:      slug,
		CapturedAt: capturedAt,
		FirstRunAt: capturedAt,
		LastRunAt:  capturedAt,
		Status:     AnalysisSnapshotStatusCaptured,
		IsPublic:   true,
	})
	if err != nil {
		f.t.Fatalf("upsert alternate snapshot: %v", err)
	}
	return snap
}

// refreshSnapshotViews mirrors the production capture-time materializer:
// computes and writes both the aggregate blobs and the per-snapshot
// entity views from the seeded fact rows. Seed helpers call this after
// each insert so the view-backed handlers see consistent data.
func (f *analysisAPITestFixture) refreshSnapshotViews(batchID string) {
	f.t.Helper()
	snap, ok := f.store.GetAnalysisCohortSnapshotByBatch(f.cohort.ID, batchID)
	if !ok {
		return
	}
	overview, err := f.store.ComputeSnapshotOverview(f.cohort.ID, batchID)
	if err != nil {
		f.t.Fatalf("compute overview for batch %q: %v", batchID, err)
	}
	if err := f.store.ReplaceSnapshotOverview(snap.ID, overview); err != nil {
		f.t.Fatalf("replace overview for batch %q: %v", batchID, err)
	}
	views, err := f.store.ComputeSnapshotEntityViews(f.cohort.ID, batchID)
	if err != nil {
		f.t.Fatalf("compute entity views for batch %q: %v", batchID, err)
	}
	if err := f.store.ReplaceSnapshotEntityViews(snap.ID, views); err != nil {
		f.t.Fatalf("replace entity views for batch %q: %v", batchID, err)
	}
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
		BatchID: f.batchID,
		Status:  JobSucceeded, CreatedAt: finishedAt.Add(-time.Minute),
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
	f.refreshSnapshotViews(f.batchID)
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
	resp := getPublic(t, f.srv, f.publicURL("domains"))
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

	resp := getPublic(t, f.srv, f.publicURL("domains"))
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
			resp := getPublic(t, f.srv, f.publicURL("domains?worst_level=")+c.bucket)
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
	resp := getPublic(t, f.srv, f.publicURL("domains?worst_level=error"))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 on lowercase filter, got %d: %s", resp.Code, resp.Body)
	}
	if got := decodeDomainList(t, resp); got.Total != 2 {
		t.Fatalf("expected lowercase filter to match 2 ERROR domains, got %d", got.Total)
	}
}

func TestPublicAnalysisDomainsFilterByGrade(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedDomainSummary("a1.example", "run-a1", ts, 95, "A", "NOTICE")
	f.seedDomainSummary("a2.example", "run-a2", ts, 93, "A", "NOTICE")
	f.seedDomainSummary("b1.example", "run-b1", ts, 80, "B", "NOTICE")
	f.seedDomainSummary("f1.example", "run-f1", ts, 10, "F", "CRITICAL")

	resp := getPublic(t, f.srv, f.publicURL("domains?grade=A"))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	got := decodeDomainList(t, resp)
	if got.Total != 2 {
		t.Fatalf("expected 2 A-grade domains, got %d (items=%+v)", got.Total, got.Items)
	}
	for _, v := range got.Items {
		if v.Grade == nil || *v.Grade != "A" {
			t.Fatalf("grade filter leaked non-A: %+v", v)
		}
	}

	// Case-sensitive exact match — grades are stored canonical. "a" must
	// not match "A" because custom scoring profiles may legitimately use
	// distinct labels differing only in case.
	resp = getPublic(t, f.srv, f.publicURL("domains?grade=a"))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	if got := decodeDomainList(t, resp); got.Total != 0 {
		t.Fatalf("lowercase filter should not match uppercase grades, got %d", got.Total)
	}

	// Unknown grade returns empty without 400 — the filter is
	// pluggable-config-friendly, not enum-validated.
	resp = getPublic(t, f.srv, f.publicURL("domains?grade=Z"))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 on unknown grade, got %d: %s", resp.Code, resp.Body)
	}
	if got := decodeDomainList(t, resp); got.Total != 0 {
		t.Fatalf("unknown grade should match zero domains, got %d", got.Total)
	}
}

func TestPublicAnalysisDomainsRejectsInvalidWorstLevel(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	resp := getPublic(t, f.srv, f.publicURL("domains?worst_level=bogus"))
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

	resp := getPublic(t, f.srv, f.publicURL("domains?search=alpha"))
	got := decodeDomainList(t, resp)
	if got.Total != 5 {
		t.Fatalf("expected 5 alpha matches, got %d", got.Total)
	}
	for _, v := range got.Items {
		if !strings.Contains(v.Domain, "alpha") {
			t.Fatalf("search leaked non-matching domain: %+v", v)
		}
	}

	resp = getPublic(t, f.srv, f.publicURL("domains?limit=3&offset=2"))
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
	resp := getPublic(t, f.srv, f.publicURL("domains?limit=99999"))
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

	resp := getPublic(t, f.srv, f.publicURL("domains?sort=score_desc"))
	got := decodeDomainList(t, resp)
	if len(got.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got.Items))
	}
	if got.Items[0].Domain != "high.example" || got.Items[2].Domain != "low.example" {
		t.Fatalf("unexpected score_desc order: %+v", got.Items)
	}
}

func TestSortAnalysisDomainViewsByCountColumns(t *testing.T) {
	base := []PublicAnalysisDomainView{
		{Domain: "a.example", NameserverCount: 2, EndpointCount: 4, ASNCount: 1, PrefixCount: 2},
		{Domain: "b.example", NameserverCount: 6, EndpointCount: 12, ASNCount: 3, PrefixCount: 8},
		{Domain: "c.example", NameserverCount: 4, EndpointCount: 8, ASNCount: 2, PrefixCount: 4},
		// Tie on nameserver_count with b.example so we can assert the
		// domain-name tiebreaker kicks in deterministically.
		{Domain: "d.example", NameserverCount: 6, EndpointCount: 9, ASNCount: 3, PrefixCount: 6},
	}
	clone := func() []PublicAnalysisDomainView {
		out := make([]PublicAnalysisDomainView, len(base))
		copy(out, base)
		return out
	}

	cases := []struct {
		mode     string
		wantOrder []string
	}{
		{"nameserver_count_asc", []string{"a.example", "c.example", "b.example", "d.example"}},
		{"nameserver_count_desc", []string{"b.example", "d.example", "c.example", "a.example"}},
		{"endpoint_count_asc", []string{"a.example", "c.example", "d.example", "b.example"}},
		{"endpoint_count_desc", []string{"b.example", "d.example", "c.example", "a.example"}},
		{"asn_count_asc", []string{"a.example", "c.example", "b.example", "d.example"}},
		{"asn_count_desc", []string{"b.example", "d.example", "c.example", "a.example"}},
		{"prefix_count_asc", []string{"a.example", "c.example", "d.example", "b.example"}},
		{"prefix_count_desc", []string{"b.example", "d.example", "c.example", "a.example"}},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			got := clone()
			sortAnalysisDomainViews(got, c.mode)
			for i, want := range c.wantOrder {
				if got[i].Domain != want {
					t.Fatalf("%s: position %d = %q, want %q (full order=%+v)", c.mode, i, got[i].Domain, want, got)
				}
			}
		})
	}
}

func TestPublicAnalysisDomainsRedactsInternalIDs(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	finishedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedDomainSummary("alpha.example", "run-alpha", finishedAt, 95, "A", "NOTICE")

	resp := getPublic(t, f.srv, f.publicURL("domains"))
	raw := resp.Body.String()
	for _, needle := range []string{
		`"domain_id"`, `"run_id"`, `"cohort_id"`, `"public_id"`,
	} {
		if strings.Contains(raw, needle) {
			t.Fatalf("domain list should not contain %s: %s", needle, raw)
		}
	}
}

