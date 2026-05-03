package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// --- GET /api/v1/runs --------------------------------------------------------

func TestListRunsEmpty(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list RunList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 0 {
		t.Fatalf("expected total=0, got %d", list.Total)
	}
}

func TestListRunsReturnsRuns(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJob(t, srv, "alpha.com", JobSucceeded)
	makeGraduatedJob(t, srv, "beta.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list RunList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListRunsFilterByDomain(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJob(t, srv, "alpha.com", JobSucceeded)
	makeGraduatedJob(t, srv, "beta.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs?domain=alpha", nil)
	srv.Handler().ServeHTTP(resp, req)
	var list RunList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	if list.Items[0].Domain != "alpha.com" {
		t.Fatalf("expected domain=alpha.com, got %q", list.Items[0].Domain)
	}
}

func TestListRunsFilterByTag(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "other.org", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs?tag=tld", nil)
	srv.Handler().ServeHTTP(resp, req)
	var list RunList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
}

func TestListRunsInvalidLimit(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs?limit=bad", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestListRunsInvalidTimeRange(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/runs?finished_after=2026-01-02T00:00:00Z&finished_before=2026-01-01T00:00:00Z", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

// --- GET /api/v1/runs/{id} ---------------------------------------------------

func TestGetRun(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+d.LatestRunID, nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var run Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.Domain != "example.com" {
		t.Fatalf("expected domain=example.com, got %q", run.Domain)
	}
}

func TestGetRunNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/no-such-run", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

// --- GET /api/v1/runs/{id}/result --------------------------------------------

func TestGetRunResult(t *testing.T) {
	srv := New(DefaultConfig())
	now := time.Now().UTC()
	job := Job{
		ID:         newID("job"),
		Domain:     "example.com",
		Status:     JobSucceeded,
		CreatedAt:  now,
		StartedAt:  now,
		FinishedAt: now,
		NameserverTimings: []NameserverTiming{
			{
				Nameserver: "ns1.example.com",
				Address:    "192.0.2.10",
				AvgMS:      24,
				MinMS:      20,
				MaxMS:      30,
				MedianMS:   22,
				StddevMS:   4,
				Count:      3,
			},
		},
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := srv.store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate job: %v", err)
	}
	d, ok := srv.store.GetDomainByName("example.com")
	if !ok {
		t.Fatal("expected domain")
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/runs/%s/result", d.LatestRunID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Status != JobSucceeded {
		t.Fatalf("expected status=succeeded, got %q", result.Status)
	}
	if len(result.NameserverTimings) != 1 {
		t.Fatalf("nameserver timings len = %d, want 1", len(result.NameserverTimings))
	}
}

func TestGetRunResultNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/no-such-run/result", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

// --- Scoring fields in run responses ----------------------------------------

func TestListRunsIncludeScore(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list RunList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	run := list.Items[0]
	if run.Score == nil {
		t.Fatal("run.Score is nil in list response")
	}
	if run.Grade == nil {
		t.Fatal("run.Grade is nil in list response")
	}
}

func TestGetRunIncludesScore(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+d.LatestRunID, nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var run Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.Score == nil {
		t.Fatal("run.Score is nil in GET /runs/{id} response")
	}
	if run.Grade == nil {
		t.Fatal("run.Grade is nil in GET /runs/{id} response")
	}
}

func TestGetRunResultIncludesScore(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/runs/%s/result", d.LatestRunID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Score == nil {
		t.Fatal("result.Score is nil in run result response")
	}
}

// --- ?grade= filter ----------------------------------------------------------

func TestListRunsFilterByGrade(t *testing.T) {
	srv := New(DefaultConfig())

	// No-entry job → score 100, grade A (no bonus criteria met → not A+).
	domA := makeGraduatedJobWithEntries(t, srv, "clean.example", nil)

	// Job with a CRITICAL entry → automatic F.
	domF := makeGraduatedJobWithEntries(t, srv, "broken.example", []engine.LogEntry{
		{Module: "BASIC", Tag: "NO_DELEGATION", Level: "CRITICAL"},
	})

	// Fetch grades so the test isn't hard-coded to specific scoring config.
	gradeOf := func(runID string) string {
		run, ok := srv.store.GetRun(runID)
		if !ok {
			t.Fatalf("GetRun(%q) returned false", runID)
		}
		if run.Grade == nil {
			t.Fatalf("run %q has nil Grade", runID)
		}
		return *run.Grade
	}
	gradeA := gradeOf(domA.LatestRunID)
	gradeF := gradeOf(domF.LatestRunID)

	if gradeA == gradeF {
		t.Skipf("both runs have the same grade %q; skipping grade filter test", gradeA)
	}

	// Filter by gradeA - should return exactly the clean run.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs?grade="+gradeA, nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var listA RunList
	if err := json.NewDecoder(resp.Body).Decode(&listA); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if listA.Total != 1 {
		t.Fatalf("grade=%s filter: expected total=1, got %d", gradeA, listA.Total)
	}
	if listA.Items[0].ID != domA.LatestRunID {
		t.Fatalf("grade=%s filter: expected run %q, got %q", gradeA, domA.LatestRunID, listA.Items[0].ID)
	}

	// Filter by gradeF - should return exactly the broken run.
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs?grade="+gradeF, nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var listF RunList
	if err := json.NewDecoder(resp.Body).Decode(&listF); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if listF.Total != 1 {
		t.Fatalf("grade=%s filter: expected total=1, got %d", gradeF, listF.Total)
	}
	if listF.Items[0].ID != domF.LatestRunID {
		t.Fatalf("grade=%s filter: expected run %q, got %q", gradeF, domF.LatestRunID, listF.Items[0].ID)
	}
}

// --- Score omission when ShowScoreAdmin=false --------------------------------

func TestRunResultOmitsScoreWhenAdminScoringDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowScoreAdmin = false
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
	d, ok := srv.store.GetDomainByName("example.com")
	if !ok {
		t.Fatal("expected domain")
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/runs/%s/result", d.LatestRunID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Score != nil {
		t.Fatal("expected Score to be nil when ShowScoreAdmin=false")
	}
	if result.NameserverTimings != nil {
		t.Fatal("expected NameserverTimings to be nil when ShowNameserverTimingsAdmin=false")
	}
}

func TestListRunsOmitsScoreWhenAdminScoringDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowScoreAdmin = false
	srv := New(cfg)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list RunList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Items) == 0 {
		t.Fatal("expected at least one run")
	}
	if list.Items[0].Score != nil || list.Items[0].Grade != nil {
		t.Fatal("expected Score/Grade to be nil in run list when ShowScoreAdmin=false")
	}
}

func TestGetRunOmitsScoreWhenAdminScoringDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowScoreAdmin = false
	srv := New(cfg)
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/runs/%s", d.LatestRunID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var run Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.Score != nil || run.Grade != nil {
		t.Fatal("expected Score/Grade to be nil when ShowScoreAdmin=false")
	}
}
