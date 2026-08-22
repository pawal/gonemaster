package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// --- GET /api/v1/runs --------------------------------------------------------

func TestListRunsEmpty(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs", nil)
	list := mustJSON[RunList](t, resp, http.StatusOK)
	if list.Total != 0 {
		t.Fatalf("expected total=0, got %d", list.Total)
	}
}

func TestListRunsReturnsRuns(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJob(t, srv, "alpha.com", JobSucceeded)
	makeGraduatedJob(t, srv, "beta.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs", nil)
	list := mustJSON[RunList](t, resp, http.StatusOK)
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListRunsFilterByDomain(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJob(t, srv, "alpha.com", JobSucceeded)
	makeGraduatedJob(t, srv, "beta.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs?domain=alpha", nil)
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
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "other.org", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs?tag=tld", nil)
	var list RunList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
}

func TestListRunsFilterByEventTag(t *testing.T) {
	// event_tag filters runs by the presence of a log-event tag in their
	// entries. This is distinct from the domain-tag tag= filter above.
	srv := newTestServer(t)
	seedBatchRuns(t, srv, "batch_evt", []seedRun{
		{domain: "signed.example", entries: []engine.LogEntry{
			{Timestamp: 1.0, Module: "DNSSEC", Testcase: "DNSSEC07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		}},
		{domain: "consistent.example", entries: []engine.LogEntry{
			{Timestamp: 1.0, Module: "Consistency", Testcase: "Consistency04", Tag: "CN04_IPV4_SAME_PREFIX", Level: "WARNING"},
		}},
	})

	get := func(query string) RunList {
		resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs"+query, nil)
		var list RunList
		_ = json.NewDecoder(resp.Body).Decode(&list)
		return list
	}

	// Matches only the run that emitted the tag.
	if list := get("?event_tag=DS07_NOT_SIGNED"); list.Total != 1 || len(list.Items) != 1 || list.Items[0].Domain != "signed.example" {
		t.Fatalf("event_tag=DS07_NOT_SIGNED should match only signed.example, got %+v", list)
	}
	// A tag no run emitted matches nothing.
	if list := get("?event_tag=NO_SUCH_TAG"); list.Total != 0 {
		t.Fatalf("unknown event_tag should match nothing, got %+v", list)
	}
	// Without event_tag, both runs are returned (no accidental narrowing).
	if list := get(""); list.Total != 2 {
		t.Fatalf("no filter should return both runs, got %d", list.Total)
	}
	// event_tag intersects with the finish-time window.
	if list := get("?event_tag=DS07_NOT_SIGNED&finished_before=2000-01-01T00:00:00Z"); list.Total != 0 {
		t.Fatalf("event_tag plus a past finished_before should be empty, got %+v", list)
	}
}

func TestListRunsInvalidLimit(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs?limit=bad", nil)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestListRunsInvalidTimeRange(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs?finished_after=2026-01-02T00:00:00Z&finished_before=2026-01-01T00:00:00Z", nil)
	wantStatus(t, resp, http.StatusBadRequest)
}

// --- GET /api/v1/runs/{id} ---------------------------------------------------

func TestGetRun(t *testing.T) {
	srv := newTestServer(t)
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs/"+d.LatestRunID, nil)
	run := mustJSON[Run](t, resp, http.StatusOK)
	if run.Domain != "example.com" {
		t.Fatalf("expected domain=example.com, got %q", run.Domain)
	}
}

func TestGetRunNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs/no-such-run", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

// --- GET /api/v1/runs/{id}/result --------------------------------------------

func TestGetRunResult(t *testing.T) {
	srv := newTestServer(t)
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

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/runs/%s/result", d.LatestRunID), nil)
	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if result.Status != JobSucceeded {
		t.Fatalf("expected status=succeeded, got %q", result.Status)
	}
	if len(result.NameserverTimings) != 1 {
		t.Fatalf("nameserver timings len = %d, want 1", len(result.NameserverTimings))
	}
}

func TestGetRunResultNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs/no-such-run/result", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

// --- Scoring fields in run responses ----------------------------------------

func TestListRunsIncludeScore(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs", nil)
	list := mustJSON[RunList](t, resp, http.StatusOK)
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
	srv := newTestServer(t)
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs/"+d.LatestRunID, nil)
	run := mustJSON[Run](t, resp, http.StatusOK)
	if run.Score == nil {
		t.Fatal("run.Score is nil in GET /runs/{id} response")
	}
	if run.Grade == nil {
		t.Fatal("run.Grade is nil in GET /runs/{id} response")
	}
}

func TestGetRunResultIncludesScore(t *testing.T) {
	srv := newTestServer(t)
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/runs/%s/result", d.LatestRunID), nil)
	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if result.Score == nil {
		t.Fatal("result.Score is nil in run result response")
	}
}

// --- ?grade= filter ----------------------------------------------------------

func TestListRunsFilterByGrade(t *testing.T) {
	srv := newTestServer(t)

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
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs?grade="+gradeA, nil)
	listA := mustJSON[RunList](t, resp, http.StatusOK)
	if listA.Total != 1 {
		t.Fatalf("grade=%s filter: expected total=1, got %d", gradeA, listA.Total)
	}
	if listA.Items[0].ID != domA.LatestRunID {
		t.Fatalf("grade=%s filter: expected run %q, got %q", gradeA, domA.LatestRunID, listA.Items[0].ID)
	}

	// Filter by gradeF - should return exactly the broken run.
	resp = doJSON(t, srv, http.MethodGet, "/api/v1/runs?grade="+gradeF, nil)
	listF := mustJSON[RunList](t, resp, http.StatusOK)
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

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/runs/%s/result", d.LatestRunID), nil)
	result := mustJSON[JobResult](t, resp, http.StatusOK)
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

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs", nil)
	list := mustJSON[RunList](t, resp, http.StatusOK)
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

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/runs/%s", d.LatestRunID), nil)
	run := mustJSON[Run](t, resp, http.StatusOK)
	if run.Score != nil || run.Grade != nil {
		t.Fatal("expected Score/Grade to be nil when ShowScoreAdmin=false")
	}
}
