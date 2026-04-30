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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

var metricsRequestNonce uint32
var prometheusSampleLine = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^{}]*\})? [-+]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][-+]?[0-9]+)?$`)

func getMetricsSnapshot(t *testing.T, srv *Server) MetricsSnapshot {
	t.Helper()
	resp := httptest.NewRecorder()
	nonce := atomic.AddUint32(&metricsRequestNonce, 1)
	limit := int((nonce % 100) + 1)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/metrics?limit_domains=%d&limit_batches=%d", limit, limit), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var snapshot MetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	return snapshot
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
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}
	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected job id")
	}

	resp = httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID, nil)
	srv.Handler().ServeHTTP(resp, getReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
}

func TestGetJobResultOmitsNameserverTimingsWhenAdminDisplayDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowNameserverTimingsAdmin = false
	srv := New(cfg)

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
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := srv.store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate job: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+job.ID+"/result", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body)
	}

	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.NameserverTimings != nil {
		t.Fatal("expected NameserverTimings to be nil when ShowNameserverTimingsAdmin=false")
	}
}

func TestCreateJobCSRFRejectsMismatchedOrigin(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "csrf_origin_mismatch" {
		t.Fatalf("expected csrf_origin_mismatch, got %q", out.Error.Code)
	}
}

func TestCreateJobCSRFAcceptsMatchingOrigin(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}
}

func TestCreateJobMinLevel(t *testing.T) {
	srv := New(DefaultConfig())

	payload := `{"domain":"example.com","min_level":"WARNING"}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}
	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	stored, ok := srv.store.Get(created.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
	if stored.MinLevel != "WARNING" {
		t.Fatalf("expected min_level WARNING, got %q", stored.MinLevel)
	}
}

func TestCreateJobValidation(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":""}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestCreateJobWithUndelegatedInput(t *testing.T) {
	srv := New(DefaultConfig())

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
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}
	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	srv := New(DefaultConfig())

	payload := `{
		"domain":"example.com",
		"nameservers":[{"ns":"ns1.example.com","ip":"not-an-ip"}]
	}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "invalid_undelegated" {
		t.Fatalf("expected invalid_undelegated, got %q", out.Error.Code)
	}
}

func TestCreateJobRejectsMalformedUndelegatedDSInput(t *testing.T) {
	srv := New(DefaultConfig())

	payload := `{
		"domain":"example.com",
		"ds_info":[{"keytag":12345,"algorithm":13,"digtype":2,"digest":"not-hex"}]
	}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "invalid_undelegated" {
		t.Fatalf("expected invalid_undelegated, got %q", out.Error.Code)
	}
}

func TestListJobs(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	srv.Handler().ServeHTTP(resp, listReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list JobList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected total 1, got %d", list.Total)
	}
}

func TestListJobsPaginationAndSort(t *testing.T) {
	srv := New(DefaultConfig())
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", Domain: "gamma.example", Status: JobQueued, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", Domain: "alpha.example", Status: JobQueued, CreatedAt: base.Add(time.Second)})
	_, _ = srv.store.Create(Job{ID: "job3", Domain: "beta.example", Status: JobQueued, CreatedAt: base.Add(2 * time.Second)})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?limit=2&sort=domain_asc&page=1", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var first JobList
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first.Total != 3 || len(first.Items) != 2 {
		t.Fatalf("expected first page with 2 of 3 jobs")
	}
	if first.Items[0].ID != "job2" || first.Items[1].ID != "job3" {
		t.Fatalf("unexpected sort order: got %s then %s", first.Items[0].ID, first.Items[1].ID)
	}
	if first.NextCursor != "2" || first.Sort != string(JobSortDomainAsc) {
		t.Fatalf("expected next cursor and sort metadata")
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs?limit=2&sort=domain_asc&cursor=2", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var second JobList
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "job1" {
		t.Fatalf("expected second page to contain only job1")
	}
	if second.PrevCursor != "0" || second.NextCursor != "" {
		t.Fatalf("expected prev cursor 0 and no next cursor, got prev=%q next=%q", second.PrevCursor, second.NextCursor)
	}
}

func TestListJobsRejectsInvalidCursor(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?cursor=not-a-number", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "invalid_cursor" {
		t.Fatalf("expected invalid_cursor, got %q", out.Error.Code)
	}
}

func TestListJobsFiltersByDomainAndTimeRange(t *testing.T) {
	srv := New(DefaultConfig())
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", Domain: "alpha.example", Status: JobQueued, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", Domain: "beta.example", Status: JobQueued, CreatedAt: base.Add(time.Second)})
	_, _ = srv.store.Create(Job{ID: "job3", Domain: "alpha.internal", Status: JobQueued, CreatedAt: base.Add(2 * time.Second)})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/jobs?domain=alpha&created_after=2026-02-03T00:00:00Z&created_before=2026-02-03T00:00:01Z",
		nil,
	)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list JobList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != "job1" {
		t.Fatalf("expected only job1 to match filters, got %+v", list.Items)
	}
}

func TestListJobsFiltersByBatchIDAndSupportsBatchSort(t *testing.T) {
	srv := New(DefaultConfig())
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", BatchID: "batch_b", Domain: "alpha.example", Status: JobQueued, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", BatchID: "batch_a", Domain: "beta.example", Status: JobQueued, CreatedAt: base.Add(time.Second)})
	_, _ = srv.store.Create(Job{ID: "job3", BatchID: "batch_b", Domain: "gamma.example", Status: JobQueued, CreatedAt: base.Add(2 * time.Second)})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?sort=batch_id_asc", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list JobList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Sort != string(JobSortBatchIDAsc) {
		t.Fatalf("expected sort metadata %q, got %q", JobSortBatchIDAsc, list.Sort)
	}
	if len(list.Items) != 3 || list.Items[0].ID != "job2" {
		t.Fatalf("expected batch_id_asc to order batch_a before batch_b")
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs?batch_id=batch_b&sort=batch_id_desc", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	srv := New(DefaultConfig())
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", Domain: "alpha.example", Status: JobQueued, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", Domain: "beta.example", Status: JobQueued, CreatedAt: base.Add(time.Second)})

	for _, sort := range []string{"error_desc", "critical_desc"} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?sort="+sort, nil)
		srv.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("sort=%s: expected 200, got %d", sort, resp.Code)
		}
		var list JobList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("sort=%s: decode: %v", sort, err)
		}
		if list.Total != 2 {
			t.Fatalf("sort=%s: expected 2 items, got %d", sort, list.Total)
		}
	}
}

func TestListJobsRejectsInvalidSort(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?sort=bad_sort", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "invalid_sort" {
		t.Fatalf("expected invalid_sort, got %q", out.Error.Code)
	}
}

func TestListJobsRejectsInvalidSeverity(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?severity=bad_filter", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "invalid_severity" {
		t.Fatalf("expected invalid_severity, got %q", out.Error.Code)
	}
}

func TestListJobsRejectsInvalidCreatedBeforeAndRange(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?created_before=not-a-date", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "invalid_created_before" {
		t.Fatalf("expected invalid_created_before, got %q", out.Error.Code)
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/jobs?created_after=2026-02-03T00:00:02Z&created_before=2026-02-03T00:00:01Z",
		nil,
	)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "invalid_time_range" {
		t.Fatalf("expected invalid_time_range, got %q", out.Error.Code)
	}
}

func TestBatchSubmit(t *testing.T) {
	srv := New(DefaultConfig())

	payload := `{"domains":["example.com","example.net"]}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.Code)
	}
	var out JobBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.BatchID == "" || len(out.JobIDs) != 2 {
		t.Fatalf("expected batch id and 2 job ids")
	}
}

func TestBatchSubmitRejectsUndelegatedFields(t *testing.T) {
	srv := New(DefaultConfig())

	payload := `{"domains":["example.com"],"nameservers":[{"ns":"ns1.example.com","ip":"192.0.2.1"}]}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "undelegated_not_supported_for_batch" {
		t.Fatalf("expected undelegated_not_supported_for_batch, got %q", out.Error.Code)
	}
}

func TestBatchSummary(t *testing.T) {
	srv := New(DefaultConfig())

	payload := `{"domains":["example.com","example.net"]}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.Code)
	}
	var batch JobBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batch); err != nil {
		t.Fatalf("decode: %v", err)
	}

	resp = httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/batches/"+batch.BatchID, nil)
	srv.Handler().ServeHTTP(resp, getReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var summary BatchSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	srv := New(DefaultConfig())

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

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/batches/"+batchID, nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var summary BatchSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary.Tag != "se-domains" {
		t.Fatalf("expected tag 'se-domains', got %q", summary.Tag)
	}
}

func TestBatchSummaryPaginationAndSort(t *testing.T) {
	srv := New(DefaultConfig())
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

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/batches/"+batchID+"?limit=1&sort=created_at_asc&cursor=1", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var summary BatchSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	srv := New(DefaultConfig())
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

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/batches/"+batchID+"?status=failed&domain=beta&created_after=2026-02-03T00:00:00Z&sort=domain_asc",
		nil,
	)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var summary BatchSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary.Total != 1 || len(summary.Items) != 1 || summary.Items[0].ID != "job2" {
		t.Fatalf("expected filtered batch summary with job2 only, got %+v", summary.Items)
	}
	if summary.Sort != string(JobSortDomainAsc) {
		t.Fatalf("expected sort metadata %q, got %q", JobSortDomainAsc, summary.Sort)
	}
	if summary.StatusCounts["queued"] != 1 || summary.StatusCounts["failed"] != 1 {
		t.Fatalf("expected full-batch status counts, got %+v", summary.StatusCounts)
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/batches/"+batchID+"?status=running", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty filtered result, got %d", resp.Code)
	}
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary.Total != 0 || len(summary.Items) != 0 {
		t.Fatalf("expected no matching items, got %+v", summary.Items)
	}
}

func TestBatchSummaryIncludesGrades(t *testing.T) {
	srv := New(DefaultConfig())
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
		if _, err := srv.store.Create(job); err != nil {
			t.Fatalf("create job: %v", err)
		}
		if err := srv.store.GraduateJob(job, nil); err != nil {
			t.Fatalf("graduate job: %v", err)
		}
		// Also record batch metadata so the handler finds it.
		_ = srv.store.CreateBatch(Batch{ID: batchID, CreatedAt: now, DomainCount: 2})
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/batches/"+batchID, nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var summary BatchSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}
	var created Job
	_ = json.NewDecoder(resp.Body).Decode(&created)

	resp = httptest.NewRecorder()
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+created.ID+"/cancel", nil)
	srv.Handler().ServeHTTP(resp, cancelReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var canceled Job
	_ = json.NewDecoder(resp.Body).Decode(&canceled)
	if canceled.Status != JobCanceled {
		t.Fatalf("expected status canceled, got %s", canceled.Status)
	}
}

func TestCancelJobCSRFRejectsMismatchedOrigin(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}
	var created Job
	_ = json.NewDecoder(resp.Body).Decode(&created)

	resp = httptest.NewRecorder()
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+created.ID+"/cancel", nil)
	cancelReq.Header.Set("Origin", "https://attacker.example")
	srv.Handler().ServeHTTP(resp, cancelReq)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.Code)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "csrf_origin_mismatch" {
		t.Fatalf("expected csrf_origin_mismatch, got %q", out.Error.Code)
	}

	stored, ok := srv.store.Get(created.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
	if stored.Status == JobCanceled {
		t.Fatalf("expected job to remain uncanceled")
	}
}

func TestCancelJobTriggersContextCancel(t *testing.T) {
	srv := New(DefaultConfig())

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

	resp := httptest.NewRecorder()
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+job.ID+"/cancel", nil)
	srv.Handler().ServeHTTP(resp, cancelReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
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
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	pauseReq := httptest.NewRequest(http.MethodPost, "/api/v1/queue/pause", nil)
	srv.Handler().ServeHTTP(resp, pauseReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/queue/resume", nil)
	srv.Handler().ServeHTTP(resp, resumeReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestQueueReorderValidation(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	payload := `{"job_ids":["job1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/reorder", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestQueueRemove(t *testing.T) {
	srv := New(DefaultConfig())
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

	resp := httptest.NewRecorder()
	payload := `{"job_ids":["job1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/remove", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

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
	srv := New(DefaultConfig())
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}
	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

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
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}
	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	resp = httptest.NewRecorder()
	pauseReq := httptest.NewRequest(http.MethodPost, "/api/v1/queue/pause", nil)
	srv.Handler().ServeHTTP(resp, pauseReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	snapshot := getMetricsSnapshot(t, srv)
	if !snapshot.Health.QueuePaused {
		t.Fatal("queue_paused = false, want true")
	}

	resp = httptest.NewRecorder()
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+created.ID+"/cancel", nil)
	srv.Handler().ServeHTTP(resp, cancelReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
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

	resp = httptest.NewRecorder()
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/queue/resume", nil)
	srv.Handler().ServeHTTP(resp, resumeReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	snapshot = getMetricsSnapshot(t, srv)
	if snapshot.Health.QueuePaused {
		t.Fatal("queue_paused = true, want false")
	}
}

func TestMetricsTracksAPIRequestsByRouteMethodStatusAndErrorCode(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	payload := `{"job_ids":["missing"]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/queue/reorder", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-missing/cancel", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}

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
	srv := New(DefaultConfig())
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

	resp := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"cancel.example"}`))
	createReq.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, createReq)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}
	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created job: %v", err)
	}

	resp = httptest.NewRecorder()
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+created.ID+"/cancel", nil)
	srv.Handler().ServeHTTP(resp, cancelReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	resultReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobSuccess.ID+"/result?locale=sv", nil)
	srv.Handler().ServeHTTP(resp, resultReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

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
	srv := New(DefaultConfig())
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
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		srv.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", tc.path, resp.Code)
		}
		var out ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("%s: decode error: %v", tc.path, err)
		}
		if out.Error.Code != tc.wantCode {
			t.Fatalf("%s: error code = %q, want %q", tc.path, out.Error.Code, tc.wantCode)
		}
	}
}

func TestMetricsEndpointSupportsIncludeWindowAndLimits(t *testing.T) {
	srv := New(DefaultConfig())
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

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?include=health,insights,trends&window=1h&limit_domains=1&limit_batches=1", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode metrics payload: %v", err)
	}
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
	srv := New(DefaultConfig())
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

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?format=prom&include=trends&window=1h&limit_domains=1&limit_batches=1", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
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
	srv := New(DefaultConfig())
	base := time.Date(2026, 2, 9, 10, 0, 0, 0, time.UTC)
	now := base
	srv.metrics.nowFn = func() time.Time { return now }

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var first MetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatalf("decode first metrics: %v", err)
	}
	if first.Jobs.SubmittedTotal != 0 {
		t.Fatalf("first submitted_total = %d, want 0", first.Jobs.SubmittedTotal)
	}

	now = base.Add(100 * time.Millisecond)
	createResp := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(`{"domain":"cached.example"}`))
	createReq.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.Code)
	}

	now = base.Add(500 * time.Millisecond)
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var second MetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatalf("decode second metrics: %v", err)
	}
	if !second.GeneratedAt.Equal(first.GeneratedAt) {
		t.Fatalf("second generated_at = %s, want cached %s", second.GeneratedAt, first.GeneratedAt)
	}
	if second.Jobs.SubmittedTotal != first.Jobs.SubmittedTotal {
		t.Fatalf("second submitted_total = %d, want cached %d", second.Jobs.SubmittedTotal, first.Jobs.SubmittedTotal)
	}

	now = base.Add(2 * time.Second)
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var third MetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&third); err != nil {
		t.Fatalf("decode third metrics: %v", err)
	}
	if !third.GeneratedAt.After(second.GeneratedAt) {
		t.Fatalf("third generated_at = %s, want after %s", third.GeneratedAt, second.GeneratedAt)
	}
	if third.Jobs.SubmittedTotal != 1 {
		t.Fatalf("third submitted_total = %d, want 1", third.Jobs.SubmittedTotal)
	}
}

func TestHealthAndMetrics(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	srv.Handler().ServeHTTP(resp, healthReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	metricsReq := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	srv.Handler().ServeHTTP(resp, metricsReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content-type, got %q", got)
	}
	var metrics MetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
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
	srv := New(DefaultConfig())

	t.Run("GET returns locales list", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/locales", nil)
		srv.Handler().ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.Code)
		}
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
		found := false
		for _, l := range body.Locales {
			if l == "en" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected \"en\" in locales, got %v", body.Locales)
		}
	})

	t.Run("POST returns 405", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/locales", nil)
		srv.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", resp.Code)
		}
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
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/purge", reqBody)
		req.Header.Set("Content-Type", "application/json")
		srv.Handler().ServeHTTP(resp, req)
		return resp
	}

	t.Run("400 when no retention configured and no body", func(t *testing.T) {
		srv := New(DefaultConfig())
		resp := postPurge(srv, "")
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.Code)
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if errObj, _ := body["error"].(map[string]any); errObj["code"] != "retention_not_configured" {
			t.Fatalf("expected retention_not_configured, got %v", errObj)
		}
	})

	t.Run("200 with older_than_days in body", func(t *testing.T) {
		srv := New(DefaultConfig())
		old := time.Now().UTC().Add(-48 * time.Hour)
		job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
		if _, err := srv.store.Create(job); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := srv.store.GraduateJob(job, nil); err != nil {
			t.Fatalf("graduate: %v", err)
		}

		resp := postPurge(srv, `{"older_than_days":1}`)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var body map[string]int64
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body["purged_jobs"] != 1 {
			t.Fatalf("expected purged_jobs=1, got %d", body["purged_jobs"])
		}
	})

	t.Run("200 using server retention_days when body omits older_than_days", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Database.RetentionDays = 1
		srv := New(cfg)
		old := time.Now().UTC().Add(-48 * time.Hour)
		job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
		if _, err := srv.store.Create(job); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := srv.store.GraduateJob(job, nil); err != nil {
			t.Fatalf("graduate: %v", err)
		}

		resp := postPurge(srv, "")
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var body map[string]int64
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body["purged_jobs"] != 1 {
			t.Fatalf("expected purged_jobs=1, got %d", body["purged_jobs"])
		}
	})

	t.Run("200 with zero purged when no old jobs", func(t *testing.T) {
		srv := New(DefaultConfig())
		resp := postPurge(srv, `{"older_than_days":90}`)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.Code)
		}
		var body map[string]int64
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body["purged_jobs"] != 0 {
			t.Fatalf("expected purged_jobs=0, got %d", body["purged_jobs"])
		}
	})

	t.Run("GET returns 405", func(t *testing.T) {
		srv := New(DefaultConfig())
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/purge", nil)
		srv.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", resp.Code)
		}
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
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	srv := New(DefaultConfig())
	payload := `{"domains":["example.com","example.net"]}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}
	var out JobBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
			cfg := DefaultConfig()
			cfg.PublicURL = tt.publicURL
			srv := New(cfg)

			req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
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
		wantLoc   string
	}{
		{
			name:      "configured root URL",
			publicURL: "https://example.com/",
			host:      "ignored.example.com",
			wantLoc:   "<loc>https://example.com/</loc>",
		},
		{
			name:      "configured subpath URL",
			publicURL: "https://example.com/public/",
			host:      "ignored.example.com",
			wantLoc:   "<loc>https://example.com/public/</loc>",
		},
		{
			name:      "auto-detected from host",
			publicURL: "",
			host:      "myhost.example.com",
			wantLoc:   "<loc>http://myhost.example.com/</loc>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.PublicURL = tt.publicURL
			srv := New(cfg)

			req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/xml; charset=utf-8" {
				t.Fatalf("Content-Type = %q", ct)
			}
			body := rr.Body.String()
			if !strings.Contains(body, tt.wantLoc) {
				t.Fatalf("sitemap.xml missing %q\ngot: %s", tt.wantLoc, body)
			}
			for _, lang := range sitemapLangs {
				if !strings.Contains(body, `hreflang="`+lang+`"`) {
					t.Fatalf("sitemap.xml missing hreflang=%q", lang)
				}
			}
		})
	}
}
