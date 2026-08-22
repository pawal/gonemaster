package server

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestNewTestServerDefaultsToDefaultConfig(t *testing.T) {
	srv := newTestServer(t)

	if srv.cfg.WorkerCount != DefaultConfig().WorkerCount {
		t.Fatalf("WorkerCount = %d, want the default", srv.cfg.WorkerCount)
	}
	// No options means open mode, the same as New(DefaultConfig()).
	if srv.cfg.Auth.AdminTokens != nil {
		t.Fatalf("expected open mode, got %d tokens", len(srv.cfg.Auth.AdminTokens))
	}
	if srv.store == nil {
		t.Fatal("expected a store")
	}
}

func TestNewTestServerConfigOptions(t *testing.T) {
	srv := newTestServer(t,
		withWorkers(3, 2),
		withPublicAPI(func(api *PublicAPIConfig) { api.RateLimitEnabled = true }),
		withConfig(func(cfg *Config) { cfg.ShowScoreAdmin = false }),
	)

	if srv.cfg.WorkerCount != 3 || srv.cfg.MaxConcurrentJobs != 2 {
		t.Fatalf("workers = %d/%d, want 3/2", srv.cfg.WorkerCount, srv.cfg.MaxConcurrentJobs)
	}
	if !srv.cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected the public rate limit enabled")
	}
	if srv.cfg.ShowScoreAdmin {
		t.Fatal("expected ShowScoreAdmin off")
	}
}

func TestWithAuthSwitchesToTokenMode(t *testing.T) {
	srv := newTestServer(t, withAuth("gm_secret"))

	if len(srv.cfg.Auth.AdminTokens) != 1 {
		t.Fatalf("expected one token, got %d", len(srv.cfg.Auth.AdminTokens))
	}
	// The token is stored hashed, never in the clear.
	if srv.cfg.Auth.AdminTokens[0].Hash == "gm_secret" {
		t.Fatal("expected the token hashed")
	}

	if resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs", nil); resp.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", resp.Code)
	}
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs", nil, withBearer("gm_secret"))
	wantStatus(t, resp, http.StatusOK)
}

func TestWithDBConstructsAroundTheGivenStore(t *testing.T) {
	store := NewInMemoryJobStore()
	srv := newTestServer(t, withDB(store))

	if srv.store != store {
		t.Fatal("expected the given store to be the server's store")
	}
}

func TestWithStoreReplacesTheStoreAfterConstruction(t *testing.T) {
	store := NewInMemoryJobStore()
	srv := newTestServer(t, withStore(store))

	if srv.store != store {
		t.Fatal("expected the store replaced")
	}
}

func TestWithEngineRunnerAndLogTo(t *testing.T) {
	var buf bytes.Buffer
	wantErr := errors.New("engine down")
	srv := newTestServer(t,
		withEngineRunner(func(engine.RunRequest) ([]engine.LogEntry, error) { return nil, wantErr }),
		withLogTo(&buf, "debug"))

	if _, err := srv.engineRunner(engine.RunRequest{}); !errors.Is(err, wantErr) {
		t.Fatalf("engineRunner err = %v, want %v", err, wantErr)
	}
	srv.logger.Info("hello from the test")
	if !strings.Contains(buf.String(), "hello from the test") {
		t.Fatalf("expected the log line captured, got %q", buf.String())
	}
}

func TestWithConfigSourcesIsApplied(t *testing.T) {
	srv := newTestServer(t, withConfigSources(map[string]SettingSource{
		"worker_count": SourceConfigFile,
	}))

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil)
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(resp.Body.String(), string(SourceConfigFile)) {
		t.Fatalf("expected the declared source reported, got %s", resp.Body.String())
	}
}
