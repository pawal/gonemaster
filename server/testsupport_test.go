package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

// reqOpt customizes a request before it is served.
type reqOpt func(*http.Request)

// withHeader sets a request header.
func withHeader(key string, value string) reqOpt {
	return func(req *http.Request) { req.Header.Set(key, value) }
}

// withOrigin sets the Origin header, for the CSRF checks.
func withOrigin(origin string) reqOpt {
	return withHeader("Origin", origin)
}

// withHost sets the request Host, which is a field rather than a header.
func withHost(host string) reqOpt {
	return func(req *http.Request) { req.Host = host }
}

// withRemoteAddr sets the peer address the rate limiter and access log read.
func withRemoteAddr(addr string) reqOpt {
	return func(req *http.Request) { req.RemoteAddr = addr }
}

// withBearer sets an admin bearer token.
func withBearer(token string) reqOpt {
	return withHeader("Authorization", "Bearer "+token)
}

// withCookie adds a cookie.
func withCookie(c *http.Cookie) reqOpt {
	return func(req *http.Request) { req.AddCookie(c) }
}

// noContentType drops the Content-Type doJSON sets for a request with a body.
func noContentType() reqOpt {
	return func(req *http.Request) { req.Header.Del("Content-Type") }
}

// withRequest is the escape hatch for a one-off request tweak.
func withRequest(fn func(*http.Request)) reqOpt {
	return fn
}

// doJSON serves method and path against srv's root handler and returns the
// recorder. See requestBody for how body is encoded.
func doJSON(t testing.TB, srv *Server, method string, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	t.Helper()
	return doHandler(t, srv.Handler(), method, path, body, opts...)
}

// doHandler is doJSON against a handler built by the test, for the middleware
// and router tests that deliberately bypass srv.Handler().
func doHandler(t testing.TB, h http.Handler, method string, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, requestBody(t, body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, opt := range opts {
		opt(req)
	}
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	return resp
}

// requestBody encodes a doJSON body: nil sends no body, a string or []byte is
// sent verbatim, an io.Reader is passed through, anything else is marshalled.
// An empty string still sends a body, which some handlers distinguish.
func requestBody(t testing.TB, body any) io.Reader {
	t.Helper()
	switch v := body.(type) {
	case nil:
		return nil
	case string:
		return strings.NewReader(v)
	case []byte:
		return bytes.NewReader(v)
	case io.Reader:
		return v
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		return bytes.NewReader(raw)
	}
}

// mustJSON checks the response status and decodes the body into T.
func mustJSON[T any](t testing.TB, resp *httptest.ResponseRecorder, wantStatus int) T {
	t.Helper()
	raw := resp.Body.Bytes()
	if resp.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", resp.Code, wantStatus, raw)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response: %v: %s", err, raw)
	}
	return out
}

// wantStatus checks the response status, for the calls with no body to decode.
func wantStatus(t testing.TB, resp *httptest.ResponseRecorder, want int) {
	t.Helper()
	if resp.Code != want {
		t.Fatalf("status = %d, want %d: %s", resp.Code, want, resp.Body.String())
	}
}

// wantErrorCode checks the status and the error code writeError produced, and
// returns the body for the tests that also assert on the message or details.
func wantErrorCode(t testing.TB, resp *httptest.ResponseRecorder, status int, code string) ErrorResponse {
	t.Helper()
	body := mustJSON[ErrorResponse](t, resp, status)
	if body.Error.Code != code {
		t.Errorf("error code = %q, want %q", body.Error.Code, code)
	}
	return body
}

// srvOpt customizes newTestServer.
type srvOpt func(*testServerSetup)

// testServerSetup collects what newTestServer applies, in order: config
// mutations, then construction, then the post-construction field swaps.
type testServerSetup struct {
	cfg   []func(*Config)
	db    JobStore
	queue Queue
	post  []func(*Server)
}

// withConfig mutates the config before construction. The named options below
// cover the recurring cases; this is the escape hatch for one-off fields.
func withConfig(fn func(*Config)) srvOpt {
	return func(s *testServerSetup) { s.cfg = append(s.cfg, fn) }
}

// withDB constructs the server around store instead of a fresh in-memory one,
// so the scoring and analysis config land in the store the test reads.
func withDB(store JobStore) srvOpt {
	return func(s *testServerSetup) { s.db = store }
}

// withQueue constructs the server around queue.
func withQueue(q Queue) srvOpt {
	return func(s *testServerSetup) { s.queue = q }
}

// withStore replaces the store after construction, which is what the tests
// that inject a spy or an error-returning store want.
func withStore(store JobStore) srvOpt {
	return func(s *testServerSetup) {
		s.post = append(s.post, func(srv *Server) { srv.store = store })
	}
}

// withPublicAPI mutates the public API config.
func withPublicAPI(fn func(*PublicAPIConfig)) srvOpt {
	return withConfig(func(cfg *Config) { fn(&cfg.PublicAPI) })
}

// withWorkers sets the worker count and the concurrent-job cap.
func withWorkers(count int, maxConcurrent int) srvOpt {
	return withConfig(func(cfg *Config) {
		cfg.WorkerCount = count
		cfg.MaxConcurrentJobs = maxConcurrent
	})
}

// withAuth switches the admin API from open mode to token mode.
func withAuth(tokens ...string) srvOpt {
	return withConfig(func(cfg *Config) {
		cfg.Auth.AdminTokens = nil
		for i, tok := range tokens {
			cfg.Auth.AdminTokens = append(cfg.Auth.AdminTokens,
				AdminToken{Label: fmt.Sprintf("t%d", i), Hash: hashToken(tok)})
		}
	})
}

// withEngineRunner replaces the engine call, so a test decides what a run
// reports without touching the network.
func withEngineRunner(fn func(engine.RunRequest) ([]engine.LogEntry, error)) srvOpt {
	return func(s *testServerSetup) {
		s.post = append(s.post, func(srv *Server) { srv.engineRunner = fn })
	}
}

// withLogTo sends the server log to w as JSON, for the tests that assert on
// log lines.
func withLogTo(w io.Writer, level string) srvOpt {
	return func(s *testServerSetup) {
		s.post = append(s.post, func(srv *Server) { srv.logger = newLogger("json", level, w) })
	}
}

// withAnalysisController installs an analysis controller.
func withAnalysisController(ctrl AnalysisController) srvOpt {
	return func(s *testServerSetup) {
		s.post = append(s.post, func(srv *Server) { srv.SetAnalysisController(ctrl) })
	}
}

// withConfigSources declares where each setting came from.
func withConfigSources(sources map[string]SettingSource) srvOpt {
	return func(s *testServerSetup) {
		s.post = append(s.post, func(srv *Server) { srv.SetConfigSources(sources) })
	}
}

// newTestServer builds a server from DefaultConfig plus the options. With no
// options it is New(DefaultConfig()).
func newTestServer(t testing.TB, opts ...srvOpt) *Server {
	t.Helper()
	setup := &testServerSetup{}
	for _, opt := range opts {
		opt(setup)
	}
	cfg := DefaultConfig()
	for _, mutate := range setup.cfg {
		mutate(&cfg)
	}
	var srv *Server
	if setup.db != nil || setup.queue != nil {
		queue := setup.queue
		if queue == nil {
			queue = NewInMemoryQueue()
		}
		store := setup.db
		if store == nil {
			store = NewInMemoryJobStore()
		}
		srv = newServer(cfg, store, queue)
	} else {
		srv = New(cfg)
	}
	for _, apply := range setup.post {
		apply(srv)
	}
	return srv
}
