package server

import "testing"

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
