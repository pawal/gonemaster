package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// makeGraduatedJob is a helper that creates a job and immediately graduates it.
// Returns the domain that was created by graduation.
func makeGraduatedJob(t *testing.T, srv *Server, domain string, status JobStatus) Domain {
	t.Helper()
	now := time.Now().UTC()
	job := Job{
		ID:         newID("job"),
		Domain:     domain,
		Status:     status,
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
	d, ok := srv.store.GetDomainByName(domain)
	if !ok {
		t.Fatalf("expected domain %q to exist after graduation", domain)
	}
	return d
}

// TestListDomainsEmpty verifies an empty store returns an empty list.
func TestListDomainsEmpty(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains", nil)

	list := mustJSON[DomainList](t, resp, http.StatusOK)
	if list.Total != 0 {
		t.Fatalf("expected total=0, got %d", list.Total)
	}
	if len(list.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(list.Items))
	}
}

// TestListDomainsReturnsDomains verifies graduated jobs appear in the domain list.
func TestListDomainsReturnsDomains(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "example.net", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains", nil)

	list := mustJSON[DomainList](t, resp, http.StatusOK)
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

// TestListDomainsFilterByName verifies the ?name= substring filter.
func TestListDomainsFilterByName(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJob(t, srv, "alpha.example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "beta.other.net", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains?name=example", nil)

	list := mustJSON[DomainList](t, resp, http.StatusOK)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	if list.Items[0].Name != "alpha.example.com" {
		t.Fatalf("expected alpha.example.com, got %q", list.Items[0].Name)
	}
}

// TestListDomainsFilterByTag verifies the ?tag= filter.
func TestListDomainsFilterByTag(t *testing.T) {
	srv := newTestServer(t)
	d1 := makeGraduatedJob(t, srv, "tagged.example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "untagged.example.com", JobSucceeded)

	if err := srv.store.CreateTag("mytag", ""); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := srv.store.TagDomains("mytag", []int64{d1.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains?tag=mytag", nil)

	list := mustJSON[DomainList](t, resp, http.StatusOK)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	if list.Items[0].Name != "tagged.example.com" {
		t.Fatalf("expected tagged.example.com, got %q", list.Items[0].Name)
	}
}

// TestListDomainsFilterByNoTag verifies the ?tag=__none__ filter returns only untagged domains.
func TestListDomainsFilterByNoTag(t *testing.T) {
	srv := newTestServer(t)
	d1 := makeGraduatedJob(t, srv, "tagged.example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "untagged.example.com", JobSucceeded)

	if err := srv.store.CreateTag("mytag", ""); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := srv.store.TagDomains("mytag", []int64{d1.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains?tag=__none__", nil)

	list := mustJSON[DomainList](t, resp, http.StatusOK)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	if list.Items[0].Name != "untagged.example.com" {
		t.Fatalf("expected untagged.example.com, got %q", list.Items[0].Name)
	}
}

// TestListDomainsInvalidLimit verifies that an out-of-range limit returns 400.
func TestListDomainsInvalidLimit(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains?limit=9999", nil)

	wantStatus(t, resp, http.StatusBadRequest)
}

// TestGetDomainByID verifies a domain can be fetched by its numeric ID.
func TestGetDomainByID(t *testing.T) {
	srv := newTestServer(t)
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/domains/%d", d.ID), nil)

	got := mustJSON[Domain](t, resp, http.StatusOK)
	if got.ID != d.ID {
		t.Fatalf("expected id=%d, got %d", d.ID, got.ID)
	}
	if got.Name != "example.com" {
		t.Fatalf("expected name=example.com, got %q", got.Name)
	}
}

// TestGetDomainByIDIncludesTags verifies that tags are populated in the response.
func TestGetDomainByIDIncludesTags(t *testing.T) {
	srv := newTestServer(t)
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	if err := srv.store.CreateTag("tld", ""); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/domains/%d", d.ID), nil)

	got := mustJSON[Domain](t, resp, http.StatusOK)
	if len(got.Tags) != 1 || got.Tags[0] != "tld" {
		t.Fatalf("expected tags=[tld], got %v", got.Tags)
	}
}

// TestGetDomainByIDNotFound verifies a 404 for a missing domain.
func TestGetDomainByIDNotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains/99999", nil)

	wantStatus(t, resp, http.StatusNotFound)
}

// TestGetDomainByIDInvalidID verifies a 404 for a non-numeric or zero ID.
func TestGetDomainByIDInvalidID(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/api/v1/domains/abc", "/api/v1/domains/0", "/api/v1/domains/-1"} {
		resp := doJSON(t, srv, http.MethodGet, path, nil)
		wantStatus(t, resp, http.StatusNotFound)
	}
}

// TestGetDomainRunsReturnsRuns verifies run history is returned for a domain.
func TestGetDomainRunsReturnsRuns(t *testing.T) {
	srv := newTestServer(t)
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded) // second run

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/domains/%d/runs", d.ID), nil)

	list := mustJSON[RunList](t, resp, http.StatusOK)
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
	for _, run := range list.Items {
		if run.Domain != "example.com" {
			t.Fatalf("unexpected domain in run: %q", run.Domain)
		}
	}
}

// TestGetDomainRunsDomainNotFound verifies a 404 for an unknown domain ID.
func TestGetDomainRunsDomainNotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains/99999/runs", nil)

	wantStatus(t, resp, http.StatusNotFound)
}

// TestGetDomainRunsPagination verifies limit and offset are respected.
func TestGetDomainRunsPagination(t *testing.T) {
	srv := newTestServer(t)
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/domains/%d/runs?limit=2&offset=0", d.ID), nil)

	list := mustJSON[RunList](t, resp, http.StatusOK)
	if list.Total != 3 {
		t.Fatalf("expected total=3, got %d", list.Total)
	}
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 items on first page, got %d", len(list.Items))
	}
}

// TestListDomainsFilterByMinLevel verifies the ?min_level= threshold filter.
func TestListDomainsFilterByMinLevel(t *testing.T) {
	srv := newTestServer(t)
	makeGraduatedJobWithEntries(t, srv, "info.example.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
	})
	makeGraduatedJobWithEntries(t, srv, "warning.example.com", []engine.LogEntry{
		{Module: "DNSSEC", Level: "WARNING"},
	})
	makeGraduatedJobWithEntries(t, srv, "error.example.com", []engine.LogEntry{
		{Module: "DNSSEC", Level: "ERROR"},
	})

	t.Run("warning_plus", func(t *testing.T) {
		resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains?min_level=WARNING", nil)
		wantStatus(t, resp, http.StatusOK)
		var list DomainList
		_ = json.NewDecoder(resp.Body).Decode(&list)
		if list.Total != 2 {
			t.Fatalf("expected total=2 for warning+, got %d", list.Total)
		}
	})

	t.Run("error_plus", func(t *testing.T) {
		resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains?min_level=ERROR", nil)
		var list DomainList
		_ = json.NewDecoder(resp.Body).Decode(&list)
		if list.Total != 1 {
			t.Fatalf("expected total=1 for error+, got %d", list.Total)
		}
		if list.Items[0].Name != "error.example.com" {
			t.Fatalf("expected error.example.com, got %q", list.Items[0].Name)
		}
	})
}
