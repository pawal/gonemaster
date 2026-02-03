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
