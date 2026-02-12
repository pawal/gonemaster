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
