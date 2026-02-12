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
