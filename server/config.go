package server

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const defaultCrossJobHotCacheTTLSeconds = 60

// DatabaseConfig controls the persistence backend.
type DatabaseConfig struct {
	// Driver selects the storage backend: "memory" (default), "sqlite",
	// "postgres", or "mariadb".
	Driver string `json:"driver,omitempty"`
	// DSN is the data source name. For sqlite this is a file path.
	// For postgres/mariadb this is a connection string. Empty for memory.
	DSN string `json:"dsn,omitempty"`
	// RetentionDays is the number of days to keep completed jobs. Zero means
	// keep forever (disabled).
	RetentionDays int `json:"retention_days,omitempty"`
}

// PublicAPIConfig controls the behaviour of the public-facing API at /pub/api/v1/.
type PublicAPIConfig struct {
	// RateLimitEnabled enables per-IP rate limiting on POST /pub/api/v1/jobs.
	RateLimitEnabled bool `json:"rate_limit_enabled"`
	// RateLimitMax is the maximum number of job submissions per window per IP.
	// Default: 10.
	RateLimitMax int `json:"rate_limit_max,omitempty"`
	// RateLimitWindow is the sliding window duration for rate limiting.
	// Default: 5m.
	RateLimitWindow Duration `json:"rate_limit_window,omitempty"`
}

// Duration is a time.Duration that marshals/unmarshals as a string (e.g. "5m").
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// Config controls HTTP server behavior.
type Config struct {
	ListenAddr  string `json:"listen_addr"`
	MaxBodySize int64  `json:"max_body_size"`
	Debug       bool   `json:"debug"`
	WorkerCount int    `json:"worker_count"`
	// AutoClampConcurrency enables safe clamping for pathological concurrency values.
	AutoClampConcurrency bool `json:"auto_clamp_concurrency"`
	// JobTestParallelism controls parallel testcase execution inside one job.
	// Values <1 are treated as 1.
	JobTestParallelism int `json:"job_test_parallelism"`
	// MaxConcurrentJobs caps engine runs across workers when >0.
	MaxConcurrentJobs int `json:"max_concurrent_jobs"`
	// CrossJobHotCache enables short-lived warm cache reuse across jobs.
	CrossJobHotCache bool `json:"cross_job_hot_cache"`
	// CrossJobHotCacheTTLSeconds controls the hot-cache entry TTL.
	CrossJobHotCacheTTLSeconds int `json:"cross_job_hot_cache_ttl_seconds"`
	// PositiveCacheTTL overrides resolver.defaults.positive_cache_ttl when set.
	PositiveCacheTTL *int `json:"positive_cache_ttl,omitempty"`
	// NegativeCacheTTL overrides resolver.defaults.negative_cache_ttl when set.
	NegativeCacheTTL *int `json:"negative_cache_ttl,omitempty"`
	// Timeout overrides resolver.defaults.timeout when set (seconds).
	Timeout *int `json:"timeout,omitempty"`
	// Retry overrides resolver.defaults.retry when set.
	Retry *int `json:"retry,omitempty"`
	// Retrans overrides resolver.defaults.retrans when set (seconds).
	Retrans *int `json:"retrans,omitempty"`
	// Fallback overrides resolver.defaults.fallback when set.
	Fallback *bool `json:"fallback,omitempty"`
	// SourceAddr4 overrides resolver.source4 when set.
	SourceAddr4 *string `json:"source_addr4,omitempty"`
	// SourceAddr6 overrides resolver.source6 when set.
	SourceAddr6 *string         `json:"source_addr6,omitempty"`
	MinLevel    string          `json:"min_level"`
	ProfilePath string          `json:"profile_path,omitempty"`
	Database    DatabaseConfig  `json:"database,omitempty"`
	PublicAPI   PublicAPIConfig `json:"public_api,omitempty"`
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

// EffectiveCrossJobHotCacheTTL returns the active TTL for hot-cache entries.
func (c Config) EffectiveCrossJobHotCacheTTL() time.Duration {
	seconds := c.CrossJobHotCacheTTLSeconds
	if seconds < 1 {
		seconds = defaultCrossJobHotCacheTTLSeconds
	}
	return time.Duration(seconds) * time.Second
}

// PublicAPIFileConfig holds optional public API configuration from JSON.
type PublicAPIFileConfig struct {
	RateLimitEnabled *bool   `json:"rate_limit_enabled,omitempty"`
	RateLimitMax     *int    `json:"rate_limit_max,omitempty"`
	RateLimitWindow  *string `json:"rate_limit_window,omitempty"`
}

// DatabaseFileConfig holds optional database configuration from JSON.
// An empty string for Driver or DSN means "not set" (inherit from default).
type DatabaseFileConfig struct {
	Driver        string `json:"driver,omitempty"`
	DSN           string `json:"dsn,omitempty"`
	RetentionDays *int   `json:"retention_days,omitempty"`
}

// FileConfig captures optional configuration fields from JSON.
type FileConfig struct {
	ListenAddr           *string              `json:"listen_addr"`
	MaxBodySize          *int64               `json:"max_body_size"`
	Debug                *bool                `json:"debug"`
	WorkerCount          *int                 `json:"worker_count"`
	AutoClampConcurrency *bool                `json:"auto_clamp_concurrency"`
	JobTestParallelism   *int                 `json:"job_test_parallelism"`
	MaxConcurrentJobs    *int                 `json:"max_concurrent_jobs"`
	CrossJobHotCache     *bool                `json:"cross_job_hot_cache"`
	CrossJobHotCacheTTL  *int                 `json:"cross_job_hot_cache_ttl_seconds"`
	PositiveCacheTTL     *int                 `json:"positive_cache_ttl"`
	NegativeCacheTTL     *int                 `json:"negative_cache_ttl"`
	Timeout              *int                 `json:"timeout"`
	Retry                *int                 `json:"retry"`
	Retrans              *int                 `json:"retrans"`
	Fallback             *bool                `json:"fallback"`
	SourceAddr4          *string              `json:"source_addr4"`
	SourceAddr6          *string              `json:"source_addr6"`
	MinLevel             *string              `json:"min_level"`
	ProfilePath          *string              `json:"profile_path"`
	Database             *DatabaseFileConfig  `json:"database,omitempty"`
	PublicAPI            *PublicAPIFileConfig `json:"public_api,omitempty"`
}

// DefaultConfig returns baseline config values.
func DefaultConfig() Config {
	return Config{
		ListenAddr:                 "127.0.0.1:8080",
		MaxBodySize:                1 << 20,
		Debug:                      false,
		WorkerCount:                4,
		AutoClampConcurrency:       false,
		JobTestParallelism:         1,
		MaxConcurrentJobs:          0,
		CrossJobHotCache:           false,
		CrossJobHotCacheTTLSeconds: defaultCrossJobHotCacheTTLSeconds,
		MinLevel:                   "INFO",
		PublicAPI: PublicAPIConfig{
			RateLimitEnabled: false,
			RateLimitMax:     10,
			RateLimitWindow:  Duration{10 * time.Minute},
		},
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
	if file.AutoClampConcurrency != nil {
		c.AutoClampConcurrency = *file.AutoClampConcurrency
	}
	if file.JobTestParallelism != nil {
		c.JobTestParallelism = *file.JobTestParallelism
	}
	if file.MaxConcurrentJobs != nil {
		c.MaxConcurrentJobs = *file.MaxConcurrentJobs
	}
	if file.CrossJobHotCache != nil {
		c.CrossJobHotCache = *file.CrossJobHotCache
	}
	if file.CrossJobHotCacheTTL != nil {
		c.CrossJobHotCacheTTLSeconds = *file.CrossJobHotCacheTTL
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
	if file.SourceAddr4 != nil {
		c.SourceAddr4 = file.SourceAddr4
	}
	if file.SourceAddr6 != nil {
		c.SourceAddr6 = file.SourceAddr6
	}
	if file.MinLevel != nil {
		c.MinLevel = *file.MinLevel
	}
	if file.ProfilePath != nil {
		c.ProfilePath = *file.ProfilePath
	}
	if file.Database != nil {
		if file.Database.Driver != "" {
			c.Database.Driver = file.Database.Driver
		}
		if file.Database.DSN != "" {
			c.Database.DSN = file.Database.DSN
		}
		if file.Database.RetentionDays != nil {
			c.Database.RetentionDays = *file.Database.RetentionDays
		}
	}
	if file.PublicAPI != nil {
		if file.PublicAPI.RateLimitEnabled != nil {
			c.PublicAPI.RateLimitEnabled = *file.PublicAPI.RateLimitEnabled
		}
		if file.PublicAPI.RateLimitMax != nil {
			c.PublicAPI.RateLimitMax = *file.PublicAPI.RateLimitMax
		}
		if file.PublicAPI.RateLimitWindow != nil {
			d, err := time.ParseDuration(*file.PublicAPI.RateLimitWindow)
			if err == nil {
				c.PublicAPI.RateLimitWindow = Duration{d}
			}
		}
	}
}

// AutoClampConcurrencyForHost clamps pathological concurrency values to host-safe bounds.
// The returned bool is true when one or more fields were adjusted.
func (c Config) AutoClampConcurrencyForHost(cpuCount int) (Config, bool) {
	if cpuCount < 1 {
		cpuCount = 1
	}
	maxWorkers := cpuCount * 2
	if maxWorkers < 4 {
		maxWorkers = 4
	}

	out := c
	changed := false

	if out.WorkerCount < 1 {
		out.WorkerCount = 1
		changed = true
	}
	if out.WorkerCount > maxWorkers {
		out.WorkerCount = maxWorkers
		changed = true
	}

	if out.MaxConcurrentJobs < 0 {
		out.MaxConcurrentJobs = 0
		changed = true
	}
	if out.MaxConcurrentJobs > maxWorkers {
		out.MaxConcurrentJobs = maxWorkers
		changed = true
	}

	return out, changed
}
