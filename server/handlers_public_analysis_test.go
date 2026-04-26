package server

import (
	"encoding/json"
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
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Handler().ServeHTTP(resp, req)
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
	srv := New(DefaultConfig())
	resp := getPublic(t, srv, "/pub/api/v1/analysis/catalog")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisCatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Cohorts) != 0 || got.SelectorEnabled || got.DefaultTag != "" {
		t.Fatalf("expected empty catalog, got %+v", got)
	}
}

func TestPublicAnalysisCatalogResolvesDefault(t *testing.T) {
	srv := New(DefaultConfig())
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
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var got PublicAnalysisCatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	srv := New(DefaultConfig())
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
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/cohorts")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var views []PublicAnalysisCohortView
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 1 || views[0].DatasetTag != "tld" {
		t.Fatalf("unexpected views: %+v", views)
	}
}

func TestPublicAnalysisOverviewResolvesDefault(t *testing.T) {
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true, IsDefault: true,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/overview")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DatasetTag != "tld" || got.Label != "TLD" || !got.IsDefault {
		t.Fatalf("unexpected overview: %+v", got)
	}
}

func TestPublicAnalysisOverviewNoDefault503(t *testing.T) {
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true,
	})
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "gov", Label: "Government",
		AnalysisEnabled: true, PublicEnabled: true,
	})

	resp := getPublic(t, srv, "/pub/api/v1/analysis/overview")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when no default set, got %d: %s", resp.Code, resp.Body)
	}
}

func TestPublicAnalysisOverviewNoCohortsConfigured503(t *testing.T) {
	srv := New(DefaultConfig())
	resp := getPublic(t, srv, "/pub/api/v1/analysis/overview")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with empty catalog, got %d: %s", resp.Code, resp.Body)
	}
}

func TestPublicAnalysisOverviewRejectsNonPublicTag(t *testing.T) {
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "hidden", Label: "Hidden",
		AnalysisEnabled: true, PublicEnabled: false,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/overview?dataset_tag=hidden")
	if resp.Code != http.StatusServiceUnavailable && resp.Code != http.StatusNotFound {
		// Empty selectable list → 503; mismatched tag → 404. Either is fine.
		t.Fatalf("expected 404 or 503, got %d: %s", resp.Code, resp.Body)
	}
}

func TestPublicAnalysisOverviewRejectsUnknownTag(t *testing.T) {
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld", Label: "TLD",
		AnalysisEnabled: true, PublicEnabled: true, IsDefault: true,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/overview?dataset_tag=unknown")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body)
	}
}

func TestPublicAnalysisOverviewRedactsInternalFields(t *testing.T) {
	srv := New(DefaultConfig())
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
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "broken", Label: "Broken",
		AnalysisEnabled: false, PublicEnabled: true,
	})
	resp := getPublic(t, srv, "/pub/api/v1/analysis/catalog")
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", resp.Code, resp.Body)
	}
}
