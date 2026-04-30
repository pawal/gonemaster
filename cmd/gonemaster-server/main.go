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
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/server"
	"codeberg.org/pawal/gonemaster/server/analysis"
)

type usageLine struct {
	flag   string
	detail string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out *os.File, errOut *os.File) int {
	var configPath string
	var listen string
	var maxBodySize int64
	var debug bool
	var workerCount int
	var maxConcurrentJobs int
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
	var dbDriver string
	var dbDSN string
	var dbRetentionDays int
	var pubAPIRateLimitEnabled bool
	var pubAPIRateLimitMax int
	var pubAPIRateLimitWindow time.Duration
	var pubAPIAllowPrivateUndelegatedIP bool
	var trustedProxyCIDRs string
	var readTimeout time.Duration
	var writeTimeout time.Duration
	var idleTimeout time.Duration
	var crossJobHotCache bool
	var noCrossJobHotCache bool
	var crossJobHotCacheTTL int
	var showVersion bool
	var dumpConfig bool
	var shutdownTimeout time.Duration

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
			{flag: "--db-driver DRIVER", detail: "Storage backend: sqlite (or leave empty for in-memory) (env: GONEMASTER_DB_DRIVER)"},
			{flag: "--db-dsn DSN", detail: "SQLite: file path e.g. /var/lib/gonemaster/jobs.db (env: GONEMASTER_DB_DSN)"},
			{flag: "--db-retention-days N", detail: "Delete completed jobs older than N days (0 = keep forever) (env: GONEMASTER_DB_RETENTION_DAYS)"},
		})
		printUsageGroup(errOut, "Reverse proxy", []usageLine{
			{flag: "--trusted-proxy-cidrs LIST", detail: "Comma-separated CIDRs (or bare IPs) of reverse proxies allowed to set X-Forwarded-For. Empty = trust nothing (RemoteAddr only). Leave empty when the server is exposed directly. (env: GONEMASTER_TRUSTED_PROXY_CIDRS)"},
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
		printUsageGroup(errOut, "Output", []usageLine{
			{flag: "--min-level LEVEL", detail: "Minimum result log level (default INFO)"},
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
	fs.StringVar(&dbDriver, "db-driver", "", "Storage backend: sqlite (empty = in-memory)")
	fs.StringVar(&dbDSN, "db-dsn", "", "Database file path or connection string (optional)")
	fs.IntVar(&dbRetentionDays, "db-retention-days", 0, "Delete completed jobs older than N days (0 = keep forever)")
	fs.BoolVar(&pubAPIRateLimitEnabled, "public-api-rate-limit-enabled", false, "Enable per-IP rate limiting on POST /pub/api/v1/jobs")
	fs.IntVar(&pubAPIRateLimitMax, "public-api-rate-limit-max", 0, "Max job submissions per IP per window (default 10)")
	fs.DurationVar(&pubAPIRateLimitWindow, "public-api-rate-limit-window", 0, "Rate limit sliding window e.g. 5m (default 10m)")
	fs.BoolVar(&pubAPIAllowPrivateUndelegatedIP, "public-api-allow-private-undelegated-ip", false, "Allow private/loopback IPs as undelegated NS targets on the public API (default off)")
	fs.StringVar(&trustedProxyCIDRs, "trusted-proxy-cidrs", "", "Comma-separated CIDRs allowed to set X-Forwarded-For (default empty = trust nothing)")
	fs.DurationVar(&readTimeout, "read-timeout", 0, "Per-connection read timeout (default 30s)")
	fs.DurationVar(&writeTimeout, "write-timeout", 0, "Per-connection write timeout (default 60s)")
	fs.DurationVar(&idleTimeout, "idle-timeout", 0, "Idle keep-alive timeout (default 60s)")
	fs.BoolVar(&crossJobHotCache, "cross-job-hot-cache", false, "Enable cross-job nameserver cache sharing (default true)")
	fs.BoolVar(&noCrossJobHotCache, "no-cross-job-hot-cache", false, "Disable cross-job nameserver cache sharing")
	fs.IntVar(&crossJobHotCacheTTL, "cross-job-hot-cache-ttl", 0, "Hot-cache entry TTL in seconds (default 60)")
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
	if flagsSet["retry"] && retryCount < 0 {
		fmt.Fprintln(errOut, "--retry must be >= 0")
		return 2
	}
	if flagsSet["retrans"] && retransSeconds < 0 {
		fmt.Fprintln(errOut, "--retrans must be >= 0")
		return 2
	}
	if flagsSet["db-retention-days"] && dbRetentionDays < 0 {
		fmt.Fprintln(errOut, "--db-retention-days must be >= 0")
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
	if flagsSet["db-driver"] {
		cfg.Database.Driver = dbDriver
	}
	if flagsSet["db-dsn"] {
		cfg.Database.DSN = dbDSN
	}
	if flagsSet["db-retention-days"] {
		cfg.Database.RetentionDays = dbRetentionDays
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
	if ctrl, ok := analysis.NewControllerFromJobStore(srv.Store()); ok {
		// Best-effort enrichment of per-address ASN/prefix and per-ASN
		// label via the engine's cymru/ripe backends. A failure to build
		// the recursor (e.g. network misconfiguration at startup) just
		// leaves projection un-enriched instead of refusing to run.
		if rec, err := recursor.New(); err == nil {
			ctrl.SetEnricher(analysis.NewAsnlookupEnricher(rec, 0))
		} else {
			fmt.Fprintf(errOut, "analysis: enrichment disabled: %v\n", err)
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

	fmt.Fprintf(errOut, "Gonemaster version %s\n", engine.VersionFull())
	fmt.Fprintf(errOut, "Miekg DNS version %s\n", moduleVersion("codeberg.org/miekg/dns"))
	fmt.Fprintf(errOut, "Started server at %s\n", formatListenURL(cfg.ListenAddr))

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-shutdownCh
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Stop(ctx)
		_ = httpServer.Shutdown(ctx)
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(errOut, err.Error())
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
		"public-api-rate-limit-enabled":           "rate_limit_enabled",
		"public-api-rate-limit-max":               "rate_limit_max",
		"public-api-rate-limit-window":            "rate_limit_window",
		"public-api-allow-private-undelegated-ip": "allow_private_undelegated_ip",
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
