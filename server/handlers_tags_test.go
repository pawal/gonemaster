package server

import (
	"fmt"
	"net/http"
	"testing"
)

// --- helpers -----------------------------------------------------------------

func createTag(t *testing.T, srv *Server, name, description string) Tag {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"description":%q}`, name, description)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags", body)
	return mustJSON[Tag](t, resp, http.StatusCreated)
}

// --- POST /api/v1/tags -------------------------------------------------------

func TestCreateTag(t *testing.T) {
	srv := newTestServer(t)
	tag := createTag(t, srv, "tld", "top-level domains")

	if tag.Name != "tld" {
		t.Fatalf("expected name=tld, got %q", tag.Name)
	}
	if tag.Description != "top-level domains" {
		t.Fatalf("expected description, got %q", tag.Description)
	}
}

func TestCreateTagMissingName(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags", `{"name":""}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestCreateTagDuplicate(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags", `{"name":"tld"}`)
	wantStatus(t, resp, http.StatusConflict)
}

// --- GET /api/v1/tags --------------------------------------------------------

func TestListTagsEmpty(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags", nil)
	tags := mustJSON[[]Tag](t, resp, http.StatusOK)
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(tags))
	}
}

func TestListTagsReturnsTags(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "alpha", "")
	createTag(t, srv, "beta", "")

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags", nil)
	tags := mustJSON[[]Tag](t, resp, http.StatusOK)
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
}

func TestListTagsDomainCount(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags", nil)
	tags := mustJSON[[]Tag](t, resp, http.StatusOK)
	if tags[0].DomainCount != 1 {
		t.Fatalf("expected domain_count=1, got %d", tags[0].DomainCount)
	}
}

// --- PUT /api/v1/tags/{name} -------------------------------------------------

func TestUpdateTag(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "old description")

	resp := doJSON(t, srv, http.MethodPut, "/api/v1/tags/tld", `{"description":"new description"}`)
	tag := mustJSON[Tag](t, resp, http.StatusOK)
	if tag.Description != "new description" {
		t.Fatalf("expected updated description, got %q", tag.Description)
	}
}

func TestUpdateTagNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/tags/ghost", `{"description":"x"}`)
	wantStatus(t, resp, http.StatusNotFound)
}

// --- DELETE /api/v1/tags/{name} ----------------------------------------------

func TestDeleteTag(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/tags/tld", nil)
	wantStatus(t, resp, http.StatusNoContent)

	// Tag should be gone.
	_, ok := srv.store.GetTag("tld")
	if ok {
		t.Fatal("expected tag to be deleted")
	}
}

func TestDeleteTagNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/tags/ghost", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestDeleteTagBlockedByCohort(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")
	if _, err := srv.store.UpsertAnalysisCohort(AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLDs",
	}); err != nil {
		t.Fatalf("UpsertAnalysisCohort: %v", err)
	}

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/tags/tld", nil)
	wantStatus(t, resp, http.StatusConflict)
	// Tag must still exist.
	if _, ok := srv.store.GetTag("tld"); !ok {
		t.Fatal("expected tag to survive a blocked delete")
	}
}

// --- POST /api/v1/tags/{name}/purge ------------------------------------------

func TestTagPurge(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "se", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags/tld/purge", nil)
	out := mustJSON[struct {
		PurgedRuns int64 `json:"purged_runs"`
	}](t, resp, http.StatusOK)
	if out.PurgedRuns != 1 {
		t.Fatalf("expected purged_runs=1, got %d", out.PurgedRuns)
	}
	if list := srv.store.ListRunsByDomain(d.ID, 10, 0); list.Total != 0 {
		t.Fatalf("expected runs purged, got %d", list.Total)
	}
}

func TestTagPurgeNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags/ghost/purge", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

// --- POST /api/v1/tags/{name}/domains ----------------------------------------

func TestAddTagDomains(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")
	makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags/tld/domains", `{"domains":["example.com"]}`)
	wantStatus(t, resp, http.StatusNoContent)

	tag, _ := srv.store.GetTag("tld")
	if tag.DomainCount != 1 {
		t.Fatalf("expected domain_count=1, got %d", tag.DomainCount)
	}
}

func TestAddTagDomainsCreatesUnknownDomain(t *testing.T) {
	// Domains that don't exist yet should be created via GetOrCreateDomain.
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags/tld/domains", `{"domains":["new.example.com"]}`)
	wantStatus(t, resp, http.StatusNoContent)

	_, ok := srv.store.GetDomainByName("new.example.com")
	if !ok {
		t.Fatal("expected domain to be created")
	}
}

func TestAddTagDomainsTagNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags/ghost/domains", `{"domains":["example.com"]}`)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestAddTagDomainsMissingBody(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags/tld/domains", `{"domains":[]}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

// --- DELETE /api/v1/tags/{name}/domains --------------------------------------

func TestRemoveTagDomains(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/tags/tld/domains", `{"domains":["example.com"]}`)
	wantStatus(t, resp, http.StatusNoContent)

	tag, _ := srv.store.GetTag("tld")
	if tag.DomainCount != 0 {
		t.Fatalf("expected domain_count=0 after removal, got %d", tag.DomainCount)
	}
}

// --- GET /api/v1/tags/{name}/domains -----------------------------------------

func TestListTagDomains(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")
	d1 := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	d2 := makeGraduatedJob(t, srv, "example.net", JobSucceeded)
	makeGraduatedJob(t, srv, "other.org", JobSucceeded) // not tagged
	if err := srv.store.TagDomains("tld", []int64{d1.ID, d2.ID}); err != nil {
		t.Fatalf("tag domains: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags/tld/domains", nil)
	list := mustJSON[DomainList](t, resp, http.StatusOK)
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListTagDomainsNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags/ghost/domains", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

// --- GET /api/v1/tags/{name}/summary -----------------------------------------

func TestTagSummary(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags/tld/summary", nil)
	summary := mustJSON[TagSummary](t, resp, http.StatusOK)
	if summary.Tag != "tld" {
		t.Fatalf("expected tag=tld, got %q", summary.Tag)
	}
	if summary.DomainCount != 1 {
		t.Fatalf("expected domain_count=1, got %d", summary.DomainCount)
	}
}

func TestTagSummaryNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags/ghost/summary", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestTagSummaryIncludesGrades(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "prod", "")

	// Graduate one clean domain (gets a grade).
	d := makeGraduatedJob(t, srv, "clean.example", JobSucceeded)
	if err := srv.store.TagDomains("prod", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags/prod/summary", nil)
	summary := mustJSON[TagSummary](t, resp, http.StatusOK)
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
