package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/server"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out *os.File, errOut *os.File) int {
	var configPath string
	var listen string
	var maxBodySize int64
	var debug bool
	var workerCount int
	var autoClampConcurrency bool
	var jobTestParallelism int
	var maxConcurrentJobs int
	var crossJobHotCache bool
	var crossJobHotCacheTTLSeconds int
	var positiveCacheTTL int
	var negativeCacheTTL int
	var timeoutSeconds int
	var retryCount int
	var retransSeconds int
	var fallback bool
	var noFallback bool
	var minLevel string
	var profilePath string
	var shutdownTimeout time.Duration
	var listenSet bool
	var maxBodySizeSet bool
	var debugSet bool
	var workerCountSet bool
	var autoClampConcurrencySet bool
	var jobTestParallelismSet bool
	var maxConcurrentJobsSet bool
	var crossJobHotCacheSet bool
	var crossJobHotCacheTTLSet bool
	var positiveCacheTTLSet bool
	var negativeCacheTTLSet bool
	var timeoutSet bool
	var retrySet bool
	var retransSet bool
	var fallbackSet bool
	var noFallbackSet bool
	var minLevelSet bool
	var profilePathSet bool

	fs := flag.NewFlagSet("gonemaster-server", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: %s [flags]\n", fs.Name())
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Flags (CLI flags override --config values):")
		fmt.Fprintln(errOut, "  General:")
		fmt.Fprintln(errOut, "    --config PATH                     JSON config file")
		fmt.Fprintln(errOut, "    --listen ADDR                     HTTP listen address (default 127.0.0.1:8080)")
		fmt.Fprintln(errOut, "    --max-body-size BYTES             Max request body size (default 1048576)")
		fmt.Fprintln(errOut, "    --debug                           Enable request/response logging")
		fmt.Fprintln(errOut, "    --shutdown-timeout DURATION       Graceful shutdown timeout (default 10s)")
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "  Concurrency:")
		fmt.Fprintln(errOut, "    --workers N                       Queue worker goroutines (default 4)")
		fmt.Fprintln(errOut, "    --max-concurrent-jobs N           Cap in-flight engine runs (0 = unlimited)")
		fmt.Fprintln(errOut, "    --job-test-parallelism N          Parallel testcases inside one job (default 1)")
		fmt.Fprintln(errOut, "    --auto-clamp-concurrency          Auto-clamp pathological worker/concurrency values")
		fmt.Fprintln(errOut, "    --cross-job-hot-cache             Reuse short-lived warmed nameserver caches across jobs")
		fmt.Fprintln(errOut, "    --cross-job-hot-cache-ttl-seconds N  TTL for hot-cache entries (default 60)")
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "  Resolver/Profile:")
		fmt.Fprintln(errOut, "    --profile PATH                    Default profile JSON/YAML path")
		fmt.Fprintln(errOut, "    --positive-cache-ttl N            Override positive DNS cache TTL in seconds")
		fmt.Fprintln(errOut, "    --negative-cache-ttl N            Override negative DNS cache TTL in seconds")
		fmt.Fprintln(errOut, "    --timeout N                       Override query timeout in seconds")
		fmt.Fprintln(errOut, "    --retry N                         Override retry count")
		fmt.Fprintln(errOut, "    --retrans N                       Override retransmit interval in seconds")
		fmt.Fprintln(errOut, "    --fallback                        Force TCP fallback on UDP failure")
		fmt.Fprintln(errOut, "    --no-fallback                     Disable TCP fallback on UDP failure")
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "  Output:")
		fmt.Fprintln(errOut, "    --min-level LEVEL                 Minimum result log level (default INFO)")
	}
	fs.StringVar(&configPath, "config", "", "JSON config file path")
	fs.StringVar(&listen, "listen", "127.0.0.1:8080", "HTTP listen address (default 127.0.0.1:8080)")
	fs.Int64Var(&maxBodySize, "max-body-size", 0, "Max request body size in bytes (default 1048576)")
	fs.BoolVar(&debug, "debug", false, "Enable request/response logging")
	fs.IntVar(&workerCount, "workers", 0, "Queue worker goroutines (default 4)")
	fs.BoolVar(&autoClampConcurrency, "auto-clamp-concurrency", false, "Auto-clamp pathological worker/concurrency values")
	fs.IntVar(&jobTestParallelism, "job-test-parallelism", 0, "Parallel testcases inside one job (default 1)")
	fs.IntVar(&maxConcurrentJobs, "max-concurrent-jobs", 0, "Cap in-flight engine runs (0 = unlimited)")
	fs.BoolVar(&crossJobHotCache, "cross-job-hot-cache", false, "Reuse short-lived warmed nameserver caches across jobs")
	fs.IntVar(&crossJobHotCacheTTLSeconds, "cross-job-hot-cache-ttl-seconds", 0, "TTL for cross-job hot-cache entries in seconds (default 60)")
	fs.IntVar(&positiveCacheTTL, "positive-cache-ttl", 0, "Override positive DNS cache TTL in seconds")
	fs.IntVar(&negativeCacheTTL, "negative-cache-ttl", 0, "Override negative DNS cache TTL in seconds")
	fs.IntVar(&timeoutSeconds, "timeout", 0, "Override query timeout in seconds")
	fs.IntVar(&retryCount, "retry", 0, "Override retry count")
	fs.IntVar(&retransSeconds, "retrans", 0, "Override retransmit interval in seconds")
	fs.BoolVar(&fallback, "fallback", false, "Force TCP fallback on UDP failure")
	fs.BoolVar(&noFallback, "no-fallback", false, "Disable TCP fallback on UDP failure")
	fs.StringVar(&minLevel, "min-level", "", "Minimum result log level (default INFO)")
	fs.StringVar(&profilePath, "profile", "", "Default profile JSON/YAML path")
	fs.DurationVar(&shutdownTimeout, "shutdown-timeout", 10*time.Second, "Graceful shutdown timeout (default 10s)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "listen":
			listenSet = true
		case "max-body-size":
			maxBodySizeSet = true
		case "debug":
			debugSet = true
		case "workers":
			workerCountSet = true
		case "auto-clamp-concurrency":
			autoClampConcurrencySet = true
		case "job-test-parallelism":
			jobTestParallelismSet = true
		case "max-concurrent-jobs":
			maxConcurrentJobsSet = true
		case "cross-job-hot-cache":
			crossJobHotCacheSet = true
		case "cross-job-hot-cache-ttl-seconds":
			crossJobHotCacheTTLSet = true
		case "positive-cache-ttl":
			positiveCacheTTLSet = true
		case "negative-cache-ttl":
			negativeCacheTTLSet = true
		case "timeout":
			timeoutSet = true
		case "retry":
			retrySet = true
		case "retrans":
			retransSet = true
		case "fallback":
			fallbackSet = true
		case "no-fallback":
			noFallbackSet = true
		case "min-level":
			minLevelSet = true
		case "profile":
			profilePathSet = true
		}
	})
	if workerCountSet && workerCount < 1 {
		fmt.Fprintln(errOut, "--workers must be >= 1")
		return 2
	}
	if jobTestParallelismSet && jobTestParallelism < 1 {
		fmt.Fprintln(errOut, "--job-test-parallelism must be >= 1")
		return 2
	}
	if maxConcurrentJobsSet && maxConcurrentJobs < 0 {
		fmt.Fprintln(errOut, "--max-concurrent-jobs must be >= 0")
		return 2
	}
	if crossJobHotCacheTTLSet && crossJobHotCacheTTLSeconds < 1 {
		fmt.Fprintln(errOut, "--cross-job-hot-cache-ttl-seconds must be >= 1")
		return 2
	}
	if positiveCacheTTLSet && positiveCacheTTL < 0 {
		fmt.Fprintln(errOut, "--positive-cache-ttl must be >= 0")
		return 2
	}
	if negativeCacheTTLSet && negativeCacheTTL < 0 {
		fmt.Fprintln(errOut, "--negative-cache-ttl must be >= 0")
		return 2
	}
	if timeoutSet && timeoutSeconds < 0 {
		fmt.Fprintln(errOut, "--timeout must be >= 0")
		return 2
	}
	if retrySet && retryCount < 0 {
		fmt.Fprintln(errOut, "--retry must be >= 0")
		return 2
	}
	if retransSet && retransSeconds < 0 {
		fmt.Fprintln(errOut, "--retrans must be >= 0")
		return 2
	}
	if fallbackSet && noFallbackSet {
		fmt.Fprintln(errOut, "--fallback cannot be combined with --no-fallback")
		return 2
	}

	cfg := server.DefaultConfig()
	if configPath != "" {
		fileCfg, err := server.LoadFileConfig(configPath)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		cfg.ApplyFileConfig(fileCfg)
	}
	if listenSet {
		cfg.ListenAddr = listen
	}
	if maxBodySizeSet {
		cfg.MaxBodySize = maxBodySize
	}
	if debugSet {
		cfg.Debug = debug
	}
	if workerCountSet {
		cfg.WorkerCount = workerCount
	}
	if autoClampConcurrencySet {
		cfg.AutoClampConcurrency = autoClampConcurrency
	}
	if jobTestParallelismSet {
		cfg.JobTestParallelism = jobTestParallelism
	}
	if maxConcurrentJobsSet {
		cfg.MaxConcurrentJobs = maxConcurrentJobs
	}
	if crossJobHotCacheSet {
		cfg.CrossJobHotCache = crossJobHotCache
	}
	if crossJobHotCacheTTLSet {
		cfg.CrossJobHotCacheTTLSeconds = crossJobHotCacheTTLSeconds
	}
	if positiveCacheTTLSet {
		value := positiveCacheTTL
		cfg.PositiveCacheTTL = &value
	}
	if negativeCacheTTLSet {
		value := negativeCacheTTL
		cfg.NegativeCacheTTL = &value
	}
	if timeoutSet {
		value := timeoutSeconds
		cfg.Timeout = &value
	}
	if retrySet {
		value := retryCount
		cfg.Retry = &value
	}
	if retransSet {
		value := retransSeconds
		cfg.Retrans = &value
	}
	if fallbackSet {
		value := true
		cfg.Fallback = &value
	}
	if noFallbackSet {
		value := false
		cfg.Fallback = &value
	}
	if minLevelSet {
		cfg.MinLevel = minLevel
	}
	if profilePathSet {
		cfg.ProfilePath = profilePath
	}
	if cfg.AutoClampConcurrency {
		cpuCount := runtime.NumCPU()
		clamped, changed := cfg.AutoClampConcurrencyForHost(cpuCount)
		if changed {
			fmt.Fprintf(errOut, "Applied concurrency auto-clamp (cpu=%d): %s\n", cpuCount, formatConcurrencySummary(clamped))
		}
		cfg = clamped
	}

	srv := server.New(cfg)
	srv.Start()
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Fprintf(errOut, "Gonemaster version %s\n", engine.VersionFull())
	fmt.Fprintf(errOut, "Started server at %s\n", formatListenURL(cfg.ListenAddr))
	fmt.Fprintf(errOut, "Effective concurrency: %s\n", formatConcurrencySummary(cfg))

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

func formatConcurrencySummary(cfg server.Config) string {
	maxConcurrentJobs := cfg.EffectiveMaxConcurrentJobs()
	maxConcurrentJobsLabel := "unlimited"
	if maxConcurrentJobs > 0 {
		maxConcurrentJobsLabel = fmt.Sprintf("%d", maxConcurrentJobs)
	}
	return fmt.Sprintf(
		"workers=%d, max-concurrent-jobs=%s, max-in-flight-engine-runs=%d, job-test-parallelism=%d",
		cfg.EffectiveWorkerCount(),
		maxConcurrentJobsLabel,
		cfg.EffectiveEngineConcurrency(),
		cfg.EffectiveJobTestParallelism(),
	)
}
