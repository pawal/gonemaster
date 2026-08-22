package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// makeGraduatedJobWithEntries graduates a job with the given log entries.
// Returns the domain created by graduation.
func makeGraduatedJobWithEntries(t *testing.T, srv *Server, domain string, entries []engine.LogEntry) Domain {
	t.Helper()
	seedGraduatedRun(t, srv.store, runSpec{Domain: domain, Entries: entries})
	d, ok := srv.store.GetDomainByName(domain)
	if !ok {
		t.Fatalf("expected domain %q after graduation", domain)
	}
	return d
}

// --- GET /api/v1/entries -----------------------------------------------------

func TestListEntriesEmpty(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries", nil)
	list := mustJSON[EntryList](t, resp, http.StatusOK)
	if list.Total != 0 {
		t.Fatalf("expected total=0, got %d", list.Total)
	}
}

func TestListEntriesReturnsEntries(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Testcase: "Basic01", Tag: "BASIC_WORKING_GLUE", Level: "INFO"},
		{Module: "DNSSEC", Testcase: "DNSSEC02", Tag: "NO_KEYS", Level: "WARNING"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries", nil)
	list := mustJSON[EntryList](t, resp, http.StatusOK)
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListEntriesFilterByRun(t *testing.T) {
	srv := newTestServer(t)
	d1 := makeGraduatedJobWithEntries(t, srv, "alpha.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
	})
	makeGraduatedJobWithEntries(t, srv, "beta.com", []engine.LogEntry{
		{Module: "DNSSEC", Level: "WARNING"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?run="+d1.LatestRunID, nil)
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
	srv := newTestServer(t)
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
		{Module: "DNSSEC", Level: "WARNING"},
		{Module: "Nameserver", Level: "WARNING"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?level=WARNING", nil)
	var list EntryList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListEntriesFilterByModule(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
		{Module: "DNSSEC", Level: "WARNING"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?module=DNSSEC", nil)
	var list EntryList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
}

func TestListEntriesFilterByTag(t *testing.T) {
	srv := newTestServer(t)
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

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?tag=tld", nil)
	var list EntryList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
}

func TestListEntriesUnknownTagReturns404(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?tag=doesnotexist", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestListEntriesLatestOnly(t *testing.T) {
	srv := newTestServer(t)
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

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?latest=true", nil)
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
	srv := newTestServer(t)
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Testcase: "Basic01", Tag: "BASIC_WORKING_GLUE", Level: "INFO"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?format=csv", nil)
	wantStatus(t, resp, http.StatusOK)
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

func TestListEntriesIncludesDomainName(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Testcase: "Basic01", Tag: "BASIC_WORKING_GLUE", Level: "INFO"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries", nil)
	list := mustJSON[EntryList](t, resp, http.StatusOK)
	if len(list.Items) == 0 {
		t.Fatal("expected at least one entry")
	}
	if list.Items[0].Domain != "example.com" {
		t.Fatalf("expected domain=example.com in entry, got %q", list.Items[0].Domain)
	}
}

func TestListEntriesCSVIncludesDomainColumn(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "Basic", Testcase: "Basic01", Tag: "BASIC_WORKING_GLUE", Level: "INFO"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?format=csv", nil)
	wantStatus(t, resp, http.StatusOK)
	body := resp.Body.String()
	if !strings.Contains(body, "domain") {
		t.Fatalf("expected 'domain' column in CSV header: %s", body)
	}
	if !strings.Contains(body, "example.com") {
		t.Fatalf("expected domain name in CSV body: %s", body)
	}
}

func TestListEntriesInvalidLimit(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?limit=0", nil)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestListEntriesInvalidDomain(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?domain=abc", nil)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestListEntriesCSVIncludesScoreAndGrade(t *testing.T) {
	srv := newTestServer(t)
	// Graduate a job with a WARNING entry - scoring will produce a non-trivial grade.
	makeGraduatedJobWithEntries(t, srv, "example.com", []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "DS07", Tag: "DS07_NOT_SIGNED", Level: "WARNING"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/entries?format=csv", nil)
	wantStatus(t, resp, http.StatusOK)
	body := resp.Body.String()
	// Header row must contain score and grade columns.
	if !strings.Contains(body, "score") {
		t.Fatalf("expected 'score' column in CSV header: %s", body)
	}
	if !strings.Contains(body, "grade") {
		t.Fatalf("expected 'grade' column in CSV header: %s", body)
	}
	// Data row must have a non-empty grade value (the run was scored at graduation).
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header + at least one data row, got: %s", body)
	}
	dataRow := lines[1]
	if dataRow == "" {
		t.Fatalf("expected non-empty data row")
	}
	// The grade column should not be empty - scoring runs at graduation.
	fields := strings.Split(dataRow, ",")
	gradeIdx := -1
	headerFields := strings.Split(lines[0], ",")
	for i, h := range headerFields {
		if h == "grade" {
			gradeIdx = i
			break
		}
	}
	if gradeIdx < 0 {
		t.Fatalf("'grade' column not found in header: %s", lines[0])
	}
	if gradeIdx >= len(fields) || fields[gradeIdx] == "" {
		t.Fatalf("expected non-empty grade in data row, got fields: %v", fields)
	}
}
