package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func seedCohort(t *testing.T, srv *Server, cohort AnalysisCohort) AnalysisCohort {
	t.Helper()
	created, err := srv.store.UpsertAnalysisCohort(cohort)
	if err != nil {
		t.Fatalf("seed cohort: %v", err)
	}
	return created
}

// getPublic follows 3xx redirects up to a small bound, matching what
// browsers do.
func getPublic(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	return getPublicNoFollow(t, srv, path, 5)
}

// getPublicNoRedirect returns the first response untouched.
func getPublicNoRedirect(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	return getPublicNoFollow(t, srv, path, 0)
}

func getPublicNoFollow(t *testing.T, srv *Server, path string, maxHops int) *httptest.ResponseRecorder {
	t.Helper()
	for hop := 0; ; hop++ {
		resp := doJSON(t, srv, http.MethodGet, path, nil)
		if hop >= maxHops {
			return resp
		}
		switch resp.Code {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
			http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			loc := resp.Header().Get("Location")
			if loc == "" {
				return resp
			}
			path = loc
			continue
		}
		return resp
	}
}

func TestPublicAnalysisCatalogEmpty(t *testing.T) {
	srv := newTestServer(t)
	resp := getPublic(t, srv, "/pub/api/v1/analysis/catalog")
	got := mustJSON[PublicAnalysisCatalogResponse](t, resp, http.StatusOK)
	if len(got.Cohorts) != 0 || got.SelectorEnabled || got.DefaultTag != "" {
		t.Fatalf("expected empty catalog, got %+v", got)
	}
}

func TestPublicAnalysisCatalogResolvesDefault(t *testing.T) {
	srv := newTestServer(t)
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true, IsDefault: true, SortOrder: 10,
	})
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "gov", Label: "Government",
		AnalysisEnabled: true, PublicEnabled: true, SortOrder: 20,
	})
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "hidden", Label: "Hidden",
		AnalysisEnabled: true, PublicEnabled: false,
	})

	resp := getPublic(t, srv, "/pub/api/v1/analysis/catalog")
	got := mustJSON[PublicAnalysisCatalogResponse](t, resp, http.StatusOK)
	if got.DefaultTag != "tld" {
		t.Fatalf("expected default_tag=tld, got %q", got.DefaultTag)
	}
	if !got.SelectorEnabled {
		t.Fatalf("expected selector enabled for two public cohorts")
	}
	if len(got.Cohorts) != 2 {
		t.Fatalf("expected 2 public cohorts, got %d", len(got.Cohorts))
	}
	if got.Cohorts[0].DatasetTag != "tld" || got.Cohorts[1].DatasetTag != "gov" {
		t.Fatalf("unexpected cohort order: %+v", got.Cohorts)
	}
}

func TestPublicAnalysisCatalogRedactsInternalFields(t *testing.T) {
	srv := newTestServer(t)
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true, IsDefault: true,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/catalog")

	raw := resp.Body.String()
	for _, needle := range []string{
		`"id"`, `"source_type"`, `"analysis_enabled"`, `"public_enabled"`,
		`"materialization_status"`, `"last_materialization_error"`,
		`"created_at"`, `"updated_at"`,
	} {
		if strings.Contains(raw, needle) {
			t.Fatalf("public catalog response should not contain %s: %s", needle, raw)
		}
	}
}

func TestPublicAnalysisCohortsEndpoint(t *testing.T) {
	srv := newTestServer(t)
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/cohorts")
	views := mustJSON[[]PublicAnalysisCohortView](t, resp, http.StatusOK)
	if len(views) != 1 || views[0].DatasetTag != "tld" {
		t.Fatalf("unexpected views: %+v", views)
	}
}

func TestPublicAnalysisOverviewRedactsInternalFields(t *testing.T) {
	srv := newTestServer(t)
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true, IsDefault: true,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/overview")
	raw := resp.Body.String()
	for _, needle := range []string{
		`"id"`, `"source_type"`, `"analysis_enabled"`, `"public_enabled"`,
	} {
		if strings.Contains(raw, needle) {
			t.Fatalf("public overview response should not contain %s: %s", needle, raw)
		}
	}
}

func TestPublicAnalysisCatalogRejectsInvalidCatalogState(t *testing.T) {
	srv := newTestServer(t)
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "broken", Label: "Broken",
		AnalysisEnabled: false, PublicEnabled: true,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/catalog")
	wantStatus(t, resp, http.StatusInternalServerError)
}
