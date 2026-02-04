package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
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
	var maxConcurrentJobs int
	var positiveCacheTTL int
	var negativeCacheTTL int
	var minLevel string
	var profilePath string
	var shutdownTimeout time.Duration
	var listenSet bool
	var maxBodySizeSet bool
	var debugSet bool
	var workerCountSet bool
	var maxConcurrentJobsSet bool
	var positiveCacheTTLSet bool
	var negativeCacheTTLSet bool
	var minLevelSet bool
	var profilePathSet bool

	fs := flag.NewFlagSet("gonemaster-server", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: %s [--config PATH] [--listen ADDR] [--max-body-size BYTES] [--debug] [--workers N] [--max-concurrent-jobs N] [--positive-cache-ttl N] [--negative-cache-ttl N] [--min-level LEVEL] [--profile PATH] [--shutdown-timeout DURATION]\n", fs.Name())
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Options:")
		fmt.Fprintln(errOut, "  --config            JSON config file path (optional)")
		fmt.Fprintln(errOut, "  --listen            Address to listen on (default :8080)")
		fmt.Fprintln(errOut, "  --max-body-size     Max request body size in bytes (default 1048576)")
		fmt.Fprintln(errOut, "  --debug             Enable request/response logging")
		fmt.Fprintln(errOut, "  --workers           Number of worker goroutines (default 4)")
		fmt.Fprintln(errOut, "  --max-concurrent-jobs  Max concurrent engine runs (0 = unlimited)")
		fmt.Fprintln(errOut, "  --positive-cache-ttl  Seconds to cache positive DNS responses (optional)")
		fmt.Fprintln(errOut, "  --negative-cache-ttl  Seconds to cache negative DNS responses (optional)")
		fmt.Fprintln(errOut, "  --min-level         Minimum log level (default INFO)")
		fmt.Fprintln(errOut, "  --profile           Profile JSON/YAML path (optional)")
		fmt.Fprintln(errOut, "  --shutdown-timeout  Graceful shutdown timeout (default 10s)")
	}
	fs.StringVar(&configPath, "config", "", "JSON config file path (optional)")
	fs.StringVar(&listen, "listen", ":8080", "Address to listen on (default :8080)")
	fs.Int64Var(&maxBodySize, "max-body-size", 0, "Max request body size in bytes (default 1048576)")
	fs.BoolVar(&debug, "debug", false, "Enable request/response logging")
	fs.IntVar(&workerCount, "workers", 0, "Number of worker goroutines (default 4)")
	fs.IntVar(&maxConcurrentJobs, "max-concurrent-jobs", 0, "Max concurrent engine runs (0 = unlimited)")
	fs.IntVar(&positiveCacheTTL, "positive-cache-ttl", 0, "Seconds to cache positive DNS responses (optional)")
	fs.IntVar(&negativeCacheTTL, "negative-cache-ttl", 0, "Seconds to cache negative DNS responses (optional)")
	fs.StringVar(&minLevel, "min-level", "", "Minimum log level (default INFO)")
	fs.StringVar(&profilePath, "profile", "", "Profile JSON/YAML path (optional)")
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
		case "max-concurrent-jobs":
			maxConcurrentJobsSet = true
		case "positive-cache-ttl":
			positiveCacheTTLSet = true
		case "negative-cache-ttl":
			negativeCacheTTLSet = true
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
	if maxConcurrentJobsSet && maxConcurrentJobs < 0 {
		fmt.Fprintln(errOut, "--max-concurrent-jobs must be >= 0")
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
	if maxConcurrentJobsSet {
		cfg.MaxConcurrentJobs = maxConcurrentJobs
	}
	if positiveCacheTTLSet {
		value := positiveCacheTTL
		cfg.PositiveCacheTTL = &value
	}
	if negativeCacheTTLSet {
		value := negativeCacheTTL
		cfg.NegativeCacheTTL = &value
	}
	if minLevelSet {
		cfg.MinLevel = minLevel
	}
	if profilePathSet {
		cfg.ProfilePath = profilePath
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
