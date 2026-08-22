package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestGetSettings(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil)

	wantStatus(t, resp, http.StatusOK)

	var settings map[string]settingEntry
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Check a few known settings exist.
	for _, key := range []string{"worker_count", "min_level", "listen_addr", "show_nameserver_timings_admin", "show_nameserver_timings_public"} {
		if _, ok := settings[key]; !ok {
			t.Errorf("missing setting %q", key)
		}
	}

	// listen_addr should be readonly.
	if !settings["listen_addr"].Readonly {
		t.Fatal("expected listen_addr to be readonly")
	}

	// worker_count should not be readonly.
	if settings["worker_count"].Readonly {
		t.Fatal("expected worker_count to be mutable")
	}

	// Default source for settings not explicitly set.
	if settings["worker_count"].Source != SourceDefault {
		t.Fatalf("expected source %q, got %q", SourceDefault, settings["worker_count"].Source)
	}
}

func TestGetSettingsWithConfigSources(t *testing.T) {
	srv := newTestServer(t)
	srv.SetConfigSources(map[string]SettingSource{
		"worker_count": SourceCLIFlag,
		"min_level":    SourceConfigFile,
	})

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil)

	var settings map[string]settingEntry
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if settings["worker_count"].Source != SourceCLIFlag {
		t.Fatalf("worker_count source: got %q, want %q", settings["worker_count"].Source, SourceCLIFlag)
	}
	if settings["min_level"].Source != SourceConfigFile {
		t.Fatalf("min_level source: got %q, want %q", settings["min_level"].Source, SourceConfigFile)
	}
}

func TestGetSettingsDatabaseOverride(t *testing.T) {
	srv := newTestServer(t)
	if err := srv.store.SetSetting("worker_count", "16"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil)

	var settings map[string]settingEntry
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if settings["worker_count"].Source != SourceDatabase {
		t.Fatalf("source: got %q, want %q", settings["worker_count"].Source, SourceDatabase)
	}
}

func TestPutSettings(t *testing.T) {
	srv := newTestServer(t)

	body := `{
		"worker_count": 16,
		"min_level": "WARNING",
		"show_nameserver_timings_admin": false,
		"show_nameserver_timings_public": false
	}`
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/settings", body)

	wantStatus(t, resp, http.StatusOK)

	// Verify settings were stored.
	v, ok := srv.store.GetSetting("worker_count")
	if !ok {
		t.Fatal("worker_count not found in store")
	}
	if v != "16" {
		t.Fatalf("worker_count: got %q, want %q", v, "16")
	}

	v, ok = srv.store.GetSetting("min_level")
	if !ok {
		t.Fatal("min_level not found in store")
	}
	if v != "WARNING" {
		t.Fatalf("min_level: got %q, want %q", v, "WARNING")
	}

	v, ok = srv.store.GetSetting("show_nameserver_timings_admin")
	if !ok {
		t.Fatal("show_nameserver_timings_admin not found in store")
	}
	if v != "false" {
		t.Fatalf("show_nameserver_timings_admin: got %q, want %q", v, "false")
	}

	v, ok = srv.store.GetSetting("show_nameserver_timings_public")
	if !ok {
		t.Fatal("show_nameserver_timings_public not found in store")
	}
	if v != "false" {
		t.Fatalf("show_nameserver_timings_public: got %q, want %q", v, "false")
	}

	if srv.cfg.ShowNameserverTimingsAdmin {
		t.Fatal("expected ShowNameserverTimingsAdmin to be false after PUT")
	}
	if srv.cfg.ShowNameserverTimingsPublic {
		t.Fatal("expected ShowNameserverTimingsPublic to be false after PUT")
	}
}

func TestPutSettingsReadonlyRejected(t *testing.T) {
	srv := newTestServer(t)

	body := `{"listen_addr": "0.0.0.0:9090"}`
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/settings", body)

	wantStatus(t, resp, http.StatusBadRequest)

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "readonly_setting" {
		t.Fatalf("error code: got %q", errResp.Error.Code)
	}
}

func TestPutSettingsInvalidBody(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPut, "/api/v1/settings", "not json")

	wantStatus(t, resp, http.StatusBadRequest)
}

func TestSettingsMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/settings", nil)

	wantStatus(t, resp, http.StatusMethodNotAllowed)
}

func TestApplyDatabaseSettingsOnStartup(t *testing.T) {
	srv := newTestServer(t)

	// Simulate DB settings that were persisted from a previous session.
	_ = srv.store.SetSetting("worker_count", "16")
	_ = srv.store.SetSetting("min_level", "WARNING")
	_ = srv.store.SetSetting("rate_limit_enabled", "true")
	_ = srv.store.SetSetting("rate_limit_max", "20")

	srv.ApplyDatabaseSettings()

	if srv.cfg.WorkerCount != 16 {
		t.Fatalf("WorkerCount: got %d, want 16", srv.cfg.WorkerCount)
	}
	if srv.cfg.MinLevel != "WARNING" {
		t.Fatalf("MinLevel: got %q, want %q", srv.cfg.MinLevel, "WARNING")
	}
	if !srv.cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected RateLimitEnabled=true")
	}
	if srv.cfg.PublicAPI.RateLimitMax != 20 {
		t.Fatalf("RateLimitMax: got %d, want 20", srv.cfg.PublicAPI.RateLimitMax)
	}
}

func TestApplyDatabaseSettingsRespectsCliFlags(t *testing.T) {
	srv := newTestServer(t)
	srv.SetConfigSources(map[string]SettingSource{
		"worker_count": SourceCLIFlag,
	})

	_ = srv.store.SetSetting("worker_count", "32")
	_ = srv.store.SetSetting("min_level", "ERROR")

	srv.ApplyDatabaseSettings()

	// CLI flag should win - worker_count stays at default (16).
	if srv.cfg.WorkerCount != 16 {
		t.Fatalf("WorkerCount: got %d, want 16 (CLI flag should take precedence)", srv.cfg.WorkerCount)
	}
	// min_level has no CLI flag override, so DB value applies.
	if srv.cfg.MinLevel != "ERROR" {
		t.Fatalf("MinLevel: got %q, want %q", srv.cfg.MinLevel, "ERROR")
	}
}

func TestPutSettingsHotReloadsRuntime(t *testing.T) {
	srv := newTestServer(t)

	// Default: rate limiting disabled, worker_count=16.
	if srv.rateLimiter != nil {
		t.Fatal("expected rateLimiter=nil initially")
	}
	if srv.cfg.WorkerCount != 16 {
		t.Fatalf("initial WorkerCount: got %d", srv.cfg.WorkerCount)
	}

	// PUT to change settings.
	body := `{"worker_count": 8, "min_level": "ERROR", "rate_limit_enabled": true, "rate_limit_max": 5}`
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/settings", body)

	wantStatus(t, resp, http.StatusOK)

	// Verify runtime config was updated.
	if srv.cfg.WorkerCount != 8 {
		t.Fatalf("WorkerCount after PUT: got %d, want 8", srv.cfg.WorkerCount)
	}
	if srv.cfg.MinLevel != "ERROR" {
		t.Fatalf("MinLevel after PUT: got %q, want %q", srv.cfg.MinLevel, "ERROR")
	}
	if !srv.cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected RateLimitEnabled=true after PUT")
	}
	if srv.rateLimiter == nil {
		t.Fatal("expected rateLimiter to be created after enabling rate limiting")
	}

	// Disable rate limiting.
	resp2 := doJSON(t, srv, http.MethodPut, "/api/v1/settings", `{"rate_limit_enabled": false}`)
	wantStatus(t, resp2, http.StatusOK)

	if srv.rateLimiter != nil {
		t.Fatal("expected rateLimiter=nil after disabling rate limiting")
	}
}

func TestApplyDatabaseSettingsRetentionDays(t *testing.T) {
	srv := newTestServer(t)
	_ = srv.store.SetSetting("retention_days", "30")
	srv.ApplyDatabaseSettings()

	if srv.cfg.Database.RetentionDays != 30 {
		t.Fatalf("RetentionDays: got %d, want 30", srv.cfg.Database.RetentionDays)
	}
}

func TestApplyDatabaseSettingsPublicURL(t *testing.T) {
	srv := newTestServer(t)
	_ = srv.store.SetSetting("public_url", "https://dns.example.com")
	srv.ApplyDatabaseSettings()

	if srv.cfg.PublicURL != "https://dns.example.com" {
		t.Fatalf("PublicURL: got %q", srv.cfg.PublicURL)
	}
}

func TestApplyDatabaseSettingsRateLimitWindow(t *testing.T) {
	srv := newTestServer(t)
	_ = srv.store.SetSetting("rate_limit_window", "5m")
	srv.ApplyDatabaseSettings()

	if srv.cfg.PublicAPI.RateLimitWindow.Duration.String() != "5m0s" {
		t.Fatalf("RateLimitWindow: got %q", srv.cfg.PublicAPI.RateLimitWindow.Duration.String())
	}
}

func TestApplyDatabaseSettingsIgnoresInvalidValues(t *testing.T) {
	srv := newTestServer(t)
	_ = srv.store.SetSetting("worker_count", "not-a-number")
	_ = srv.store.SetSetting("rate_limit_window", "invalid")
	srv.ApplyDatabaseSettings()

	// Should stay at defaults.
	if srv.cfg.WorkerCount != 16 {
		t.Fatalf("WorkerCount: got %d, want 16", srv.cfg.WorkerCount)
	}
}

func TestPutSettingsResizesWorkerPool(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 2
	srv := New(cfg)
	srv.Start()
	defer srv.Stop(t.Context())

	// Confirm initial worker count.
	srv.workers.mu.Lock()
	initial := len(srv.workers.cancels)
	srv.workers.mu.Unlock()
	if initial != 2 {
		t.Fatalf("expected 2 initial workers, got %d", initial)
	}

	// Scale up to 5 via the settings API.
	body := `{"worker_count": 5}`
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/settings", body)
	wantStatus(t, resp, http.StatusOK)

	srv.workers.mu.Lock()
	after := len(srv.workers.cancels)
	srv.workers.mu.Unlock()
	if after != 5 {
		t.Fatalf("expected 5 workers after scale-up, got %d", after)
	}

	// Scale down to 3.
	body = `{"worker_count": 3}`
	resp = doJSON(t, srv, http.MethodPut, "/api/v1/settings", body)
	wantStatus(t, resp, http.StatusOK)

	srv.workers.mu.Lock()
	after = len(srv.workers.cancels)
	srv.workers.mu.Unlock()
	if after != 3 {
		t.Fatalf("expected 3 workers after scale-down, got %d", after)
	}
}

func TestApplyDatabaseSettingsHotCacheTTL(t *testing.T) {
	srv := newTestServer(t)
	_ = srv.store.SetSetting("cross_job_hot_cache_ttl_seconds", "120")
	srv.ApplyDatabaseSettings()

	if srv.cfg.CrossJobHotCacheTTLSeconds != 120 {
		t.Fatalf("CrossJobHotCacheTTLSeconds: got %d, want 120", srv.cfg.CrossJobHotCacheTTLSeconds)
	}
}

func TestApplyDatabaseSettingsHotCacheTTLIgnoresInvalid(t *testing.T) {
	srv := newTestServer(t)
	_ = srv.store.SetSetting("cross_job_hot_cache_ttl_seconds", "not-a-number")
	srv.ApplyDatabaseSettings()

	if srv.cfg.CrossJobHotCacheTTLSeconds != defaultCrossJobHotCacheTTLSeconds {
		t.Fatalf("CrossJobHotCacheTTLSeconds: got %d, want %d", srv.cfg.CrossJobHotCacheTTLSeconds, defaultCrossJobHotCacheTTLSeconds)
	}
}

func TestPutSettingsUpdatesHotCacheTTL(t *testing.T) {
	srv := newTestServer(t)

	body := `{"cross_job_hot_cache_ttl_seconds": 300}`
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/settings", body)

	wantStatus(t, resp, http.StatusOK)

	if srv.cfg.CrossJobHotCacheTTLSeconds != 300 {
		t.Fatalf("CrossJobHotCacheTTLSeconds after PUT: got %d, want 300", srv.cfg.CrossJobHotCacheTTLSeconds)
	}
}

func TestGetSettingsIncludesHotCacheTTL(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil)

	var settings map[string]settingEntry
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode: %v", err)
	}

	entry, ok := settings["cross_job_hot_cache_ttl_seconds"]
	if !ok {
		t.Fatal("missing setting cross_job_hot_cache_ttl_seconds")
	}
	// Default value should be 60.
	val, ok2 := entry.Value.(float64)
	if !ok2 {
		t.Fatalf("expected float64 value, got %T", entry.Value)
	}
	if int(val) != defaultCrossJobHotCacheTTLSeconds {
		t.Fatalf("value: got %v, want %d", val, defaultCrossJobHotCacheTTLSeconds)
	}
	if entry.Readonly {
		t.Fatal("expected cross_job_hot_cache_ttl_seconds to be mutable")
	}
}

func TestPutSettingsUpdatesPurgeInterval(t *testing.T) {
	srv := newTestServer(t)

	body := `{"purge_interval_seconds": 1800}`
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/settings", body)

	wantStatus(t, resp, http.StatusOK)
	if srv.cfg.Database.PurgeIntervalSeconds != 1800 {
		t.Fatalf("PurgeIntervalSeconds after PUT: got %d, want 1800", srv.cfg.Database.PurgeIntervalSeconds)
	}
	// The purge loop reads the interval from the atomic, so it must track the
	// new value for the change to take effect without a restart.
	if got := srv.purgeIntervalSec.Load(); got != 1800 {
		t.Fatalf("purgeIntervalSec atomic after PUT: got %d, want 1800", got)
	}
}

func TestGetSettingsIncludesPurgeInterval(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil)

	var settings map[string]settingEntry
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode: %v", err)
	}

	entry, ok := settings["purge_interval_seconds"]
	if !ok {
		t.Fatal("missing setting purge_interval_seconds")
	}
	val, ok2 := entry.Value.(float64)
	if !ok2 {
		t.Fatalf("expected float64 value, got %T", entry.Value)
	}
	// The endpoint reports the effective interval, so an unset config field
	// surfaces as the 3600s default rather than 0.
	if int(val) != defaultPurgeIntervalSeconds {
		t.Fatalf("value: got %v, want %d", val, defaultPurgeIntervalSeconds)
	}
	if entry.Readonly {
		t.Fatal("expected purge_interval_seconds to be mutable")
	}
}

func TestPublicInfoEndpointDefault(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/info", nil)

	wantStatus(t, resp, http.StatusOK)

	var info publicInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !info.ShowScorePublic {
		t.Fatal("expected show_score_public=true by default")
	}
	if !info.ShowNameserverTimingsPublic {
		t.Fatal("expected show_nameserver_timings_public=true by default")
	}
}

func TestPublicInfoEndpointReflectsConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowScorePublic = false
	cfg.ShowNameserverTimingsPublic = false
	srv := New(cfg)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/info", nil)

	wantStatus(t, resp, http.StatusOK)

	var info publicInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.ShowScorePublic {
		t.Fatal("expected show_score_public=false when disabled in config")
	}
	if info.ShowNameserverTimingsPublic {
		t.Fatal("expected show_nameserver_timings_public=false when disabled in config")
	}
}

func TestFeaturesEndpointDefault(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/features", nil)

	wantStatus(t, resp, http.StatusOK)

	var feat featuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&feat); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !feat.ShowScoreAdmin {
		t.Fatal("expected show_score_admin=true by default")
	}
	if !feat.ShowNameserverTimingsAdmin {
		t.Fatal("expected show_nameserver_timings_admin=true by default")
	}
}

func TestFeaturesEndpointReflectsConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowScoreAdmin = false
	cfg.ShowNameserverTimingsAdmin = false
	srv := New(cfg)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/features", nil)

	wantStatus(t, resp, http.StatusOK)

	var feat featuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&feat); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if feat.ShowScoreAdmin {
		t.Fatal("expected show_score_admin=false when disabled in config")
	}
	if feat.ShowNameserverTimingsAdmin {
		t.Fatal("expected show_nameserver_timings_admin=false when disabled in config")
	}
}

func TestPutSettingsAllowNonGlobalTargets(t *testing.T) {
	// The query-guard server policy is a settable admin setting that hot-reloads
	// into the running config and is reported by GET /settings.
	srv := newTestServer(t)
	if srv.cfg.PublicAPI.AllowNonGlobalTargets {
		t.Fatal("expected AllowNonGlobalTargets default false")
	}

	body := `{"allow_non_global_targets": true}`
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/settings", body)
	wantStatus(t, resp, http.StatusOK)
	if !srv.cfg.PublicAPI.AllowNonGlobalTargets {
		t.Fatal("expected AllowNonGlobalTargets true after PUT")
	}

	getResp := doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil)
	var settings map[string]settingEntry
	if err := json.NewDecoder(getResp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := settings["allow_non_global_targets"]; !ok {
		t.Fatal("missing setting allow_non_global_targets")
	}
}
