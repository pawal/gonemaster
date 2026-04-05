package server

import (
	"encoding/json"
	"net/http"
)

// SettingSource identifies where a config value was set.
type SettingSource string

const (
	SourceDefault    SettingSource = "default"
	SourceConfigFile SettingSource = "config_file"
	SourceDatabase   SettingSource = "database"
	SourceCLIFlag    SettingSource = "cli_flag"
)

// settingEntry describes one setting in the GET /settings response.
type settingEntry struct {
	Value    any           `json:"value"`
	Source   SettingSource `json:"source"`
	Readonly bool          `json:"readonly,omitempty"`
}

// readonlySettings are boot-time-only settings that cannot be changed at runtime.
var readonlySettings = map[string]bool{
	"listen_addr":  true,
	"db_driver":    true,
	"db_dsn":       true,
	"profile_path": true,
}

// SetConfigSources allows the cmd layer to record where each config value came from.
func (s *Server) SetConfigSources(sources map[string]SettingSource) {
	s.configSources = sources
}

// settingSource returns the source for a given setting key.
func (s *Server) settingSource(key string) SettingSource {
	// Database overrides take priority when they exist.
	if _, ok := s.store.GetSetting(key); ok {
		return SourceDatabase
	}
	if s.configSources != nil {
		if src, ok := s.configSources[key]; ok {
			return src
		}
	}
	return SourceDefault
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSettings(w, r)
	case http.MethodPut:
		if !enforceCSRF(w, r) {
			return
		}
		s.handlePutSettings(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	cfg := s.cfg
	dbSettings := s.store.ListSettings()

	// Build the settings map with current effective values.
	settings := map[string]settingEntry{
		"listen_addr":        {Value: cfg.ListenAddr, Source: s.settingSource("listen_addr"), Readonly: true},
		"db_driver":          {Value: cfg.Database.Driver, Source: s.settingSource("db_driver"), Readonly: true},
		"db_dsn":             {Value: cfg.Database.DSN, Source: s.settingSource("db_dsn"), Readonly: true},
		"profile_path":       {Value: cfg.ProfilePath, Source: s.settingSource("profile_path"), Readonly: true},
		"worker_count":       {Value: cfg.WorkerCount, Source: s.settingSource("worker_count")},
		"max_concurrent_jobs": {Value: cfg.MaxConcurrentJobs, Source: s.settingSource("max_concurrent_jobs")},
		"min_level":          {Value: cfg.MinLevel, Source: s.settingSource("min_level")},
		"retention_days":     {Value: cfg.Database.RetentionDays, Source: s.settingSource("retention_days")},
		"public_url":         {Value: cfg.PublicURL, Source: s.settingSource("public_url")},
		"rate_limit_enabled": {Value: cfg.PublicAPI.RateLimitEnabled, Source: s.settingSource("rate_limit_enabled")},
		"rate_limit_max":     {Value: cfg.PublicAPI.RateLimitMax, Source: s.settingSource("rate_limit_max")},
		"rate_limit_window":  {Value: cfg.PublicAPI.RateLimitWindow.Duration.String(), Source: s.settingSource("rate_limit_window")},
	}

	// Apply database overrides to the value display.
	for key, val := range dbSettings {
		if entry, ok := settings[key]; ok {
			entry.Value = json.Number(val)
			entry.Source = SourceDatabase
			settings[key] = entry
		}
	}

	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var updates map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body", nil)
		return
	}

	for key := range updates {
		if readonlySettings[key] {
			writeError(w, http.StatusBadRequest, "readonly_setting",
				"setting is read-only: "+key, nil)
			return
		}
	}

	for key, raw := range updates {
		// Store as the raw JSON string value.
		val := string(raw)
		// Strip quotes from JSON strings for storage.
		var s2 string
		if json.Unmarshal(raw, &s2) == nil {
			val = s2
		}
		if err := s.store.SetSetting(key, val); err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
