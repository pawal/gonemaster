package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine/logger"
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
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	for key, val := range dbSettings {
		if s.configSources != nil {
			if s.configSources[key] == SourceCLIFlag {
				continue
			}
		}
		s.applySetting(key, val)
	}
}

// settingKind is the value type of a writable setting.
type settingKind int

const (
	settingBool settingKind = iota
	settingInt
	settingDuration
	settingLevel
	settingString
)

// settingSpec is the type and integer range of a writable setting.
type settingSpec struct {
	kind     settingKind
	min, max int
}

// writableSettings lists the keys PUT /settings accepts.
var writableSettings = map[string]settingSpec{
	"worker_count":                    {kind: settingInt, min: 1, max: 1024},
	"max_concurrent_jobs":             {kind: settingInt, min: 0, max: 1024},
	"stuck_job_timeout_minutes":       {kind: settingInt, min: 0, max: 525600},
	"min_level":                       {kind: settingLevel},
	"retention_days":                  {kind: settingInt, min: 0, max: 36500},
	"purge_interval_seconds":          {kind: settingInt, min: 1, max: 31536000},
	"public_url":                      {kind: settingString},
	"rate_limit_enabled":              {kind: settingBool},
	"rate_limit_max":                  {kind: settingInt, min: 1, max: 100000},
	"rate_limit_get_max":              {kind: settingInt, min: 1, max: 100000},
	"rate_limit_window":               {kind: settingDuration},
	"allow_private_undelegated_ip":    {kind: settingBool},
	"allow_non_global_targets":        {kind: settingBool},
	"show_score_admin":                {kind: settingBool},
	"show_score_public":               {kind: settingBool},
	"show_nameserver_timings_admin":   {kind: settingBool},
	"show_nameserver_timings_public":  {kind: settingBool},
	"show_dnssec_chain_public":        {kind: settingBool},
	"show_asn_names_public":           {kind: settingBool},
	"mcp_enabled":                     {kind: settingBool},
	"mcp_allow_write":                 {kind: settingBool},
	"cross_job_hot_cache_ttl_seconds": {kind: settingInt, min: 1, max: 86400},
}

// checkSetting validates the stored string form of a writable setting.
func checkSetting(key, val string) error {
	spec, ok := writableSettings[key]
	if !ok {
		return fmt.Errorf("unknown setting: %s", key)
	}
	switch spec.kind {
	case settingBool:
		if val != "true" && val != "false" {
			return fmt.Errorf("%s must be true or false", key)
		}
	case settingInt:
		if v, err := strconv.Atoi(val); err != nil || v < spec.min || v > spec.max {
			return fmt.Errorf("%s must be an integer from %d to %d", key, spec.min, spec.max)
		}
	case settingDuration:
		if d, err := time.ParseDuration(val); err != nil || d <= 0 {
			return fmt.Errorf("%s must be a positive duration such as 1m", key)
		}
	case settingLevel:
		if _, ok := logger.Levels()[strings.ToUpper(val)]; !ok {
			return fmt.Errorf("%s is not a log level: %s", key, val)
		}
	}
	return nil
}

// applySetting writes a single setting value into s.cfg.
func (s *Server) applySetting(key, val string) {
	if key == "scoring_config" {
		var cfg scoring.Config
		if err := json.Unmarshal([]byte(val), &cfg); err == nil {
			applyScoringConfigToStore(s.store, cfg)
		}
		return
	}
	if _, ok := writableSettings[key]; !ok {
		return
	}
	if err := checkSetting(key, val); err != nil {
		s.logger.Warn("stored setting ignored", "key", key, "error", err)
		return
	}
	num, _ := strconv.Atoi(val)
	switch key {
	case "worker_count":
		s.cfg.WorkerCount = num
	case "max_concurrent_jobs":
		s.cfg.MaxConcurrentJobs = num
	case "stuck_job_timeout_minutes":
		s.cfg.StuckJobTimeoutMinutes = num
	case "min_level":
		s.cfg.MinLevel = val
	case "retention_days":
		s.cfg.Database.RetentionDays = num
		s.retentionDays.Store(int64(num))
	case "purge_interval_seconds":
		s.cfg.Database.PurgeIntervalSeconds = num
		s.purgeIntervalSec.Store(int64(num))
	case "public_url":
		s.cfg.PublicURL = val
	case "rate_limit_enabled":
		s.cfg.PublicAPI.RateLimitEnabled = val == "true"
	case "rate_limit_max":
		s.cfg.PublicAPI.RateLimitMax = num
	case "rate_limit_get_max":
		s.cfg.PublicAPI.RateLimitGetMax = num
	case "rate_limit_window":
		d, _ := time.ParseDuration(val)
		s.cfg.PublicAPI.RateLimitWindow = Duration{d}
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
	case "show_dnssec_chain_public":
		s.cfg.ShowDNSSECChainPublic = val == "true"
	case "show_asn_names_public":
		s.cfg.ShowASNNamesPublic = val == "true"
	case "mcp_enabled":
		if enabled := val == "true"; enabled != s.cfg.MCPEnabled {
			s.logger.Info("mcp endpoint", "enabled", enabled, "path", "/api/v1/mcp")
			s.cfg.MCPEnabled = enabled
		}
	case "mcp_allow_write":
		s.cfg.MCPAllowWrite = val == "true"
	case "cross_job_hot_cache_ttl_seconds":
		s.cfg.CrossJobHotCacheTTLSeconds = num
	}
}

// liveConfig returns a copy of s.cfg taken under the settings lock.
func (s *Server) liveConfig() Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

// applySettingsToRuntime updates live server components after settings change.
// This handles hot-reload of mutable settings that affect runtime behavior.
func (s *Server) applySettingsToRuntime() {
	// Re-apply all DB settings to cfg (respecting CLI flag precedence).
	s.ApplyDatabaseSettings()
	cfg := s.liveConfig()

	// Resize the worker pool to match the new worker_count.
	s.resizeWorkerPool(cfg.WorkerCount)

	// Update engine concurrency limiter.
	s.engineLimiter.Store(newEngineLimiter(cfg.MaxConcurrentJobs))

	// Update hot cache TTL.
	s.hotCache.SetTTL(cfg.EffectiveCrossJobHotCacheTTL())

	s.applyRateLimit(cfg.PublicAPI)
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
	cfg := s.liveConfig()
	writeJSON(w, http.StatusOK, featuresResponse{
		ShowScoreAdmin:             cfg.ShowScoreAdmin,
		ShowNameserverTimingsAdmin: cfg.ShowNameserverTimingsAdmin,
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSettings(w, r)
	case http.MethodPut:
		s.handlePutSettings(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	cfg := s.liveConfig()
	dbSettings := s.store.ListSettings()

	// Build the settings map with current effective values.
	settings := map[string]settingEntry{
		"listen_addr":                     {Value: cfg.ListenAddr, Source: s.settingSource("listen_addr"), Readonly: true},
		"db_driver":                       {Value: cfg.Database.Driver, Source: s.settingSource("db_driver"), Readonly: true},
		"db_dsn":                          {Value: redactDSN(cfg.Database.Driver, cfg.Database.DSN), Source: s.settingSource("db_dsn"), Readonly: true},
		"profile_path":                    {Value: cfg.ProfilePath, Source: s.settingSource("profile_path"), Readonly: true},
		"worker_count":                    {Value: cfg.WorkerCount, Source: s.settingSource("worker_count")},
		"max_concurrent_jobs":             {Value: cfg.MaxConcurrentJobs, Source: s.settingSource("max_concurrent_jobs")},
		"stuck_job_timeout_minutes":       {Value: cfg.StuckJobTimeoutMinutes, Source: s.settingSource("stuck_job_timeout_minutes")},
		"min_level":                       {Value: cfg.MinLevel, Source: s.settingSource("min_level")},
		"retention_days":                  {Value: cfg.Database.RetentionDays, Source: s.settingSource("retention_days")},
		"purge_interval_seconds":          {Value: int(cfg.EffectivePurgeInterval() / time.Second), Source: s.settingSource("purge_interval_seconds")},
		"public_url":                      {Value: cfg.PublicURL, Source: s.settingSource("public_url")},
		"rate_limit_enabled":              {Value: cfg.PublicAPI.RateLimitEnabled, Source: s.settingSource("rate_limit_enabled")},
		"rate_limit_max":                  {Value: cfg.PublicAPI.RateLimitMax, Source: s.settingSource("rate_limit_max")},
		"rate_limit_get_max":              {Value: cfg.PublicAPI.RateLimitGetMax, Source: s.settingSource("rate_limit_get_max")},
		"rate_limit_window":               {Value: cfg.PublicAPI.RateLimitWindow.Duration.String(), Source: s.settingSource("rate_limit_window")},
		"allow_private_undelegated_ip":    {Value: cfg.PublicAPI.AllowPrivateUndelegatedIP, Source: s.settingSource("allow_private_undelegated_ip")},
		"allow_non_global_targets":        {Value: cfg.PublicAPI.AllowNonGlobalTargets, Source: s.settingSource("allow_non_global_targets")},
		"show_score_admin":                {Value: cfg.ShowScoreAdmin, Source: s.settingSource("show_score_admin")},
		"show_score_public":               {Value: cfg.ShowScorePublic, Source: s.settingSource("show_score_public")},
		"show_nameserver_timings_admin":   {Value: cfg.ShowNameserverTimingsAdmin, Source: s.settingSource("show_nameserver_timings_admin")},
		"show_nameserver_timings_public":  {Value: cfg.ShowNameserverTimingsPublic, Source: s.settingSource("show_nameserver_timings_public")},
		"show_dnssec_chain_public":        {Value: cfg.ShowDNSSECChainPublic, Source: s.settingSource("show_dnssec_chain_public")},
		"show_asn_names_public":           {Value: cfg.ShowASNNamesPublic, Source: s.settingSource("show_asn_names_public")},
		"mcp_enabled":                     {Value: cfg.MCPEnabled, Source: s.settingSource("mcp_enabled")},
		"mcp_allow_write":                 {Value: cfg.MCPAllowWrite, Source: s.settingSource("mcp_allow_write")},
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
	if err := readJSON(r, s.cfg.MaxBodySize, &updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body", nil)
		return
	}

	for key := range updates {
		if readonlySettings[key] {
			writeError(w, http.StatusBadRequest, "readonly_setting",
				"setting is read-only: "+key, nil)
			return
		}
		if _, ok := writableSettings[key]; !ok {
			writeError(w, http.StatusBadRequest, "unknown_setting", "unknown setting: "+key, nil)
			return
		}
	}

	values := make(map[string]string, len(updates))
	for key, raw := range updates {
		// Store as the raw JSON string value.
		val := string(raw)
		// Strip quotes from JSON strings for storage.
		var s2 string
		if json.Unmarshal(raw, &s2) == nil {
			val = strings.TrimSpace(s2)
		}
		if err := checkSetting(key, val); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_setting", err.Error(), nil)
			return
		}
		values[key] = val
	}

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	for key, val := range values {
		if err := s.store.SetSetting(key, val); err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
	}

	s.applySettingsToRuntime()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
