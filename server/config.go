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
}

// FileConfig captures optional configuration fields from JSON.
type FileConfig struct {
	ListenAddr  *string `json:"listen_addr"`
	MaxBodySize *int64  `json:"max_body_size"`
	Debug       *bool   `json:"debug"`
}

// DefaultConfig returns baseline config values.
func DefaultConfig() Config {
	return Config{
		ListenAddr:  ":8080",
		MaxBodySize: 1 << 20,
		Debug:       false,
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
}
