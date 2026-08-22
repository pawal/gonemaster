package server

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

// makeGraduatedJob graduates one run and returns the domain it created.
func makeGraduatedJob(t *testing.T, srv *Server, domain string, status JobStatus) Domain {
	t.Helper()
	seedGraduatedRun(t, srv.store, runSpec{Domain: domain, Status: status})
	d, ok := srv.store.GetDomainByName(domain)
	if !ok {
		t.Fatalf("expected domain %q to exist after graduation", domain)
	}
	return d
}

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

func TestListDomainsFilters(t *testing.T) {
	srv := newTestServer(t)
	tagged := makeGraduatedJobWithEntries(t, srv, "alpha.example.com", []engine.LogEntry{
		{Module: "Basic", Level: "INFO"},
	})
	makeGraduatedJobWithEntries(t, srv, "beta.other.net", []engine.LogEntry{
		{Module: "DNSSEC", Level: "WARNING"},
	})
	makeGraduatedJobWithEntries(t, srv, "gamma.example.com", []engine.LogEntry{
		{Module: "DNSSEC", Level: "ERROR"},
	})
	if err := srv.store.CreateTag("mytag", ""); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := srv.store.TagDomains("mytag", []int64{tagged.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	for _, tc := range []struct {
		name  string
		query string
		want  []string
	}{
		{name: "unfiltered", want: []string{"alpha.example.com", "beta.other.net", "gamma.example.com"}},
		{name: "name substring", query: "?name=example", want: []string{"alpha.example.com", "gamma.example.com"}},
		{name: "tag", query: "?tag=mytag", want: []string{"alpha.example.com"}},
		{name: "no tag", query: "?tag=__none__", want: []string{"beta.other.net", "gamma.example.com"}},
		{name: "min level warning", query: "?min_level=WARNING", want: []string{"beta.other.net", "gamma.example.com"}},
		{name: "min level error", query: "?min_level=ERROR", want: []string{"gamma.example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains"+tc.query, nil)

			list := mustJSON[DomainList](t, resp, http.StatusOK)
			if list.Total != len(tc.want) {
				t.Fatalf("total = %d, want %d", list.Total, len(tc.want))
			}
			names := make([]string, 0, len(list.Items))
			for _, item := range list.Items {
				names = append(names, item.Name)
			}
			slices.Sort(names)
			if !slices.Equal(names, tc.want) {
				t.Fatalf("domains = %v, want %v", names, tc.want)
			}
		})
	}
}

func TestListDomainsInvalidLimit(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains?limit=9999", nil)

	wantStatus(t, resp, http.StatusBadRequest)
}

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

func TestGetDomainByIDNotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains/99999", nil)

	wantStatus(t, resp, http.StatusNotFound)
}

func TestGetDomainByIDInvalidID(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/api/v1/domains/abc", "/api/v1/domains/0", "/api/v1/domains/-1"} {
		resp := doJSON(t, srv, http.MethodGet, path, nil)
		wantStatus(t, resp, http.StatusNotFound)
	}
}

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

func TestGetDomainRunsDomainNotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/domains/99999/runs", nil)

	wantStatus(t, resp, http.StatusNotFound)
}

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
