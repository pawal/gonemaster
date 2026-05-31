// Command gonemaster-mcp is a stdio MCP bridge to gonemaster-server's admin API.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
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
// an MCP client (Claude Code / Claude Desktop) as a subprocess, so configuration
// is env-only; the only command-line flags are --help and --version.
type config struct {
	serverURL  string
	token      string
	timeout    time.Duration
	allowWrite bool
}

func configFromEnv() config {
	return config{
		serverURL:  envOr("GONEMASTER_URL", defaultServerURL),
		token:      os.Getenv("GONEMASTER_TOKEN"),
		timeout:    defaultTimeout,
		allowWrite: envBool("GONEMASTER_MCP_ALLOW_WRITE"),
	}
}

// envBool reports whether an env var is set to an enabling value (1/true/yes/on).
func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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

// handleInfoFlags prints help or version to out and returns true when one was
// requested, so the caller can exit before starting the stdio loop.
func handleInfoFlags(args []string, out io.Writer) bool {
	for _, a := range args {
		switch a {
		case "-h", "--help", "-help":
			fmt.Fprint(out, usageText())
			return true
		case "-v", "-V", "--version", "-version":
			fmt.Fprintf(out, "%s %s\n", serverName, version)
			return true
		}
	}
	return false
}

func usageText() string {
	return serverName + ` - Model Context Protocol (stdio) bridge to gonemaster-server.

Launched by an MCP client (e.g. Claude Code or Claude Desktop) as a subprocess
and configured through environment variables:

  GONEMASTER_URL              Admin API base URL (default ` + defaultServerURL + `)
  GONEMASTER_TOKEN            Admin token for Authorization: Bearer (optional)
  GONEMASTER_MCP_ALLOW_WRITE  Set to 1 to enable the write tools

Options:
  -h, --help     Show this help and exit
  -v, --version  Print version and exit

See gonemaster-mcp(1) for details.
`
}

func main() {
	if handleInfoFlags(os.Args[1:], os.Stdout) {
		return
	}

	// Logs go to stderr; stdout is the MCP stdio channel and must carry only
	// protocol messages.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg := configFromEnv()
	api, err := newAPIClient(cfg)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	srv := newMCPServer(api, cfg.allowWrite)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting "+serverName, "version", version, "server_url", api.baseURL, "auth", cfg.authMode(), "write_tools", cfg.allowWrite)
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(serverName+" stopped", "error", err)
		os.Exit(1)
	}
}
