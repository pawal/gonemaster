package server

import (
	"encoding/json"
	"os"
	"testing"
)

func TestDefaultConfigListenAddr(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenAddr != "127.0.0.1:8080" {
		t.Fatalf("expected default listen addr 127.0.0.1:8080, got %q", cfg.ListenAddr)
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
