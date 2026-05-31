// Command gonemaster-mcp is a stdio MCP bridge to gonemaster-server's admin API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName       = "gonemaster-mcp"
	defaultServerURL = "http://localhost:8080/api/v1"
	defaultTimeout   = 30 * time.Second
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

// config holds settings sourced from the environment. The bridge is launched by
// an MCP client (Claude Code / Claude Desktop), so it takes no flags.
type config struct {
	serverURL string
	token     string
	timeout   time.Duration
}

func configFromEnv() config {
	return config{
		serverURL: envOr("GONEMASTER_URL", defaultServerURL),
		token:     os.Getenv("GONEMASTER_TOKEN"),
		timeout:   defaultTimeout,
	}
}

// authMode describes how the bridge authenticates, for the startup log.
func (c config) authMode() string {
	if c.token != "" {
		return "bearer token"
	}
	return "none"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	// Logs go to stderr; stdout is the MCP stdio channel and must carry only
	// protocol messages.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg := configFromEnv()
	api, err := newAPIClient(cfg)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	srv := newMCPServer(api)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting "+serverName, "version", version, "server_url", api.baseURL, "auth", cfg.authMode())
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(serverName+" stopped", "error", err)
		os.Exit(1)
	}
}
