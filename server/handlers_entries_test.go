package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// makeGraduatedJobWithEntries graduates a job with the given log entries.
// Returns the domain created by graduation.
func makeGraduatedJobWithEntries(t *testing.T, srv *Server, domain string, entries []engine.LogEntry) Domain {
	t.Helper()
	now := time.Now().UTC()
	job := Job{
		ID:        newID("job"),
		Domain:    domain,
		Status:    JobSucceeded,
		CreatedAt: now,
		StartedAt: now,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	job.FinishedAt = now
	if err := srv.store.GraduateJob(job, entries); err != nil {
		t.Fatalf("graduate job: %v", err)
	}
	d, ok := srv.store.GetDomainByName(domain)
	if !ok {
		t.Fatalf("expected domain %q after graduation", domain)
	}
	return d
}

// --- GET /api/v1/entries -----------------------------------------------------

func TestListEntriesEmpty(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list EntryList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 0 {
		t.Fatalf("expected total=0, got %d", list.Total)
	}
}

func TestListEntriesReturnsEntries(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Testcase: "Basic01", Tag: "BASIC_WORKING_GLUE", Level: "INFO"},
		{Module: "DNSSEC", Testcase: "DNSSEC02", Tag: "NO_KEYS", Level: "WARNING"},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list EntryList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListEntriesFilterByRun(t *testing.T) {
	srv := New(DefaultConfig())
	d1 := makeGraduatedJobWithEntries(t, srv, "alpha.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
	})
	makeGraduatedJobWithEntries(t, srv, "beta.com", []engine.LogEntry{
		{Module: "DNSSEC", Level: "WARNING"},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?run="+d1.LatestRunID, nil)
	srv.Handler().ServeHTTP(resp, req)
	var list EntryList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	if list.Items[0].Module != "Basic" {
		t.Fatalf("expected module=Basic, got %q", list.Items[0].Module)
	}
}

func TestListEntriesFilterByLevel(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
		{Module: "DNSSEC", Level: "WARNING"},
		{Module: "Nameserver", Level: "WARNING"},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?level=WARNING", nil)
	srv.Handler().ServeHTTP(resp, req)
	var list EntryList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListEntriesFilterByModule(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
		{Module: "DNSSEC", Level: "WARNING"},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?module=DNSSEC", nil)
	srv.Handler().ServeHTTP(resp, req)
	var list EntryList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
}

func TestListEntriesFilterByTag(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d := makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
	})
	makeGraduatedJobWithEntries(t, srv, "other.org", []engine.LogEntry{
		{Module: "DNSSEC", Level: "WARNING"},
	})
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?tag=tld", nil)
	srv.Handler().ServeHTTP(resp, req)
	var list EntryList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
}

func TestListEntriesLatestOnly(t *testing.T) {
	srv := New(DefaultConfig())
	// Graduate the same domain twice; latest_only should return only entries from the second run.
	now := time.Now().UTC()
	job1 := Job{ID: newID("job"), Domain: "example.com", Status: JobSucceeded,
		CreatedAt: now, StartedAt: now, FinishedAt: now}
	_, _ = srv.store.Create(job1)
	_ = srv.store.GraduateJob(job1, []engine.LogEntry{{Module: "Old", Level: "INFO"}})

	job2 := Job{ID: newID("job"), Domain: "example.com", Status: JobSucceeded,
		CreatedAt: now.Add(time.Second), StartedAt: now.Add(time.Second), FinishedAt: now.Add(time.Second)}
	_, _ = srv.store.Create(job2)
	_ = srv.store.GraduateJob(job2, []engine.LogEntry{{Module: "New", Level: "WARNING"}})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?latest=true", nil)
	srv.Handler().ServeHTTP(resp, req)
	var list EntryList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1 with latest=true, got %d", list.Total)
	}
	if list.Items[0].Module != "New" {
		t.Fatalf("expected module=New, got %q", list.Items[0].Module)
	}
}

func TestListEntriesCSV(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Testcase: "Basic01", Tag: "BASIC_WORKING_GLUE", Level: "INFO"},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?format=csv", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	ct := resp.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("expected text/csv content-type, got %q", ct)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "module") {
		t.Fatalf("expected CSV header row, got: %s", body)
	}
	if !strings.Contains(body, "Basic") {
		t.Fatalf("expected Basic in CSV body, got: %s", body)
	}
}

func TestListEntriesInvalidLimit(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?limit=0", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestListEntriesInvalidDomain(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?domain=abc", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}
