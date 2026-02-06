package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

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

func TestListJobsIncludesSeverityTotals(t *testing.T) {
	srv := New(DefaultConfig())
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", Domain: "alpha.example", Status: JobSucceeded, CreatedAt: base})
	_ = srv.store.SetResult("job1", JobResult{
		JobID:  "job1",
		Status: JobSucceeded,
		Summary: map[string]any{
			"levels": map[string]int{
				"NOTICE":   2,
				"WARNING":  1,
				"ERROR":    3,
				"CRITICAL": 0,
			},
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list JobList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one item")
	}
	totals := list.Items[0].SeverityTotals
	if totals["NOTICE"] != 2 || totals["WARNING"] != 1 || totals["ERROR"] != 3 || totals["CRITICAL"] != 0 {
		t.Fatalf("unexpected severity_totals: %+v", totals)
	}
}

func TestListJobsSortBySeverityTotals(t *testing.T) {
	srv := New(DefaultConfig())
	base := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	_, _ = srv.store.Create(Job{ID: "job1", Domain: "alpha.example", Status: JobSucceeded, CreatedAt: base})
	_, _ = srv.store.Create(Job{ID: "job2", Domain: "beta.example", Status: JobFailed, CreatedAt: base.Add(time.Second)})
	_, _ = srv.store.Create(Job{ID: "job3", Domain: "gamma.example", Status: JobFailed, CreatedAt: base.Add(2 * time.Second)})

	_ = srv.store.SetResult("job1", JobResult{
		JobID:  "job1",
		Status: JobSucceeded,
		Summary: map[string]any{
			"levels": map[string]int{"ERROR": 1},
		},
	})
	_ = srv.store.SetResult("job2", JobResult{
		JobID:  "job2",
		Status: JobFailed,
		Summary: map[string]any{
			"levels": map[string]int{"CRITICAL": 2},
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?sort=error_desc", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list JobList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Sort != string(JobSortErrorDesc) {
		t.Fatalf("expected sort metadata %q, got %q", JobSortErrorDesc, list.Sort)
	}
	if len(list.Items) != 3 || list.Items[0].ID != "job2" || list.Items[1].ID != "job1" {
		t.Fatalf("expected error_desc order job2, job1, ..., got %+v", list.Items)
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs?sort=critical_desc", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Sort != string(JobSortCriticalDesc) {
		t.Fatalf("expected sort metadata %q, got %q", JobSortCriticalDesc, list.Sort)
	}
	if len(list.Items) != 3 || list.Items[0].ID != "job2" {
		t.Fatalf("expected critical_desc order to prioritize job2")
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
	stored, ok := srv.store.Get(job.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
	if stored.Status != JobCanceled {
		t.Fatalf("expected status canceled, got %s", stored.Status)
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
	if err := srv.queue.Enqueue(job.ID); err != nil {
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

	stored, ok := srv.store.Get(job.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
	if stored.Status != JobCanceled {
		t.Fatalf("expected status canceled, got %s", stored.Status)
	}
	if stored.Error != "removed_from_queue" {
		t.Fatalf("expected error removed_from_queue, got %q", stored.Error)
	}

	result, ok := srv.store.GetResult(job.ID)
	if !ok {
		t.Fatalf("expected job result")
	}
	if result.Status != JobCanceled {
		t.Fatalf("expected result status canceled, got %s", result.Status)
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
}
