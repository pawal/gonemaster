package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	engineprofile "codeberg.org/pawal/gonemaster/engine/profile"
)

func fetchCompatibilitySummaries(t *testing.T, srv *Server) []CompatibilitySummary {
	t.Helper()
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/compatibility", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("fetchCompatibilitySummaries: expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var summaries []CompatibilitySummary
	if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
		t.Fatalf("decode summaries: %v", err)
	}
	return summaries
}

func summaryFor(summaries []CompatibilitySummary, id int64) *CompatibilitySummary {
	for i := range summaries {
		if summaries[i].ID == id {
			return &summaries[i]
		}
	}
	return nil
}

func TestCheckProfileCompatibilityReviewedWaivesIssues(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	// A narrow test_cases list stamped with the current engine version: the
	// omissions are declared intentional, so the profile stays compatible, but
	// the review has to say what it is waiving.
	result := checkProfileCompatibility(`{"test_cases":["address01"]}`, engine.VersionFull(), defaultP)

	if !result.Compatible {
		t.Fatalf("expected compatible=true when reviewed, got issues: %+v", result.Issues)
	}
	if !result.Reviewed {
		t.Fatal("expected reviewed=true when the stamp matches the engine version")
	}
	if len(result.Issues) != 0 {
		t.Fatalf("expected 0 open issues when reviewed, got %d", len(result.Issues))
	}
	if len(result.WaivedIssues) == 0 {
		t.Fatal("expected the waived issues to stay visible")
	}
	for _, issue := range result.WaivedIssues {
		if issue.Type == "missing_test_case" {
			return
		}
	}
	t.Fatalf("expected a waived missing_test_case issue, got %+v", result.WaivedIssues)
}

func TestCheckProfileCompatibilityCompatibleOnMeritsHasNoWaivedIssues(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	// Nothing in this config can drift, so the stamp waives nothing.
	result := checkProfileCompatibility(`{"resolver":{"defaults":{"timeout":5}}}`, engine.VersionFull(), defaultP)

	if !result.Reviewed {
		t.Fatal("expected reviewed=true")
	}
	if len(result.WaivedIssues) != 0 {
		t.Fatalf("expected no waived issues, got %+v", result.WaivedIssues)
	}
}

func TestCheckProfileCompatibilityUnreviewedHasNoWaivedIssues(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	result := checkProfileCompatibility(`{"test_cases":["address01"]}`, "old-version", defaultP)

	if result.Reviewed {
		t.Fatal("expected reviewed=false for a stale stamp")
	}
	if result.Compatible {
		t.Fatal("expected compatible=false for an unreviewed profile with gaps")
	}
	if len(result.WaivedIssues) != 0 {
		t.Fatalf("expected no waived issues before review, got %+v", result.WaivedIssues)
	}
}

func TestCheckProfileCompatibilityInvalidConfigIsNeverWaived(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	// A config the engine cannot parse is broken whatever the stamp claims.
	result := checkProfileCompatibility(`not json`, engine.VersionFull(), defaultP)

	if result.Compatible {
		t.Fatal("expected compatible=false for an unparseable config")
	}
	if len(result.Issues) == 0 || result.Issues[0].Type != "invalid_config" {
		t.Fatalf("expected an open invalid_config issue, got %+v", result.Issues)
	}
	if len(result.WaivedIssues) != 0 {
		t.Fatalf("expected no waived issues, got %+v", result.WaivedIssues)
	}
}

func TestProfilesCompatibilitySummaryCarriesWaivedCount(t *testing.T) {
	srv := New(DefaultConfig())
	// Created through the API, so it carries the current schema_version stamp.
	profile := createProfile(t, srv, `{"name":"waiver","config":{"test_cases":["address01"]}}`)

	summary := summaryFor(fetchCompatibilitySummaries(t, srv), profile.ID)
	if summary == nil {
		t.Fatal("expected a summary for the created profile")
	}
	if !summary.Compatible || summary.IssueCount != 0 {
		t.Fatalf("expected a compatible summary with no open issues, got %+v", summary)
	}
	if summary.WaivedCount == 0 {
		t.Fatalf("expected a non-zero waived count, got %+v", summary)
	}
}

func TestPatchProfileClearReviewed(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"clear-rev","config":{"test_cases":["address01"]}}`)

	// Freshly saved profiles are stamped, so the gaps start out waived.
	before := fetchCompatibility(t, srv, profile.ID)
	if !before.Reviewed || !before.Compatible || len(before.WaivedIssues) == 0 {
		t.Fatalf("expected a reviewed profile with waived issues, got %+v", before)
	}

	updated := patchProfile(t, srv, profile.ID, `{"op":"clear_reviewed"}`)
	if updated.SchemaVersion != "" {
		t.Fatalf("expected an empty schema_version, got %q", updated.SchemaVersion)
	}
	// The config must survive the re-check untouched.
	cases, ok := updated.Config["test_cases"].([]any)
	if !ok || len(cases) != 1 || cases[0] != "address01" {
		t.Fatalf("expected the config preserved, got %#v", updated.Config)
	}

	after := fetchCompatibility(t, srv, profile.ID)
	if after.Reviewed {
		t.Fatal("expected reviewed=false after clear_reviewed")
	}
	if after.Compatible {
		t.Fatal("expected the waived issues to come back as real issues")
	}
	if len(after.Issues) != len(before.WaivedIssues) {
		t.Fatalf("expected %d issues to return, got %d", len(before.WaivedIssues), len(after.Issues))
	}
	if len(after.WaivedIssues) != 0 {
		t.Fatalf("expected no waived issues once the stamp is gone, got %+v", after.WaivedIssues)
	}
}

func TestPatchProfileClearReviewedThenFixConvergesOnMerits(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"converge","config":{"test_cases":["address01"]}}`)

	patchProfile(t, srv, profile.ID, `{"op":"clear_reviewed"}`)
	patchProfile(t, srv, profile.ID, `{"op":"add_missing_test_cases"}`)

	after := fetchCompatibility(t, srv, profile.ID)
	if !after.Compatible {
		t.Fatalf("expected the fixed profile to be compatible, got %+v", after.Issues)
	}
	// The fix ops must not stamp, so the profile is compatible on its merits.
	if after.Reviewed {
		t.Fatal("expected the fix to leave the profile unstamped")
	}
	if len(after.WaivedIssues) != 0 {
		t.Fatalf("expected nothing waived, got %+v", after.WaivedIssues)
	}
}
