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
