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
	if cfg.JobTestParallelism != 1 {
		t.Fatalf("expected default job_test_parallelism 1, got %d", cfg.JobTestParallelism)
	}
	if cfg.CrossJobHotCache {
		t.Fatalf("expected default cross_job_hot_cache false")
	}
	if cfg.CrossJobHotCacheTTLSeconds != defaultCrossJobHotCacheTTLSeconds {
		t.Fatalf("expected default cross_job_hot_cache_ttl_seconds %d, got %d", defaultCrossJobHotCacheTTLSeconds, cfg.CrossJobHotCacheTTLSeconds)
	}
}

func TestApplyFileConfigJobTestParallelism(t *testing.T) {
	cfg := DefaultConfig()
	value := 3
	cfg.ApplyFileConfig(FileConfig{
		JobTestParallelism: &value,
	})
	if cfg.JobTestParallelism != 3 {
		t.Fatalf("expected job_test_parallelism 3, got %d", cfg.JobTestParallelism)
	}
}

func TestApplyFileConfigAutoClampConcurrency(t *testing.T) {
	cfg := DefaultConfig()
	value := true
	cfg.ApplyFileConfig(FileConfig{
		AutoClampConcurrency: &value,
	})
	if !cfg.AutoClampConcurrency {
		t.Fatalf("expected auto_clamp_concurrency true")
	}
}

func TestApplyFileConfigCrossJobHotCache(t *testing.T) {
	cfg := DefaultConfig()
	enabled := true
	ttl := 15
	cfg.ApplyFileConfig(FileConfig{
		CrossJobHotCache:    &enabled,
		CrossJobHotCacheTTL: &ttl,
	})
	if !cfg.CrossJobHotCache {
		t.Fatalf("expected cross_job_hot_cache true")
	}
	if cfg.CrossJobHotCacheTTLSeconds != 15 {
		t.Fatalf("expected cross_job_hot_cache_ttl_seconds 15, got %d", cfg.CrossJobHotCacheTTLSeconds)
	}
}

func TestConfigEffectiveWorkerCount(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "zero", in: 0, want: 1},
		{name: "negative", in: -4, want: 1},
		{name: "positive", in: 7, want: 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{WorkerCount: tc.in}
			if got := cfg.EffectiveWorkerCount(); got != tc.want {
				t.Fatalf("EffectiveWorkerCount() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestConfigEffectiveJobTestParallelism(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "zero", in: 0, want: 1},
		{name: "negative", in: -3, want: 1},
		{name: "positive", in: 5, want: 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{JobTestParallelism: tc.in}
			if got := cfg.EffectiveJobTestParallelism(); got != tc.want {
				t.Fatalf("EffectiveJobTestParallelism() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestConfigEffectiveMaxConcurrentJobs(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "zero-is-unlimited", in: 0, want: 0},
		{name: "negative-is-unlimited", in: -2, want: 0},
		{name: "positive", in: 9, want: 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{MaxConcurrentJobs: tc.in}
			if got := cfg.EffectiveMaxConcurrentJobs(); got != tc.want {
				t.Fatalf("EffectiveMaxConcurrentJobs() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestConfigEffectiveEngineConcurrency(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want int
	}{
		{
			name: "unlimited-bounded-by-workers",
			cfg:  Config{WorkerCount: 8, MaxConcurrentJobs: 0},
			want: 8,
		},
		{
			name: "limiter-lower-than-workers",
			cfg:  Config{WorkerCount: 8, MaxConcurrentJobs: 3},
			want: 3,
		},
		{
			name: "limiter-higher-than-workers",
			cfg:  Config{WorkerCount: 5, MaxConcurrentJobs: 20},
			want: 5,
		},
		{
			name: "clamped-workers-minimum",
			cfg:  Config{WorkerCount: 0, MaxConcurrentJobs: 0},
			want: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EffectiveEngineConcurrency(); got != tc.want {
				t.Fatalf("EffectiveEngineConcurrency() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestConfigEffectiveCrossJobHotCacheTTL(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "default-when-zero", in: 0, want: defaultCrossJobHotCacheTTLSeconds},
		{name: "default-when-negative", in: -4, want: defaultCrossJobHotCacheTTLSeconds},
		{name: "explicit", in: 45, want: 45},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{CrossJobHotCacheTTLSeconds: tc.in}
			if got := int(cfg.EffectiveCrossJobHotCacheTTL().Seconds()); got != tc.want {
				t.Fatalf("EffectiveCrossJobHotCacheTTL() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestConfigAutoClampConcurrencyForHost(t *testing.T) {
	tests := []struct {
		name        string
		cpuCount    int
		cfg         Config
		wantWorkers int
		wantMaxJobs int
		wantChanged bool
	}{
		{
			name:        "clamp-low-and-negative",
			cpuCount:    4,
			cfg:         Config{WorkerCount: 0, MaxConcurrentJobs: -2},
			wantWorkers: 1,
			wantMaxJobs: 0,
			wantChanged: true,
		},
		{
			name:        "clamp-upper-bound",
			cpuCount:    4,
			cfg:         Config{WorkerCount: 64, MaxConcurrentJobs: 32},
			wantWorkers: 8,
			wantMaxJobs: 8,
			wantChanged: true,
		},
		{
			name:        "no-change-within-bounds",
			cpuCount:    8,
			cfg:         Config{WorkerCount: 12, MaxConcurrentJobs: 10},
			wantWorkers: 12,
			wantMaxJobs: 10,
			wantChanged: false,
		},
		{
			name:        "minimum-cpu-fallback",
			cpuCount:    0,
			cfg:         Config{WorkerCount: 20, MaxConcurrentJobs: 20},
			wantWorkers: 4,
			wantMaxJobs: 4,
			wantChanged: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := tc.cfg.AutoClampConcurrencyForHost(tc.cpuCount)
			if changed != tc.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tc.wantChanged)
			}
			if got.WorkerCount != tc.wantWorkers {
				t.Fatalf("WorkerCount = %d, want %d", got.WorkerCount, tc.wantWorkers)
			}
			if got.MaxConcurrentJobs != tc.wantMaxJobs {
				t.Fatalf("MaxConcurrentJobs = %d, want %d", got.MaxConcurrentJobs, tc.wantMaxJobs)
			}
		})
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
