package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bearerTransport adds one bearer token to every request.
type bearerTransport struct {
	token string
	next  http.RoundTripper
}

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.token != "" {
		r.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.next.RoundTrip(r)
}

// mcpSession connects an SDK client to the server's MCP endpoint over HTTP.
func mcpSession(t *testing.T, srv *Server, token string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	hc := &http.Client{Transport: bearerTransport{token: token, next: http.DefaultTransport}}
	tr := &mcp.StreamableClientTransport{Endpoint: ts.URL + "/api/v1/mcp", HTTPClient: hc}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	ctx := t.Context()
	session, err := client.Connect(ctx, tr, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}

func mcpToolNames(t *testing.T, session *mcp.ClientSession, ctx context.Context) map[string]bool {
	t.Helper()
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range res.Tools {
		names[tl.Name] = true
	}
	return names
}

func withMCP(allowWrite bool) srvOpt {
	return withConfig(func(cfg *Config) {
		cfg.MCPEnabled = true
		cfg.MCPAllowWrite = allowWrite
	})
}

// structuredContent decodes a tool result's structured content into T.
func structuredContent[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	return out
}

func TestMCPEndpointDisabledIs404(t *testing.T) {
	srv := newTestServer(t)
	wantStatus(t, doJSON(t, srv, http.MethodPost, "/api/v1/mcp", `{}`), http.StatusNotFound)
}

func TestMCPEndpointRequiresBearerInTokenMode(t *testing.T) {
	srv := newTestServer(t, withAuth("gm_secret"), withMCP(false))
	wantStatus(t, doJSON(t, srv, http.MethodPost, "/api/v1/mcp", `{}`), http.StatusUnauthorized)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/mcp", `{}`, withCookie(&http.Cookie{Name: adminCookieName, Value: "gm_secret"}))
	if resp.Code != http.StatusUnauthorized || !strings.Contains(resp.Body.String(), "bearer token required") {
		t.Fatalf("cookie: status = %d body = %s, want 401 bearer token required", resp.Code, resp.Body.String())
	}
}

func TestMCPEndpointListsReadToolsAndPings(t *testing.T) {
	srv := newTestServer(t, withAuth("gm_secret"), withMCP(false))
	session, ctx := mcpSession(t, srv, "gm_secret")

	names := mcpToolNames(t, session, ctx)
	if len(names) != 18 || !names["test_domain"] || !names["cohort_schedule_list"] || names["batch_enqueue"] {
		t.Fatalf("tools = %v, want 18 read tools", names)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("ping: %v %+v", err, res)
	}
	out := structuredContent[struct {
		Reachable     bool   `json:"reachable"`
		AuthMode      string `json:"auth_mode"`
		Authenticated bool   `json:"authenticated"`
	}](t, res)
	if !out.Reachable || out.AuthMode != "token" || !out.Authenticated {
		t.Fatalf("ping = %+v, want reachable token authenticated", out)
	}
}

func TestMCPEndpointWriteToolsFollowTheSetting(t *testing.T) {
	srv := newTestServer(t, withMCP(true))
	session, ctx := mcpSession(t, srv, "")
	names := mcpToolNames(t, session, ctx)
	if len(names) != 21 || !names["batch_enqueue"] || !names["batch_cancel"] || !names["cancel_job"] {
		t.Fatalf("tools = %v, want 21 with the write tools", names)
	}
}

func TestMCPEndpointLatestForReadsTheStore(t *testing.T) {
	srv := newTestServer(t, withMCP(false))
	makeGraduatedJob(t, srv, "example.se", JobSucceeded)
	makeGraduatedJob(t, srv, "myexample.se", JobSucceeded)
	session, ctx := mcpSession(t, srv, "")

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "latest_for", Arguments: map[string]any{"domain": "example.se"}})
	if err != nil || res.IsError {
		t.Fatalf("latest_for: %v %+v", err, res)
	}
	out := structuredContent[struct {
		Count int `json:"count"`
		Runs  []struct {
			Domain string `json:"domain"`
		} `json:"runs"`
	}](t, res)
	if out.Count != 1 || out.Runs[0].Domain != "example.se" {
		t.Fatalf("latest_for = %+v, want the one exact run", out)
	}
}

// lockedBuffer is a log sink the server goroutine and the test may share.
type lockedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	await string
	once  sync.Once
	seen  chan struct{}
}

// awaitingBuffer is a lockedBuffer that closes seen on the first Write containing substr.
func awaitingBuffer(substr string) *lockedBuffer {
	return &lockedBuffer{await: substr, seen: make(chan struct{})}
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seen != nil && bytes.Contains(p, []byte(b.await)) {
		b.once.Do(func() { close(b.seen) })
	}
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestMCPEndpointSharesTheRequestID(t *testing.T) {
	logs := awaitingBuffer(`"path":"/api/v1/mcp"`)
	srv := newTestServer(t, withMCP(false), withLogTo(logs, "info"))
	session, ctx := mcpSession(t, srv, "")
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "latest_for", Arguments: map[string]any{"domain": "example.se"}}); err != nil {
		t.Fatalf("latest_for: %v", err)
	}
	// The access log line lands after the response; the httptest server runs on the real clock.
	select {
	case <-logs.seen:
	case <-time.After(2 * time.Second):
		t.Fatal("no access log line for /api/v1/mcp within 2s")
	}

	ids := map[string]string{}
	for _, line := range strings.Split(logs.String(), "\n") {
		var rec struct {
			Path      string `json:"path"`
			RequestID string `json:"request_id"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil || rec.RequestID == "" {
			continue
		}
		if rec.Path == "/api/v1/mcp" || rec.Path == "/api/v1/runs" {
			ids[rec.Path] = rec.RequestID
		}
	}
	if ids["/api/v1/mcp"] == "" || ids["/api/v1/runs"] == "" {
		t.Fatalf("expected access log lines for the endpoint and the inner call, got %v", ids)
	}
	if ids["/api/v1/mcp"] != ids["/api/v1/runs"] {
		t.Fatalf("request ids differ: %v", ids)
	}
}

func TestMCPToolCallsReachMetrics(t *testing.T) {
	srv := newTestServer(t, withMCP(false))
	session, ctx := mcpSession(t, srv, "")
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping", Arguments: map[string]any{}}); err != nil {
		t.Fatalf("ping: %v", err)
	}
	calls := srv.metrics.Snapshot().API.MCPToolCalls
	if len(calls) != 1 || calls[0].Tool != "ping" || calls[0].Total != 1 || calls[0].Failed != 0 {
		t.Fatalf("mcp tool calls = %+v", calls)
	}
}

func TestSettingsMCPRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	settings := mustJSON[map[string]settingEntry](t, doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil), http.StatusOK)
	if v, ok := settings["mcp_enabled"].Value.(bool); !ok || v {
		t.Fatalf("mcp_enabled = %#v, want false", settings["mcp_enabled"].Value)
	}
	wantStatus(t, doJSON(t, srv, http.MethodPut, "/api/v1/settings", `{"mcp_enabled": true, "mcp_allow_write": true}`), http.StatusOK)
	if !srv.cfg.MCPEnabled || !srv.cfg.MCPAllowWrite {
		t.Fatalf("cfg = %v/%v after PUT, want true/true", srv.cfg.MCPEnabled, srv.cfg.MCPAllowWrite)
	}
	if resp := doJSON(t, srv, http.MethodPost, "/api/v1/mcp", `{}`); resp.Code == http.StatusNotFound {
		t.Fatal("endpoint still 404 after enabling through settings")
	}
}

func TestMCPEndpointRunsTestDomain(t *testing.T) {
	entries := []engine.LogEntry{
		{Module: "Nameserver", Testcase: "Nameserver01", Tag: "NS01", Level: "INFO"},
		{Module: "Consistency", Testcase: "Consistency01", Tag: "SOATIME", Level: "WARNING"},
	}
	srv := newTestServer(t, withMCP(false), withWorkers(1, 1),
		withEngineRunner(func(engine.RunRequest) ([]engine.LogEntry, error) { return entries, nil }))
	srv.Start()
	defer srv.Stop(t.Context())
	session, ctx := mcpSession(t, srv, "")

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "test_domain", Arguments: map[string]any{"domain": "example.com", "min_level": "WARNING"}})
	if err != nil || res.IsError {
		t.Fatalf("test_domain: %v %+v", err, res)
	}
	out := structuredContent[struct {
		Status      string                 `json:"status"`
		RunID       string                 `json:"run_id"`
		Findings    []struct{ Tag string } `json:"findings"`
		LevelCounts map[string]int         `json:"level_counts"`
	}](t, res)
	if out.Status != "succeeded" || out.RunID == "" {
		t.Fatalf("result = %+v, want a succeeded run", out)
	}
	if len(out.Findings) != 1 || out.Findings[0].Tag != "SOATIME" || out.LevelCounts["INFO"] != 1 {
		t.Fatalf("findings = %+v level_counts = %v", out.Findings, out.LevelCounts)
	}
}
