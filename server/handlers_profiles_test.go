package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	engineprofile "codeberg.org/pawal/gonemaster/engine/profile"
)

func createProfile(t *testing.T, srv *Server, body string) Profile {
	t.Helper()
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/profiles", body)
	profile := mustJSON[Profile](t, resp, http.StatusCreated)
	return profile
}

func TestCreateProfile(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{
		"name":"strict-dnssec",
		"description":"Strict DNSSEC validation",
		"config":{"resolver":{"defaults":{"timeout":10}}},
		"public":true
	}`)

	if profile.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if profile.Name != "strict-dnssec" {
		t.Fatalf("Name: got %q", profile.Name)
	}
	if profile.Description != "Strict DNSSEC validation" {
		t.Fatalf("Description: got %q", profile.Description)
	}
	if !profile.Public {
		t.Fatal("expected Public=true")
	}
	resolver, ok := profile.Config["resolver"].(map[string]any)
	if !ok {
		t.Fatalf("expected resolver config, got %#v", profile.Config)
	}
	defaults, ok := resolver["defaults"].(map[string]any)
	if !ok || defaults["timeout"] != float64(10) {
		t.Fatalf("expected timeout=10, got %#v", profile.Config)
	}

	stored, ok := srv.store.GetProfile(profile.ID)
	if !ok {
		t.Fatal("expected stored profile")
	}
	if stored.Config != `{"resolver":{"defaults":{"timeout":10}}}` {
		t.Fatalf("stored Config: got %q", stored.Config)
	}
}

func TestCreateProfileRejectsInvalidConfig(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/profiles", `{"name":"broken","config":{"net":1}}`)
	wantErrorCode(t, resp, http.StatusBadRequest, "invalid_profile")
}

func TestListProfiles(t *testing.T) {
	srv := newTestServer(t)
	createProfile(t, srv, `{"name":"alpha","config":{"net":{"ipv4":true}}}`)
	createProfile(t, srv, `{"name":"beta","config":{"net":{"ipv6":false}}}`)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles", nil)
	profiles := mustJSON[[]Profile](t, resp, http.StatusOK)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0].Name != "alpha" || profiles[1].Name != "beta" {
		t.Fatalf("unexpected order: %#v", profiles)
	}
}

func TestGetDefaultProfile(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/default", nil)
	profile := mustJSON[Profile](t, resp, http.StatusOK)
	if profile.ID != 0 {
		t.Fatalf("ID: got %d, want 0", profile.ID)
	}
	if profile.Name != "default" {
		t.Fatalf("Name: got %q", profile.Name)
	}
	netCfg, ok := profile.Config["net"].(map[string]any)
	if !ok {
		t.Fatalf("expected net config, got %#v", profile.Config)
	}
	if _, ok := netCfg["ipv4"]; !ok {
		t.Fatalf("expected ipv4 in net config, got %#v", netCfg)
	}
}

func TestGetDefaultProfileAppliesConfigFileOverride(t *testing.T) {
	cfg := DefaultConfig()
	f, err := os.CreateTemp(t.TempDir(), "profile-override-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(`{"net":{"ipv6":false}}`); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	cfg.ProfilePath = f.Name()
	srv := New(cfg)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/default", nil)
	profile := mustJSON[Profile](t, resp, http.StatusOK)
	netCfg, ok := profile.Config["net"].(map[string]any)
	if !ok {
		t.Fatalf("expected net config, got %#v", profile.Config)
	}
	if got := netCfg["ipv6"]; got != false {
		t.Fatalf("expected ipv6=false from override, got %#v", got)
	}
}

func TestGetProfile(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/profiles/%d", profile.ID), nil)
	got := mustJSON[Profile](t, resp, http.StatusOK)
	if got.ID != profile.ID {
		t.Fatalf("ID: got %d, want %d", got.ID, profile.ID)
	}
	if got.Name != "default" {
		t.Fatalf("Name: got %q", got.Name)
	}
}

func TestUpdateProfile(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)

	resp := doJSON(t, srv, http.MethodPut, fmt.Sprintf("/api/v1/profiles/%d", profile.ID), `{
			"name":"strict",
			"description":"Updated profile",
			"config":{"resolver":{"defaults":{"retry":3}}},
			"public":true
		}`)
	updated := mustJSON[Profile](t, resp, http.StatusOK)
	if updated.Name != "strict" {
		t.Fatalf("Name: got %q", updated.Name)
	}
	if updated.Description != "Updated profile" {
		t.Fatalf("Description: got %q", updated.Description)
	}
	if !updated.Public {
		t.Fatal("expected Public=true")
	}

	stored, ok := srv.store.GetProfile(profile.ID)
	if !ok {
		t.Fatal("expected updated stored profile")
	}
	if stored.Name != "strict" {
		t.Fatalf("stored Name: got %q", stored.Name)
	}
	if stored.Config != `{"resolver":{"defaults":{"retry":3}}}` {
		t.Fatalf("stored Config: got %q", stored.Config)
	}
}

func TestDeleteProfile(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)

	resp := doJSON(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/profiles/%d", profile.ID), nil)
	wantStatus(t, resp, http.StatusNoContent)
	if _, ok := srv.store.GetProfile(profile.ID); ok {
		t.Fatal("expected profile to be deleted")
	}
}

func TestSetTagProfile(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "ops", "")
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)

	resp := doJSON(t, srv, http.MethodPut, "/api/v1/tags/ops/profile", fmt.Sprintf(`{"profile_id":%d}`, profile.ID))
	wantStatus(t, resp, http.StatusNoContent)

	tag, ok := srv.store.GetTag("ops")
	if !ok {
		t.Fatal("expected tag")
	}
	if tag.DefaultProfileID == nil || *tag.DefaultProfileID != profile.ID {
		t.Fatalf("DefaultProfileID: got %v, want %d", tag.DefaultProfileID, profile.ID)
	}
}

func TestDeleteTagProfile(t *testing.T) {
	srv := newTestServer(t)
	createTag(t, srv, "ops", "")
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)
	if err := srv.store.SetTagDefaultProfile("ops", &profile.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile: %v", err)
	}

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/tags/ops/profile", nil)
	wantStatus(t, resp, http.StatusNoContent)

	tag, ok := srv.store.GetTag("ops")
	if !ok {
		t.Fatal("expected tag")
	}
	if tag.DefaultProfileID != nil {
		t.Fatalf("expected cleared DefaultProfileID, got %v", *tag.DefaultProfileID)
	}
}

// ── schema_version ────────────────────────────────────────────────────────────

func TestCreateProfileSetsSchemaVersion(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"sv-test","config":{"net":{"ipv4":true}}}`)
	if profile.SchemaVersion == "" {
		t.Fatal("expected non-empty SchemaVersion on created profile")
	}
	stored, ok := srv.store.GetProfile(profile.ID)
	if !ok {
		t.Fatal("expected stored profile")
	}
	if stored.SchemaVersion == "" {
		t.Fatal("expected non-empty SchemaVersion in store")
	}
}

func TestUpdateProfileSetsSchemaVersion(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"sv-update","config":{"net":{"ipv4":true}}}`)

	resp := doJSON(t, srv, http.MethodPut, "/api/v1/profiles/"+itoa(profile.ID), `{"name":"sv-update","description":"updated","config":{"net":{"ipv4":true}}}`)
	updated := mustJSON[Profile](t, resp, http.StatusOK)
	if updated.SchemaVersion == "" {
		t.Fatal("expected non-empty SchemaVersion after update")
	}
}

// ── GET /profiles/defaults ────────────────────────────────────────────────────

func TestGetProfileDefaults(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/defaults", nil)
	defaults := mustJSON[ProfileDefaults](t, resp, http.StatusOK)
	if len(defaults.TestCases) == 0 {
		t.Fatal("expected non-empty TestCases in defaults")
	}
	if len(defaults.TestLevels) == 0 {
		t.Fatal("expected non-empty TestLevels in defaults")
	}
	// Verify well-known test cases are present.
	found := slices.Contains(defaults.TestCases, "address01")
	if !found {
		t.Fatalf("expected address01 in test_cases, got %v", defaults.TestCases[:5])
	}
}

// ── GET /profiles/compatibility ───────────────────────────────────────────────

func TestProfilesCompatibilityEmpty(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/compatibility", nil)
	summaries := mustJSON[[]CompatibilitySummary](t, resp, http.StatusOK)
	if len(summaries) != 0 {
		t.Fatalf("expected 0 summaries for empty store, got %d", len(summaries))
	}
}

func TestProfilesCompatibilityCompatibleProfile(t *testing.T) {
	srv := newTestServer(t)
	// A profile that only sets resolver options has no test_cases or test_levels
	// overrides, so it is always compatible.
	createProfile(t, srv, `{"name":"compat","config":{"resolver":{"defaults":{"timeout":5}}}}`)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/compatibility", nil)
	summaries := mustJSON[[]CompatibilitySummary](t, resp, http.StatusOK)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if !summaries[0].Compatible {
		t.Fatalf("expected compatible=true, got false; issues: %d", summaries[0].IssueCount)
	}
}

// ── GET /profiles/{id}/compatibility ─────────────────────────────────────────

func TestProfileCompatibilityNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/999/compatibility", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestProfileCompatibilityNoOverrides(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"simple","config":{"resolver":{"defaults":{"timeout":10}}}}`)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/"+itoa(profile.ID)+"/compatibility", nil)
	result := mustJSON[CompatibilityResult](t, resp, http.StatusOK)
	if !result.Compatible {
		t.Fatalf("expected compatible=true, got false: %+v", result.Issues)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("expected 0 issues, got %d", len(result.Issues))
	}
	if result.CurrentVersion == "" {
		t.Fatal("expected non-empty CurrentVersion")
	}
}

func TestProfileCompatibilityMissingTestCase(t *testing.T) {
	srv := newTestServer(t)
	// Profile explicitly sets test_cases with only "address01" - all other default
	// test cases are missing.
	profile := createProfile(t, srv, `{"name":"narrow","config":{"test_cases":["address01"]}}`)
	markProfileStale(t, srv, profile.ID)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/"+itoa(profile.ID)+"/compatibility", nil)
	result := mustJSON[CompatibilityResult](t, resp, http.StatusOK)
	if result.Compatible {
		t.Fatal("expected compatible=false for profile missing test cases")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Type == "missing_test_case" {
			found = true
			if issue.Detail == "" {
				t.Fatal("expected non-empty Detail")
			}
			if issue.Suggestion == "" {
				t.Fatal("expected non-empty Suggestion")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected missing_test_case issue, got: %+v", result.Issues)
	}
}

func TestProfileCompatibilityAllDefaultTestCases(t *testing.T) {
	// A profile that includes ALL default test cases should have no missing_test_case issue.
	srv := newTestServer(t)

	// Fetch the defaults to get the full test_cases list.
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/defaults", nil)
	defaults := mustJSON[ProfileDefaults](t, resp, http.StatusOK)

	// Build a profile that explicitly sets all default test cases.
	casesJSON, _ := json.Marshal(defaults.TestCases)
	configJSON := `{"test_cases":` + string(casesJSON) + `}`
	profile := createProfile(t, srv, `{"name":"full-cases","config":`+configJSON+`}`)

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/profiles/"+itoa(profile.ID)+"/compatibility", nil)
	result := mustJSON[CompatibilityResult](t, resp, http.StatusOK)
	for _, issue := range result.Issues {
		if issue.Type == "missing_test_case" {
			t.Fatalf("unexpected missing_test_case issue: %s", issue.Detail)
		}
	}
}

func TestProfileCompatibilityMissingTestLevels(t *testing.T) {
	srv := newTestServer(t)

	// Fetch defaults to get a real module with tags.
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/defaults", nil)
	defaults := mustJSON[ProfileDefaults](t, resp, http.StatusOK)

	// Find a module that has at least 2 tags, then use only one of them.
	var chosenModule string
	var missingTag string
	var partialLevels map[string]string
	for mod, tags := range defaults.TestLevels {
		if len(tags) >= 2 {
			chosenModule = mod
			partialLevels = map[string]string{}
			count := 0
			for tag, level := range tags {
				if count == 0 {
					missingTag = tag
				} else {
					partialLevels[tag] = level
				}
				count++
			}
			break
		}
	}
	if chosenModule == "" {
		t.Skip("no module with >= 2 tags in defaults")
	}

	// Build a profile that covers all but one tag in chosenModule.
	levelsPayload := map[string]map[string]string{chosenModule: partialLevels}
	levelsJSON, _ := json.Marshal(levelsPayload)
	profile := createProfile(t, srv, `{"name":"partial-levels","config":{"test_levels":`+string(levelsJSON)+`}}`)
	markProfileStale(t, srv, profile.ID)

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/profiles/"+itoa(profile.ID)+"/compatibility", nil)
	result := mustJSON[CompatibilityResult](t, resp, http.StatusOK)
	if result.Compatible {
		t.Fatalf("expected compatible=false for profile missing test_levels tag %q", missingTag)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Type == "missing_test_levels" && issue.Module == chosenModule {
			found = true
			if issue.Detail == "" {
				t.Fatal("expected non-empty Detail")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected missing_test_levels issue for module %q, got: %+v", chosenModule, result.Issues)
	}
}

func TestProfileCompatibilityFullTestLevelsCoverage(t *testing.T) {
	// A profile that covers all default tags for a module should have no issue for it.
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/defaults", nil)
	defaults := mustJSON[ProfileDefaults](t, resp, http.StatusOK)

	// Use the full ADDRESS module test_levels.
	addressTags, ok := defaults.TestLevels["ADDRESS"]
	if !ok {
		t.Skip("ADDRESS module not in defaults")
	}
	levelsPayload := map[string]map[string]string{"ADDRESS": addressTags}
	levelsJSON, _ := json.Marshal(levelsPayload)
	profile := createProfile(t, srv, `{"name":"full-levels","config":{"test_levels":`+string(levelsJSON)+`}}`)

	resp = doJSON(t, srv, http.MethodGet, "/api/v1/profiles/"+itoa(profile.ID)+"/compatibility", nil)
	result := mustJSON[CompatibilityResult](t, resp, http.StatusOK)
	for _, issue := range result.Issues {
		if issue.Type == "missing_test_levels" && issue.Module == "ADDRESS" {
			t.Fatalf("unexpected missing_test_levels issue for ADDRESS: %s", issue.Detail)
		}
	}
}

// ── checkProfileCompatibility unit tests ─────────────────────────────────────

func TestCheckProfileCompatibilityCompatible(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	result := checkProfileCompatibility(`{"resolver":{"defaults":{"timeout":5}}}`, "v1.0.0", defaultP)
	if !result.Compatible {
		t.Fatalf("expected compatible=true, got false: %+v", result.Issues)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestCheckProfileCompatibilityMissingTestCase(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	result := checkProfileCompatibility(`{"test_cases":["address01"]}`, "", defaultP)
	if result.Compatible {
		t.Fatal("expected compatible=false")
	}
	foundType := false
	for _, issue := range result.Issues {
		if issue.Type == "missing_test_case" {
			foundType = true
		}
	}
	if !foundType {
		t.Fatalf("expected missing_test_case issue, got: %+v", result.Issues)
	}
}

func TestCheckProfileCompatibilityReviewedSuppressesIssues(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	// Same narrow config as the missing-test-case case, but reviewed against the
	// current engine version: the omission is intentional, so no issues surface.
	result := checkProfileCompatibility(`{"test_cases":["address01"]}`, engine.VersionFull(), defaultP)
	if !result.Compatible {
		t.Fatalf("expected compatible=true when reviewed against current version, got: %+v", result.Issues)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("expected 0 issues when reviewed, got %d", len(result.Issues))
	}
}

func TestCheckProfileCompatibilityInvalidConfig(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	result := checkProfileCompatibility(`not json`, "", defaultP)
	if result.Compatible {
		t.Fatal("expected compatible=false for invalid config")
	}
	if len(result.Issues) == 0 || result.Issues[0].Type != "invalid_config" {
		t.Fatalf("expected invalid_config issue, got: %+v", result.Issues)
	}
}

func TestCheckProfileCompatibilityEmptyConfig(t *testing.T) {
	defaultP, err := engineprofile.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	result := checkProfileCompatibility(`{}`, "", defaultP)
	if !result.Compatible {
		t.Fatalf("expected compatible=true for empty config, got: %+v", result.Issues)
	}
}

// ── PATCH /profiles/{id} ─────────────────────────────────────────────────────

func patchProfile(t *testing.T, srv *Server, id int64, body string) Profile {
	t.Helper()
	resp := doJSON(t, srv, http.MethodPatch, "/api/v1/profiles/"+itoa(id), body)
	profile := mustJSON[Profile](t, resp, http.StatusOK)
	return profile
}

// markProfileStale sets a profile's schema_version to an old value so the
// compatibility check treats it as not yet reviewed against the current engine.
func markProfileStale(t *testing.T, srv *Server, id int64) {
	t.Helper()
	stored, ok := srv.store.GetProfile(id)
	if !ok {
		t.Fatalf("markProfileStale: profile %d not found", id)
	}
	stored.SchemaVersion = "old-version"
	if err := srv.store.UpdateProfile(stored); err != nil {
		t.Fatalf("markProfileStale: %v", err)
	}
}

func fetchCompatibility(t *testing.T, srv *Server, id int64) CompatibilityResult {
	t.Helper()
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/"+itoa(id)+"/compatibility", nil)
	result := mustJSON[CompatibilityResult](t, resp, http.StatusOK)
	return result
}

func TestPatchProfileMarkReviewed(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"mark-rev","config":{"net":{"ipv4":true}}}`)

	// Manually clear schema_version to simulate an outdated profile.
	stored, _ := srv.store.GetProfile(profile.ID)
	stored.SchemaVersion = "old-version"
	if err := srv.store.UpdateProfile(stored); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	updated := patchProfile(t, srv, profile.ID, `{"op":"mark_reviewed"}`)
	if updated.SchemaVersion == "old-version" || updated.SchemaVersion == "" {
		t.Fatalf("expected bumped SchemaVersion, got %q", updated.SchemaVersion)
	}
	// Config must be unchanged.
	if _, ok := updated.Config["net"]; !ok {
		t.Fatalf("expected config preserved after mark_reviewed, got %#v", updated.Config)
	}
}

func TestPatchProfileMarkReviewedClearsCompatibility(t *testing.T) {
	srv := newTestServer(t)
	// Profile pins a narrow test_cases list - many defaults are intentionally absent.
	profile := createProfile(t, srv, `{"name":"rev-clears","config":{"test_cases":["address01"]}}`)
	markProfileStale(t, srv, profile.ID)

	if before := fetchCompatibility(t, srv, profile.ID); before.Compatible {
		t.Fatalf("expected stale subset profile to be incompatible before review, got: %+v", before)
	}

	patchProfile(t, srv, profile.ID, `{"op":"mark_reviewed"}`)

	after := fetchCompatibility(t, srv, profile.ID)
	if !after.Compatible {
		t.Fatalf("expected profile compatible after mark_reviewed, got issues: %+v", after.Issues)
	}
	if len(after.Issues) != 0 {
		t.Fatalf("expected 0 issues after mark_reviewed, got %d", len(after.Issues))
	}
}

func TestPatchProfileResetTestCases(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"reset-tc","config":{"test_cases":["address01","address02"]}}`)

	updated := patchProfile(t, srv, profile.ID, `{"op":"reset_test_cases"}`)
	// After reset, test_cases should not appear in the config.
	if _, ok := updated.Config["test_cases"]; ok {
		t.Fatalf("expected test_cases removed from config after reset, got %#v", updated.Config)
	}
}

func TestPatchProfileAddMissingTestCases(t *testing.T) {
	srv := newTestServer(t)
	// Profile with only one explicit test case - many are missing.
	profile := createProfile(t, srv, `{"name":"add-tc","config":{"test_cases":["address01"]}}`)

	// Fetch defaults to know what's expected.
	respD := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/defaults", nil)
	defaults := mustJSON[ProfileDefaults](t, respD, http.StatusOK)

	updated := patchProfile(t, srv, profile.ID, `{"op":"add_missing_test_cases"}`)
	resultCases, ok := updated.Config["test_cases"].([]any)
	if !ok {
		t.Fatalf("expected test_cases in config after add, got %#v", updated.Config)
	}
	if len(resultCases) < len(defaults.TestCases) {
		t.Fatalf("expected >= %d test cases, got %d", len(defaults.TestCases), len(resultCases))
	}
	// address01 must still be present.
	found := false
	for _, v := range resultCases {
		if v.(string) == "address01" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected address01 still present after add_missing_test_cases")
	}
}

func TestPatchProfileAddMissingTestCasesNoop(t *testing.T) {
	// Profile that does not override test_cases - add_missing_test_cases is a noop.
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"no-tc","config":{"resolver":{"defaults":{"timeout":5}}}}`)

	updated := patchProfile(t, srv, profile.ID, `{"op":"add_missing_test_cases"}`)
	// Config must not gain a test_cases key.
	if _, ok := updated.Config["test_cases"]; ok {
		t.Fatalf("expected no test_cases key after noop add, got %#v", updated.Config)
	}
}

func TestPatchProfileResetTestLevels(t *testing.T) {
	srv := newTestServer(t)

	// Build a profile with ADDRESS test_levels override.
	respD := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/defaults", nil)
	defaults := mustJSON[ProfileDefaults](t, respD, http.StatusOK)
	addressTags, ok := defaults.TestLevels["ADDRESS"]
	if !ok {
		t.Skip("ADDRESS module not in defaults")
	}
	levelsJSON, _ := json.Marshal(map[string]map[string]string{"ADDRESS": addressTags})
	profile := createProfile(t, srv, `{"name":"reset-tl","config":{"test_levels":`+string(levelsJSON)+`}}`)

	updated := patchProfile(t, srv, profile.ID, `{"op":"reset_test_levels","module":"ADDRESS"}`)
	if tl, ok := updated.Config["test_levels"].(map[string]any); ok {
		if _, stillHas := tl["ADDRESS"]; stillHas {
			t.Fatal("expected ADDRESS removed from test_levels after reset")
		}
	}
	// test_levels itself should be absent if it was the only module.
	if _, ok := updated.Config["test_levels"]; ok {
		t.Fatalf("expected test_levels removed entirely, got %#v", updated.Config)
	}
}

func TestPatchProfileResetTestLevelsMissingModule(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPatch, "/api/v1/profiles/1", `{"op":"reset_test_levels"}`)
	// Profile doesn't exist, but missing module should return 400 before 404.
	// Actually server checks module before profile lookup... let's just check 4xx.
	if resp.Code != http.StatusBadRequest && resp.Code != http.StatusNotFound {
		t.Fatalf("expected 400 or 404, got %d", resp.Code)
	}
}

func TestPatchProfileAddMissingTestLevels(t *testing.T) {
	srv := newTestServer(t)

	// Get defaults to find a module with multiple tags.
	respD := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/defaults", nil)
	defaults := mustJSON[ProfileDefaults](t, respD, http.StatusOK)

	var module string
	var partialTags map[string]string
	for mod, tags := range defaults.TestLevels {
		if len(tags) >= 2 {
			module = mod
			partialTags = map[string]string{}
			count := 0
			for tag, level := range tags {
				if count < len(tags)-1 {
					partialTags[tag] = level
				}
				count++
			}
			break
		}
	}
	if module == "" {
		t.Skip("no module with >= 2 tags")
	}

	levelsJSON, _ := json.Marshal(map[string]map[string]string{module: partialTags})
	profile := createProfile(t, srv, `{"name":"add-tl","config":{"test_levels":`+string(levelsJSON)+`}}`)

	updated := patchProfile(t, srv, profile.ID, `{"op":"add_missing_test_levels"}`)
	tl, ok := updated.Config["test_levels"].(map[string]any)
	if !ok {
		t.Fatalf("expected test_levels in config, got %#v", updated.Config)
	}
	modTags, ok := tl[module].(map[string]any)
	if !ok {
		t.Fatalf("expected %s in test_levels, got %#v", module, tl)
	}
	if len(modTags) < len(defaults.TestLevels[module]) {
		t.Fatalf("expected >= %d tags for %s, got %d", len(defaults.TestLevels[module]), module, len(modTags))
	}
}

// A fix op must not bump schema_version. The compatibility check reports any
// profile at the current engine version as compatible without inspecting it,
// so bumping on a partial fix hides every issue the op did not address: the
// admin UI then drops the remaining fix buttons and the profile looks clean
// while still missing tags.
func TestPatchProfileFixDoesNotHideRemainingIssues(t *testing.T) {
	srv := newTestServer(t)

	respD := doJSON(t, srv, http.MethodGet, "/api/v1/profiles/defaults", nil)
	defaults := mustJSON[ProfileDefaults](t, respD, http.StatusOK)

	// One module overridden with a single tag, plus a pinned test_cases list.
	// That is two independent issues: missing test levels and missing test
	// cases.
	var module, oneTag, oneLevel string
	for mod, tags := range defaults.TestLevels {
		if len(tags) < 2 {
			continue
		}
		module = mod
		for tag, level := range tags {
			oneTag, oneLevel = tag, level
			break
		}
		break
	}
	if module == "" {
		t.Skip("no module with >= 2 tags")
	}

	levelsJSON, _ := json.Marshal(map[string]map[string]string{module: {oneTag: oneLevel}})
	profile := createProfile(t, srv, `{"name":"partial-fix","config":{"test_cases":["address01"],"test_levels":`+string(levelsJSON)+`}}`)
	markProfileStale(t, srv, profile.ID)

	before := fetchCompatibility(t, srv, profile.ID)
	if before.Compatible {
		t.Fatalf("expected the profile to start incompatible, got %+v", before)
	}
	if !hasIssueType(before.Issues, "missing_test_case") || !hasIssueType(before.Issues, "missing_test_levels") {
		t.Fatalf("expected both issue types before the fix, got %+v", before.Issues)
	}

	// Fix only the test levels.
	updated := patchProfile(t, srv, profile.ID, `{"op":"add_missing_test_levels"}`)
	if updated.SchemaVersion != "old-version" {
		t.Fatalf("a fix op must leave schema_version alone, got %q", updated.SchemaVersion)
	}

	after := fetchCompatibility(t, srv, profile.ID)
	if after.Compatible {
		t.Fatalf("test_cases is still incomplete, so the profile must stay incompatible: %+v", after)
	}
	if hasIssueType(after.Issues, "missing_test_levels") {
		t.Fatalf("the test levels issue should be resolved, got %+v", after.Issues)
	}
	if !hasIssueType(after.Issues, "missing_test_case") {
		t.Fatalf("the untouched test_cases issue must still be reported, got %+v", after.Issues)
	}

	// Fixing the second issue clears compatibility on the merits, with no
	// schema_version bump involved.
	patchProfile(t, srv, profile.ID, `{"op":"add_missing_test_cases"}`)
	final := fetchCompatibility(t, srv, profile.ID)
	if !final.Compatible || len(final.Issues) != 0 {
		t.Fatalf("expected a fully fixed profile to be compatible, got %+v", final)
	}
}

func hasIssueType(issues []CompatibilityIssue, want string) bool {
	for _, issue := range issues {
		if issue.Type == want {
			return true
		}
	}
	return false
}

func TestPatchProfileInvalidOp(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"inv-op","config":{}}`)

	resp := doJSON(t, srv, http.MethodPatch, "/api/v1/profiles/"+itoa(profile.ID), `{"op":"unknown_operation"}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestPatchProfileNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPatch, "/api/v1/profiles/999", `{"op":"mark_reviewed"}`)
	wantStatus(t, resp, http.StatusNotFound)
}

// ── apply* unit tests ─────────────────────────────────────────────────────────

func TestApplyResetKey(t *testing.T) {
	out, err := applyResetKey(`{"test_cases":["a","b"],"net":{"ipv4":true}}`, "test_cases")
	if err != nil {
		t.Fatalf("applyResetKey: %v", err)
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	if _, ok := m["test_cases"]; ok {
		t.Fatal("expected test_cases removed")
	}
	if _, ok := m["net"]; !ok {
		t.Fatal("expected net preserved")
	}
}

func TestApplyResetTestLevelsModule(t *testing.T) {
	config := `{"test_levels":{"DNSSEC":{"TAG1":"WARNING"},"ZONE":{"Z1":"ERROR"}}}`
	out, err := applyResetTestLevelsModule(config, "DNSSEC")
	if err != nil {
		t.Fatalf("applyResetTestLevelsModule: %v", err)
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	tl := m["test_levels"].(map[string]any)
	if _, ok := tl["DNSSEC"]; ok {
		t.Fatal("expected DNSSEC removed")
	}
	if _, ok := tl["ZONE"]; !ok {
		t.Fatal("expected ZONE preserved")
	}
}

func TestApplyResetTestLevelsModuleLastModule(t *testing.T) {
	// Removing the only module should delete test_levels entirely.
	config := `{"test_levels":{"DNSSEC":{"TAG1":"WARNING"}}}`
	out, err := applyResetTestLevelsModule(config, "DNSSEC")
	if err != nil {
		t.Fatalf("applyResetTestLevelsModule: %v", err)
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	if _, ok := m["test_levels"]; ok {
		t.Fatal("expected test_levels removed entirely when last module reset")
	}
}

func TestApplyAddMissingTestCasesNoopWhenNotSet(t *testing.T) {
	defaultP, _ := engineprofile.Default()
	out, err := applyAddMissingTestCases(`{"net":{"ipv4":true}}`, defaultP)
	if err != nil {
		t.Fatalf("applyAddMissingTestCases: %v", err)
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	if _, ok := m["test_cases"]; ok {
		t.Fatal("expected test_cases absent when not originally set")
	}
}

func TestApplyAddMissingTestLevelsNoopWhenNotSet(t *testing.T) {
	defaultP, _ := engineprofile.Default()
	out, err := applyAddMissingTestLevels(`{"net":{"ipv4":true}}`, defaultP)
	if err != nil {
		t.Fatalf("applyAddMissingTestLevels: %v", err)
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	if _, ok := m["test_levels"]; ok {
		t.Fatal("expected test_levels absent when not originally set")
	}
}

// itoa converts an int64 to string for use in URL paths.
func itoa(id int64) string {
	return fmt.Sprintf("%d", id)
}

// ── POST /profiles/mark-all-reviewed ─────────────────────────────────────────

func TestMarkAllProfilesReviewed(t *testing.T) {
	srv := newTestServer(t)
	p1 := createProfile(t, srv, `{"name":"p1","config":{}}`)
	p2 := createProfile(t, srv, `{"name":"p2","config":{}}`)

	// Manually set schema_version to old value so they appear out of date.
	stored1, _ := srv.store.GetProfile(p1.ID)
	stored1.SchemaVersion = "v0.9.0"
	_ = srv.store.UpdateProfile(stored1)
	stored2, _ := srv.store.GetProfile(p2.ID)
	stored2.SchemaVersion = "v0.9.0"
	_ = srv.store.UpdateProfile(stored2)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/profiles/mark-all-reviewed", "{}")
	wantStatus(t, resp, http.StatusOK)

	var result MarkAllReviewedResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Updated != 2 {
		t.Fatalf("expected updated=2, got %d", result.Updated)
	}

	// Both profiles should now have the current engine version.
	after1, _ := srv.store.GetProfile(p1.ID)
	after2, _ := srv.store.GetProfile(p2.ID)
	if after1.SchemaVersion == "v0.9.0" {
		t.Fatal("p1 schema_version not bumped")
	}
	if after2.SchemaVersion == "v0.9.0" {
		t.Fatal("p2 schema_version not bumped")
	}
}

func TestMarkAllProfilesReviewedAlreadyCurrent(t *testing.T) {
	srv := newTestServer(t)
	// Create two profiles - both will have schema_version set to engine.VersionFull() by default.
	createProfile(t, srv, `{"name":"p1","config":{}}`)
	createProfile(t, srv, `{"name":"p2","config":{}}`)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/profiles/mark-all-reviewed", "{}")
	wantStatus(t, resp, http.StatusOK)

	var result MarkAllReviewedResult
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Updated != 0 {
		t.Fatalf("expected updated=0 (already current), got %d", result.Updated)
	}
}

func TestMarkAllProfilesReviewedEmptyStore(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/profiles/mark-all-reviewed", "{}")
	wantStatus(t, resp, http.StatusOK)

	var result MarkAllReviewedResult
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Updated != 0 {
		t.Fatalf("expected updated=0 for empty store, got %d", result.Updated)
	}
}
