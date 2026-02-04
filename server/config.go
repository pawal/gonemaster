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
	// MaxConcurrentJobs caps engine runs across workers when >0.
	MaxConcurrentJobs int
	// PositiveCacheTTL overrides resolver.defaults.positive_cache_ttl when set.
	PositiveCacheTTL *int
	// NegativeCacheTTL overrides resolver.defaults.negative_cache_ttl when set.
	NegativeCacheTTL *int
	MinLevel         string
	ProfilePath      string
}

// FileConfig captures optional configuration fields from JSON.
type FileConfig struct {
	ListenAddr        *string `json:"listen_addr"`
	MaxBodySize       *int64  `json:"max_body_size"`
	Debug             *bool   `json:"debug"`
	WorkerCount       *int    `json:"worker_count"`
	MaxConcurrentJobs *int    `json:"max_concurrent_jobs"`
	PositiveCacheTTL  *int    `json:"positive_cache_ttl"`
	NegativeCacheTTL  *int    `json:"negative_cache_ttl"`
	MinLevel          *string `json:"min_level"`
	ProfilePath       *string `json:"profile_path"`
}

// DefaultConfig returns baseline config values.
func DefaultConfig() Config {
	return Config{
		ListenAddr:        ":8080",
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

// ApplyFileConfig overwrites config fields when provided.
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
	if file.MinLevel != nil {
		c.MinLevel = *file.MinLevel
	}
	if file.ProfilePath != nil {
		c.ProfilePath = *file.ProfilePath
	}
}
