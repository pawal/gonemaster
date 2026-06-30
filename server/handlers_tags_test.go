package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- helpers -----------------------------------------------------------------

func createTag(t *testing.T, srv *Server, name, description string) Tag {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"description":%q}`, name, description)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("createTag %q: expected 201, got %d: %s", name, resp.Code, resp.Body)
	}
	var tag Tag
	if err := json.NewDecoder(resp.Body).Decode(&tag); err != nil {
		t.Fatalf("createTag decode: %v", err)
	}
	return tag
}

// --- POST /api/v1/tags -------------------------------------------------------

func TestCreateTag(t *testing.T) {
	srv := New(DefaultConfig())
	tag := createTag(t, srv, "tld", "top-level domains")

	if tag.Name != "tld" {
		t.Fatalf("expected name=tld, got %q", tag.Name)
	}
	if tag.Description != "top-level domains" {
		t.Fatalf("expected description, got %q", tag.Description)
	}
}

func TestCreateTagMissingName(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewBufferString(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestCreateTagDuplicate(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewBufferString(`{"name":"tld"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.Code)
	}
}

// --- GET /api/v1/tags --------------------------------------------------------

func TestListTagsEmpty(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var tags []Tag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(tags))
	}
}

func TestListTagsReturnsTags(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "alpha", "")
	createTag(t, srv, "beta", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var tags []Tag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
}

func TestListTagsDomainCount(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	srv.Handler().ServeHTTP(resp, req)
	var tags []Tag
	_ = json.NewDecoder(resp.Body).Decode(&tags)
	if tags[0].DomainCount != 1 {
		t.Fatalf("expected domain_count=1, got %d", tags[0].DomainCount)
	}
}

// --- PUT /api/v1/tags/{name} -------------------------------------------------

func TestUpdateTag(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "old description")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/tld", bytes.NewBufferString(`{"description":"new description"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var tag Tag
	if err := json.NewDecoder(resp.Body).Decode(&tag); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tag.Description != "new description" {
		t.Fatalf("expected updated description, got %q", tag.Description)
	}
}

func TestUpdateTagNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/ghost", bytes.NewBufferString(`{"description":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

// --- DELETE /api/v1/tags/{name} ----------------------------------------------

func TestDeleteTag(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/tld", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.Code)
	}

	// Tag should be gone.
	_, ok := srv.store.GetTag("tld")
	if ok {
		t.Fatal("expected tag to be deleted")
	}
}

func TestDeleteTagNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/ghost", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestDeleteTagBlockedByCohort(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	if _, err := srv.store.UpsertAnalysisCohort(AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLDs",
	}); err != nil {
		t.Fatalf("UpsertAnalysisCohort: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/tld", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.Code, resp.Body)
	}
	// Tag must still exist.
	if _, ok := srv.store.GetTag("tld"); !ok {
		t.Fatal("expected tag to survive a blocked delete")
	}
}

// --- POST /api/v1/tags/{name}/purge ------------------------------------------

func TestTagPurge(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "se", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/tld/purge", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var out struct {
		PurgedRuns int64 `json:"purged_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.PurgedRuns != 1 {
		t.Fatalf("expected purged_runs=1, got %d", out.PurgedRuns)
	}
	if list := srv.store.ListRunsByDomain(d.ID, 10, 0); list.Total != 0 {
		t.Fatalf("expected runs purged, got %d", list.Total)
	}
}

func TestTagPurgeNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/ghost/purge", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

// --- POST /api/v1/tags/{name}/domains ----------------------------------------

func TestAddTagDomains(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/tld/domains",
		bytes.NewBufferString(`{"domains":["example.com"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body)
	}

	tag, _ := srv.store.GetTag("tld")
	if tag.DomainCount != 1 {
		t.Fatalf("expected domain_count=1, got %d", tag.DomainCount)
	}
}

func TestAddTagDomainsCreatesUnknownDomain(t *testing.T) {
	// Domains that don't exist yet should be created via GetOrCreateDomain.
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/tld/domains",
		bytes.NewBufferString(`{"domains":["new.example.com"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body)
	}

	_, ok := srv.store.GetDomainByName("new.example.com")
	if !ok {
		t.Fatal("expected domain to be created")
	}
}

func TestAddTagDomainsTagNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/ghost/domains",
		bytes.NewBufferString(`{"domains":["example.com"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestAddTagDomainsMissingBody(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/tld/domains",
		bytes.NewBufferString(`{"domains":[]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

// --- DELETE /api/v1/tags/{name}/domains --------------------------------------

func TestRemoveTagDomains(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/tld/domains",
		bytes.NewBufferString(`{"domains":["example.com"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body)
	}

	tag, _ := srv.store.GetTag("tld")
	if tag.DomainCount != 0 {
		t.Fatalf("expected domain_count=0 after removal, got %d", tag.DomainCount)
	}
}

// --- GET /api/v1/tags/{name}/domains -----------------------------------------

func TestListTagDomains(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d1 := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	d2 := makeGraduatedJob(t, srv, "example.net", JobSucceeded)
	makeGraduatedJob(t, srv, "other.org", JobSucceeded) // not tagged
	if err := srv.store.TagDomains("tld", []int64{d1.ID, d2.ID}); err != nil {
		t.Fatalf("tag domains: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/tld/domains", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var list DomainList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListTagDomainsNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/ghost/domains", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

// --- GET /api/v1/tags/{name}/summary -----------------------------------------

func TestTagSummary(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/tld/summary", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var summary TagSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary.Tag != "tld" {
		t.Fatalf("expected tag=tld, got %q", summary.Tag)
	}
	if summary.DomainCount != 1 {
		t.Fatalf("expected domain_count=1, got %d", summary.DomainCount)
	}
}

func TestTagSummaryNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/ghost/summary", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestTagSummaryIncludesGrades(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "prod", "")

	// Graduate one clean domain (gets a grade).
	d := makeGraduatedJob(t, srv, "clean.example", JobSucceeded)
	if err := srv.store.TagDomains("prod", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/prod/summary", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var summary TagSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(summary.Grades) == 0 {
		t.Fatal("expected non-empty grades map in tag summary")
	}
	total := 0
	for _, count := range summary.Grades {
		total += count
	}
	if total != summary.DomainCount {
		t.Fatalf("grades total %d != domain_count %d", total, summary.DomainCount)
	}
}
