package server

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestDefaultConfigListenAddr(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenAddr != "127.0.0.1:8080" {
		t.Fatalf("expected default listen addr 127.0.0.1:8080, got %q", cfg.ListenAddr)
	}
}

func TestDefaultConfigHTTPTimeouts(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ReadTimeout.Duration != 30*time.Second {
		t.Fatalf("ReadTimeout: got %v, want 30s", cfg.ReadTimeout.Duration)
	}
	if cfg.WriteTimeout.Duration != 60*time.Second {
		t.Fatalf("WriteTimeout: got %v, want 60s", cfg.WriteTimeout.Duration)
	}
	if cfg.IdleTimeout.Duration != 60*time.Second {
		t.Fatalf("IdleTimeout: got %v, want 60s", cfg.IdleTimeout.Duration)
	}
	if cfg.WriteTimeout.Duration <= cfg.PublicAPI.AnalysisRequestTimeout.Duration {
		t.Fatalf("WriteTimeout (%v) must exceed AnalysisRequestTimeout (%v)",
			cfg.WriteTimeout.Duration, cfg.PublicAPI.AnalysisRequestTimeout.Duration)
	}
}

func TestApplyFileConfigHTTPTimeouts(t *testing.T) {
	cfg := DefaultConfig()
	read, write, idle := "5s", "120s", "2m"
	cfg.ApplyFileConfig(FileConfig{
		ReadTimeout:  &read,
		WriteTimeout: &write,
		IdleTimeout:  &idle,
	})
	if cfg.ReadTimeout.Duration != 5*time.Second {
		t.Fatalf("ReadTimeout: got %v, want 5s", cfg.ReadTimeout.Duration)
	}
	if cfg.WriteTimeout.Duration != 120*time.Second {
		t.Fatalf("WriteTimeout: got %v, want 120s", cfg.WriteTimeout.Duration)
	}
	if cfg.IdleTimeout.Duration != 2*time.Minute {
		t.Fatalf("IdleTimeout: got %v, want 2m", cfg.IdleTimeout.Duration)
	}
}

func TestDefaultConfigDatabaseIsMemory(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Database.Driver != "" {
		t.Fatalf("expected empty database driver (memory), got %q", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "" {
		t.Fatalf("expected empty database DSN, got %q", cfg.Database.DSN)
	}
}

func TestApplyFileConfigSourceAddrs(t *testing.T) {
	cfg := DefaultConfig()
	source4 := "192.0.2.60"
	source6 := "2001:db8::60"

	cfg.ApplyFileConfig(FileConfig{
		SourceAddr4: &source4,
		SourceAddr6: &source6,
	})

	if cfg.SourceAddr4 == nil || *cfg.SourceAddr4 != source4 {
		t.Fatalf("expected source_addr4 %q, got %#v", source4, cfg.SourceAddr4)
	}
	if cfg.SourceAddr6 == nil || *cfg.SourceAddr6 != source6 {
		t.Fatalf("expected source_addr6 %q, got %#v", source6, cfg.SourceAddr6)
	}
}

func TestDefaultConfigShowScoreAdmin(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.ShowScoreAdmin {
		t.Fatal("expected ShowScoreAdmin to default to true")
	}
}

func TestApplyFileConfigShowScoreAdmin(t *testing.T) {
	cfg := DefaultConfig()
	f := false
	cfg.ApplyFileConfig(FileConfig{ShowScoreAdmin: &f})
	if cfg.ShowScoreAdmin {
		t.Fatal("expected ShowScoreAdmin to be false after applying file config")
	}

	// Nil means "not set" - should not change the value.
	cfg2 := DefaultConfig()
	cfg2.ApplyFileConfig(FileConfig{ShowScoreAdmin: nil})
	if !cfg2.ShowScoreAdmin {
		t.Fatal("expected ShowScoreAdmin to stay true when file config has nil")
	}
}

func TestDefaultConfigShowScorePublic(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.ShowScorePublic {
		t.Fatal("expected ShowScorePublic to default to true")
	}
}

func TestDefaultConfigShowNameserverTimingsAdmin(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.ShowNameserverTimingsAdmin {
		t.Fatal("expected ShowNameserverTimingsAdmin to default to true")
	}
}

func TestApplyFileConfigShowNameserverTimingsAdmin(t *testing.T) {
	cfg := DefaultConfig()
	f := false
	cfg.ApplyFileConfig(FileConfig{ShowNameserverTimingsAdmin: &f})
	if cfg.ShowNameserverTimingsAdmin {
		t.Fatal("expected ShowNameserverTimingsAdmin to be false after applying file config")
	}

	cfg2 := DefaultConfig()
	cfg2.ApplyFileConfig(FileConfig{ShowNameserverTimingsAdmin: nil})
	if !cfg2.ShowNameserverTimingsAdmin {
		t.Fatal("expected ShowNameserverTimingsAdmin to stay true when file config has nil")
	}
}

func TestDefaultConfigShowNameserverTimingsPublic(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.ShowNameserverTimingsPublic {
		t.Fatal("expected ShowNameserverTimingsPublic to default to true")
	}
}

func TestApplyFileConfigShowNameserverTimingsPublic(t *testing.T) {
	cfg := DefaultConfig()
	f := false
	cfg.ApplyFileConfig(FileConfig{ShowNameserverTimingsPublic: &f})
	if cfg.ShowNameserverTimingsPublic {
		t.Fatal("expected ShowNameserverTimingsPublic to be false after applying file config")
	}

	cfg2 := DefaultConfig()
	cfg2.ApplyFileConfig(FileConfig{ShowNameserverTimingsPublic: nil})
	if !cfg2.ShowNameserverTimingsPublic {
		t.Fatal("expected ShowNameserverTimingsPublic to stay true when file config has nil")
	}
}

func TestApplyFileConfigShowScorePublic(t *testing.T) {
	cfg := DefaultConfig()
	f := false
	cfg.ApplyFileConfig(FileConfig{ShowScorePublic: &f})
	if cfg.ShowScorePublic {
		t.Fatal("expected ShowScorePublic to be false after applying file config")
	}

	// Nil means "not set" - should not change the value.
	cfg2 := DefaultConfig()
	cfg2.ApplyFileConfig(FileConfig{ShowScorePublic: nil})
	if !cfg2.ShowScorePublic {
		t.Fatal("expected ShowScorePublic to stay true when file config has nil")
	}
}

func TestApplyFileConfigDatabase(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ApplyFileConfig(FileConfig{
		Database: &DatabaseFileConfig{
			Driver: "sqlite",
			DSN:    "/tmp/test.db",
		},
	})
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected driver sqlite, got %q", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "/tmp/test.db" {
		t.Fatalf("expected DSN /tmp/test.db, got %q", cfg.Database.DSN)
	}
}

func TestApplyFileConfigDatabaseNilIsNoop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = "/existing.db"

	cfg.ApplyFileConfig(FileConfig{Database: nil})

	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected driver unchanged, got %q", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "/existing.db" {
		t.Fatalf("expected DSN unchanged, got %q", cfg.Database.DSN)
	}
}

func TestApplyFileConfigDatabaseDriverOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.DSN = "/existing.db"

	cfg.ApplyFileConfig(FileConfig{
		Database: &DatabaseFileConfig{Driver: "sqlite"},
	})

	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected driver sqlite, got %q", cfg.Database.Driver)
	}
	// DSN should be unchanged since file only set Driver.
	if cfg.Database.DSN != "/existing.db" {
		t.Fatalf("expected DSN /existing.db unchanged, got %q", cfg.Database.DSN)
	}
}

func TestApplyFileConfigDatabaseEmptyDriverIsNoop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.Driver = "sqlite"

	// An explicit empty driver in the file should not overwrite the current value.
	cfg.ApplyFileConfig(FileConfig{
		Database: &DatabaseFileConfig{Driver: "", DSN: "/tmp/test.db"},
	})

	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected driver unchanged, got %q", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "/tmp/test.db" {
		t.Fatalf("expected DSN /tmp/test.db, got %q", cfg.Database.DSN)
	}
}

func TestLoadFileConfigDatabase(t *testing.T) {
	raw := `{
		"listen_addr": "0.0.0.0:9090",
		"database": {
			"driver": "sqlite",
			"dsn": "/var/lib/gonemaster/db.sqlite"
		}
	}`
	f, err := os.CreateTemp("", "gm-config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(raw); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	fileCfg, err := LoadFileConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if fileCfg.Database == nil {
		t.Fatal("expected database section in file config, got nil")
	}
	if fileCfg.Database.Driver != "sqlite" {
		t.Fatalf("expected driver sqlite, got %q", fileCfg.Database.Driver)
	}
	if fileCfg.Database.DSN != "/var/lib/gonemaster/db.sqlite" {
		t.Fatalf("unexpected DSN %q", fileCfg.Database.DSN)
	}

	cfg := DefaultConfig()
	cfg.ApplyFileConfig(fileCfg)
	if cfg.ListenAddr != "0.0.0.0:9090" {
		t.Fatalf("expected listen addr 0.0.0.0:9090, got %q", cfg.ListenAddr)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected driver sqlite after apply, got %q", cfg.Database.Driver)
	}
}

func TestDatabaseFileConfigJSON(t *testing.T) {
	// Verify the JSON round-trip for DatabaseFileConfig.
	raw := `{"driver":"sqlite","dsn":"/tmp/x.db"}`
	var d DatabaseFileConfig
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Driver != "sqlite" {
		t.Fatalf("expected sqlite, got %q", d.Driver)
	}
	if d.DSN != "/tmp/x.db" {
		t.Fatalf("expected /tmp/x.db, got %q", d.DSN)
	}
}

func TestDefaultConfigRetentionDaysIsZero(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Database.RetentionDays != 0 {
		t.Fatalf("expected default retention_days 0, got %d", cfg.Database.RetentionDays)
	}
}

func TestApplyFileConfigRetentionDays(t *testing.T) {
	cfg := DefaultConfig()
	days := 90
	cfg.ApplyFileConfig(FileConfig{
		Database: &DatabaseFileConfig{RetentionDays: &days},
	})
	if cfg.Database.RetentionDays != 90 {
		t.Fatalf("expected retention_days 90, got %d", cfg.Database.RetentionDays)
	}
}

func TestApplyFileConfigRetentionDaysNilIsNoop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.RetentionDays = 30
	cfg.ApplyFileConfig(FileConfig{
		Database: &DatabaseFileConfig{Driver: "sqlite"},
	})
	if cfg.Database.RetentionDays != 30 {
		t.Fatalf("expected retention_days 30 unchanged, got %d", cfg.Database.RetentionDays)
	}
}

func TestApplyFileConfigRetentionDaysZeroOverwrites(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.RetentionDays = 30
	zero := 0
	cfg.ApplyFileConfig(FileConfig{
		Database: &DatabaseFileConfig{RetentionDays: &zero},
	})
	if cfg.Database.RetentionDays != 0 {
		t.Fatalf("expected retention_days 0 after explicit zero, got %d", cfg.Database.RetentionDays)
	}
}

func TestDatabaseFileConfigRetentionDaysJSON(t *testing.T) {
	raw := `{"driver":"sqlite","dsn":"/tmp/x.db","retention_days":90}`
	var d DatabaseFileConfig
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.RetentionDays == nil || *d.RetentionDays != 90 {
		t.Fatalf("expected retention_days 90, got %v", d.RetentionDays)
	}
}

func TestDefaultConfigPublicAPIDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected rate limiting disabled by default")
	}
	if cfg.PublicAPI.RateLimitMax != 10 {
		t.Fatalf("expected default rate_limit_max 10, got %d", cfg.PublicAPI.RateLimitMax)
	}
	if cfg.PublicAPI.RateLimitWindow.Duration != 10*time.Minute {
		t.Fatalf("expected default rate_limit_window 10m, got %v", cfg.PublicAPI.RateLimitWindow)
	}
}

func TestApplyFileConfigPublicAPIRateLimitEnabled(t *testing.T) {
	cfg := DefaultConfig()
	enabled := true
	cfg.ApplyFileConfig(FileConfig{
		PublicAPI: &PublicAPIFileConfig{RateLimitEnabled: &enabled},
	})
	if !cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected rate limiting enabled after apply")
	}
}

func TestApplyFileConfigPublicAPIRateLimitMax(t *testing.T) {
	cfg := DefaultConfig()
	max := 50
	cfg.ApplyFileConfig(FileConfig{
		PublicAPI: &PublicAPIFileConfig{RateLimitMax: &max},
	})
	if cfg.PublicAPI.RateLimitMax != 50 {
		t.Fatalf("expected rate_limit_max 50, got %d", cfg.PublicAPI.RateLimitMax)
	}
}

func TestApplyFileConfigPublicAPIRateLimitWindow(t *testing.T) {
	cfg := DefaultConfig()
	window := "1m"
	cfg.ApplyFileConfig(FileConfig{
		PublicAPI: &PublicAPIFileConfig{RateLimitWindow: &window},
	})
	if cfg.PublicAPI.RateLimitWindow.Duration != time.Minute {
		t.Fatalf("expected rate_limit_window 1m, got %v", cfg.PublicAPI.RateLimitWindow)
	}
}

func TestApplyFileConfigPublicAPINilIsNoop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ApplyFileConfig(FileConfig{PublicAPI: nil})
	if cfg.PublicAPI.RateLimitMax != 10 {
		t.Fatalf("expected rate_limit_max unchanged at 10, got %d", cfg.PublicAPI.RateLimitMax)
	}
}

func TestDurationMarshalJSON(t *testing.T) {
	d := Duration{5 * time.Minute}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `"5m0s"` {
		t.Fatalf("expected \"5m0s\", got %s", b)
	}
}

func TestDurationUnmarshalJSON(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte(`"10m"`), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Duration != 10*time.Minute {
		t.Fatalf("expected 10m, got %v", d.Duration)
	}
}

func TestDurationUnmarshalJSONInvalid(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte(`"notaduration"`), &d); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestLoadFileConfigPublicAPI(t *testing.T) {
	raw := `{
		"public_api": {
			"rate_limit_enabled": true,
			"rate_limit_max": 20,
			"rate_limit_window": "2m"
		}
	}`
	f, err := os.CreateTemp("", "gm-config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(raw); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	fileCfg, err := LoadFileConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if fileCfg.PublicAPI == nil {
		t.Fatal("expected public_api section, got nil")
	}
	if fileCfg.PublicAPI.RateLimitEnabled == nil || !*fileCfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected rate_limit_enabled true")
	}
	if fileCfg.PublicAPI.RateLimitMax == nil || *fileCfg.PublicAPI.RateLimitMax != 20 {
		t.Fatalf("expected rate_limit_max 20, got %v", fileCfg.PublicAPI.RateLimitMax)
	}
	if fileCfg.PublicAPI.RateLimitWindow == nil || *fileCfg.PublicAPI.RateLimitWindow != "2m" {
		t.Fatalf("expected rate_limit_window 2m, got %v", fileCfg.PublicAPI.RateLimitWindow)
	}

	cfg := DefaultConfig()
	cfg.ApplyFileConfig(fileCfg)
	if !cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected rate limiting enabled after apply")
	}
	if cfg.PublicAPI.RateLimitMax != 20 {
		t.Fatalf("expected rate_limit_max 20 after apply, got %d", cfg.PublicAPI.RateLimitMax)
	}
	if cfg.PublicAPI.RateLimitWindow.Duration != 2*time.Minute {
		t.Fatalf("expected rate_limit_window 2m after apply, got %v", cfg.PublicAPI.RateLimitWindow)
	}
}

func TestLoadFileConfigRetentionDays(t *testing.T) {
	raw := `{
		"database": {
			"driver": "sqlite",
			"dsn": "/var/lib/gonemaster/gonemaster.db",
			"retention_days": 90
		}
	}`
	f, err := os.CreateTemp("", "gm-config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(raw); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	fileCfg, err := LoadFileConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if fileCfg.Database == nil {
		t.Fatal("expected database section, got nil")
	}
	if fileCfg.Database.RetentionDays == nil || *fileCfg.Database.RetentionDays != 90 {
		t.Fatalf("expected retention_days 90, got %v", fileCfg.Database.RetentionDays)
	}

	cfg := DefaultConfig()
	cfg.ApplyFileConfig(fileCfg)
	if cfg.Database.RetentionDays != 90 {
		t.Fatalf("expected Database.RetentionDays 90 after apply, got %d", cfg.Database.RetentionDays)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected driver sqlite, got %q", cfg.Database.Driver)
	}
}
