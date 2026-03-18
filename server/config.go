package server

import (
	"encoding/json"
	"fmt"
	"os"
)

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

// Config controls HTTP server behavior.
type Config struct {
	ListenAddr  string `json:"listen_addr"`
	MaxBodySize int64  `json:"max_body_size"`
	Debug       bool   `json:"debug"`
	WorkerCount int    `json:"worker_count"`
	// MaxConcurrentJobs caps engine runs across workers when >0.
	MaxConcurrentJobs int `json:"max_concurrent_jobs"`
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
	SourceAddr6 *string `json:"source_addr6,omitempty"`
	MinLevel    string  `json:"min_level"`
	ProfilePath string  `json:"profile_path,omitempty"`
	Database    DatabaseConfig `json:"database,omitempty"`
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
	ListenAddr        *string             `json:"listen_addr"`
	MaxBodySize       *int64              `json:"max_body_size"`
	Debug             *bool               `json:"debug"`
	WorkerCount       *int                `json:"worker_count"`
	MaxConcurrentJobs *int                `json:"max_concurrent_jobs"`
	PositiveCacheTTL  *int                `json:"positive_cache_ttl"`
	NegativeCacheTTL  *int                `json:"negative_cache_ttl"`
	Timeout           *int                `json:"timeout"`
	Retry             *int                `json:"retry"`
	Retrans           *int                `json:"retrans"`
	Fallback          *bool               `json:"fallback"`
	SourceAddr4       *string             `json:"source_addr4"`
	SourceAddr6       *string             `json:"source_addr6"`
	MinLevel          *string             `json:"min_level"`
	ProfilePath       *string             `json:"profile_path"`
	Database          *DatabaseFileConfig `json:"database,omitempty"`
}

// DefaultConfig returns baseline config values.
func DefaultConfig() Config {
	return Config{
		ListenAddr:        "127.0.0.1:8080",
		MaxBodySize:       1 << 20,
		Debug:             false,
		WorkerCount:       4,
		MaxConcurrentJobs: 0,
		MinLevel:          "INFO",
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
}
