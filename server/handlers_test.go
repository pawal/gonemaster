package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	serverpublic "codeberg.org/pawal/gonemaster/server/public"
)

var metricsRequestNonce uint32
var prometheusSampleLine = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^{}]*\})? [-+]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][-+]?[0-9]+)?$`)

func getMetricsSnapshot(t *testing.T, srv *Server) MetricsSnapshot {
	t.Helper()
	nonce := atomic.AddUint32(&metricsRequestNonce, 1)
	limit := int((nonce % 100) + 1)
	resp := doJSON(t, srv, http.MethodGet,
		fmt.Sprintf("/api/v1/metrics?limit_domains=%d&limit_batches=%d", limit, limit), nil)
	return mustJSON[MetricsSnapshot](t, resp, http.StatusOK)
}

func findAPIRouteMetrics(t *testing.T, snapshot MetricsSnapshot, method string, route string) MetricsAPIRouteMetrics {
	t.Helper()
	for _, metrics := range snapshot.API.Routes {
		if metrics.Method == method && metrics.Route == route {
			return metrics
		}
	}
	t.Fatalf("route metrics not found for %s %s", method, route)
	return MetricsAPIRouteMetrics{}
}

func assertPrometheusTextWellFormed(t *testing.T, body string) {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(body))
	sampleCount := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# HELP ") || strings.HasPrefix(line, "# TYPE ") {
			continue
		}
		if !prometheusSampleLine.MatchString(line) {
			t.Fatalf("invalid Prometheus sample line: %q", line)
		}
		sampleCount++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan Prometheus output: %v", err)
	}
	if sampleCount == 0 {
		t.Fatal("expected at least one Prometheus sample line")
	}
}

func TestCreateAndGetJob(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`)

	created := mustJSON[Job](t, resp, http.StatusCreated)
	if created.ID == "" {
		t.Fatalf("expected job id")
	}

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/jobs/"+created.ID, nil)
	wantStatus(t, resp, http.StatusOK)
}

func TestGetJobResultOmitsNameserverTimingsWhenAdminDisplayDisabled(t *testing.T) {
	srv := newTestServer(t, withConfig(func(c *Config) { c.ShowNameserverTimingsAdmin = false }))

	now := time.Now().UTC()
	job := Job{
		ID:         newID("job"),
		Domain:     "example.com",
		Status:     JobSucceeded,
		CreatedAt:  now,
		StartedAt:  now,
		FinishedAt: now,
		NameserverTimings: []NameserverTiming{
			{Nameserver: "ns1.example.com", Address: "192.0.2.10", AvgMS: 24, MinMS: 20, MaxMS: 30, Count: 3},
		},
	}
	createAndGraduate(t, srv.store, job, nil)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs/"+job.ID+"/result", nil)
	wantStatus(t, resp, http.StatusOK)

	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if result.NameserverTimings != nil {
		t.Fatal("expected NameserverTimings to be nil when ShowNameserverTimingsAdmin=false")
	}
}

func TestCreateJobCSRFOriginMatrix(t *testing.T) {
	csrfOriginMatrix(t, http.StatusCreated, func(t *testing.T, opts ...reqOpt) *httptest.ResponseRecorder {
		return doJSON(t, newTestServer(t), http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`, opts...)
	})
}

func TestCreateJobMinLevel(t *testing.T) {
	srv := newTestServer(t)

	payload := `{"domain":"example.com","min_level":"WARNING"}`
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", payload)

	created := mustJSON[Job](t, resp, http.StatusCreated)
	stored, ok := srv.store.Get(created.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
	if stored.MinLevel != "WARNING" {
		t.Fatalf("expected min_level WARNING, got %q", stored.MinLevel)
	}
}

func TestCreateJobValidation(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":""}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestCreateJobWithUndelegatedInput(t *testing.T) {
	srv := newTestServer(t)

	payload := `{
		"domain":"Example.COM",
		"nameservers":[
			{"ns":"NS1.Example.COM","ip":"192.0.2.1"},
			{"ns":"ns1.example.com","ip":"2001:db8::1"},
			{"ns":"ns2.example.net"}
		],
		"ds_info":[
			{"keytag":12345,"algorithm":13,"digtype":2,"digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
		]
	}`
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", payload)

	created := mustJSON[Job](t, resp, http.StatusCreated)
	stored, ok := srv.store.Get(created.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
	if stored.Domain != "example.com" {
		t.Fatalf("expected normalized domain example.com, got %q", stored.Domain)
	}
	if len(stored.UndelegatedNS) != 3 {
		t.Fatalf("expected 3 undelegated nameserver rows, got %d", len(stored.UndelegatedNS))
	}
	if stored.UndelegatedNS[0].Name != "ns1.example.com" || stored.UndelegatedNS[0].IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver row: %+v", stored.UndelegatedNS[0])
	}
	if stored.UndelegatedNS[1].Name != "ns1.example.com" || stored.UndelegatedNS[1].IP != "2001:db8::1" {
		t.Fatalf("unexpected second nameserver row: %+v", stored.UndelegatedNS[1])
	}
	if stored.UndelegatedNS[2].Name != "ns2.example.net" || stored.UndelegatedNS[2].IP != "" {
		t.Fatalf("unexpected third nameserver row: %+v", stored.UndelegatedNS[2])
	}
	if len(stored.UndelegatedDS) != 1 {
		t.Fatalf("expected one undelegated DS row, got %d", len(stored.UndelegatedDS))
	}
	if stored.UndelegatedDS[0].Digest != "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" {
		t.Fatalf("expected uppercase DS digest, got %q", stored.UndelegatedDS[0].Digest)
	}
}

func TestCreateJobRejectsMalformedUndelegatedInput(t *testing.T) {
	srv := newTestServer(t)

	payload := `{
		"domain":"example.com",
		"nameservers":[{"ns":"ns1.example.com","ip":"not-an-ip"}]
	}`
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", payload)
	wantErrorCode(t, resp, http.StatusBadRequest, "invalid_undelegated")
}

func TestCreateJobRejectsMalformedUndelegatedDSInput(t *testing.T) {
	srv := newTestServer(t)

	payload := `{
		"domain":"example.com",
		"ds_info":[{"keytag":12345,"algorithm":13,"digtype":2,"digest":"not-hex"}]
	}`
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", payload)
	wantErrorCode(t, resp, http.StatusBadRequest, "invalid_undelegated")
}

func TestListJobs(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`)
	wantStatus(t, resp, http.StatusCreated)

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/jobs", nil)
	list := mustJSON[JobList](t, resp, http.StatusOK)
	if list.Total != 1 {
		t.Fatalf("expected total 1, got %d", list.Total)
	}
}

// A severity filter is applied server-side to graduated runs (by their sev_*
// totals) and excludes in-flight jobs, which have no results yet.
func TestListJobsFiltersBySeverity(t *testing.T) {
	srv := newTestServer(t)
	now := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	// One in-flight (queued) job.
	if _, err := srv.store.Create(Job{ID: "job_queued", Domain: "queued.example", Status: JobQueued, CreatedAt: now}); err != nil {
		t.Fatalf("create queued: %v", err)
	}
	// Three graduated runs: clean (NOTICE only), a WARNING, and an ERROR.
	graduate := func(id, domain, level string) {
		job := Job{ID: id, Domain: domain, Status: JobSucceeded, CreatedAt: now, StartedAt: now, FinishedAt: now}
		createAndGraduate(t, srv.store, job, []engine.LogEntry{{Level: level}})
	}
	graduate("job_clean", "clean.example", "NOTICE")
	graduate("job_warn", "warn.example", "WARNING")
	graduate("job_err", "err.example", "ERROR")

	domainsFor := func(query string) map[string]bool {
		resp := doJSON(t, srv, http.MethodGet, query, nil)
		l := mustJSON[JobList](t, resp, http.StatusOK)
		out := map[string]bool{}
		for _, it := range l.Items {
			out[it.Domain] = true
		}
		return out
	}

	wp := domainsFor("/api/v1/jobs?severity=warnings_plus")
	if !wp["warn.example"] || !wp["err.example"] {
		t.Fatalf("warnings_plus should include warn and err runs: %v", wp)
	}
	if wp["clean.example"] || wp["queued.example"] {
		t.Fatalf("warnings_plus should exclude clean run and in-flight job: %v", wp)
	}

	eo := domainsFor("/api/v1/jobs?severity=errors_only")
	if !eo["err.example"] {
		t.Fatalf("errors_only should include the err run: %v", eo)
	}
	if eo["warn.example"] || eo["clean.example"] || eo["queued.example"] {
		t.Fatalf("errors_only should exclude warn/clean/in-flight: %v", eo)
	}

	all := domainsFor("/api/v1/jobs")
	for _, d := range []string{"queued.example", "clean.example", "warn.example", "err.example"} {
		if !all[d] {
			t.Fatalf("unfiltered list should include %s: %v", d, all)
		}
	}
}

func TestListJobsPaginationAndSort(t *testing.T) {
	srv := newTestServer(t)
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", Domain: "gamma.example", Status: JobQueued, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", Domain: "alpha.example", Status: JobQueued, CreatedAt: base.Add(time.Second)})
	_, _ = srv.store.Create(Job{ID: "job3", Domain: "beta.example", Status: JobQueued, CreatedAt: base.Add(2 * time.Second)})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs?limit=2&sort=domain_asc&page=1", nil)
	first := mustJSON[JobList](t, resp, http.StatusOK)
	if first.Total != 3 || len(first.Items) != 2 {
		t.Fatalf("expected first page with 2 of 3 jobs")
	}
	if first.Items[0].ID != "job2" || first.Items[1].ID != "job3" {
		t.Fatalf("unexpected sort order: got %s then %s", first.Items[0].ID, first.Items[1].ID)
	}
	if first.NextCursor != "2" || first.Sort != string(JobSortDomainAsc) {
		t.Fatalf("expected next cursor and sort metadata")
	}

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/jobs?limit=2&sort=domain_asc&cursor=2", nil)
	second := mustJSON[JobList](t, resp, http.StatusOK)
	if len(second.Items) != 1 || second.Items[0].ID != "job1" {
		t.Fatalf("expected second page to contain only job1")
	}
	if second.PrevCursor != "0" || second.NextCursor != "" {
		t.Fatalf("expected prev cursor 0 and no next cursor, got prev=%q next=%q", second.PrevCursor, second.NextCursor)
	}
}

func TestListJobsFiltersByDomainAndTimeRange(t *testing.T) {
	srv := newTestServer(t)
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", Domain: "alpha.example", Status: JobQueued, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", Domain: "beta.example", Status: JobQueued, CreatedAt: base.Add(time.Second)})
	_, _ = srv.store.Create(Job{ID: "job3", Domain: "alpha.internal", Status: JobQueued, CreatedAt: base.Add(2 * time.Second)})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs?domain=alpha&created_after=2026-02-03T00:00:00Z&created_before=2026-02-03T00:00:01Z", nil)
	list := mustJSON[JobList](t, resp, http.StatusOK)
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != "job1" {
		t.Fatalf("expected only job1 to match filters, got %+v", list.Items)
	}
}

func TestListJobsFiltersByBatchIDAndSupportsBatchSort(t *testing.T) {
	srv := newTestServer(t)
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", BatchID: "batch_b", Domain: "alpha.example", Status: JobQueued, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", BatchID: "batch_a", Domain: "beta.example", Status: JobQueued, CreatedAt: base.Add(time.Second)})
	_, _ = srv.store.Create(Job{ID: "job3", BatchID: "batch_b", Domain: "gamma.example", Status: JobQueued, CreatedAt: base.Add(2 * time.Second)})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs?sort=batch_id_asc", nil)
	list := mustJSON[JobList](t, resp, http.StatusOK)
	if list.Sort != string(JobSortBatchIDAsc) {
		t.Fatalf("expected sort metadata %q, got %q", JobSortBatchIDAsc, list.Sort)
	}
	if len(list.Items) != 3 || list.Items[0].ID != "job2" {
		t.Fatalf("expected batch_id_asc to order batch_a before batch_b")
	}

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/jobs?batch_id=batch_b&sort=batch_id_desc", nil)
	list = mustJSON[JobList](t, resp, http.StatusOK)
	if list.Total != 2 || len(list.Items) != 2 {
		t.Fatalf("expected two jobs in batch_b, got %+v", list.Items)
	}
	for _, item := range list.Items {
		if item.BatchID != "batch_b" {
			t.Fatalf("expected all items in batch_b, got %q", item.BatchID)
		}
	}
}

func TestListJobsSortBySeverityAcceptsParam(t *testing.T) {
	// error_desc and critical_desc are accepted sort params (fall back to started_at for in-flight jobs).
	srv := newTestServer(t)
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", Domain: "alpha.example", Status: JobQueued, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", Domain: "beta.example", Status: JobQueued, CreatedAt: base.Add(time.Second)})

	for _, sort := range []string{"error_desc", "critical_desc"} {
		resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs?sort="+sort, nil)
		list := mustJSON[JobList](t, resp, http.StatusOK)
		if list.Total != 2 {
			t.Fatalf("sort=%s: expected 2 items, got %d", sort, list.Total)
		}
	}
}

func TestListJobsRejectsInvalidQuery(t *testing.T) {
	srv := newTestServer(t)

	for _, tc := range []struct {
		query string
		code  string
	}{
		{query: "cursor=not-a-number", code: "invalid_cursor"},
		{query: "sort=bad_sort", code: "invalid_sort"},
		{query: "severity=bad_filter", code: "invalid_severity"},
		{query: "created_before=not-a-date", code: "invalid_created_before"},
		{query: "created_after=2026-02-03T00:00:02Z&created_before=2026-02-03T00:00:01Z", code: "invalid_time_range"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs?"+tc.query, nil)
			wantErrorCode(t, resp, http.StatusBadRequest, tc.code)
		})
	}
}

func TestBatchSubmit(t *testing.T) {
	srv := newTestServer(t)

	payload := `{"domains":["example.com","example.net"]}`
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs/batch", payload)

	out := mustJSON[JobBatchResponse](t, resp, http.StatusAccepted)
	if out.BatchID == "" || len(out.JobIDs) != 2 {
		t.Fatalf("expected batch id and 2 job ids")
	}
}

// TestBatchSubmitRecordsPromoteDefaultIntent verifies the batch handler
// records the per-batch pin-on-capture intent only when both snapshot_intent
// and promote_snapshot_default are set. The intent is stored in the settings
// table under a batch-scoped key; the capture loop consumes it later.
func TestBatchSubmitRecordsPromoteDefaultIntent(t *testing.T) {
	submit := func(t *testing.T, payload string) (*Server, string) {
		t.Helper()
		srv := newTestServer(t)
		resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs/batch", payload)
		out := mustJSON[JobBatchResponse](t, resp, http.StatusAccepted)
		return srv, out.BatchID
	}

	t.Run("records intent with snapshot_intent and promote", func(t *testing.T) {
		srv, batchID := submit(t, `{"domains":["example.com"],"snapshot_intent":true,"promote_snapshot_default":true}`)
		v, ok := srv.store.GetSetting(PromoteDefaultSettingKey(batchID))
		if !ok || v != "1" {
			t.Fatalf("expected promote-default setting = 1, got %q ok=%v", v, ok)
		}
	})

	t.Run("omits intent for plain snapshot batch", func(t *testing.T) {
		srv, batchID := submit(t, `{"domains":["example.com"],"snapshot_intent":true}`)
		if _, ok := srv.store.GetSetting(PromoteDefaultSettingKey(batchID)); ok {
			t.Fatal("plain snapshot batch must not record the promote-default intent")
		}
	})

	t.Run("omits intent when promote set without snapshot_intent", func(t *testing.T) {
		srv, batchID := submit(t, `{"domains":["example.com"],"promote_snapshot_default":true}`)
		if _, ok := srv.store.GetSetting(PromoteDefaultSettingKey(batchID)); ok {
			t.Fatal("promote without snapshot_intent must not record the intent")
		}
	})
}

func TestBatchSubmitRejectsUndelegatedFields(t *testing.T) {
	srv := newTestServer(t)

	payload := `{"domains":["example.com"],"nameservers":[{"ns":"ns1.example.com","ip":"192.0.2.1"}]}`
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs/batch", payload)

	wantErrorCode(t, resp, http.StatusBadRequest, "undelegated_not_supported_for_batch")
}

func TestBatchSummary(t *testing.T) {
	srv := newTestServer(t)

	payload := `{"domains":["example.com","example.net"]}`
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs/batch", payload)
	batch := mustJSON[JobBatchResponse](t, resp, http.StatusAccepted)

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/batches/"+batch.BatchID, nil)
	summary := mustJSON[BatchSummary](t, resp, http.StatusOK)
	if summary.BatchID != batch.BatchID {
		t.Fatalf("expected batch id %s, got %s", batch.BatchID, summary.BatchID)
	}
	if summary.Total != 2 || len(summary.Items) != 2 {
		t.Fatalf("expected 2 items in summary")
	}
	if summary.CreatedAt.IsZero() {
		t.Fatalf("expected created_at to be set")
	}
	if summary.StatusCounts["queued"] != 2 {
		t.Fatalf("expected queued count 2, got %d", summary.StatusCounts["queued"])
	}
}

func TestBatchSummaryIncludesTag(t *testing.T) {
	srv := newTestServer(t)

	// Create a batch record with a tag directly in the store.
	batchID := "batch_tagged"
	_ = srv.store.CreateBatch(Batch{
		ID:          batchID,
		Tag:         "se-domains",
		DomainCount: 1,
		CreatedAt:   time.Now(),
	})
	// Add a job to the batch so the summary has content.
	srv.store.Create(Job{ID: "job_t1", Domain: "example.se", BatchID: batchID, Status: JobQueued, CreatedAt: time.Now()})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/batches/"+batchID, nil)
	summary := mustJSON[BatchSummary](t, resp, http.StatusOK)
	if summary.Tag != "se-domains" {
		t.Fatalf("expected tag 'se-domains', got %q", summary.Tag)
	}
}

func TestBatchSummaryPaginationAndSort(t *testing.T) {
	srv := newTestServer(t)
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	batchID := "batch_1"

	_, _ = srv.store.Create(Job{
		ID:        "job1",
		BatchID:   batchID,
		Domain:    "gamma.example",
		Status:    JobQueued,
		CreatedAt: base,
	})
	_, _ = srv.store.Create(Job{
		ID:        "job2",
		BatchID:   batchID,
		Domain:    "alpha.example",
		Status:    JobRunning,
		CreatedAt: base.Add(time.Second),
	})
	_, _ = srv.store.Create(Job{
		ID:         "job3",
		BatchID:    batchID,
		Domain:     "beta.example",
		Status:     JobFailed,
		CreatedAt:  base.Add(2 * time.Second),
		FinishedAt: base.Add(3 * time.Second),
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/batches/"+batchID+"?limit=1&sort=created_at_asc&cursor=1", nil)
	summary := mustJSON[BatchSummary](t, resp, http.StatusOK)
	if summary.Total != 3 || len(summary.Items) != 1 || summary.Items[0].ID != "job2" {
		t.Fatalf("expected paginated batch summary with one middle item")
	}
	if summary.Limit != 1 || summary.Offset != 1 || summary.NextCursor != "2" || summary.PrevCursor != "0" {
		t.Fatalf("unexpected pagination metadata: limit=%d offset=%d next=%q prev=%q", summary.Limit, summary.Offset, summary.NextCursor, summary.PrevCursor)
	}
	if summary.Sort != string(JobSortCreatedAtAsc) {
		t.Fatalf("expected sort metadata %q, got %q", JobSortCreatedAtAsc, summary.Sort)
	}
	if summary.StatusCounts["queued"] != 1 || summary.StatusCounts["running"] != 1 || summary.StatusCounts["failed"] != 1 {
		t.Fatalf("expected status counts across full batch, got %+v", summary.StatusCounts)
	}
}

func TestBatchSummarySupportsFiltersAndEmptyMatches(t *testing.T) {
	srv := newTestServer(t)
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	batchID := "batch_filters"

	_, _ = srv.store.Create(Job{
		ID:        "job1",
		BatchID:   batchID,
		Domain:    "alpha.example",
		Status:    JobQueued,
		CreatedAt: base,
	})
	_, _ = srv.store.Create(Job{
		ID:        "job2",
		BatchID:   batchID,
		Domain:    "beta.example",
		Status:    JobFailed,
		CreatedAt: base.Add(time.Second),
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/batches/"+batchID+"?status=failed&domain=beta&created_after=2026-02-03T00:00:00Z&sort=domain_asc", nil)
	summary := mustJSON[BatchSummary](t, resp, http.StatusOK)
	if summary.Total != 1 || len(summary.Items) != 1 || summary.Items[0].ID != "job2" {
		t.Fatalf("expected filtered batch summary with job2 only, got %+v", summary.Items)
	}
	if summary.Sort != string(JobSortDomainAsc) {
		t.Fatalf("expected sort metadata %q, got %q", JobSortDomainAsc, summary.Sort)
	}
	if summary.StatusCounts["queued"] != 1 || summary.StatusCounts["failed"] != 1 {
		t.Fatalf("expected full-batch status counts, got %+v", summary.StatusCounts)
	}

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/batches/"+batchID+"?status=running", nil)
	summary = mustJSON[BatchSummary](t, resp, http.StatusOK)
	if summary.Total != 0 || len(summary.Items) != 0 {
		t.Fatalf("expected no matching items, got %+v", summary.Items)
	}
}

func TestBatchSummaryIncludesGrades(t *testing.T) {
	srv := newTestServer(t)
	batchID := "batch_grades"

	// Graduate two jobs in the same batch.
	now := time.Now().UTC()
	for _, domain := range []string{"alpha.example", "beta.example"} {
		job := Job{
			ID:         newID("job"),
			BatchID:    batchID,
			Domain:     domain,
			Status:     JobSucceeded,
			CreatedAt:  now,
			StartedAt:  now,
			FinishedAt: now,
		}
		createAndGraduate(t, srv.store, job, nil)
		// Also record batch metadata so the handler finds it.
		_ = srv.store.CreateBatch(Batch{ID: batchID, CreatedAt: now, DomainCount: 2})
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/batches/"+batchID, nil)
	summary := mustJSON[BatchSummary](t, resp, http.StatusOK)
	if len(summary.Grades) == 0 {
		t.Fatal("expected non-empty grades map in batch summary")
	}
	total := 0
	for _, count := range summary.Grades {
		total += count
	}
	if total != 2 {
		t.Fatalf("expected grades total=2 (one per graduated run), got %d", total)
	}
}

func TestCancelJob(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`)
	created := mustJSON[Job](t, resp, http.StatusCreated)

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/jobs/"+created.ID+"/cancel", nil)
	canceled := mustJSON[Job](t, resp, http.StatusOK)
	if canceled.Status != JobCanceled {
		t.Fatalf("expected status canceled, got %s", canceled.Status)
	}
}

func TestCancelJobCSRFRejectsMismatchedOrigin(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`)
	created := mustJSON[Job](t, resp, http.StatusCreated)

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/jobs/"+created.ID+"/cancel", nil, withOrigin("https://attacker.example"))
	wantErrorCode(t, resp, http.StatusForbidden, "csrf_origin_mismatch")

	stored, ok := srv.store.Get(created.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
	if stored.Status == JobCanceled {
		t.Fatalf("expected job to remain uncanceled")
	}
}

func TestCancelJobTriggersContextCancel(t *testing.T) {
	srv := newTestServer(t)

	job := Job{
		ID:        "job-running",
		Domain:    "example.com",
		Status:    JobRunning,
		CreatedAt: time.Now().UTC(),
		StartedAt: time.Now().UTC(),
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	canceled := make(chan struct{})
	var once sync.Once
	srv.registerCancel(job.ID, func() { once.Do(func() { close(canceled) }) })

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs/"+job.ID+"/cancel", nil)
	wantStatus(t, resp, http.StatusOK)
	select {
	case <-canceled:
	default:
		t.Fatalf("expected cancel function to be called")
	}
	// Running jobs are graduated asynchronously by the worker after context cancel.
	// The job remains in-flight (JobRunning) in the store until the worker handles it.
	_, ok := srv.store.Get(job.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
}

func TestQueueEndpoints(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/queue/pause", nil)
	wantStatus(t, resp, http.StatusOK)

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/queue/resume", nil)
	wantStatus(t, resp, http.StatusOK)
}

func TestQueueReorderValidation(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/queue/reorder", `{"job_ids":["job1"]}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestQueueRemove(t *testing.T) {
	srv := newTestServer(t)
	now := time.Now().UTC()
	job := Job{
		ID:        "job1",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: now,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := srv.queue.Enqueue(job.ID, PriorityNormal); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/queue/remove", `{"job_ids":["job1"]}`)
	wantStatus(t, resp, http.StatusOK)

	// After queue remove, the job is graduated and only accessible via GetRun/Get.
	stored, ok := srv.store.Get(job.ID)
	if !ok {
		t.Fatalf("expected job in store (reconstructed from run)")
	}
	if stored.Status != JobCanceled {
		t.Fatalf("expected status canceled, got %s", stored.Status)
	}

	result, ok := srv.store.GetResult(job.ID)
	if !ok {
		t.Fatalf("expected job result")
	}
	if result.Status != JobCanceled {
		t.Fatalf("expected result status canceled, got %s", result.Status)
	}
}

func TestMetricsTracksCreateAndRunLifecycle(t *testing.T) {
	srv := newTestServer(t)
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`)
	created := mustJSON[Job](t, resp, http.StatusCreated)

	snapshot := getMetricsSnapshot(t, srv)
	if snapshot.Jobs.SubmittedTotal != 1 {
		t.Fatalf("submitted_total = %d, want 1", snapshot.Jobs.SubmittedTotal)
	}
	if snapshot.Health.QueueDepth != 1 {
		t.Fatalf("queue_depth = %d, want 1", snapshot.Health.QueueDepth)
	}
	if snapshot.Jobs.StatusCounts[string(JobQueued)] != 1 {
		t.Fatalf("status_counts[queued] = %d, want 1", snapshot.Jobs.StatusCounts[string(JobQueued)])
	}

	if err := srv.runJob(created.ID); err != nil {
		t.Fatalf("run job: %v", err)
	}

	snapshot = getMetricsSnapshot(t, srv)
	if snapshot.Health.QueueDepth != 0 {
		t.Fatalf("queue_depth = %d, want 0", snapshot.Health.QueueDepth)
	}
	if snapshot.Health.InFlightJobs != 0 {
		t.Fatalf("in_flight_jobs = %d, want 0", snapshot.Health.InFlightJobs)
	}
	if snapshot.Jobs.StartedTotal != 1 {
		t.Fatalf("started_total = %d, want 1", snapshot.Jobs.StartedTotal)
	}
	if snapshot.Jobs.CompletedTotal != 1 {
		t.Fatalf("completed_total = %d, want 1", snapshot.Jobs.CompletedTotal)
	}
	if snapshot.Jobs.CanceledTotal != 0 {
		t.Fatalf("canceled_total = %d, want 0", snapshot.Jobs.CanceledTotal)
	}
	if snapshot.Jobs.StatusCounts[string(JobSucceeded)] != 1 {
		t.Fatalf("status_counts[succeeded] = %d, want 1", snapshot.Jobs.StatusCounts[string(JobSucceeded)])
	}
}

func TestMetricsTracksPauseResumeAndCancel(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`)
	created := mustJSON[Job](t, resp, http.StatusCreated)

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/queue/pause", nil)
	wantStatus(t, resp, http.StatusOK)
	snapshot := getMetricsSnapshot(t, srv)
	if !snapshot.Health.QueuePaused {
		t.Fatal("queue_paused = false, want true")
	}

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/jobs/"+created.ID+"/cancel", nil)
	wantStatus(t, resp, http.StatusOK)
	snapshot = getMetricsSnapshot(t, srv)
	if snapshot.Jobs.CanceledTotal != 1 {
		t.Fatalf("canceled_total = %d, want 1", snapshot.Jobs.CanceledTotal)
	}
	if snapshot.Jobs.CompletedTotal != 1 {
		t.Fatalf("completed_total = %d, want 1", snapshot.Jobs.CompletedTotal)
	}
	if snapshot.Health.QueueDepth != 0 {
		t.Fatalf("queue_depth = %d, want 0", snapshot.Health.QueueDepth)
	}
	if snapshot.Jobs.StatusCounts[string(JobCanceled)] != 1 {
		t.Fatalf("status_counts[canceled] = %d, want 1", snapshot.Jobs.StatusCounts[string(JobCanceled)])
	}

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/queue/resume", nil)
	wantStatus(t, resp, http.StatusOK)
	snapshot = getMetricsSnapshot(t, srv)
	if snapshot.Health.QueuePaused {
		t.Fatal("queue_paused = true, want false")
	}
}

func TestMetricsTracksAPIRequestsByRouteMethodStatusAndErrorCode(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/healthz", nil)
	wantStatus(t, resp, http.StatusOK)

	payload := `{"job_ids":["missing"]}`
	resp = doJSON(t, srv, http.MethodPost, "/api/v1/queue/reorder", payload)
	wantStatus(t, resp, http.StatusBadRequest)

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/jobs/job-missing/cancel", nil)
	wantStatus(t, resp, http.StatusNotFound)

	snapshot := srv.metrics.Snapshot()
	if snapshot.API.RequestsTotal != 3 {
		t.Fatalf("api.requests_total = %d, want 3", snapshot.API.RequestsTotal)
	}
	if snapshot.API.StatusClassCounts["2xx"] != 1 {
		t.Fatalf("api.status_class_counts[2xx] = %d, want 1", snapshot.API.StatusClassCounts["2xx"])
	}
	if snapshot.API.StatusClassCounts["4xx"] != 2 {
		t.Fatalf("api.status_class_counts[4xx] = %d, want 2", snapshot.API.StatusClassCounts["4xx"])
	}
	if snapshot.API.ErrorCodeCounts["invalid_queue"] != 1 {
		t.Fatalf("api.error_code_counts[invalid_queue] = %d, want 1", snapshot.API.ErrorCodeCounts["invalid_queue"])
	}
	if snapshot.API.ErrorCodeCounts["not_found"] != 1 {
		t.Fatalf("api.error_code_counts[not_found] = %d, want 1", snapshot.API.ErrorCodeCounts["not_found"])
	}

	health := findAPIRouteMetrics(t, snapshot, http.MethodGet, "/api/v1/healthz")
	if health.RequestsTotal != 1 || health.StatusClassCounts["2xx"] != 1 {
		t.Fatalf("unexpected health route metrics: %+v", health)
	}
	if health.LatencyMs.P50 == 0 || health.LatencyMs.P90 == 0 || health.LatencyMs.P99 == 0 {
		t.Fatalf("expected non-zero latency percentiles, got %+v", health.LatencyMs)
	}

	reorder := findAPIRouteMetrics(t, snapshot, http.MethodPost, "/api/v1/queue/reorder")
	if reorder.RequestsTotal != 1 || reorder.StatusClassCounts["4xx"] != 1 {
		t.Fatalf("unexpected reorder route metrics: %+v", reorder)
	}

	cancel := findAPIRouteMetrics(t, snapshot, http.MethodPost, "/api/v1/jobs/{job_id}/cancel")
	if cancel.RequestsTotal != 1 || cancel.StatusClassCounts["4xx"] != 1 {
		t.Fatalf("unexpected cancel route metrics: %+v", cancel)
	}
}

func TestMetricsTracksQualityAcrossMixedOutcomesAndLocaleRequests(t *testing.T) {
	srv := newTestServer(t)
	callCount := 0
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		callCount++
		switch callCount {
		case 1:
			return []engine.LogEntry{
				{Level: "NOTICE"},
				{Level: "ERROR"},
			}, nil
		case 2:
			return []engine.LogEntry{
				{Level: "WARNING"},
				{Level: "CRITICAL"},
			}, errors.New("run failed")
		default:
			return nil, nil
		}
	}

	now := time.Now().UTC()
	jobSuccess := Job{ID: "job-success", Domain: "ok.example", Status: JobQueued, CreatedAt: now}
	jobFailure := Job{ID: "job-failure", Domain: "fail.example", Status: JobQueued, CreatedAt: now.Add(time.Second)}
	if _, err := srv.store.Create(jobSuccess); err != nil {
		t.Fatalf("create success job: %v", err)
	}
	if _, err := srv.store.Create(jobFailure); err != nil {
		t.Fatalf("create failure job: %v", err)
	}

	if err := srv.runJob(jobSuccess.ID); err != nil {
		t.Fatalf("run success job: %v", err)
	}
	if err := srv.runJob(jobFailure.ID); err == nil {
		t.Fatal("expected run error for failure job")
	}

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"cancel.example"}`)
	created := mustJSON[Job](t, resp, http.StatusCreated)

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/jobs/"+created.ID+"/cancel", nil)
	wantStatus(t, resp, http.StatusOK)

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/jobs/"+jobSuccess.ID+"/result?locale=sv", nil)
	wantStatus(t, resp, http.StatusOK)

	snapshot := srv.metrics.Snapshot()
	if snapshot.Quality.Outcomes.SuccessTotal != 1 || snapshot.Quality.Outcomes.FailedTotal != 1 || snapshot.Quality.Outcomes.CanceledTotal != 1 {
		t.Fatalf("unexpected quality outcomes: %+v", snapshot.Quality.Outcomes)
	}
	if snapshot.Quality.JobDurationMs.Count != 2 {
		t.Fatalf("quality.job_duration_ms.count = %d, want 2", snapshot.Quality.JobDurationMs.Count)
	}
	if snapshot.Quality.Severity.Totals["NOTICE"] != 1 || snapshot.Quality.Severity.Totals["WARNING"] != 1 || snapshot.Quality.Severity.Totals["ERROR"] != 1 || snapshot.Quality.Severity.Totals["CRITICAL"] != 1 {
		t.Fatalf("unexpected severity totals: %+v", snapshot.Quality.Severity.Totals)
	}
	if snapshot.Quality.LocaleUsage.Counts["sv"] != 1 {
		t.Fatalf("quality.locale_usage.counts[sv] = %d, want 1", snapshot.Quality.LocaleUsage.Counts["sv"])
	}
	if len(snapshot.Insights.Domains.Items) == 0 {
		t.Fatal("expected non-empty domain insights")
	}
	if snapshot.Insights.Batches.Limit == 0 || snapshot.Insights.Batches.Cap == 0 {
		t.Fatalf("expected batch insight limit/cap metadata, got %+v", snapshot.Insights.Batches)
	}
}

func TestMetricsEndpointValidatesQueryParams(t *testing.T) {
	srv := newTestServer(t)
	tests := []struct {
		path     string
		wantCode string
	}{
		{path: "/api/v1/metrics?window=12h", wantCode: "invalid_window"},
		{path: "/api/v1/metrics?format=yaml", wantCode: "invalid_format"},
		{path: "/api/v1/metrics?include=unknown", wantCode: "invalid_include"},
		{path: "/api/v1/metrics?limit_domains=0", wantCode: "invalid_limit_domains"},
		{path: "/api/v1/metrics?limit_batches=999", wantCode: "invalid_limit_batches"},
	}

	for _, tc := range tests {
		resp := doJSON(t, srv, http.MethodGet, tc.path, nil)
		out := mustJSON[ErrorResponse](t, resp, http.StatusBadRequest)
		if out.Error.Code != tc.wantCode {
			t.Fatalf("%s: error code = %q, want %q", tc.path, out.Error.Code, tc.wantCode)
		}
	}
}

func TestMetricsEndpointSupportsIncludeWindowAndLimits(t *testing.T) {
	srv := newTestServer(t)
	now := time.Date(2026, 2, 9, 10, 0, 0, 0, time.UTC)
	srv.metrics.nowFn = func() time.Time { return now }
	srv.metrics.ObserveJobSubmittedWithContext("batch-a", "alpha.example", JobQueued)
	srv.metrics.ObserveJobStatusTransition(JobQueued, JobSucceeded)
	srv.metrics.ObserveJobCompletionWithContext("batch-a", "alpha.example", JobSucceeded, 1200*time.Millisecond, map[string]int64{
		"NOTICE": 1,
	})
	srv.metrics.ObserveJobSubmittedWithContext("batch-b", "beta.example", JobQueued)
	srv.metrics.ObserveJobStatusTransition(JobQueued, JobFailed)
	srv.metrics.ObserveJobCompletionWithContext("batch-b", "beta.example", JobFailed, 1800*time.Millisecond, map[string]int64{
		"ERROR": 2,
	})
	srv.metrics.ObserveCacheMetrics(9, 3, 1)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/metrics?include=health,insights,trends&window=1h&limit_domains=1&limit_batches=1", nil)
	wantStatus(t, resp, http.StatusOK)

	payload := mustJSON[map[string]any](t, resp, http.StatusOK)
	if _, ok := payload["schema_version"]; !ok {
		t.Fatal("missing schema_version")
	}
	if _, ok := payload["server_version"]; !ok {
		t.Fatal("missing server_version")
	}
	if _, ok := payload["generated_at"]; !ok {
		t.Fatal("missing generated_at")
	}
	if _, ok := payload["health"]; !ok {
		t.Fatal("missing health")
	}
	if _, ok := payload["insights"]; !ok {
		t.Fatal("missing insights")
	}
	if _, ok := payload["trends"]; !ok {
		t.Fatal("missing trends")
	}
	if _, ok := payload["api"]; ok {
		t.Fatal("api should not be included")
	}
	if _, ok := payload["jobs"]; ok {
		t.Fatal("jobs should not be included")
	}
	if _, ok := payload["quality"]; ok {
		t.Fatal("quality should not be included")
	}

	insights := payload["insights"].(map[string]any)
	batches := insights["batches"].(map[string]any)
	domains := insights["domains"].(map[string]any)
	if intFromAny(batches["limit"]) != 1 {
		t.Fatalf("batches.limit = %v, want 1", batches["limit"])
	}
	if intFromAny(domains["limit"]) != 1 {
		t.Fatalf("domains.limit = %v, want 1", domains["limit"])
	}
	if got := len(batches["items"].([]any)); got != 1 {
		t.Fatalf("batches.items size = %d, want 1", got)
	}
	if got := len(domains["items"].([]any)); got != 1 {
		t.Fatalf("domains.items size = %d, want 1", got)
	}

	trends := payload["trends"].(map[string]any)
	windows := trends["windows"].(map[string]any)
	if len(windows) != 1 {
		t.Fatalf("trends.windows size = %d, want 1", len(windows))
	}
	window1h, ok := windows["1h"]
	if !ok {
		t.Fatalf("expected trends window 1h, got %+v", windows)
	}
	points := window1h.(map[string]any)["points"].([]any)
	if len(points) == 0 {
		t.Fatal("expected at least one trend point")
	}
	maxHitRate := 0.0
	for _, rawPoint := range points {
		point, ok := rawPoint.(map[string]any)
		if !ok {
			t.Fatalf("unexpected trend point type: %T", rawPoint)
		}
		value, ok := point["dns_cache_hit_rate"]
		if !ok {
			t.Fatalf("expected dns_cache_hit_rate in trend point, got %+v", point)
		}
		rate, ok := value.(float64)
		if !ok {
			t.Fatalf("unexpected dns_cache_hit_rate type: %T", value)
		}
		if rate > maxHitRate {
			maxHitRate = rate
		}
	}
	if maxHitRate <= 0 {
		t.Fatalf("expected positive dns_cache_hit_rate, got %v", maxHitRate)
	}
}

func TestMetricsEndpointSupportsPrometheusFormat(t *testing.T) {
	srv := newTestServer(t)
	srv.metrics.ObserveQueuePaused(true)
	srv.metrics.ObserveDNSQueries(11, 7)
	srv.metrics.ObserveCacheMetrics(9, 3, 1)
	srv.metrics.ObserveJobSubmittedWithContext("batch-a", "alpha.example", JobQueued)
	srv.metrics.ObserveJobStatusTransition(JobQueued, JobRunning)
	srv.metrics.ObserveJobStatusTransition(JobRunning, JobFailed)
	srv.metrics.ObserveJobCompletionWithContext("batch-a", "alpha.example", JobFailed, 1500*time.Millisecond, map[string]int64{
		"ERROR":    2,
		"CRITICAL": 1,
	})
	srv.metrics.ObserveAPIRequest("/api/v1/jobs", http.MethodGet, http.StatusOK, 320*time.Millisecond, "")
	srv.metrics.ObserveAPIRequest("/api/v1/jobs", http.MethodGet, http.StatusBadRequest, 90*time.Millisecond, "invalid_domain")
	srv.metrics.ObserveResultLocale("sv-SE")

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/metrics?format=prom&include=trends&window=1h&limit_domains=1&limit_batches=1", nil)
	wantStatus(t, resp, http.StatusOK)
	if got := resp.Header().Get("Content-Type"); got != prometheusMetricsContentType {
		t.Fatalf("expected %q content-type, got %q", prometheusMetricsContentType, got)
	}

	body := resp.Body.String()
	assertPrometheusTextWellFormed(t, body)
	if !strings.Contains(body, "# TYPE gonemaster_api_request_duration_seconds histogram") {
		t.Fatalf("missing API histogram header in Prometheus output:\n%s", body)
	}
	if !strings.Contains(body, "gonemaster_dns_cache_lookups_total{result=\"hit\"} 9") {
		t.Fatalf("missing cache hit counter in Prometheus output:\n%s", body)
	}
	if !strings.Contains(body, "gonemaster_api_route_requests_total{method=\"GET\",route=\"/api/v1/jobs\"} 2") {
		t.Fatalf("missing per-route API counter in Prometheus output:\n%s", body)
	}
	if !strings.Contains(body, "gonemaster_job_duration_seconds_bucket{le=\"+Inf\"} 1") {
		t.Fatalf("missing job duration histogram bucket in Prometheus output:\n%s", body)
	}
	if !strings.Contains(body, "gonemaster_job_severity_total{severity=\"CRITICAL\"} 1") {
		t.Fatalf("missing severity counter in Prometheus output:\n%s", body)
	}
	if !strings.Contains(body, "gonemaster_result_locale_requests_total{locale=\"sv_se\"} 1") {
		t.Fatalf("missing locale counter in Prometheus output:\n%s", body)
	}
	if strings.Contains(body, "alpha.example") {
		t.Fatalf("unexpected domain label leaked into Prometheus output:\n%s", body)
	}
	if strings.Contains(body, "batch-a") {
		t.Fatalf("unexpected batch label leaked into Prometheus output:\n%s", body)
	}
	if strings.Contains(body, "dns_cache_hit_rate") {
		t.Fatalf("unexpected derived JSON trend metric leaked into Prometheus output:\n%s", body)
	}
}

func TestMetricsEndpointCachesByQueryForOneSecond(t *testing.T) {
	srv := newTestServer(t)
	base := time.Date(2026, 2, 9, 10, 0, 0, 0, time.UTC)
	now := base
	srv.metrics.nowFn = func() time.Time { return now }

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/metrics", nil)
	first := mustJSON[MetricsSnapshot](t, resp, http.StatusOK)
	if first.Jobs.SubmittedTotal != 0 {
		t.Fatalf("first submitted_total = %d, want 0", first.Jobs.SubmittedTotal)
	}

	now = base.Add(100 * time.Millisecond)
	createResp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"cached.example"}`)
	wantStatus(t, createResp, http.StatusCreated)

	now = base.Add(500 * time.Millisecond)
	resp = doJSON(t, srv, http.MethodGet, "/api/v1/metrics", nil)
	second := mustJSON[MetricsSnapshot](t, resp, http.StatusOK)
	if !second.GeneratedAt.Equal(first.GeneratedAt) {
		t.Fatalf("second generated_at = %s, want cached %s", second.GeneratedAt, first.GeneratedAt)
	}
	if second.Jobs.SubmittedTotal != first.Jobs.SubmittedTotal {
		t.Fatalf("second submitted_total = %d, want cached %d", second.Jobs.SubmittedTotal, first.Jobs.SubmittedTotal)
	}

	now = base.Add(2 * time.Second)
	resp = doJSON(t, srv, http.MethodGet, "/api/v1/metrics", nil)
	third := mustJSON[MetricsSnapshot](t, resp, http.StatusOK)
	if !third.GeneratedAt.After(second.GeneratedAt) {
		t.Fatalf("third generated_at = %s, want after %s", third.GeneratedAt, second.GeneratedAt)
	}
	if third.Jobs.SubmittedTotal != 1 {
		t.Fatalf("third submitted_total = %d, want 1", third.Jobs.SubmittedTotal)
	}
}

func TestHealthAndMetrics(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/healthz", nil)
	wantStatus(t, resp, http.StatusOK)

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/metrics", nil)
	wantStatus(t, resp, http.StatusOK)
	if got := resp.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content-type, got %q", got)
	}
	metrics := mustJSON[MetricsSnapshot](t, resp, http.StatusOK)
	if metrics.SchemaVersion == "" {
		t.Fatal("expected schema_version in metrics response")
	}
	if metrics.ServerVersion == "" {
		t.Fatal("expected server_version in metrics response")
	}
	if metrics.GeneratedAt.IsZero() {
		t.Fatal("expected generated_at in metrics response")
	}
}

func TestLocalesEndpoint(t *testing.T) {
	srv := newTestServer(t)

	t.Run("GET returns locales list", func(t *testing.T) {
		resp := doJSON(t, srv, http.MethodGet, "/api/v1/locales", nil)

		wantStatus(t, resp, http.StatusOK)
		if ct := resp.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected application/json, got %q", ct)
		}

		var body struct {
			Locales []string `json:"locales"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(body.Locales) == 0 {
			t.Fatal("expected at least one locale in response")
		}
		found := slices.Contains(body.Locales, "en")
		if !found {
			t.Fatalf("expected \"en\" in locales, got %v", body.Locales)
		}
	})

	t.Run("POST returns 405", func(t *testing.T) {
		resp := doJSON(t, srv, http.MethodPost, "/api/v1/locales", nil)
		wantStatus(t, resp, http.StatusMethodNotAllowed)
	})
}

func TestHandleJobsPurge(t *testing.T) {
	postPurge := func(srv *Server, body string) *httptest.ResponseRecorder {
		t.Helper()
		var reqBody *bytes.Buffer
		if body != "" {
			reqBody = bytes.NewBufferString(body)
		} else {
			reqBody = &bytes.Buffer{}
		}
		resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs/purge", reqBody)
		return resp
	}

	t.Run("400 when no retention configured and no body", func(t *testing.T) {
		srv := newTestServer(t)
		resp := postPurge(srv, "")
		body := mustJSON[map[string]any](t, resp, http.StatusBadRequest)
		if errObj, _ := body["error"].(map[string]any); errObj["code"] != "retention_not_configured" {
			t.Fatalf("expected retention_not_configured, got %v", errObj)
		}
	})

	t.Run("200 with older_than_days in body", func(t *testing.T) {
		srv := newTestServer(t)
		old := time.Now().UTC().Add(-48 * time.Hour)
		job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
		createAndGraduate(t, srv.store, job, nil)

		resp := postPurge(srv, `{"older_than_days":1}`)
		body := mustJSON[map[string]int64](t, resp, http.StatusOK)
		if body["purged_jobs"] != 1 {
			t.Fatalf("expected purged_jobs=1, got %d", body["purged_jobs"])
		}
	})

	t.Run("200 using server retention_days when body omits older_than_days", func(t *testing.T) {
		srv := newTestServer(t, withConfig(func(c *Config) { c.Database.RetentionDays = 1 }))
		old := time.Now().UTC().Add(-48 * time.Hour)
		job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
		createAndGraduate(t, srv.store, job, nil)

		resp := postPurge(srv, "")
		body := mustJSON[map[string]int64](t, resp, http.StatusOK)
		if body["purged_jobs"] != 1 {
			t.Fatalf("expected purged_jobs=1, got %d", body["purged_jobs"])
		}
	})

	t.Run("200 with zero purged when no old jobs", func(t *testing.T) {
		srv := newTestServer(t)
		resp := postPurge(srv, `{"older_than_days":90}`)
		body := mustJSON[map[string]int64](t, resp, http.StatusOK)
		if body["purged_jobs"] != 0 {
			t.Fatalf("expected purged_jobs=0, got %d", body["purged_jobs"])
		}
	})

	t.Run("GET returns 405", func(t *testing.T) {
		srv := newTestServer(t)
		resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs/purge", nil)
		wantStatus(t, resp, http.StatusMethodNotAllowed)
	})
}

// intFromAny converts a JSON-decoded any (float64) to int for test assertions.
func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func TestCreateJobHasNormalPriority(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`)
	created := mustJSON[Job](t, resp, http.StatusCreated)
	if created.Priority != PriorityNormal {
		t.Fatalf("response priority: expected PriorityNormal(0), got %d", created.Priority)
	}
	stored, ok := srv.store.Get(created.ID)
	if !ok {
		t.Fatalf("job not in store")
	}
	if stored.Priority != PriorityNormal {
		t.Fatalf("stored priority: expected PriorityNormal(0), got %d", stored.Priority)
	}
}

func TestBatchSubmitHasBatchPriority(t *testing.T) {
	srv := newTestServer(t)
	payload := `{"domains":["example.com","example.net"]}`
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs/batch", payload)
	out := mustJSON[JobBatchResponse](t, resp, http.StatusAccepted)
	for _, jobID := range out.JobIDs {
		job, ok := srv.store.Get(jobID)
		if !ok {
			t.Fatalf("job %q not in store", jobID)
		}
		if job.Priority != PriorityBatch {
			t.Fatalf("job %q: expected PriorityBatch(1), got %d", jobID, job.Priority)
		}
	}
}

func TestRobotsTxt(t *testing.T) {
	tests := []struct {
		name        string
		publicURL   string
		host        string
		wantSitemap string
	}{
		{
			name:        "configured URL",
			publicURL:   "https://example.com/",
			host:        "ignored.example.com",
			wantSitemap: "Sitemap: https://example.com/sitemap.xml",
		},
		{
			name:        "configured subpath URL",
			publicURL:   "https://example.com/public/",
			host:        "ignored.example.com",
			wantSitemap: "Sitemap: https://example.com/public/sitemap.xml",
		},
		{
			name:        "auto-detected from host",
			publicURL:   "",
			host:        "myhost.example.com",
			wantSitemap: "Sitemap: http://myhost.example.com/sitemap.xml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, withConfig(func(cfg *Config) { cfg.PublicURL = tt.publicURL }))

			rr := doJSON(t, srv, http.MethodGet, "/robots.txt", nil, withHost(tt.host))
			wantStatus(t, rr, http.StatusOK)
			if ct := rr.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
				t.Fatalf("Content-Type = %q", ct)
			}
			body := rr.Body.String()
			if !strings.Contains(body, tt.wantSitemap) {
				t.Fatalf("robots.txt missing %q\ngot: %s", tt.wantSitemap, body)
			}
		})
	}
}

func TestSitemapXML(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		host      string
		wantBase  string
	}{
		{
			name:      "configured root URL",
			publicURL: "https://example.com/",
			host:      "ignored.example.com",
			wantBase:  "https://example.com/",
		},
		{
			name:      "configured without trailing slash gets one",
			publicURL: "https://example.com",
			host:      "ignored.example.com",
			wantBase:  "https://example.com/",
		},
		{
			name:      "auto-detected from host",
			publicURL: "",
			host:      "myhost.example.com",
			wantBase:  "http://myhost.example.com/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, withConfig(func(cfg *Config) { cfg.PublicURL = tt.publicURL }))

			rr := doJSON(t, srv, http.MethodGet, "/sitemap.xml", nil, withHost(tt.host))
			wantStatus(t, rr, http.StatusOK)
			if ct := rr.Header().Get("Content-Type"); ct != "application/xml; charset=utf-8" {
				t.Fatalf("Content-Type = %q", ct)
			}
			body := rr.Body.String()

			// The public UI lives under public/, not at the site root, which
			// serves the admin UI and must not be advertised.
			home := serverpublic.HomeURL(tt.wantBase)
			if !strings.Contains(body, "<loc>"+home+"</loc>") {
				t.Fatalf("sitemap.xml missing <loc>%s</loc>\ngot: %s", home, body)
			}
			if strings.Contains(body, "<loc>"+tt.wantBase+"</loc>") {
				t.Error("sitemap.xml advertises the site root")
			}
			for _, lang := range serverpublic.ShippedLocales() {
				loc := serverpublic.LocaleURL(tt.wantBase, lang)
				if !strings.Contains(body, "<loc>"+loc+"</loc>") {
					t.Errorf("sitemap.xml has no <url> for %s", loc)
				}
			}
			analysisBase := strings.TrimRight(tt.wantBase, "/")
			for _, p := range analysisSitemapPaths {
				wantURL := "<loc>" + analysisBase + p + "</loc>"
				if !strings.Contains(body, wantURL) {
					t.Fatalf("sitemap.xml missing analysis URL %q\ngot: %s", wantURL, body)
				}
			}
		})
	}
}

// Sitemap hreflang is only read when every listed URL repeats the whole
// alternate set, itself included.
func TestSitemapAlternatesAreReciprocal(t *testing.T) {
	srv := newTestServer(t, withConfig(func(cfg *Config) { cfg.PublicURL = "https://example.com/" }))
	rr := doJSON(t, srv, http.MethodGet, "/sitemap.xml", nil, withHost("ignored.example.com"))
	wantStatus(t, rr, http.StatusOK)

	const base = "https://example.com/"
	wantURLs := serverpublic.PageURLs(base)

	blocks := strings.Split(rr.Body.String(), "<url>")[1:]
	found := 0
	for _, block := range blocks {
		loc := block[strings.Index(block, "<loc>")+len("<loc>") : strings.Index(block, "</loc>")]
		if !slices.Contains(wantURLs, loc) {
			continue
		}
		found++
		for _, want := range wantURLs {
			if !strings.Contains(block, `href="`+want+`"`) {
				t.Errorf("the <url> for %s does not list %s as an alternate", loc, want)
			}
		}
	}
	if found != len(wantURLs) {
		t.Errorf("found %d public UI <url> entries, want %d", found, len(wantURLs))
	}
}

// English shares the x-default URL, so it gets no <url> of its own.
func TestSitemapHasNoEnglishQueryURL(t *testing.T) {
	srv := newTestServer(t, withConfig(func(cfg *Config) { cfg.PublicURL = "https://example.com/" }))
	rr := doJSON(t, srv, http.MethodGet, "/sitemap.xml", nil, withHost("ignored.example.com"))
	wantStatus(t, rr, http.StatusOK)

	body := rr.Body.String()
	if strings.Contains(body, "?lang=en") {
		t.Error("sitemap.xml duplicates the default page under ?lang=en")
	}
	if got := strings.Count(body, "<loc>https://example.com/public/</loc>"); got != 1 {
		t.Errorf("the default page is listed %d times, want 1", got)
	}
}
