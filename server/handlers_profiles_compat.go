package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	engineprofile "codeberg.org/pawal/gonemaster/engine/profile"
)

// CompatibilityIssue describes a single compatibility gap between a stored
// profile and the current engine defaults.
type CompatibilityIssue struct {
	Type       string `json:"type"`             // "missing_test_case" | "missing_test_levels"
	Module     string `json:"module,omitempty"` // set for "missing_test_levels"
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion"`
}

// CompatibilityResult is the response body for GET /profiles/{id}/compatibility.
// A reviewed profile reports its remaining gaps as waived issues rather than
// hiding them, so the review can be inspected and undone.
type CompatibilityResult struct {
	Compatible     bool                 `json:"compatible"`
	Reviewed       bool                 `json:"reviewed"`
	SchemaVersion  string               `json:"schema_version"`
	CurrentVersion string               `json:"current_version"`
	Issues         []CompatibilityIssue `json:"issues"`
	WaivedIssues   []CompatibilityIssue `json:"waived_issues,omitempty"`
}

// CompatibilitySummary is one entry in the GET /profiles/compatibility response.
type CompatibilitySummary struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Compatible  bool   `json:"compatible"`
	IssueCount  int    `json:"issue_count"`
	WaivedCount int    `json:"waived_count"`
}

// ProfileDefaults is the response body for GET /profiles/defaults.
type ProfileDefaults struct {
	TestCases  []string                     `json:"test_cases"`
	TestLevels map[string]map[string]string `json:"test_levels"`
}

// handleProfileCompatibility handles GET /profiles/{id}/compatibility.
func (s *Server) handleProfileCompatibility(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	id, ok := parseProfileID(w, r)
	if !ok {
		return
	}
	stored, ok := s.store.GetProfile(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "profile not found", nil)
		return
	}
	defaultP, err := engineprofile.Default()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "engine_error", fmt.Sprintf("load default profile: %s", err), nil)
		return
	}
	result := checkProfileCompatibility(stored.Config, stored.SchemaVersion, defaultP)
	writeJSON(w, http.StatusOK, result)
}

// handleProfilesCompatibility handles GET /profiles/compatibility.
func (s *Server) handleProfilesCompatibility(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	defaultP, err := engineprofile.Default()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "engine_error", fmt.Sprintf("load default profile: %s", err), nil)
		return
	}
	profiles := s.store.ListProfiles()
	summaries := make([]CompatibilitySummary, 0, len(profiles))
	for _, p := range profiles {
		result := checkProfileCompatibility(p.Config, p.SchemaVersion, defaultP)
		summaries = append(summaries, CompatibilitySummary{
			ID:          p.ID,
			Name:        p.Name,
			Compatible:  result.Compatible,
			IssueCount:  len(result.Issues),
			WaivedCount: len(result.WaivedIssues),
		})
	}
	writeJSON(w, http.StatusOK, summaries)
}

// handleProfileDefaults handles GET /profiles/defaults.
// Returns the engine default profile's test_cases and test_levels for use as
// the canonical reference when checking stored profile compatibility.
func (s *Server) handleProfileDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	defaultP, err := engineprofile.Default()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "engine_error", fmt.Sprintf("load default profile: %s", err), nil)
		return
	}

	var testCases []string
	if raw, _ := defaultP.Get("test_cases"); raw != nil {
		for _, v := range raw.([]any) {
			if s, ok := v.(string); ok {
				testCases = append(testCases, s)
			}
		}
	}
	sort.Strings(testCases)

	var testLevels map[string]map[string]string
	if raw, _ := defaultP.Get("test_levels"); raw != nil {
		testLevels = raw.(map[string]map[string]string)
	}
	if testLevels == nil {
		testLevels = map[string]map[string]string{}
	}

	writeJSON(w, http.StatusOK, ProfileDefaults{
		TestCases:  testCases,
		TestLevels: testLevels,
	})
}

// checkProfileCompatibility returns the compatibility result for a stored profile
// config JSON against the provided engine default profile.
//
// The comparison always runs. A profile reviewed against the current engine
// version keeps reporting compatible, but its remaining gaps are listed as
// waived issues instead of being dropped silently.
func checkProfileCompatibility(configJSON, schemaVersion string, defaultP *engineprofile.Profile) CompatibilityResult {
	currentVersion := engine.VersionFull()
	result := CompatibilityResult{
		Compatible:     true,
		Reviewed:       schemaVersion == currentVersion,
		SchemaVersion:  schemaVersion,
		CurrentVersion: currentVersion,
		Issues:         []CompatibilityIssue{},
	}

	storedP, err := engineprofile.FromJSON(configJSON)
	if err != nil {
		// A config the engine cannot parse is broken whatever the stamp says,
		// so this issue is never waived.
		result.Compatible = false
		result.Issues = append(result.Issues, CompatibilityIssue{
			Type:       "invalid_config",
			Detail:     fmt.Sprintf("profile config cannot be parsed: %s", err),
			Suggestion: "Edit the profile config to contain valid JSON matching the engine profile schema.",
		})
		return result
	}

	var found []CompatibilityIssue

	// Check test_cases: if the stored profile explicitly sets test_cases,
	// compare against default's test_cases.
	if storedRaw, _ := storedP.Get("test_cases"); storedRaw != nil {
		storedSet := make(map[string]bool)
		for _, v := range storedRaw.([]any) {
			if s, ok := v.(string); ok {
				storedSet[s] = true
			}
		}
		var missing []string
		if defaultRaw, _ := defaultP.Get("test_cases"); defaultRaw != nil {
			for _, v := range defaultRaw.([]any) {
				if s, ok := v.(string); ok {
					if !storedSet[s] {
						missing = append(missing, s)
					}
				}
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			found = append(found, CompatibilityIssue{
				Type:       "missing_test_case",
				Detail:     fmt.Sprintf("Profile sets test_cases but is missing: %s", joinStrings(missing)),
				Suggestion: `Add the missing test case IDs to test_cases, or remove test_cases entirely to inherit all defaults.`,
			})
		}
	}

	// Check test_levels: if the stored profile explicitly sets test_levels,
	// compare per-module tag coverage against default's test_levels.
	if storedRaw, _ := storedP.Get("test_levels"); storedRaw != nil {
		storedLevels := storedRaw.(map[string]map[string]string)
		var defaultLevels map[string]map[string]string
		if defaultRaw, _ := defaultP.Get("test_levels"); defaultRaw != nil {
			defaultLevels = defaultRaw.(map[string]map[string]string)
		}
		for module, storedTags := range storedLevels {
			defaultTags, ok := defaultLevels[module]
			if !ok {
				continue
			}
			var missing []string
			for tag := range defaultTags {
				if _, covered := storedTags[tag]; !covered {
					missing = append(missing, tag)
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				found = append(found, CompatibilityIssue{
					Type:       "missing_test_levels",
					Module:     module,
					Detail:     fmt.Sprintf("Profile overrides %s test_levels but is missing tags: %s", module, joinStrings(missing)),
					Suggestion: fmt.Sprintf(`Add severity levels for the missing %s tags, or remove %s from test_levels to inherit defaults.`, module, module),
				})
			}
		}
	}

	// A reviewed profile declares its remaining gaps intentional, so they are
	// waived rather than reported - but they stay visible.
	if result.Reviewed {
		result.WaivedIssues = found
		return result
	}
	if len(found) > 0 {
		result.Compatible = false
		result.Issues = found
	}
	return result
}

func joinStrings(ss []string) string {
	var out strings.Builder
	for i, s := range ss {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(s)
	}
	return out.String()
}

// ── PATCH /profiles/{id} ─────────────────────────────────────────────────────

type profilePatchRequest struct {
	Op     string `json:"op"`               // operation name
	Module string `json:"module,omitempty"` // for reset_test_levels
}

// handlePatchProfile handles PATCH /profiles/{id}.
// Supported operations:
//
//	add_missing_test_cases  - append any default test cases absent from the profile
//	add_missing_test_levels - fill missing tags in already-overridden test_levels modules
//	reset_test_cases        - remove test_cases override (profile inherits all defaults)
//	reset_test_levels       - remove one test_levels module (requires module field)
//	mark_reviewed           - bump schema_version without touching config
//	clear_reviewed          - clear schema_version without touching config
//
// The fix ops change config only; a fixed profile then reports compatible on
// its own merits. Only mark_reviewed declares remaining gaps intentional, and
// clear_reviewed takes that declaration back.
func (s *Server) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !s.enforceCSRF(w, r) {
		return
	}
	id, ok := parseProfileID(w, r)
	if !ok {
		return
	}
	stored, ok := s.store.GetProfile(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "profile not found", nil)
		return
	}

	var req profilePatchRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}

	defaultP, err := engineprofile.Default()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "engine_error", fmt.Sprintf("load default profile: %s", err), nil)
		return
	}

	var newConfigJSON string
	// Only mark_reviewed bumps schema_version. A fix op that also bumped it
	// would silence the checks for every issue it did not fix, since a
	// profile at the current version is reported compatible unconditionally.
	markReviewed := false
	clearReviewed := false
	switch req.Op {
	case "add_missing_test_cases":
		newConfigJSON, err = applyAddMissingTestCases(stored.Config, defaultP)
	case "add_missing_test_levels":
		newConfigJSON, err = applyAddMissingTestLevels(stored.Config, defaultP)
	case "reset_test_cases":
		newConfigJSON, err = applyResetKey(stored.Config, "test_cases")
	case "reset_test_levels":
		if strings.TrimSpace(req.Module) == "" {
			writeError(w, http.StatusBadRequest, "missing_module", "module is required for reset_test_levels", nil)
			return
		}
		newConfigJSON, err = applyResetTestLevelsModule(stored.Config, req.Module)
	case "mark_reviewed":
		// no config change - just bump schema_version below
		newConfigJSON = stored.Config
		markReviewed = true
	case "clear_reviewed":
		// no config change - just clear schema_version below
		newConfigJSON = stored.Config
		clearReviewed = true
	default:
		writeError(w, http.StatusBadRequest, "invalid_op", fmt.Sprintf("unknown op %q", req.Op), nil)
		return
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error(), nil)
		return
	}

	stored.Config = newConfigJSON
	switch {
	case markReviewed:
		stored.SchemaVersion = engine.VersionFull()
	case clearReviewed:
		stored.SchemaVersion = ""
	}
	if err := s.store.UpdateProfile(stored); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}

	updated, ok := s.store.GetProfile(id)
	if !ok {
		writeError(w, http.StatusInternalServerError, "store_error", "updated profile not found", nil)
		return
	}
	apiProfile, err := apiProfileFromStored(updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, apiProfile)
}

// MarkAllReviewedResult is the response body for POST /profiles/mark-all-reviewed.
type MarkAllReviewedResult struct {
	Updated int `json:"updated"`
}

// handleMarkAllProfilesReviewed handles POST /profiles/mark-all-reviewed.
// Bumps schema_version to the current engine version for every stored profile.
func (s *Server) handleMarkAllProfilesReviewed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !s.enforceCSRF(w, r) {
		return
	}
	profiles := s.store.ListProfiles()
	currentVersion := engine.VersionFull()
	updated := 0
	for _, p := range profiles {
		if p.SchemaVersion == currentVersion {
			continue
		}
		p.SchemaVersion = currentVersion
		if err := s.store.UpdateProfile(p); err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
		updated++
	}
	writeJSON(w, http.StatusOK, MarkAllReviewedResult{Updated: updated})
}

// applyAddMissingTestCases returns updated configJSON with any default test
// cases that are absent from the profile's test_cases list appended and sorted.
// If the profile does not explicitly set test_cases, the original JSON is
// returned unchanged (no override means all defaults are inherited already).
func applyAddMissingTestCases(configJSON string, defaultP *engineprofile.Profile) (string, error) {
	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "", fmt.Errorf("parse profile config: %w", err)
	}

	existing, ok := config["test_cases"].([]any)
	if !ok {
		// test_cases not set - nothing to add (already inherits everything)
		return configJSON, nil
	}

	existingSet := make(map[string]bool, len(existing))
	for _, v := range existing {
		if s, ok := v.(string); ok {
			existingSet[s] = true
		}
	}

	if defaultRaw, _ := defaultP.Get("test_cases"); defaultRaw != nil {
		for _, v := range defaultRaw.([]any) {
			if s, ok := v.(string); ok && !existingSet[s] {
				existing = append(existing, s)
				existingSet[s] = true
			}
		}
	}
	sort.Slice(existing, func(i, j int) bool {
		return existing[i].(string) < existing[j].(string)
	})
	config["test_cases"] = existing

	out, err := json.Marshal(config)
	return string(out), err
}

// applyAddMissingTestLevels returns updated configJSON where each test_levels
// module that is already overridden gets any tags present in the default but
// absent from the stored version filled in with the default severity.
func applyAddMissingTestLevels(configJSON string, defaultP *engineprofile.Profile) (string, error) {
	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "", fmt.Errorf("parse profile config: %w", err)
	}

	levelsRaw, ok := config["test_levels"].(map[string]any)
	if !ok {
		// test_levels not set - nothing to do
		return configJSON, nil
	}

	var defaultLevels map[string]map[string]string
	if raw, _ := defaultP.Get("test_levels"); raw != nil {
		defaultLevels = raw.(map[string]map[string]string)
	}

	for module, tagsRaw := range levelsRaw {
		tagsMap, ok := tagsRaw.(map[string]any)
		if !ok {
			continue
		}
		defTags, hasModule := defaultLevels[module]
		if !hasModule {
			continue
		}
		for tag, level := range defTags {
			if _, covered := tagsMap[tag]; !covered {
				tagsMap[tag] = level
			}
		}
		levelsRaw[module] = tagsMap
	}
	config["test_levels"] = levelsRaw

	out, err := json.Marshal(config)
	return string(out), err
}

// applyResetKey removes a top-level key from the config JSON, causing the
// profile to inherit defaults for that property.
func applyResetKey(configJSON, key string) (string, error) {
	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "", fmt.Errorf("parse profile config: %w", err)
	}
	delete(config, key)
	out, err := json.Marshal(config)
	return string(out), err
}

// applyResetTestLevelsModule removes one module from the test_levels map,
// causing that module to inherit all default tag severities.
func applyResetTestLevelsModule(configJSON, module string) (string, error) {
	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "", fmt.Errorf("parse profile config: %w", err)
	}
	if levelsRaw, ok := config["test_levels"].(map[string]any); ok {
		delete(levelsRaw, module)
		if len(levelsRaw) == 0 {
			delete(config, "test_levels")
		} else {
			config["test_levels"] = levelsRaw
		}
	}
	out, err := json.Marshal(config)
	return string(out), err
}
