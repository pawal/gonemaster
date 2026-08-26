package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	serverpublic "codeberg.org/pawal/gonemaster/server/public"
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
	// PurgeIntervalSeconds is how often the retention purge loop runs. Zero
	// uses the default (3600s / 1h).
	PurgeIntervalSeconds int `json:"purge_interval_seconds,omitempty"`
	// MaxOpenConns caps the connection pool size. Zero uses the
	// driver-appropriate default (1 for sqlite, 25 otherwise).
	MaxOpenConns int `json:"max_open_conns,omitempty"`
	// MaxIdleConns caps idle connections held in the pool. Zero uses
	// the default (5 for client/server drivers).
	MaxIdleConns int `json:"max_idle_conns,omitempty"`
	// ConnMaxLifetimeSeconds rotates pooled connections after this many
	// seconds. Zero uses the default (300s).
	ConnMaxLifetimeSeconds int `json:"conn_max_lifetime_seconds,omitempty"`
}

// AnalysisConfig controls capture-time policy for the analysis layer.
type AnalysisConfig struct {
	// TagViewMinLevel is the floor for tags written to
	// analysis_snapshot_tag_view at capture time. Tags whose worst-level
	// in the snapshot is below this threshold get no row, no detail
	// page, and do not appear in the listing. Default "NOTICE".
	// Valid: INFO, NOTICE, WARNING, ERROR, CRITICAL.
	TagViewMinLevel string `json:"tag_view_min_level,omitempty"`
}

// AuthConfig configures admin-API auth. Empty AdminTokens means open mode.
type AuthConfig struct {
	AdminTokens []AdminToken `json:"admin_tokens,omitempty"`
}

// AdminToken is one admin credential, stored hashed. Scopes is reserved.
type AdminToken struct {
	Label  string   `json:"label,omitempty"`
	Hash   string   `json:"hash"`
	Scopes []string `json:"scopes,omitempty"`
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
	RateLimitWindow Duration `json:"rate_limit_window"`
	// AnalysisRequestTimeout caps the wall time of a public analysis
	// request. Zero disables. Default: 10s.
	AnalysisRequestTimeout Duration `json:"analysis_request_timeout"`
	// AllowPrivateUndelegatedIP allows undelegated NS IPs in loopback /
	// link-local / private / CGNAT / multicast / broadcast ranges. Default
	// false; set true on private/internal deployments that need it.
	AllowPrivateUndelegatedIP bool `json:"allow_private_undelegated_ip,omitempty"`
	// AllowNonGlobalTargets permits querying non-globally-reachable addresses.
	// Default false: the engine guard is clamped on for every job. Set true on
	// private/internal deployments that test such zones.
	AllowNonGlobalTargets bool `json:"allow_non_global_targets,omitempty"`
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

const defaultCrossJobHotCacheTTLSeconds = 60

const defaultPurgeIntervalSeconds = 3600

// defaultStuckJobTimeoutMinutes is the age at which a "running" job with no
// worker on it is considered abandoned.
const defaultStuckJobTimeoutMinutes = 20

// stuckJobSweepInterval is how often the reaper looks for abandoned jobs.
const stuckJobSweepInterval = time.Minute

// Config controls HTTP server behavior.
type Config struct {
	ListenAddr  string `json:"listen_addr"`
	MaxBodySize int64  `json:"max_body_size"`
	Debug       bool   `json:"debug"`
	WorkerCount int    `json:"worker_count"`
	// MaxConcurrentJobs caps engine runs across workers when >0.
	MaxConcurrentJobs int `json:"max_concurrent_jobs"`
	// StuckJobTimeoutMinutes is how long a job may sit at "running" with no
	// worker on it before the reaper fails it. Zero disables the reaper.
	StuckJobTimeoutMinutes int `json:"stuck_job_timeout_minutes"`
	// CrossJobHotCache enables sharing of warmed nameserver query/error caches
	// across consecutive jobs. Caches are keyed by profile and network settings
	// so jobs with different resolver configs get independent stores.
	CrossJobHotCache bool `json:"cross_job_hot_cache"`
	// CrossJobHotCacheTTLSeconds sets how long a hot-cache entry is kept after
	// its last use. Zero uses the built-in default (60 s).
	CrossJobHotCacheTTLSeconds int `json:"cross_job_hot_cache_ttl_seconds,omitempty"`
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
	// LogFormat selects the operational log encoding: "text" (default) or "json".
	LogFormat string `json:"log_format,omitempty"`
	// LogLevel sets the minimum operational log level: debug|info|warn|error.
	// Empty means "info".
	LogLevel string `json:"log_level,omitempty"`
	// TrustedProxyCIDRs lists CIDR blocks (or bare IPs) whose requests are
	// allowed to set X-Forwarded-For. Empty means trust nothing and use
	// RemoteAddr. Without this, XFF is spoofable and rate limits can be
	// bypassed.
	TrustedProxyCIDRs []string `json:"trusted_proxy_cidrs,omitempty"`
	// Connection-level timeouts on the http.Server. Defaults: 30s/60s/60s.
	// WriteTimeout must exceed public_api.analysis_request_timeout.
	ReadTimeout  Duration `json:"read_timeout"`
	WriteTimeout Duration `json:"write_timeout"`
	IdleTimeout  Duration `json:"idle_timeout"`
	// PublicURL is the site root (e.g. "https://example.com/"), not the public
	// UI's own URL: that is PublicURL + PublicUIPath. Used for og:url, hreflang,
	// robots.txt, and sitemap.xml. When empty, the URL is auto-detected from
	// the request's Host and X-Forwarded-Proto headers.
	PublicURL string `json:"public_url,omitempty"`
	// PublicUIPath is where visitors reach the UI under PublicURL. Empty means
	// the root. Changes advertised URLs, not where the server mounts.
	PublicUIPath string          `json:"public_ui_path"`
	Database     DatabaseConfig  `json:"database"`
	PublicAPI    PublicAPIConfig `json:"public_api"`
	Analysis     AnalysisConfig  `json:"analysis"`
	Auth         AuthConfig      `json:"auth"`
	// ScoringConfigPath is an optional path to a JSON file that overrides the
	// default scoring configuration (weights, penalties, tag overrides, etc.).
	// When empty, scoring.DefaultConfig() is used.
	ScoringConfigPath string `json:"scoring_config_path,omitempty"`
	// ShowScoreAdmin controls whether scoring-related UI elements are displayed
	// in the admin interface. Defaults to true. Set to false to hide grades and
	// scores from admin users, e.g. when scoring is not meaningful for the
	// deployment.
	ShowScoreAdmin bool `json:"show_score_admin"`
	// ShowScorePublic controls whether scoring-related UI elements are displayed
	// in the public results interface. Defaults to true. Set to false to hide
	// grades and scores from visitors.
	ShowScorePublic bool `json:"show_score_public"`
	// ShowNameserverTimingsAdmin controls whether nameserver timing UI elements
	// are displayed in the admin interface. Defaults to true.
	ShowNameserverTimingsAdmin bool `json:"show_nameserver_timings_admin"`
	// ShowNameserverTimingsPublic controls whether nameserver timing UI elements
	// are displayed in the public results interface. Defaults to true.
	ShowNameserverTimingsPublic bool `json:"show_nameserver_timings_public"`
	// ShowDNSSECChainPublic gates DNSSEC chain collection and the public chain
	// endpoint and marker. Defaults to true.
	ShowDNSSECChainPublic bool `json:"show_dnssec_chain_public"`
}

// PublicAPIFileConfig holds optional public API configuration from JSON.
type PublicAPIFileConfig struct {
	RateLimitEnabled          *bool   `json:"rate_limit_enabled,omitempty"`
	RateLimitMax              *int    `json:"rate_limit_max,omitempty"`
	RateLimitWindow           *string `json:"rate_limit_window,omitempty"`
	AnalysisRequestTimeout    *string `json:"analysis_request_timeout,omitempty"`
	AllowPrivateUndelegatedIP *bool   `json:"allow_private_undelegated_ip,omitempty"`
	AllowNonGlobalTargets     *bool   `json:"allow_non_global_targets,omitempty"`
}

// DatabaseFileConfig holds optional database configuration from JSON.
// An empty string for Driver or DSN means "not set" (inherit from default).
type DatabaseFileConfig struct {
	Driver                 string `json:"driver,omitempty"`
	DSN                    string `json:"dsn,omitempty"`
	RetentionDays          *int   `json:"retention_days,omitempty"`
	PurgeIntervalSeconds   *int   `json:"purge_interval_seconds,omitempty"`
	MaxOpenConns           *int   `json:"max_open_conns,omitempty"`
	MaxIdleConns           *int   `json:"max_idle_conns,omitempty"`
	ConnMaxLifetimeSeconds *int   `json:"conn_max_lifetime_seconds,omitempty"`
}

// AnalysisFileConfig holds optional analysis-layer configuration from JSON.
type AnalysisFileConfig struct {
	TagViewMinLevel *string `json:"tag_view_min_level,omitempty"`
}

// FileConfig captures optional configuration fields from JSON.
type FileConfig struct {
	ListenAddr                  *string              `json:"listen_addr"`
	MaxBodySize                 *int64               `json:"max_body_size"`
	Debug                       *bool                `json:"debug"`
	WorkerCount                 *int                 `json:"worker_count"`
	MaxConcurrentJobs           *int                 `json:"max_concurrent_jobs"`
	StuckJobTimeoutMinutes      *int                 `json:"stuck_job_timeout_minutes,omitempty"`
	PositiveCacheTTL            *int                 `json:"positive_cache_ttl"`
	NegativeCacheTTL            *int                 `json:"negative_cache_ttl"`
	Timeout                     *int                 `json:"timeout"`
	Retry                       *int                 `json:"retry"`
	Retrans                     *int                 `json:"retrans"`
	Fallback                    *bool                `json:"fallback"`
	SourceAddr4                 *string              `json:"source_addr4"`
	SourceAddr6                 *string              `json:"source_addr6"`
	MinLevel                    *string              `json:"min_level"`
	ProfilePath                 *string              `json:"profile_path"`
	LogFormat                   *string              `json:"log_format,omitempty"`
	LogLevel                    *string              `json:"log_level,omitempty"`
	TrustedProxyCIDRs           *[]string            `json:"trusted_proxy_cidrs,omitempty"`
	ReadTimeout                 *string              `json:"read_timeout,omitempty"`
	WriteTimeout                *string              `json:"write_timeout,omitempty"`
	IdleTimeout                 *string              `json:"idle_timeout,omitempty"`
	PublicURL                   *string              `json:"public_url,omitempty"`
	PublicUIPath                *string              `json:"public_ui_path,omitempty"`
	Database                    *DatabaseFileConfig  `json:"database,omitempty"`
	PublicAPI                   *PublicAPIFileConfig `json:"public_api,omitempty"`
	Analysis                    *AnalysisFileConfig  `json:"analysis,omitempty"`
	Auth                        *AuthConfig          `json:"auth,omitempty"`
	ScoringConfigPath           *string              `json:"scoring_config_path,omitempty"`
	ShowScoreAdmin              *bool                `json:"show_score_admin,omitempty"`
	ShowScorePublic             *bool                `json:"show_score_public,omitempty"`
	ShowNameserverTimingsAdmin  *bool                `json:"show_nameserver_timings_admin,omitempty"`
	ShowNameserverTimingsPublic *bool                `json:"show_nameserver_timings_public,omitempty"`
	ShowDNSSECChainPublic       *bool                `json:"show_dnssec_chain_public,omitempty"`
	CrossJobHotCache            *bool                `json:"cross_job_hot_cache,omitempty"`
	CrossJobHotCacheTTLSeconds  *int                 `json:"cross_job_hot_cache_ttl_seconds,omitempty"`
}

// DefaultConfig returns baseline config values.
func DefaultConfig() Config {
	return Config{
		ListenAddr:                  "127.0.0.1:8080",
		MaxBodySize:                 1 << 20,
		Debug:                       false,
		WorkerCount:                 16,
		MaxConcurrentJobs:           0,
		StuckJobTimeoutMinutes:      defaultStuckJobTimeoutMinutes,
		MinLevel:                    "INFO",
		LogFormat:                   "text",
		LogLevel:                    "info",
		ShowScoreAdmin:              true,
		ShowScorePublic:             true,
		PublicUIPath:                serverpublic.DefaultUIPath,
		ShowNameserverTimingsAdmin:  true,
		ShowNameserverTimingsPublic: true,
		ShowDNSSECChainPublic:       true,
		CrossJobHotCache:            true,
		CrossJobHotCacheTTLSeconds:  defaultCrossJobHotCacheTTLSeconds,
		ReadTimeout:                 Duration{30 * time.Second},
		WriteTimeout:                Duration{60 * time.Second},
		IdleTimeout:                 Duration{60 * time.Second},
		PublicAPI: PublicAPIConfig{
			RateLimitEnabled:       false,
			RateLimitMax:           10,
			RateLimitWindow:        Duration{10 * time.Minute},
			AnalysisRequestTimeout: Duration{10 * time.Second},
		},
		Analysis: AnalysisConfig{
			TagViewMinLevel: "NOTICE",
		},
	}
}

// EffectiveCrossJobHotCacheTTL returns the hot-cache TTL as a time.Duration.
func (c Config) EffectiveCrossJobHotCacheTTL() time.Duration {
	secs := c.CrossJobHotCacheTTLSeconds
	if secs <= 0 {
		secs = defaultCrossJobHotCacheTTLSeconds
	}
	return time.Duration(secs) * time.Second
}

// EffectiveStuckJobTimeout returns the stuck-job age limit as a Duration.
// A zero or negative setting disables the reaper.
func (c Config) EffectiveStuckJobTimeout() time.Duration {
	if c.StuckJobTimeoutMinutes <= 0 {
		return 0
	}
	return time.Duration(c.StuckJobTimeoutMinutes) * time.Minute
}

// EffectivePurgeInterval returns the retention purge interval as a time.Duration.
func (c Config) EffectivePurgeInterval() time.Duration {
	secs := c.Database.PurgeIntervalSeconds
	if secs <= 0 {
		secs = defaultPurgeIntervalSeconds
	}
	return time.Duration(secs) * time.Second
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
	if file.StuckJobTimeoutMinutes != nil {
		c.StuckJobTimeoutMinutes = *file.StuckJobTimeoutMinutes
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
	if file.LogFormat != nil {
		c.LogFormat = *file.LogFormat
	}
	if file.LogLevel != nil {
		c.LogLevel = *file.LogLevel
	}
	if file.TrustedProxyCIDRs != nil {
		c.TrustedProxyCIDRs = append(c.TrustedProxyCIDRs[:0], *file.TrustedProxyCIDRs...)
	}
	if file.ReadTimeout != nil {
		if d, err := time.ParseDuration(*file.ReadTimeout); err == nil {
			c.ReadTimeout = Duration{d}
		}
	}
	if file.WriteTimeout != nil {
		if d, err := time.ParseDuration(*file.WriteTimeout); err == nil {
			c.WriteTimeout = Duration{d}
		}
	}
	if file.IdleTimeout != nil {
		if d, err := time.ParseDuration(*file.IdleTimeout); err == nil {
			c.IdleTimeout = Duration{d}
		}
	}
	if file.PublicURL != nil {
		c.PublicURL = *file.PublicURL
	}
	if file.PublicUIPath != nil {
		c.PublicUIPath = *file.PublicUIPath
	}
	if file.ScoringConfigPath != nil {
		c.ScoringConfigPath = *file.ScoringConfigPath
	}
	if file.ShowScoreAdmin != nil {
		c.ShowScoreAdmin = *file.ShowScoreAdmin
	}
	if file.ShowScorePublic != nil {
		c.ShowScorePublic = *file.ShowScorePublic
	}
	if file.ShowNameserverTimingsAdmin != nil {
		c.ShowNameserverTimingsAdmin = *file.ShowNameserverTimingsAdmin
	}
	if file.ShowNameserverTimingsPublic != nil {
		c.ShowNameserverTimingsPublic = *file.ShowNameserverTimingsPublic
	}
	if file.ShowDNSSECChainPublic != nil {
		c.ShowDNSSECChainPublic = *file.ShowDNSSECChainPublic
	}
	if file.CrossJobHotCache != nil {
		c.CrossJobHotCache = *file.CrossJobHotCache
	}
	if file.CrossJobHotCacheTTLSeconds != nil {
		c.CrossJobHotCacheTTLSeconds = *file.CrossJobHotCacheTTLSeconds
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
		if file.Database.PurgeIntervalSeconds != nil {
			c.Database.PurgeIntervalSeconds = *file.Database.PurgeIntervalSeconds
		}
		if file.Database.MaxOpenConns != nil {
			c.Database.MaxOpenConns = *file.Database.MaxOpenConns
		}
		if file.Database.MaxIdleConns != nil {
			c.Database.MaxIdleConns = *file.Database.MaxIdleConns
		}
		if file.Database.ConnMaxLifetimeSeconds != nil {
			c.Database.ConnMaxLifetimeSeconds = *file.Database.ConnMaxLifetimeSeconds
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
		if file.PublicAPI.AnalysisRequestTimeout != nil {
			d, err := time.ParseDuration(*file.PublicAPI.AnalysisRequestTimeout)
			if err == nil {
				c.PublicAPI.AnalysisRequestTimeout = Duration{d}
			}
		}
		if file.PublicAPI.AllowPrivateUndelegatedIP != nil {
			c.PublicAPI.AllowPrivateUndelegatedIP = *file.PublicAPI.AllowPrivateUndelegatedIP
		}
		if file.PublicAPI.AllowNonGlobalTargets != nil {
			c.PublicAPI.AllowNonGlobalTargets = *file.PublicAPI.AllowNonGlobalTargets
		}
	}
	if file.Analysis != nil {
		if file.Analysis.TagViewMinLevel != nil {
			level := strings.ToUpper(strings.TrimSpace(*file.Analysis.TagViewMinLevel))
			if isValidTagViewMinLevel(level) {
				c.Analysis.TagViewMinLevel = level
			}
		}
	}
	if file.Auth != nil {
		c.Auth = *file.Auth
	}
}

// isValidTagViewMinLevel returns true when level is one of the levels the
// projector can stamp on a tag-summary row. Anything else is treated as a
// no-op so a typo in the config file does not silently disable filtering.
func isValidTagViewMinLevel(level string) bool {
	switch level {
	case "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL":
		return true
	}
	return false
}
