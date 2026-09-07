package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// analysisFixture bundles a server backed by a real SQL store plus
// helpers for seeding analysis data.
type analysisFixture struct {
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

// analysisFixtureOpt customizes newAnalysisFixture.
type analysisFixtureOpt func(*analysisFixtureSetup)

type analysisFixtureSetup struct {
	backend      testBackend
	batchID      string
	isDefault    bool
	runWindow    bool
	seededRun    string
	skipSnapshot bool
}

// onBackend picks the database the fixture runs against. Without it the
// fixture uses SQLite, which is what a single-backend test wants.
func onBackend(b testBackend) analysisFixtureOpt {
	return func(s *analysisFixtureSetup) { s.backend = b }
}

// withFixtureBatch names the fixture's batch.
func withFixtureBatch(id string) analysisFixtureOpt {
	return func(s *analysisFixtureSetup) { s.batchID = id }
}

// asDefaultCohort marks the cohort default, which the public read path needs
// when the URL names no cohort.
func asDefaultCohort() analysisFixtureOpt {
	return func(s *analysisFixtureSetup) { s.isDefault = true }
}

// withSnapshotRunWindow stamps the snapshot's first and last run times.
func withSnapshotRunWindow() analysisFixtureOpt {
	return func(s *analysisFixtureSetup) { s.runWindow = true }
}

// withSeededRun inserts one run under the fixture batch, which rematerialize
// needs a source row for.
func withSeededRun(domain string) analysisFixtureOpt {
	return func(s *analysisFixtureSetup) { s.seededRun = domain }
}

// withoutSnapshot leaves the cohort with no batch and no snapshot, for the
// tests that exercise the no_snapshot branch.
func withoutSnapshot() analysisFixtureOpt {
	return func(s *analysisFixtureSetup) { s.skipSnapshot = true }
}

// newAnalysisFixture builds a SQLite-backed server with one analysis cohort and,
// unless withoutSnapshot is given, one captured public snapshot over one batch.
func newAnalysisFixture(t *testing.T, opts ...analysisFixtureOpt) *analysisFixture {
	t.Helper()
	setup := &analysisFixtureSetup{
		batchID: testAnalysisFixtureBatchID,
		backend: testBackends(t)[0], // SQLite, always present
	}
	for _, opt := range opts {
		opt(setup)
	}

	store := testStoreForBackend(t, setup.backend)
	srv := newSQLTestServer(t, store)
	cohort, err := store.UpsertAnalysisCohort(AnalysisCohort{
		SourceType:      "tag",
		SourceTag:       "tld",
		Label:           "TLD",
		AnalysisEnabled: true,
		PublicEnabled:   true,
		IsDefault:       setup.isDefault,
	})
	if err != nil {
		t.Fatalf("upsert cohort: %v", err)
	}
	fixture := &analysisFixture{t: t, srv: srv, store: store, cohort: cohort, batchID: setup.batchID}
	if setup.skipSnapshot {
		return fixture
	}

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	if err := store.CreateBatch(Batch{
		ID:             setup.batchID,
		Tag:            "tld",
		CreatedAt:      now,
		SnapshotIntent: true,
	}); err != nil {
		t.Fatalf("create fixture batch: %v", err)
	}
	if setup.seededRun != "" {
		insertTestRun(t, store, Run{
			ID:         "run-admin-1",
			DomainID:   1,
			Domain:     setup.seededRun,
			BatchID:    setup.batchID,
			Status:     JobSucceeded,
			CreatedAt:  now,
			StartedAt:  now,
			FinishedAt: now,
		})
	}
	snap := AnalysisCohortSnapshot{
		CohortID:   cohort.ID,
		BatchID:    setup.batchID,
		Slug:       "2026-04-20-fixture",
		Label:      "Fixture",
		CapturedAt: now,
		Status:     AnalysisSnapshotStatusCaptured,
		IsPublic:   true,
	}
	if setup.runWindow {
		snap.FirstRunAt = now
		snap.LastRunAt = now
	}
	stored, err := store.UpsertAnalysisCohortSnapshot(snap)
	if err != nil {
		t.Fatalf("upsert fixture snapshot: %v", err)
	}
	fixture.snapshot = stored
	return fixture
}

// forEachAnalysisFixture runs fn as a subtest against every configured backend,
// each with its own fixture. This is what takes the public analysis read path
// onto PostgreSQL and MariaDB when TEST_*_DSN are set; without them it is one
// SQLite subtest, so the default test run pays nothing.
func forEachAnalysisFixture(t *testing.T, fn func(t *testing.T, f *analysisFixture), opts ...analysisFixtureOpt) {
	t.Helper()
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			fn(t, newAnalysisFixture(t, append([]analysisFixtureOpt{onBackend(b)}, opts...)...))
		})
	}
}

// forEachAnalysisAPIFixture is forEachAnalysisFixture with the public API
// fixture's options, matching newAnalysisAPITestFixture.
func forEachAnalysisAPIFixture(t *testing.T, fn func(t *testing.T, f *analysisFixture)) {
	t.Helper()
	forEachAnalysisFixture(t, fn, asDefaultCohort(), withSnapshotRunWindow())
}

// forEachAdminSnapshotFixture is forEachAnalysisFixture with the admin
// fixture's options, matching newAdminSnapshotFixture.
func forEachAdminSnapshotFixture(t *testing.T, fn func(t *testing.T, f *analysisFixture)) {
	t.Helper()
	forEachAnalysisFixture(t, fn, withFixtureBatch("batch-admin"), withSeededRun("example.test"))
}

func newAnalysisAPITestFixture(t *testing.T) *analysisFixture {
	t.Helper()
	return newAnalysisFixture(t, asDefaultCohort(), withSnapshotRunWindow())
}

// publicURL builds a path-segmented public read URL anchored to the
// fixture's auto-latest snapshot. sub is the analysis sub-path (e.g.
// "domains", "nameservers/ns1.example", or "prefix?prefix=...").
func (f *analysisFixture) publicURL(sub string) string {
	return f.publicURLForSnapshot(f.snapshot.Slug, sub)
}

// publicURLForSnapshot is publicURL with an explicit snapshot slug.
func (f *analysisFixture) publicURLForSnapshot(slug, sub string) string {
	return "/pub/api/v1/analysis/cohorts/" + f.cohort.SourceTag +
		"/snapshots/" + slug + "/" + sub
}

// seedAlternateSnapshot creates a second snapshot-intent batch plus a
// captured public snapshot with a capturedAt older than the fixture
// default, so the fixture snapshot stays auto-latest. Tests use it to
// scope old vs. new runs into separate snapshots and verify that
// ?snapshot= and auto-latest both pin to a specific materialization.
func (f *analysisFixture) seedAlternateSnapshot(batchID, slug string, capturedAt time.Time) AnalysisCohortSnapshot {
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
func (f *analysisFixture) refreshSnapshotViews(batchID string) {
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
	views, err := f.store.ComputeSnapshotEntityViews(f.cohort.ID, batchID, "")
	if err != nil {
		f.t.Fatalf("compute entity views for batch %q: %v", batchID, err)
	}
	if err := f.store.ReplaceSnapshotEntityViews(snap.ID, views); err != nil {
		f.t.Fatalf("replace entity views for batch %q: %v", batchID, err)
	}
}

// seedDomainSummary writes one analysis_run_domain_summary row and ensures
// the backing domain and run rows exist.
func (f *analysisFixture) seedDomainSummary(domainName, runID string, finishedAt time.Time, score int, grade, worstLevel string) {
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
	severityKey := worstLevel
	if severityKey == "" {
		severityKey = "OK"
	}
	if err := f.store.ReplaceAnalysisRunDomainFacts(f.cohort.ID, runID, []AnalysisRunDomainFact{
		{CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID, Category: FactCategorySeverity, Key: severityKey},
		{CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID, Category: FactCategoryGrade, Key: grade},
	}); err != nil {
		f.t.Fatalf("replace domain facts: %v", err)
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
	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisDomainView]](t, body, http.StatusOK)
	return got
}

func TestPublicAnalysisDomainsEmpty(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		resp := getPublic(t, f.srv, f.publicURL("domains"))
		wantStatus(t, resp, http.StatusOK)
		got := decodeDomainList(t, resp)
		if got.Total != 0 || len(got.Items) != 0 {
			t.Fatalf("expected empty list, got %+v", got)
		}
		if got.Limit != defaultAnalysisListLimit {
			t.Fatalf("expected default limit, got %d", got.Limit)
		}
	})
}

func TestPublicAnalysisDomainsReturnsLatestPerDomain(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		t1 := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		f.seedDomainSummary("alpha.example", "run-alpha-1", t1, 60, "C", "ERROR")
		f.seedDomainSummary("alpha.example", "run-alpha-2", t2, 95, "A", "NOTICE")
		f.seedDomainSummary("beta.example", "run-beta-1", t1, 80, "B", "WARNING")

		resp := getPublic(t, f.srv, f.publicURL("domains"))
		wantStatus(t, resp, http.StatusOK)
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
	})
}

func TestPublicAnalysisDomainsFilterByWorstLevel(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
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
				wantStatus(t, resp, http.StatusOK)
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
		wantStatus(t, resp, http.StatusOK)
		if got := decodeDomainList(t, resp); got.Total != 2 {
			t.Fatalf("expected lowercase filter to match 2 ERROR domains, got %d", got.Total)
		}
	})
}

func TestPublicAnalysisDomainsFilterByGrade(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		f.seedDomainSummary("a1.example", "run-a1", ts, 95, "A", "NOTICE")
		f.seedDomainSummary("a2.example", "run-a2", ts, 93, "A", "NOTICE")
		f.seedDomainSummary("b1.example", "run-b1", ts, 80, "B", "NOTICE")
		f.seedDomainSummary("f1.example", "run-f1", ts, 10, "F", "CRITICAL")

		resp := getPublic(t, f.srv, f.publicURL("domains?grade=A"))
		wantStatus(t, resp, http.StatusOK)
		got := decodeDomainList(t, resp)
		if got.Total != 2 {
			t.Fatalf("expected 2 A-grade domains, got %d (items=%+v)", got.Total, got.Items)
		}
		for _, v := range got.Items {
			if v.Grade == nil || *v.Grade != "A" {
				t.Fatalf("grade filter leaked non-A: %+v", v)
			}
		}

		// Case-sensitive exact match - grades are stored canonical. "a" must
		// not match "A" because custom scoring profiles may legitimately use
		// distinct labels differing only in case.
		resp = getPublic(t, f.srv, f.publicURL("domains?grade=a"))
		wantStatus(t, resp, http.StatusOK)
		if got := decodeDomainList(t, resp); got.Total != 0 {
			t.Fatalf("lowercase filter should not match uppercase grades, got %d", got.Total)
		}

		// Unknown grade returns empty without 400 - the filter is
		// pluggable-config-friendly, not enum-validated.
		resp = getPublic(t, f.srv, f.publicURL("domains?grade=Z"))
		wantStatus(t, resp, http.StatusOK)
		if got := decodeDomainList(t, resp); got.Total != 0 {
			t.Fatalf("unknown grade should match zero domains, got %d", got.Total)
		}
	})
}

func TestPublicAnalysisDomainsRejectsInvalidWorstLevel(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		resp := getPublic(t, f.srv, f.publicURL("domains?worst_level=bogus"))
		wantStatus(t, resp, http.StatusBadRequest)
		if !strings.Contains(resp.Body.String(), "invalid_worst_level") {
			t.Fatalf("expected invalid_worst_level error code, got %s", resp.Body)
		}
	})
}

func TestPublicAnalysisDomainsSearchAndPagination(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		finishedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		for i := range 5 {
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
	})
}

func TestPublicAnalysisDomainsRejectsInvalidLimit(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		resp := getPublic(t, f.srv, f.publicURL("domains?limit=99999"))
		wantStatus(t, resp, http.StatusBadRequest)
	})
}

func TestPublicAnalysisDomainsSortScoreDesc(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
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
	})
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
		mode      string
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

// TestSortAnalysisDomainViewsByZoneFactColumns covers the four sort modes
// added with the zone-fact columns. The algorithm mode sorts by weakness
// rank, not by number: 5 (deprecated RSASHA1) must lead 13 (a modern
// curve) ascending even though 13 is the larger number. Unsigned rows carry
// no algorithm and no key count, so they sort last in both directions.
// TestPublicAnalysisDomainsServesZoneFactColumns proves the list handler
// maps the four zone-fact columns out of the view row, including the
// algorithm label and tone that travel with the number.
func TestPublicAnalysisDomainsServesZoneFactColumns(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, nil)
		runID := "run-alpha.example-" + now.Format("20060102150405")
		f.seedEndpoint(runID, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 0, "")
		f.seedEndpoint(runID, "alpha.example", "ns1.example", "2001:db8::1", "ipv6", now, 0, "")
		f.seedEndpoint(runID, "alpha.example", "ns2.example", "192.0.2.2", "ipv4", now, 0, "")

		domain, ok := f.store.GetDomainByName("alpha.example")
		if !ok {
			t.Fatal("seeded domain missing")
		}
		keys := int64(2)
		if err := f.store.ReplaceAnalysisRunDomainFacts(f.cohort.ID, runID, []AnalysisRunDomainFact{{
			CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
			Category: FactCategoryDNSKEYAlgoWeakest, Key: "5", ValueNum: &keys,
		}}); err != nil {
			t.Fatalf("replace domain facts: %v", err)
		}
		f.refreshSnapshotViews(f.batchID)

		resp := getPublic(t, f.srv, f.publicURL("domains"))
		got := decodeDomainList(t, resp)
		if len(got.Items) != 1 {
			t.Fatalf("expected 1 item, got %d: %+v", len(got.Items), got.Items)
		}
		row := got.Items[0]
		if row.IPv4NSCount != 2 || row.IPv6NSCount != 1 {
			t.Errorf("family counts = (%d,%d), want (2,1)", row.IPv4NSCount, row.IPv6NSCount)
		}
		if row.DNSKEYAlgoWeakest == nil || *row.DNSKEYAlgoWeakest != 5 {
			t.Fatalf("weakest algo = %v, want 5", row.DNSKEYAlgoWeakest)
		}
		if row.DNSKEYAlgoWeakestLabel != "RSASHA1" || row.DNSKEYAlgoWeakestTone != "error" {
			t.Errorf("algo display = (%q,%q), want (RSASHA1, error)",
				row.DNSKEYAlgoWeakestLabel, row.DNSKEYAlgoWeakestTone)
		}
		if row.DNSKEYCount == nil || *row.DNSKEYCount != 2 {
			t.Errorf("key count = %v, want 2", row.DNSKEYCount)
		}
	})
}

func TestSortAnalysisDomainViewsByZoneFactColumns(t *testing.T) {
	algo := func(n int) *int { return &n }
	base := []PublicAnalysisDomainView{
		{Domain: "weak.example", IPv4NSCount: 2, IPv6NSCount: 0, DNSKEYAlgoWeakest: algo(5), DNSKEYCount: algo(4)},
		{Domain: "modern.example", IPv4NSCount: 3, IPv6NSCount: 3, DNSKEYAlgoWeakest: algo(13), DNSKEYCount: algo(2)},
		{Domain: "rsa.example", IPv4NSCount: 1, IPv6NSCount: 1, DNSKEYAlgoWeakest: algo(8), DNSKEYCount: algo(6)},
		{Domain: "unsigned.example", IPv4NSCount: 4, IPv6NSCount: 2},
	}
	clone := func() []PublicAnalysisDomainView {
		out := make([]PublicAnalysisDomainView, len(base))
		copy(out, base)
		return out
	}

	cases := []struct {
		mode      string
		wantOrder []string
	}{
		{"ipv4_ns_count_asc", []string{"rsa.example", "weak.example", "modern.example", "unsigned.example"}},
		{"ipv4_ns_count_desc", []string{"unsigned.example", "modern.example", "weak.example", "rsa.example"}},
		{"ipv6_ns_count_asc", []string{"weak.example", "rsa.example", "unsigned.example", "modern.example"}},
		{"ipv6_ns_count_desc", []string{"modern.example", "unsigned.example", "rsa.example", "weak.example"}},
		{"dnskey_algo_weakest_asc", []string{"weak.example", "rsa.example", "modern.example", "unsigned.example"}},
		{"dnskey_algo_weakest_desc", []string{"modern.example", "rsa.example", "weak.example", "unsigned.example"}},
		{"dnskey_count_asc", []string{"modern.example", "weak.example", "rsa.example", "unsigned.example"}},
		{"dnskey_count_desc", []string{"rsa.example", "weak.example", "modern.example", "unsigned.example"}},
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
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
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
	})
}
