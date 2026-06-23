package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"codeberg.org/pawal/gonemaster/scoring"
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

// ApplyDatabaseSettings loads settings from the DB and merges them into s.cfg.
// CLI flags (tracked in configSources) take precedence and are not overridden.
func (s *Server) ApplyDatabaseSettings() {
	dbSettings := s.store.ListSettings()
	for key, val := range dbSettings {
		if s.configSources != nil {
			if s.configSources[key] == SourceCLIFlag {
				continue
			}
		}
		s.applySetting(key, val)
	}
}

// applySetting writes a single setting value into s.cfg.
func (s *Server) applySetting(key, val string) {
	switch key {
	case "worker_count":
		if v, err := strconv.Atoi(val); err == nil && v >= 1 {
			s.cfg.WorkerCount = v
		}
	case "max_concurrent_jobs":
		if v, err := strconv.Atoi(val); err == nil && v >= 0 {
			s.cfg.MaxConcurrentJobs = v
		}
	case "min_level":
		s.cfg.MinLevel = val
	case "retention_days":
		if v, err := strconv.Atoi(val); err == nil && v >= 0 {
			s.cfg.Database.RetentionDays = v
			s.retentionDays.Store(int64(v))
		}
	case "purge_interval_seconds":
		if v, err := strconv.Atoi(val); err == nil && v >= 1 {
			s.cfg.Database.PurgeIntervalSeconds = v
			s.purgeIntervalSec.Store(int64(v))
		}
	case "public_url":
		s.cfg.PublicURL = val
	case "rate_limit_enabled":
		s.cfg.PublicAPI.RateLimitEnabled = val == "true"
	case "rate_limit_max":
		if v, err := strconv.Atoi(val); err == nil && v >= 1 {
			s.cfg.PublicAPI.RateLimitMax = v
		}
	case "rate_limit_window":
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			s.cfg.PublicAPI.RateLimitWindow = Duration{d}
		}
	case "allow_private_undelegated_ip":
		s.cfg.PublicAPI.AllowPrivateUndelegatedIP = val == "true"
	case "allow_non_global_targets":
		s.cfg.PublicAPI.AllowNonGlobalTargets = val == "true"
	case "show_score_admin":
		s.cfg.ShowScoreAdmin = val == "true"
	case "show_score_public":
		s.cfg.ShowScorePublic = val == "true"
	case "show_nameserver_timings_admin":
		s.cfg.ShowNameserverTimingsAdmin = val == "true"
	case "show_nameserver_timings_public":
		s.cfg.ShowNameserverTimingsPublic = val == "true"
	case "cross_job_hot_cache_ttl_seconds":
		if v, err := strconv.Atoi(val); err == nil && v >= 1 {
			s.cfg.CrossJobHotCacheTTLSeconds = v
		}
	case "scoring_config":
		var cfg scoring.Config
		if err := json.Unmarshal([]byte(val), &cfg); err == nil {
			applyScoringConfigToStore(s.store, cfg)
		}
	}
}

// applySettingsToRuntime updates live server components after settings change.
// This handles hot-reload of mutable settings that affect runtime behavior.
func (s *Server) applySettingsToRuntime() {
	// Re-apply all DB settings to cfg (respecting CLI flag precedence).
	s.ApplyDatabaseSettings()

	// Resize the worker pool to match the new worker_count.
	s.resizeWorkerPool(s.cfg.WorkerCount)

	// Update engine concurrency limiter.
	s.engineLimiter = newEngineLimiter(s.cfg.MaxConcurrentJobs)

	// Update hot cache TTL.
	s.hotCache.SetTTL(s.cfg.EffectiveCrossJobHotCacheTTL())

	// Update rate limiter.
	if s.cfg.PublicAPI.RateLimitEnabled {
		s.rateLimiter = NewRateLimiter(
			s.cfg.PublicAPI.RateLimitMax,
			s.cfg.PublicAPI.RateLimitWindow.Duration,
		)
	} else {
		s.rateLimiter = nil
	}
}

// featuresResponse holds server-side feature flags exposed to the admin UI.
type featuresResponse struct {
	ShowScoreAdmin             bool `json:"show_score_admin"`
	ShowNameserverTimingsAdmin bool `json:"show_nameserver_timings_admin"`
}

// handleFeatures handles GET /api/v1/features.
// Returns a lightweight set of feature flags the admin UI reads on startup
// to decide which UI components to display.
func (s *Server) handleFeatures(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, featuresResponse{
		ShowScoreAdmin:             s.cfg.ShowScoreAdmin,
		ShowNameserverTimingsAdmin: s.cfg.ShowNameserverTimingsAdmin,
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSettings(w, r)
	case http.MethodPut:
		if !s.enforceCSRF(w, r) {
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
		"listen_addr":                     {Value: cfg.ListenAddr, Source: s.settingSource("listen_addr"), Readonly: true},
		"db_driver":                       {Value: cfg.Database.Driver, Source: s.settingSource("db_driver"), Readonly: true},
		"db_dsn":                          {Value: cfg.Database.DSN, Source: s.settingSource("db_dsn"), Readonly: true},
		"profile_path":                    {Value: cfg.ProfilePath, Source: s.settingSource("profile_path"), Readonly: true},
		"worker_count":                    {Value: cfg.WorkerCount, Source: s.settingSource("worker_count")},
		"max_concurrent_jobs":             {Value: cfg.MaxConcurrentJobs, Source: s.settingSource("max_concurrent_jobs")},
		"min_level":                       {Value: cfg.MinLevel, Source: s.settingSource("min_level")},
		"retention_days":                  {Value: cfg.Database.RetentionDays, Source: s.settingSource("retention_days")},
		"purge_interval_seconds":          {Value: int(cfg.EffectivePurgeInterval() / time.Second), Source: s.settingSource("purge_interval_seconds")},
		"public_url":                      {Value: cfg.PublicURL, Source: s.settingSource("public_url")},
		"rate_limit_enabled":              {Value: cfg.PublicAPI.RateLimitEnabled, Source: s.settingSource("rate_limit_enabled")},
		"rate_limit_max":                  {Value: cfg.PublicAPI.RateLimitMax, Source: s.settingSource("rate_limit_max")},
		"rate_limit_window":               {Value: cfg.PublicAPI.RateLimitWindow.Duration.String(), Source: s.settingSource("rate_limit_window")},
		"allow_private_undelegated_ip":    {Value: cfg.PublicAPI.AllowPrivateUndelegatedIP, Source: s.settingSource("allow_private_undelegated_ip")},
		"allow_non_global_targets":        {Value: cfg.PublicAPI.AllowNonGlobalTargets, Source: s.settingSource("allow_non_global_targets")},
		"show_score_admin":                {Value: cfg.ShowScoreAdmin, Source: s.settingSource("show_score_admin")},
		"show_score_public":               {Value: cfg.ShowScorePublic, Source: s.settingSource("show_score_public")},
		"show_nameserver_timings_admin":   {Value: cfg.ShowNameserverTimingsAdmin, Source: s.settingSource("show_nameserver_timings_admin")},
		"show_nameserver_timings_public":  {Value: cfg.ShowNameserverTimingsPublic, Source: s.settingSource("show_nameserver_timings_public")},
		"cross_job_hot_cache_ttl_seconds": {Value: cfg.CrossJobHotCacheTTLSeconds, Source: s.settingSource("cross_job_hot_cache_ttl_seconds")},
	}

	// Apply database overrides to the value display.
	for key, val := range dbSettings {
		if entry, ok := settings[key]; ok {
			entry.Value = parseSettingValue(val)
			entry.Source = SourceDatabase
			settings[key] = entry
		}
	}

	writeJSON(w, http.StatusOK, settings)
}

// parseSettingValue converts a stored string to a typed value for JSON output.
// It tries JSON number, then JSON boolean, then falls back to string.
func parseSettingValue(s string) any {
	var n json.Number
	if json.Unmarshal([]byte(s), &n) == nil {
		return n
	}
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	return s
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

	s.applySettingsToRuntime()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
