package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/server"
	"codeberg.org/pawal/gonemaster/server/analysis"
)

// checkProfileOverride validates a flag against the profile property it overrides.
func checkProfileOverride(property, flag string, value int) error {
	p, err := profile.Default()
	if err != nil {
		return fmt.Errorf("%s: %v", flag, err)
	}
	if err := p.Set(property, value); err != nil {
		return fmt.Errorf("%s: %v", flag, err)
	}
	return nil
}

type usageLine struct {
	flag   string
	detail string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out io.Writer, errOut io.Writer) int {
	if len(args) > 0 && args[0] == "auth" {
		return runAuthCommand(args[1:], out, errOut)
	}
	var configPath string
	var listen string
	var maxBodySize int64
	var debug bool
	var workerCount int
	var maxConcurrentJobs int
	var stuckJobTimeoutMinutes int
	var positiveCacheTTL int
	var negativeCacheTTL int
	var timeoutSeconds int
	var retryCount int
	var retransSeconds int
	var fallback bool
	var noFallback bool
	var sourceAddr4 string
	var sourceAddr6 string
	var minLevel string
	var profilePath string
	var logFormat string
	var logLevel string
	var dbDriver string
	var dbDSN string
	var dbRetentionDays int
	var dbPurgeInterval int
	var pubAPIRateLimitEnabled bool
	var pubAPIRateLimitMax int
	var pubAPIRateLimitWindow time.Duration
	var pubAPIAllowPrivateUndelegatedIP bool
	var pubAPIAllowNonGlobalTargets bool
	var trustedProxyCIDRs string
	var readTimeout time.Duration
	var writeTimeout time.Duration
	var idleTimeout time.Duration
	var crossJobHotCache bool
	var noCrossJobHotCache bool
	var crossJobHotCacheTTL int
	var extDataEnabled bool
	var extDataRefreshInterval time.Duration
	var extDataRecordTTL time.Duration
	var extDataNegativeTTL time.Duration
	var extDataTimeout time.Duration
	var extDataMaxRequestsPerMinute int
	var extDataMaxCachedRecords int
	var analysisVantageLabel string
	var showVersion bool
	var dumpConfig bool
	var shutdownTimeout time.Duration
	var adminTokenHashes string

	flagsSet := make(map[string]bool)

	fs := flag.NewFlagSet("gonemaster-server", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: %s [flags]\n\n", fs.Name())
		fmt.Fprintln(errOut, "Flags (CLI flags override environment variables and --config values):")
		printUsageGroup(errOut, "General", []usageLine{
			{flag: "--config PATH", detail: "JSON config file path"},
			{flag: "--listen ADDR", detail: "Address to listen on (default 127.0.0.1:8080)"},
			{flag: "--max-body-size BYTES", detail: "Max request body size in bytes (default 1048576)"},
			{flag: "--debug", detail: "Enable request/response logging"},
			{flag: "--dump-config", detail: "Print effective config as JSON and exit"},
			{flag: "--version", detail: "Print version information and exit"},
			{flag: "--shutdown-timeout DURATION", detail: "Graceful shutdown timeout (default 10s)"},
		})
		printUsageGroup(errOut, "Concurrency", []usageLine{
			{flag: "--workers N", detail: "Number of worker goroutines (default 16)"},
			{flag: "--max-concurrent-jobs N", detail: "Max concurrent engine runs (0 = unlimited)"},
			{flag: "--stuck-job-timeout N", detail: "Fail abandoned running jobs after N minutes (0 = disable, default 20) (env: GONEMASTER_STUCK_JOB_TIMEOUT)"},
			{flag: "--cross-job-hot-cache", detail: "Enable cross-job nameserver cache sharing (default true)"},
			{flag: "--no-cross-job-hot-cache", detail: "Disable cross-job nameserver cache sharing"},
			{flag: "--cross-job-hot-cache-ttl N", detail: "Hot-cache entry TTL in seconds (default 60)"},
		})
		printUsageGroup(errOut, "Resolver/Profile", []usageLine{
			{flag: "--profile PATH", detail: "Profile JSON/YAML path"},
			{flag: "--positive-cache-ttl N", detail: "Cache positive DNS responses (seconds)"},
			{flag: "--negative-cache-ttl N", detail: "Cache negative DNS responses (seconds)"},
			{flag: "--timeout N", detail: "Override resolver.defaults.timeout (seconds)"},
			{flag: "--retry N", detail: "Override resolver.defaults.retry"},
			{flag: "--retrans N", detail: "Override resolver.defaults.retrans (seconds)"},
			{flag: "--fallback", detail: "Enable TCP fallback on UDP failure"},
			{flag: "--no-fallback", detail: "Disable TCP fallback on UDP failure"},
			{flag: "--sourceaddr4 IPADDR", detail: "Override resolver.source4 (IPv4 source address)"},
			{flag: "--sourceaddr6 IPADDR", detail: "Override resolver.source6 (IPv6 source address)"},
		})
		printUsageGroup(errOut, "Database", []usageLine{
			{flag: "--db-driver DRIVER", detail: "Storage backend: memory (default), sqlite, postgres, or mariadb (env: GONEMASTER_DB_DRIVER)"},
			{flag: "--db-dsn DSN", detail: "SQLite file path (e.g. /var/lib/gonemaster/jobs.db) or postgres/mariadb connection string (env: GONEMASTER_DB_DSN)"},
			{flag: "--db-retention-days N", detail: "Delete completed jobs older than N days (0 = keep forever) (env: GONEMASTER_DB_RETENTION_DAYS)"},
			{flag: "--db-purge-interval N", detail: "Retention purge sweep interval in seconds (default 3600) (env: GONEMASTER_DB_PURGE_INTERVAL)"},
		})
		printUsageGroup(errOut, "Reverse proxy", []usageLine{
			{flag: "--trusted-proxy-cidrs LIST", detail: "Comma-separated CIDRs (or bare IPs) of reverse proxies allowed to set X-Forwarded-For. Empty = trust nothing (RemoteAddr only). Leave empty when the server is exposed directly. (env: GONEMASTER_TRUSTED_PROXY_CIDRS)"},
		})
		printUsageGroup(errOut, "Authentication", []usageLine{
			{flag: "--admin-token-hashes LIST", detail: "Comma-separated admin token hashes (label=sha256:hex) gating /api/v1. Empty = open mode. Mint with 'gonemaster-server auth add-token'. (env: GONEMASTER_ADMIN_TOKEN_HASHES)"},
		})
		printUsageGroup(errOut, "HTTP timeouts", []usageLine{
			{flag: "--read-timeout DURATION", detail: "Per-connection read timeout (default 30s). Caps slow request bodies. (env: GONEMASTER_READ_TIMEOUT)"},
			{flag: "--write-timeout DURATION", detail: "Per-connection write timeout (default 60s). Must exceed --public-api-analysis-request-timeout. (env: GONEMASTER_WRITE_TIMEOUT)"},
			{flag: "--idle-timeout DURATION", detail: "Idle keep-alive timeout (default 60s). (env: GONEMASTER_IDLE_TIMEOUT)"},
		})
		printUsageGroup(errOut, "Public API", []usageLine{
			{flag: "--public-api-rate-limit-enabled", detail: "Enable per-IP rate limiting on POST /pub/api/v1/jobs (env: GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED)"},
			{flag: "--public-api-rate-limit-max N", detail: "Max job submissions per IP per window (default 10) (env: GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX)"},
			{flag: "--public-api-rate-limit-window DURATION", detail: "Rate limit sliding window e.g. 5m (default 10m) (env: GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW)"},
			{flag: "--public-api-allow-private-undelegated-ip", detail: "Allow private/loopback IPs as undelegated NS targets on the public API (default off; enable for internal deployments) (env: GONEMASTER_PUBLIC_API_ALLOW_PRIVATE_UNDELEGATED_IP)"},
		})
		printUsageGroup(errOut, "External data", []usageLine{
			{flag: "--external-data-enabled", detail: "Fetch public registry reference data (IANA lists, RDAP) for the analysis dashboard. Default off; enabling it makes the server contact IANA and registry RDAP servers. (env: GONEMASTER_EXTERNAL_DATA_ENABLED)"},
			{flag: "--external-data-refresh-interval DURATION", detail: "How often the reference datasets are re-fetched (default 24h) (env: GONEMASTER_EXTERNAL_DATA_REFRESH_INTERVAL)"},
			{flag: "--external-data-record-ttl DURATION", detail: "How long a cached per-domain record is served before refresh (default 168h) (env: GONEMASTER_EXTERNAL_DATA_RECORD_TTL)"},
			{flag: "--external-data-negative-ttl DURATION", detail: "How long a failed fetch suppresses retries (default 1h) (env: GONEMASTER_EXTERNAL_DATA_NEGATIVE_TTL)"},
			{flag: "--external-data-timeout DURATION", detail: "Per-request timeout for outbound fetches (default 10s) (env: GONEMASTER_EXTERNAL_DATA_TIMEOUT)"},
			{flag: "--external-data-max-requests-per-minute N", detail: "Outbound fetch budget shared by all sources (default 30) (env: GONEMASTER_EXTERNAL_DATA_MAX_REQUESTS_PER_MINUTE)"},
			{flag: "--external-data-max-cached-records N", detail: "Maximum cached per-domain records (default 20000) (env: GONEMASTER_EXTERNAL_DATA_MAX_CACHED_RECORDS)"},
			{flag: "--analysis-vantage-label LABEL", detail: "Network location the analysis latency figures were measured from, shown in the dashboard's latency footnotes (env: GONEMASTER_ANALYSIS_VANTAGE_LABEL)"},
		})
		printUsageGroup(errOut, "Output", []usageLine{
			{flag: "--min-level LEVEL", detail: "Minimum result log level (default INFO)"},
		})
		printUsageGroup(errOut, "Logging", []usageLine{
			{flag: "--log-format FORMAT", detail: "Operational log encoding: text (default) or json (env: GONEMASTER_LOG_FORMAT)"},
			{flag: "--log-level LEVEL", detail: "Operational log level: debug|info|warn|error (default info) (env: GONEMASTER_LOG_LEVEL)"},
		})
	}
	fs.StringVar(&configPath, "config", "", "JSON config file path (optional)")
	fs.StringVar(&listen, "listen", "127.0.0.1:8080", "Address to listen on (default 127.0.0.1:8080)")
	fs.Int64Var(&maxBodySize, "max-body-size", 0, "Max request body size in bytes (default 1048576)")
	fs.BoolVar(&debug, "debug", false, "Enable request/response logging")
	fs.IntVar(&workerCount, "workers", 0, "Number of worker goroutines (default 16)")
	fs.IntVar(&maxConcurrentJobs, "max-concurrent-jobs", 0, "Max concurrent engine runs (0 = unlimited)")
	fs.IntVar(&positiveCacheTTL, "positive-cache-ttl", 0, "Seconds to cache positive DNS responses (optional)")
	fs.IntVar(&negativeCacheTTL, "negative-cache-ttl", 0, "Seconds to cache negative DNS responses (optional)")
	fs.IntVar(&timeoutSeconds, "timeout", 0, "Override resolver.defaults.timeout in seconds (optional)")
	fs.IntVar(&retryCount, "retry", 0, "Override resolver.defaults.retry (optional)")
	fs.IntVar(&retransSeconds, "retrans", 0, "Override resolver.defaults.retrans in seconds (optional)")
	fs.BoolVar(&fallback, "fallback", false, "Enable TCP fallback on UDP failure (optional)")
	fs.BoolVar(&noFallback, "no-fallback", false, "Disable TCP fallback on UDP failure (optional)")
	fs.StringVar(&sourceAddr4, "sourceaddr4", "", "Override resolver.source4 (IPv4 source address) (optional)")
	fs.StringVar(&sourceAddr6, "sourceaddr6", "", "Override resolver.source6 (IPv6 source address) (optional)")
	fs.StringVar(&minLevel, "min-level", "", "Minimum log level (default INFO)")
	fs.StringVar(&profilePath, "profile", "", "Profile JSON/YAML path (optional)")
	fs.StringVar(&logFormat, "log-format", "", "Operational log encoding: text (default) or json")
	fs.StringVar(&logLevel, "log-level", "", "Operational log level: debug|info|warn|error (default info)")
	fs.StringVar(&dbDriver, "db-driver", "", "Storage backend: memory (default), sqlite, postgres, or mariadb")
	fs.StringVar(&dbDSN, "db-dsn", "", "Database file path or connection string (optional)")
	fs.IntVar(&dbRetentionDays, "db-retention-days", 0, "Delete completed jobs older than N days (0 = keep forever)")
	fs.IntVar(&dbPurgeInterval, "db-purge-interval", 0, "Retention purge sweep interval in seconds (default 3600)")
	fs.IntVar(&stuckJobTimeoutMinutes, "stuck-job-timeout", 0, "Fail abandoned running jobs after N minutes (0 = disable, default 20)")
	fs.BoolVar(&pubAPIRateLimitEnabled, "public-api-rate-limit-enabled", false, "Enable per-IP rate limiting on POST /pub/api/v1/jobs")
	fs.IntVar(&pubAPIRateLimitMax, "public-api-rate-limit-max", 0, "Max job submissions per IP per window (default 10)")
	fs.DurationVar(&pubAPIRateLimitWindow, "public-api-rate-limit-window", 0, "Rate limit sliding window e.g. 5m (default 10m)")
	fs.BoolVar(&pubAPIAllowPrivateUndelegatedIP, "public-api-allow-private-undelegated-ip", false, "Allow private/loopback IPs as undelegated NS targets on the public API (default off)")
	fs.BoolVar(&pubAPIAllowNonGlobalTargets, "public-api-allow-non-global-targets", false, "Permit querying non-globally-reachable addresses; off clamps the engine guard on for every job (default off)")
	fs.StringVar(&trustedProxyCIDRs, "trusted-proxy-cidrs", "", "Comma-separated CIDRs allowed to set X-Forwarded-For (default empty = trust nothing)")
	fs.StringVar(&adminTokenHashes, "admin-token-hashes", "", "Comma-separated admin token hashes (label=sha256:...) gating /api/v1 (default empty = open mode)")
	fs.DurationVar(&readTimeout, "read-timeout", 0, "Per-connection read timeout (default 30s)")
	fs.DurationVar(&writeTimeout, "write-timeout", 0, "Per-connection write timeout (default 60s)")
	fs.DurationVar(&idleTimeout, "idle-timeout", 0, "Idle keep-alive timeout (default 60s)")
	fs.BoolVar(&crossJobHotCache, "cross-job-hot-cache", false, "Enable cross-job nameserver cache sharing (default true)")
	fs.BoolVar(&noCrossJobHotCache, "no-cross-job-hot-cache", false, "Disable cross-job nameserver cache sharing")
	fs.IntVar(&crossJobHotCacheTTL, "cross-job-hot-cache-ttl", 0, "Hot-cache entry TTL in seconds (default 60)")
	fs.BoolVar(&extDataEnabled, "external-data-enabled", false, "Fetch public registry reference data for the analysis dashboard (default off)")
	fs.DurationVar(&extDataRefreshInterval, "external-data-refresh-interval", 0, "Reference dataset refresh interval (default 24h)")
	fs.DurationVar(&extDataRecordTTL, "external-data-record-ttl", 0, "Cached per-domain record TTL (default 168h)")
	fs.DurationVar(&extDataNegativeTTL, "external-data-negative-ttl", 0, "How long a failed fetch suppresses retries (default 1h)")
	fs.DurationVar(&extDataTimeout, "external-data-timeout", 0, "Per-request timeout for outbound fetches (default 10s)")
	fs.IntVar(&extDataMaxRequestsPerMinute, "external-data-max-requests-per-minute", 0, "Outbound fetch budget per minute (default 30)")
	fs.IntVar(&extDataMaxCachedRecords, "external-data-max-cached-records", 0, "Maximum cached per-domain records (default 20000)")
	fs.StringVar(&analysisVantageLabel, "analysis-vantage-label", "", "Network location the analysis latency figures were measured from, e.g. \"Stockholm, SE\" (default empty)")
	fs.BoolVar(&showVersion, "version", false, "Print version and exit (optional)")
	fs.BoolVar(&dumpConfig, "dump-config", false, "Print effective config as JSON and exit")
	fs.DurationVar(&shutdownTimeout, "shutdown-timeout", 10*time.Second, "Graceful shutdown timeout (default 10s)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(out, "Gonemaster version %s\n", engine.VersionFull())
		fmt.Fprintf(out, "Miekg DNS version %s\n", moduleVersion("codeberg.org/miekg/dns"))
		return 0
	}
	fs.Visit(func(f *flag.Flag) {
		flagsSet[f.Name] = true
	})

	// Validate flags that were explicitly set.
	if flagsSet["workers"] && workerCount < 1 {
		fmt.Fprintln(errOut, "--workers must be >= 1")
		return 2
	}
	if flagsSet["max-concurrent-jobs"] && maxConcurrentJobs < 0 {
		fmt.Fprintln(errOut, "--max-concurrent-jobs must be >= 0")
		return 2
	}
	if flagsSet["positive-cache-ttl"] && positiveCacheTTL < 0 {
		fmt.Fprintln(errOut, "--positive-cache-ttl must be >= 0")
		return 2
	}
	if flagsSet["negative-cache-ttl"] && negativeCacheTTL < 0 {
		fmt.Fprintln(errOut, "--negative-cache-ttl must be >= 0")
		return 2
	}
	if flagsSet["timeout"] && timeoutSeconds < 0 {
		fmt.Fprintln(errOut, "--timeout must be >= 0")
		return 2
	}
	if flagsSet["retry"] {
		if err := checkProfileOverride("resolver.defaults.retry", "--retry", retryCount); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
	}
	if flagsSet["retrans"] {
		if err := checkProfileOverride("resolver.defaults.retrans", "--retrans", retransSeconds); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
	}
	if flagsSet["db-retention-days"] && dbRetentionDays < 0 {
		fmt.Fprintln(errOut, "--db-retention-days must be >= 0")
		return 2
	}
	if flagsSet["db-purge-interval"] && dbPurgeInterval < 1 {
		fmt.Fprintln(errOut, "--db-purge-interval must be >= 1")
		return 2
	}
	if flagsSet["stuck-job-timeout"] && stuckJobTimeoutMinutes < 0 {
		fmt.Fprintln(errOut, "--stuck-job-timeout must be >= 0")
		return 2
	}
	if flagsSet["public-api-rate-limit-max"] && pubAPIRateLimitMax < 1 {
		fmt.Fprintln(errOut, "--public-api-rate-limit-max must be >= 1")
		return 2
	}
	if flagsSet["public-api-rate-limit-window"] && pubAPIRateLimitWindow <= 0 {
		fmt.Fprintln(errOut, "--public-api-rate-limit-window must be positive")
		return 2
	}
	if flagsSet["fallback"] && flagsSet["no-fallback"] {
		fmt.Fprintln(errOut, "--fallback cannot be combined with --no-fallback")
		return 2
	}
	if flagsSet["cross-job-hot-cache"] && flagsSet["no-cross-job-hot-cache"] {
		fmt.Fprintln(errOut, "--cross-job-hot-cache cannot be combined with --no-cross-job-hot-cache")
		return 2
	}
	if flagsSet["sourceaddr4"] {
		addr, err := netip.ParseAddr(strings.TrimSpace(sourceAddr4))
		if err != nil || !addr.Is4() {
			fmt.Fprintln(errOut, "--sourceaddr4 must be a valid IPv4 address")
			return 2
		}
	}
	if flagsSet["sourceaddr6"] {
		addr, err := netip.ParseAddr(strings.TrimSpace(sourceAddr6))
		if err != nil || !addr.Is6() {
			fmt.Fprintln(errOut, "--sourceaddr6 must be a valid IPv6 address")
			return 2
		}
	}

	// Build config: defaults → file → env vars → CLI flags.
	cfg := server.DefaultConfig()
	if configPath != "" {
		fileCfg, err := server.LoadFileConfig(configPath)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		cfg.ApplyFileConfig(fileCfg)
	}
	applyEnvVars(&cfg, flagsSet, os.Getenv, errOut)
	if flagsSet["listen"] {
		cfg.ListenAddr = listen
	}
	if flagsSet["max-body-size"] {
		cfg.MaxBodySize = maxBodySize
	}
	if flagsSet["debug"] {
		cfg.Debug = debug
	}
	if flagsSet["workers"] {
		cfg.WorkerCount = workerCount
	}
	if flagsSet["max-concurrent-jobs"] {
		cfg.MaxConcurrentJobs = maxConcurrentJobs
	}
	if flagsSet["positive-cache-ttl"] {
		value := positiveCacheTTL
		cfg.PositiveCacheTTL = &value
	}
	if flagsSet["negative-cache-ttl"] {
		value := negativeCacheTTL
		cfg.NegativeCacheTTL = &value
	}
	if flagsSet["timeout"] {
		value := timeoutSeconds
		cfg.Timeout = &value
	}
	if flagsSet["retry"] {
		value := retryCount
		cfg.Retry = &value
	}
	if flagsSet["retrans"] {
		value := retransSeconds
		cfg.Retrans = &value
	}
	if flagsSet["fallback"] {
		value := true
		cfg.Fallback = &value
	}
	if flagsSet["no-fallback"] {
		value := false
		cfg.Fallback = &value
	}
	if flagsSet["cross-job-hot-cache"] {
		cfg.CrossJobHotCache = true
	}
	if flagsSet["no-cross-job-hot-cache"] {
		cfg.CrossJobHotCache = false
	}
	if flagsSet["cross-job-hot-cache-ttl"] {
		cfg.CrossJobHotCacheTTLSeconds = crossJobHotCacheTTL
	}
	if flagsSet["sourceaddr4"] {
		value := strings.TrimSpace(sourceAddr4)
		cfg.SourceAddr4 = &value
	}
	if flagsSet["sourceaddr6"] {
		value := strings.TrimSpace(sourceAddr6)
		cfg.SourceAddr6 = &value
	}
	if flagsSet["min-level"] {
		cfg.MinLevel = minLevel
	}
	if flagsSet["profile"] {
		cfg.ProfilePath = profilePath
	}
	if flagsSet["log-format"] {
		cfg.LogFormat = logFormat
	}
	if flagsSet["log-level"] {
		cfg.LogLevel = logLevel
	}
	if flagsSet["db-driver"] {
		cfg.Database.Driver = dbDriver
	}
	if flagsSet["db-dsn"] {
		cfg.Database.DSN = dbDSN
	}
	if flagsSet["db-retention-days"] {
		cfg.Database.RetentionDays = dbRetentionDays
	}
	if flagsSet["db-purge-interval"] {
		cfg.Database.PurgeIntervalSeconds = dbPurgeInterval
	}
	if flagsSet["stuck-job-timeout"] {
		cfg.StuckJobTimeoutMinutes = stuckJobTimeoutMinutes
	}
	if flagsSet["public-api-rate-limit-enabled"] {
		cfg.PublicAPI.RateLimitEnabled = pubAPIRateLimitEnabled
	}
	if flagsSet["public-api-rate-limit-max"] {
		cfg.PublicAPI.RateLimitMax = pubAPIRateLimitMax
	}
	if flagsSet["public-api-rate-limit-window"] {
		cfg.PublicAPI.RateLimitWindow = server.Duration{Duration: pubAPIRateLimitWindow}
	}
	if flagsSet["public-api-allow-private-undelegated-ip"] {
		cfg.PublicAPI.AllowPrivateUndelegatedIP = pubAPIAllowPrivateUndelegatedIP
	}
	if flagsSet["public-api-allow-non-global-targets"] {
		cfg.PublicAPI.AllowNonGlobalTargets = pubAPIAllowNonGlobalTargets
	}
	if flagsSet["external-data-enabled"] {
		cfg.ExternalData.Enabled = extDataEnabled
	}
	if flagsSet["external-data-refresh-interval"] {
		cfg.ExternalData.RefreshInterval = server.Duration{Duration: extDataRefreshInterval}
	}
	if flagsSet["external-data-record-ttl"] {
		cfg.ExternalData.RecordTTL = server.Duration{Duration: extDataRecordTTL}
	}
	if flagsSet["external-data-negative-ttl"] {
		cfg.ExternalData.NegativeTTL = server.Duration{Duration: extDataNegativeTTL}
	}
	if flagsSet["external-data-timeout"] {
		cfg.ExternalData.Timeout = server.Duration{Duration: extDataTimeout}
	}
	if flagsSet["external-data-max-requests-per-minute"] {
		cfg.ExternalData.MaxRequestsPerMinute = extDataMaxRequestsPerMinute
	}
	if flagsSet["external-data-max-cached-records"] {
		cfg.ExternalData.MaxCachedRecords = extDataMaxCachedRecords
	}
	if flagsSet["analysis-vantage-label"] {
		cfg.Analysis.VantageLabel = analysisVantageLabel
	}
	if flagsSet["trusted-proxy-cidrs"] {
		cfg.TrustedProxyCIDRs = strings.Split(trustedProxyCIDRs, ",")
	}
	if flagsSet["read-timeout"] {
		cfg.ReadTimeout = server.Duration{Duration: readTimeout}
	}
	if flagsSet["write-timeout"] {
		cfg.WriteTimeout = server.Duration{Duration: writeTimeout}
	}
	if flagsSet["idle-timeout"] {
		cfg.IdleTimeout = server.Duration{Duration: idleTimeout}
	}

	// Admin auth tokens: file (already applied) < env < flag.
	if env := os.Getenv("GONEMASTER_ADMIN_TOKEN_HASHES"); env != "" {
		cfg.Auth.AdminTokens = parseAdminTokenHashes(env)
	}
	if flagsSet["admin-token-hashes"] {
		cfg.Auth.AdminTokens = parseAdminTokenHashes(adminTokenHashes)
	}
	if err := server.ValidateAuthConfig(cfg.Auth); err != nil {
		fmt.Fprintf(errOut, "invalid admin tokens: %v\n", err)
		return 2
	}
	if err := server.ValidateLogConfig(cfg); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	if dumpConfig {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cfg); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}

	srv, err := server.NewWithOptions(cfg)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	logger := srv.Logger()
	if ctrl, ok := analysis.NewControllerFromJobStore(srv.Store()); ok {
		// Best-effort enrichment of per-address ASN/prefix and per-ASN
		// label via the engine's cymru/ripe backends. A failure to build
		// the recursor (e.g. network misconfiguration at startup) just
		// leaves projection un-enriched instead of refusing to run.
		if rec, err := recursor.New(); err == nil {
			// ASN/prefix mappings change rarely, so cache results a month.
			ctrl.SetEnricher(analysis.NewAsnlookupEnricher(rec, 30*24*time.Hour))
		} else {
			logger.Warn("analysis enrichment disabled", "err", err)
		}
		srv.SetAnalysisController(ctrl)
	}

	// Track where each setting value came from so the settings API
	// can show the source and respect CLI flag precedence.
	configSources := buildConfigSources(flagsSet, configPath != "")
	srv.SetConfigSources(configSources)
	srv.ApplyDatabaseSettings()

	srv.Start()
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.ReadTimeout.Duration,
		WriteTimeout:      cfg.WriteTimeout.Duration,
		IdleTimeout:       cfg.IdleTimeout.Duration,
	}

	logger.Info("server starting",
		"version", engine.VersionFull(),
		"miekg_dns_version", moduleVersion("codeberg.org/miekg/dns"),
		"listen", formatListenURL(cfg.ListenAddr),
		"log_format", cfg.LogFormat)
	if n := len(cfg.Auth.AdminTokens); n == 0 {
		logger.Info("auth open mode", "tokens", 0)
	} else {
		logger.Info("auth token mode", "tokens", n)
	}

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-shutdownCh
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Stop(ctx)
		_ = httpServer.Shutdown(ctx)
	}()

	flagHashes := ""
	if flagsSet["admin-token-hashes"] {
		flagHashes = adminTokenHashes
	}
	hupCh := make(chan os.Signal, 1)
	signal.Notify(hupCh, syscall.SIGHUP)
	go func() {
		for range hupCh {
			auth, err := resolveAuthConfig(configPath, os.Getenv("GONEMASTER_ADMIN_TOKEN_HASHES"), flagHashes)
			if err != nil {
				logger.Error("auth reload failed", "err", err)
				continue
			}
			if err := srv.ReloadAuth(auth); err != nil {
				logger.Error("auth reload rejected", "err", err)
				continue
			}
			if n := len(auth.AdminTokens); n == 0 {
				logger.Info("auth reloaded", "mode", "open", "tokens", 0)
			} else {
				logger.Info("auth reloaded", "mode", "token", "tokens", n)
			}
		}
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("listen failed", "err", err)
		return 2
	}

	return 0
}

func formatListenURL(addr string) string {
	if strings.TrimSpace(addr) == "" {
		return "http://localhost"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	if port == "" {
		return "http://" + host
	}
	return "http://" + host + ":" + port
}

func printUsageGroup(out io.Writer, title string, lines []usageLine) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(out, "  %s:\n", title)
	for _, line := range lines {
		fmt.Fprintf(out, "    %-50s %s\n", line.flag, line.detail)
	}
	fmt.Fprintln(out, "")
}

func moduleVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return "unknown"
	}
	if info.Main.Path == path {
		return normalizeVersion(info.Main.Version)
	}
	for _, dep := range info.Deps {
		if dep == nil || dep.Path != path {
			continue
		}
		if dep.Replace != nil {
			if dep.Replace.Version != "" {
				return normalizeVersion(dep.Replace.Version)
			}
			return dep.Replace.Path
		}
		return normalizeVersion(dep.Version)
	}
	return "unknown"
}

func normalizeVersion(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}

// buildConfigSources maps setting keys to their origin so the settings API
// can display sources and preserve CLI flag precedence.
func buildConfigSources(flagsSet map[string]bool, hasConfigFile bool) map[string]server.SettingSource {
	// Map CLI flag names to settings API keys.
	flagToKey := map[string]string{
		"listen":                        "listen_addr",
		"workers":                       "worker_count",
		"max-concurrent-jobs":           "max_concurrent_jobs",
		"min-level":                     "min_level",
		"profile":                       "profile_path",
		"db-driver":                     "db_driver",
		"db-dsn":                        "db_dsn",
		"db-retention-days":             "retention_days",
		"db-purge-interval":             "purge_interval_seconds",
		"stuck-job-timeout":             "stuck_job_timeout_minutes",
		"public-api-rate-limit-enabled": "rate_limit_enabled",
		"public-api-rate-limit-max":     "rate_limit_max",
		"public-api-rate-limit-window":  "rate_limit_window",
		"public-api-allow-private-undelegated-ip": "allow_private_undelegated_ip",
		"public-api-allow-non-global-targets":     "allow_non_global_targets",
		"trusted-proxy-cidrs":                     "trusted_proxy_cidrs",
		"read-timeout":                            "read_timeout",
		"write-timeout":                           "write_timeout",
		"idle-timeout":                            "idle_timeout",
	}

	sources := make(map[string]server.SettingSource)
	for flag, key := range flagToKey {
		if flagsSet[flag] {
			sources[key] = server.SourceCLIFlag
		} else if hasConfigFile {
			sources[key] = server.SourceConfigFile
		}
	}
	return sources
}
