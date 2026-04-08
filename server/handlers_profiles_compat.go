package server

import (
	"fmt"
	"net/http"
	"sort"

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
type CompatibilityResult struct {
	Compatible     bool                 `json:"compatible"`
	SchemaVersion  string               `json:"schema_version"`
	CurrentVersion string               `json:"current_version"`
	Issues         []CompatibilityIssue `json:"issues"`
}

// CompatibilitySummary is one entry in the GET /profiles/compatibility response.
type CompatibilitySummary struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Compatible bool   `json:"compatible"`
	IssueCount int    `json:"issue_count"`
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
			ID:         p.ID,
			Name:       p.Name,
			Compatible: result.Compatible,
			IssueCount: len(result.Issues),
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
func checkProfileCompatibility(configJSON, schemaVersion string, defaultP *engineprofile.Profile) CompatibilityResult {
	currentVersion := engine.VersionFull()
	result := CompatibilityResult{
		Compatible:     true,
		SchemaVersion:  schemaVersion,
		CurrentVersion: currentVersion,
		Issues:         []CompatibilityIssue{},
	}

	storedP, err := engineprofile.FromJSON(configJSON)
	if err != nil {
		result.Compatible = false
		result.Issues = append(result.Issues, CompatibilityIssue{
			Type:       "invalid_config",
			Detail:     fmt.Sprintf("profile config cannot be parsed: %s", err),
			Suggestion: "Edit the profile config to contain valid JSON matching the engine profile schema.",
		})
		return result
	}

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
			result.Compatible = false
			result.Issues = append(result.Issues, CompatibilityIssue{
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
				result.Compatible = false
				result.Issues = append(result.Issues, CompatibilityIssue{
					Type:       "missing_test_levels",
					Module:     module,
					Detail:     fmt.Sprintf("Profile overrides %s test_levels but is missing tags: %s", module, joinStrings(missing)),
					Suggestion: fmt.Sprintf(`Add severity levels for the missing %s tags, or remove %s from test_levels to inherit defaults.`, module, module),
				})
			}
		}
	}

	return result
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
