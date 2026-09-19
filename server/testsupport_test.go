package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// sameOrigin sets Origin from the request's own Host, so a test asserts the
// CSRF check passes rather than pinning httptest's default host.
func sameOrigin() reqOpt {
	return func(req *http.Request) { req.Header.Set("Origin", "http://"+req.Host) }
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

// csrfOriginMatrix runs the three Origin cases every state-changing endpoint
// must satisfy: an absent Origin passes (CLI clients send none), the server's
// own origin passes, and a cross origin is refused. call issues one request
// with the opts applied on top of whatever the endpoint itself needs.
func csrfOriginMatrix(t *testing.T, okStatus int, call func(t *testing.T, opts ...reqOpt) *httptest.ResponseRecorder) {
	t.Helper()
	for _, tc := range []struct {
		name string
		opts []reqOpt
		want int
	}{
		{name: "no origin", want: okStatus},
		{name: "same origin", opts: []reqOpt{sameOrigin()}, want: okStatus},
		{name: "cross origin", opts: []reqOpt{withOrigin("https://evil.example")}, want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := call(t, tc.opts...)
			if tc.want == http.StatusForbidden {
				wantErrorCode(t, resp, http.StatusForbidden, "csrf_origin_mismatch")
				return
			}
			wantStatus(t, resp, tc.want)
		})
	}
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

// withLookupResolvers points delegation lookups at a test DNS server.
func withLookupResolvers(res lookupResolvers) srvOpt {
	return func(s *testServerSetup) {
		s.post = append(s.post, func(srv *Server) { srv.lookup = res })
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
	// Closed port by default, so no test reaches a real resolver.
	srv.lookup = lookupResolvers{servers: []string{"127.0.0.1:1"}, host: blackholeResolver}
	for _, apply := range setup.post {
		apply(srv)
	}
	return srv
}

// blackholeResolver refuses instead of querying the system resolver.
var blackholeResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", "127.0.0.1:1")
	},
}

// fakeJobStore wraps a store so a test can observe or fail individual calls.
// The embedded JobStore supplies the 50-odd methods no test cares about.
type fakeJobStore struct {
	JobStore
	// updates counts Update calls without locking, so the contention
	// benchmark measures the server rather than this wrapper.
	updates atomic.Int64

	mu             sync.Mutex
	recordProgress bool
	progresses     []int

	// The hooks below make one call fail, standing in for a store that will
	// not commit. Set them before the call under test runs.
	updateHook  func(job Job) error
	createErr   error
	graduateErr error
	chainErr    error
}

// newFakeJobStore wraps a fresh in-memory store.
func newFakeJobStore() *fakeJobStore {
	return &fakeJobStore{JobStore: NewInMemoryJobStore()}
}

// newProgressSpy is newFakeJobStore with Update recording every progress value
// it is handed, for the tests that assert which writes reached the store.
func newProgressSpy() *fakeJobStore {
	s := newFakeJobStore()
	s.recordProgress = true
	return s
}

// wrapStore replaces srv's store with a fake around it, keeping whatever the
// server already seeded, and returns the fake.
func wrapStore(t testing.TB, srv *Server) *fakeJobStore {
	t.Helper()
	fake := &fakeJobStore{JobStore: srv.store}
	srv.store = fake
	return fake
}

func (s *fakeJobStore) Create(job Job) (Job, error) {
	if s.createErr != nil {
		return Job{}, s.createErr
	}
	return s.JobStore.Create(job)
}

func (s *fakeJobStore) Update(job Job) error {
	s.updates.Add(1)
	// The progress is recorded before the hook runs, so a test can see the
	// write that was attempted even when it fails.
	s.mu.Lock()
	if s.recordProgress {
		s.progresses = append(s.progresses, job.Progress)
	}
	hook := s.updateHook
	s.mu.Unlock()
	if hook != nil {
		if err := hook(job); err != nil {
			return err
		}
	}
	return s.JobStore.Update(job)
}

func (s *fakeJobStore) GraduateJob(job Job, entries []engine.LogEntry) error {
	s.mu.Lock()
	failure := s.graduateErr
	s.mu.Unlock()
	if failure != nil {
		return failure
	}
	return s.JobStore.GraduateJob(job, entries)
}

func (s *fakeJobStore) GetRunDNSSECChain(runID string) (string, bool, error) {
	if s.chainErr != nil {
		return "", false, s.chainErr
	}
	return s.JobStore.GetRunDNSSECChain(runID)
}

// Progresses returns the progress values Update was handed, in order.
func (s *fakeJobStore) Progresses() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.progresses)
}

// UpdateCount returns how many times Update was called.
func (s *fakeJobStore) UpdateCount() int64 {
	return s.updates.Load()
}

// runSpec describes one graduated run for seedGraduatedRun. Every field is
// optional: the zero value is a succeeded run for example.com, timestamped now,
// under a generated job id.
type runSpec struct {
	ID      string
	Domain  string
	BatchID string
	Status  JobStatus
	// At is the finish time; zero means now. Duration backdates the created
	// and started times relative to it, for the tests that read elapsed time.
	At       time.Time
	Duration time.Duration
	Entries  []engine.LogEntry
	Timings  []NameserverTiming
	Origin   string
	Progress int
	// ChainJSON is stored on the run, which only graduation can do because
	// Create assigns the public id the chain is looked up by.
	ChainJSON string
	// ResolveDomain creates the domain up front and sets DomainID, which the
	// batch and analysis handlers expect on a run.
	ResolveDomain bool
}

// graduate graduates a job the caller built, filling in the status and finish
// time it left unset. It returns the job as stored, so the caller does not hold
// a copy that disagrees with the store.
func graduate(t testing.TB, store JobStore, job Job, entries []engine.LogEntry) Job {
	t.Helper()
	if job.Status == "" || job.Status == JobQueued {
		job.Status = JobSucceeded
	}
	if job.FinishedAt.IsZero() {
		if job.CreatedAt.IsZero() {
			job.FinishedAt = time.Now().UTC()
		} else {
			job.FinishedAt = job.CreatedAt.Add(time.Second)
		}
	}
	if err := store.GraduateJob(job, entries); err != nil {
		t.Fatalf("graduate job %q: %v", job.ID, err)
	}
	return job
}

// forEachStore runs fn as a subtest against the in-memory store and every SQL
// backend, for the contract tests that must hold for all of them.
func forEachStore(t *testing.T, fn func(t *testing.T, s JobStore)) {
	t.Helper()
	t.Run("inmemory", func(t *testing.T) {
		fn(t, NewInMemoryJobStore())
	})
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			fn(t, testStoreForBackend(t, b))
		})
	}
}

// createAndGraduate creates and graduates a job the caller built, for the store
// tests that set fields runSpec deliberately does not model (priority, profile
// snapshot, an explicit domain id).
func createAndGraduate(t testing.TB, store JobStore, job Job, entries []engine.LogEntry) Job {
	t.Helper()
	created, err := store.Create(job)
	if err != nil {
		t.Fatalf("create job %q: %v", job.ID, err)
	}
	return graduate(t, store, created, entries)
}

// seedGraduatedRun creates and graduates one run, returning the stored job so
// callers can read the public id and domain id Create assigned.
func seedGraduatedRun(t testing.TB, store JobStore, spec runSpec) Job {
	t.Helper()
	at := spec.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	domain := spec.Domain
	if domain == "" {
		domain = "example.com"
	}
	id := spec.ID
	if id == "" {
		id = newID("job")
	}
	job := Job{
		ID:                id,
		Domain:            domain,
		BatchID:           spec.BatchID,
		Status:            spec.Status,
		CreatedAt:         at.Add(-spec.Duration),
		StartedAt:         at.Add(-spec.Duration),
		FinishedAt:        at,
		Progress:          spec.Progress,
		Origin:            spec.Origin,
		NameserverTimings: spec.Timings,
	}
	if spec.ResolveDomain {
		d, err := store.GetOrCreateDomain(domain)
		if err != nil {
			t.Fatalf("get or create domain %q: %v", domain, err)
		}
		job.DomainID = d.ID
	}
	if spec.ChainJSON != "" {
		// The chain is set between Create and graduation, because Create is
		// what assigns the public id the chain is looked up by.
		created, err := store.Create(job)
		if err != nil {
			t.Fatalf("create job %q: %v", job.ID, err)
		}
		created.DNSSECChainJSON = spec.ChainJSON
		return graduate(t, store, created, spec.Entries)
	}
	return createAndGraduate(t, store, job, spec.Entries)
}

// systemStartEntry is the one log entry the fixtures graduate a run with when
// the test does not care about findings.
func systemStartEntry() []engine.LogEntry {
	return []engine.LogEntry{{Timestamp: 1.0, Module: "System", Tag: "MODULE_START", Level: "INFO"}}
}

// writeTempJSON writes raw to a temp file and returns its path, for the tests
// that load a config from disk.
func writeTempJSON(t testing.TB, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write temp json: %v", err)
	}
	return path
}
