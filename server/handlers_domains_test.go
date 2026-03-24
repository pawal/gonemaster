package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list DomainList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 0 {
		t.Fatalf("expected total=0, got %d", list.Total)
	}
	if len(list.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(list.Items))
	}
}

// TestListDomainsReturnsDomains verifies graduated jobs appear in the domain list.
func TestListDomainsReturnsDomains(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "example.net", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list DomainList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

// TestListDomainsFilterByName verifies the ?name= substring filter.
func TestListDomainsFilterByName(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJob(t, srv, "alpha.example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "beta.other.net", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains?name=example", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list DomainList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	if list.Items[0].Name != "alpha.example.com" {
		t.Fatalf("expected alpha.example.com, got %q", list.Items[0].Name)
	}
}

// TestListDomainsFilterByTag verifies the ?tag= filter.
func TestListDomainsFilterByTag(t *testing.T) {
	srv := New(DefaultConfig())
	d1 := makeGraduatedJob(t, srv, "tagged.example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "untagged.example.com", JobSucceeded)

	if err := srv.store.CreateTag("mytag", ""); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := srv.store.TagDomains("mytag", []int64{d1.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains?tag=mytag", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list DomainList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	if list.Items[0].Name != "tagged.example.com" {
		t.Fatalf("expected tagged.example.com, got %q", list.Items[0].Name)
	}
}

// TestListDomainsInvalidLimit verifies that an out-of-range limit returns 400.
func TestListDomainsInvalidLimit(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains?limit=9999", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

// TestGetDomainByID verifies a domain can be fetched by its numeric ID.
func TestGetDomainByID(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/domains/%d", d.ID), nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got Domain
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != d.ID {
		t.Fatalf("expected id=%d, got %d", d.ID, got.ID)
	}
	if got.Name != "example.com" {
		t.Fatalf("expected name=example.com, got %q", got.Name)
	}
}

// TestGetDomainByIDIncludesTags verifies that tags are populated in the response.
func TestGetDomainByIDIncludesTags(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	if err := srv.store.CreateTag("tld", ""); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/domains/%d", d.ID), nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var got Domain
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "tld" {
		t.Fatalf("expected tags=[tld], got %v", got.Tags)
	}
}

// TestGetDomainByIDNotFound verifies a 404 for a missing domain.
func TestGetDomainByIDNotFound(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/99999", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

// TestGetDomainByIDInvalidID verifies a 404 for a non-numeric or zero ID.
func TestGetDomainByIDInvalidID(t *testing.T) {
	srv := New(DefaultConfig())

	for _, path := range []string{"/api/v1/domains/abc", "/api/v1/domains/0", "/api/v1/domains/-1"} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Handler().ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("path %s: expected 404, got %d", path, resp.Code)
		}
	}
}

// TestGetDomainRunsReturnsRuns verifies run history is returned for a domain.
func TestGetDomainRunsReturnsRuns(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded) // second run

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/domains/%d/runs", d.ID), nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var list RunList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/99999/runs", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

// TestGetDomainRunsPagination verifies limit and offset are respected.
func TestGetDomainRunsPagination(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/domains/%d/runs?limit=2&offset=0", d.ID), nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list RunList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 3 {
		t.Fatalf("expected total=3, got %d", list.Total)
	}
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 items on first page, got %d", len(list.Items))
	}
}
