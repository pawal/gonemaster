package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	var shutdownTimeout time.Duration
	var listenSet bool
	var maxBodySizeSet bool
	var debugSet bool
	var workerCountSet bool

	fs := flag.NewFlagSet("gonemaster-server", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: %s [--config PATH] [--listen ADDR] [--max-body-size BYTES] [--debug] [--workers N] [--shutdown-timeout DURATION]\n", fs.Name())
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Options:")
		fmt.Fprintln(errOut, "  --config            JSON config file path (optional)")
		fmt.Fprintln(errOut, "  --listen            Address to listen on (default :8080)")
		fmt.Fprintln(errOut, "  --max-body-size     Max request body size in bytes (default 1048576)")
		fmt.Fprintln(errOut, "  --debug             Enable request/response logging")
		fmt.Fprintln(errOut, "  --workers           Number of worker goroutines (default 4)")
		fmt.Fprintln(errOut, "  --shutdown-timeout  Graceful shutdown timeout (default 10s)")
	}
	fs.StringVar(&configPath, "config", "", "JSON config file path (optional)")
	fs.StringVar(&listen, "listen", ":8080", "Address to listen on (default :8080)")
	fs.Int64Var(&maxBodySize, "max-body-size", 0, "Max request body size in bytes (default 1048576)")
	fs.BoolVar(&debug, "debug", false, "Enable request/response logging")
	fs.IntVar(&workerCount, "workers", 0, "Number of worker goroutines (default 4)")
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
		}
	})
	if workerCountSet && workerCount < 1 {
		fmt.Fprintln(errOut, "--workers must be >= 1")
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

	srv := server.New(cfg)
	srv.Start()
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
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

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	return 0
}
