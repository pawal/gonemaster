package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSettings(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}

	var settings map[string]settingEntry
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Check a few known settings exist.
	for _, key := range []string{"worker_count", "min_level", "listen_addr"} {
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
	srv := New(DefaultConfig())
	srv.SetConfigSources(map[string]SettingSource{
		"worker_count": SourceCLIFlag,
		"min_level":    SourceConfigFile,
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	srv.Handler().ServeHTTP(resp, req)

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
	srv := New(DefaultConfig())
	if err := srv.store.SetSetting("worker_count", "16"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	srv.Handler().ServeHTTP(resp, req)

	var settings map[string]settingEntry
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if settings["worker_count"].Source != SourceDatabase {
		t.Fatalf("source: got %q, want %q", settings["worker_count"].Source, SourceDatabase)
	}
}

func TestPutSettings(t *testing.T) {
	srv := New(DefaultConfig())

	body := `{"worker_count": 16, "min_level": "WARNING"}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}

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
}

func TestPutSettingsReadonlyRejected(t *testing.T) {
	srv := New(DefaultConfig())

	body := `{"listen_addr": "0.0.0.0:9090"}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "readonly_setting" {
		t.Fatalf("error code: got %q", errResp.Error.Code)
	}
}

func TestPutSettingsInvalidBody(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestSettingsMethodNotAllowed(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/settings", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}
}

func TestApplyDatabaseSettingsOnStartup(t *testing.T) {
	srv := New(DefaultConfig())

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
	srv := New(DefaultConfig())
	srv.SetConfigSources(map[string]SettingSource{
		"worker_count": SourceCLIFlag,
	})

	_ = srv.store.SetSetting("worker_count", "32")
	_ = srv.store.SetSetting("min_level", "ERROR")

	srv.ApplyDatabaseSettings()

	// CLI flag should win — worker_count stays at default (4).
	if srv.cfg.WorkerCount != 4 {
		t.Fatalf("WorkerCount: got %d, want 4 (CLI flag should take precedence)", srv.cfg.WorkerCount)
	}
	// min_level has no CLI flag override, so DB value applies.
	if srv.cfg.MinLevel != "ERROR" {
		t.Fatalf("MinLevel: got %q, want %q", srv.cfg.MinLevel, "ERROR")
	}
}

func TestPutSettingsHotReloadsRuntime(t *testing.T) {
	srv := New(DefaultConfig())

	// Default: rate limiting disabled, worker_count=4.
	if srv.rateLimiter != nil {
		t.Fatal("expected rateLimiter=nil initially")
	}
	if srv.cfg.WorkerCount != 4 {
		t.Fatalf("initial WorkerCount: got %d", srv.cfg.WorkerCount)
	}

	// PUT to change settings.
	body := `{"worker_count": 8, "min_level": "ERROR", "rate_limit_enabled": true, "rate_limit_max": 5}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}

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
	body2 := `{"rate_limit_enabled": false}`
	resp2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(body2))
	req2.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp2, req2)

	if srv.rateLimiter != nil {
		t.Fatal("expected rateLimiter=nil after disabling rate limiting")
	}
}

func TestApplyDatabaseSettingsRetentionDays(t *testing.T) {
	srv := New(DefaultConfig())
	_ = srv.store.SetSetting("retention_days", "30")
	srv.ApplyDatabaseSettings()

	if srv.cfg.Database.RetentionDays != 30 {
		t.Fatalf("RetentionDays: got %d, want 30", srv.cfg.Database.RetentionDays)
	}
}

func TestApplyDatabaseSettingsPublicURL(t *testing.T) {
	srv := New(DefaultConfig())
	_ = srv.store.SetSetting("public_url", "https://dns.example.com")
	srv.ApplyDatabaseSettings()

	if srv.cfg.PublicURL != "https://dns.example.com" {
		t.Fatalf("PublicURL: got %q", srv.cfg.PublicURL)
	}
}

func TestApplyDatabaseSettingsRateLimitWindow(t *testing.T) {
	srv := New(DefaultConfig())
	_ = srv.store.SetSetting("rate_limit_window", "5m")
	srv.ApplyDatabaseSettings()

	if srv.cfg.PublicAPI.RateLimitWindow.Duration.String() != "5m0s" {
		t.Fatalf("RateLimitWindow: got %q", srv.cfg.PublicAPI.RateLimitWindow.Duration.String())
	}
}

func TestApplyDatabaseSettingsIgnoresInvalidValues(t *testing.T) {
	srv := New(DefaultConfig())
	_ = srv.store.SetSetting("worker_count", "not-a-number")
	_ = srv.store.SetSetting("rate_limit_window", "invalid")
	srv.ApplyDatabaseSettings()

	// Should stay at defaults.
	if srv.cfg.WorkerCount != 4 {
		t.Fatalf("WorkerCount: got %d, want 4", srv.cfg.WorkerCount)
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
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("PUT settings: expected 200, got %d: %s", resp.Code, resp.Body)
	}

	srv.workers.mu.Lock()
	after := len(srv.workers.cancels)
	srv.workers.mu.Unlock()
	if after != 5 {
		t.Fatalf("expected 5 workers after scale-up, got %d", after)
	}

	// Scale down to 3.
	body = `{"worker_count": 3}`
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("PUT settings: expected 200, got %d: %s", resp.Code, resp.Body)
	}

	srv.workers.mu.Lock()
	after = len(srv.workers.cancels)
	srv.workers.mu.Unlock()
	if after != 3 {
		t.Fatalf("expected 3 workers after scale-down, got %d", after)
	}
}

func TestFeaturesEndpointDefault(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/features", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var feat featuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&feat); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !feat.ShowScoreAdmin {
		t.Fatal("expected show_score_admin=true by default")
	}
}

func TestFeaturesEndpointReflectsConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowScoreAdmin = false
	srv := New(cfg)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/features", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var feat featuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&feat); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if feat.ShowScoreAdmin {
		t.Fatal("expected show_score_admin=false when disabled in config")
	}
}
