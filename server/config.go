package server

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config controls HTTP server behavior.
type Config struct {
	ListenAddr  string
	MaxBodySize int64
	Debug       bool
	WorkerCount int
	// JobTestParallelism controls parallel testcase execution inside one job.
	// Values <1 are treated as 1.
	JobTestParallelism int
	// MaxConcurrentJobs caps engine runs across workers when >0.
	MaxConcurrentJobs int
	// PositiveCacheTTL overrides resolver.defaults.positive_cache_ttl when set.
	PositiveCacheTTL *int
	// NegativeCacheTTL overrides resolver.defaults.negative_cache_ttl when set.
	NegativeCacheTTL *int
	// Timeout overrides resolver.defaults.timeout when set (seconds).
	Timeout *int
	// Retry overrides resolver.defaults.retry when set.
	Retry *int
	// Retrans overrides resolver.defaults.retrans when set (seconds).
	Retrans *int
	// Fallback overrides resolver.defaults.fallback when set.
	Fallback    *bool
	MinLevel    string
	ProfilePath string
}

// EffectiveWorkerCount returns the worker count the server will actually use.
func (c Config) EffectiveWorkerCount() int {
	if c.WorkerCount < 1 {
		return 1
	}
	return c.WorkerCount
}

// EffectiveJobTestParallelism returns testcase parallelism used inside one job.
func (c Config) EffectiveJobTestParallelism() int {
	if c.JobTestParallelism < 1 {
		return 1
	}
	return c.JobTestParallelism
}

// EffectiveMaxConcurrentJobs returns the active engine limiter size.
// A return value of 0 means "unlimited".
func (c Config) EffectiveMaxConcurrentJobs() int {
	if c.MaxConcurrentJobs < 1 {
		return 0
	}
	return c.MaxConcurrentJobs
}

// EffectiveEngineConcurrency returns the maximum number of in-flight engine runs.
func (c Config) EffectiveEngineConcurrency() int {
	workers := c.EffectiveWorkerCount()
	maxConcurrentJobs := c.EffectiveMaxConcurrentJobs()
	if maxConcurrentJobs == 0 || maxConcurrentJobs > workers {
		return workers
	}
	return maxConcurrentJobs
}

// FileConfig captures optional configuration fields from JSON.
type FileConfig struct {
	ListenAddr         *string `json:"listen_addr"`
	MaxBodySize        *int64  `json:"max_body_size"`
	Debug              *bool   `json:"debug"`
	WorkerCount        *int    `json:"worker_count"`
	JobTestParallelism *int    `json:"job_test_parallelism"`
	MaxConcurrentJobs  *int    `json:"max_concurrent_jobs"`
	PositiveCacheTTL   *int    `json:"positive_cache_ttl"`
	NegativeCacheTTL   *int    `json:"negative_cache_ttl"`
	Timeout            *int    `json:"timeout"`
	Retry              *int    `json:"retry"`
	Retrans            *int    `json:"retrans"`
	Fallback           *bool   `json:"fallback"`
	MinLevel           *string `json:"min_level"`
	ProfilePath        *string `json:"profile_path"`
}

// DefaultConfig returns baseline config values.
func DefaultConfig() Config {
	return Config{
		ListenAddr:         "127.0.0.1:8080",
		MaxBodySize:        1 << 20,
		Debug:              false,
		WorkerCount:        4,
		JobTestParallelism: 1,
		MaxConcurrentJobs:  0,
		MinLevel:           "INFO",
	}
}

// LoadFileConfig loads configuration overrides from a JSON file.
func LoadFileConfig(path string) (FileConfig, error) {
	var cfg FileConfig
	payload, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// ApplyFileConfig overwrites config fields when provided by file.
func (c *Config) ApplyFileConfig(file FileConfig) {
	if file.ListenAddr != nil {
		c.ListenAddr = *file.ListenAddr
	}
	if file.MaxBodySize != nil {
		c.MaxBodySize = *file.MaxBodySize
	}
	if file.Debug != nil {
		c.Debug = *file.Debug
	}
	if file.WorkerCount != nil {
		c.WorkerCount = *file.WorkerCount
	}
	if file.JobTestParallelism != nil {
		c.JobTestParallelism = *file.JobTestParallelism
	}
	if file.MaxConcurrentJobs != nil {
		c.MaxConcurrentJobs = *file.MaxConcurrentJobs
	}
	if file.PositiveCacheTTL != nil {
		c.PositiveCacheTTL = file.PositiveCacheTTL
	}
	if file.NegativeCacheTTL != nil {
		c.NegativeCacheTTL = file.NegativeCacheTTL
	}
	if file.Timeout != nil {
		c.Timeout = file.Timeout
	}
	if file.Retry != nil {
		c.Retry = file.Retry
	}
	if file.Retrans != nil {
		c.Retrans = file.Retrans
	}
	if file.Fallback != nil {
		c.Fallback = file.Fallback
	}
	if file.MinLevel != nil {
		c.MinLevel = *file.MinLevel
	}
	if file.ProfilePath != nil {
		c.ProfilePath = *file.ProfilePath
	}
}
