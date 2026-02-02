package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndGetJob(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
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
	getReq := httptest.NewRequest(http.MethodGet, "/jobs/"+created.ID, nil)
	srv.Handler().ServeHTTP(resp, getReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
}

func TestCreateJobValidation(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"domain":""}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestListJobs(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/jobs", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/jobs/batch", bytes.NewBufferString(payload))
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

func TestCancelJob(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}
	var created Job
	_ = json.NewDecoder(resp.Body).Decode(&created)

	resp = httptest.NewRecorder()
	cancelReq := httptest.NewRequest(http.MethodPost, "/jobs/"+created.ID+"/cancel", nil)
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

func TestQueueEndpoints(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	pauseReq := httptest.NewRequest(http.MethodPost, "/queue/pause", nil)
	srv.Handler().ServeHTTP(resp, pauseReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	resumeReq := httptest.NewRequest(http.MethodPost, "/queue/resume", nil)
	srv.Handler().ServeHTTP(resp, resumeReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestQueueReorderValidation(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	payload := `{"job_ids":["job1"]}`
	req := httptest.NewRequest(http.MethodPost, "/queue/reorder", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestHealthAndMetrics(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.Handler().ServeHTTP(resp, healthReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	resp = httptest.NewRecorder()
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Handler().ServeHTTP(resp, metricsReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}
