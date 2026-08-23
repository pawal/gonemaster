package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/cmd/internal/mcptest"
	"codeberg.org/pawal/gonemaster/internal/apitest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerInstructions(t *testing.T) {
	ro := serverInstructions(false)
	if ro == "" {
		t.Fatal("expected non-empty instructions")
	}
	for _, want := range []string{"test_domain", "run_diff", "cohort_tag_values", "spec_get_testcase"} {
		if !strings.Contains(ro, want) {
			t.Errorf("read-only instructions missing %q", want)
		}
	}
	if strings.Contains(ro, "batch_enqueue") {
		t.Error("read-only instructions must not mention the write tool batch_enqueue")
	}

	rw := serverInstructions(true)
	if !strings.Contains(rw, "batch_enqueue") || !strings.Contains(rw, "batch_cancel") {
		t.Error("write-enabled instructions should describe the write tools")
	}
}

// TestServerInstructionsReachClient proves the instructions are delivered to a
// connected client in the initialize result, not just stored server-side.
func TestServerInstructionsReachClient(t *testing.T) {
	srv := newMCPServer(clientFor(t, "http://localhost:0", ""), false)
	mcptest.Session(t, srv, func(_ context.Context, session *mcp.ClientSession) {
		got := session.InitializeResult().Instructions
		if got != serverInstructions(false) {
			t.Errorf("client instructions = %q, want server instructions", got)
		}
	})
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", defaultServerURL},
		{"http://localhost:8080", "http://localhost:8080/api/v1"},
		{"http://localhost:8080/", "http://localhost:8080/api/v1"},
		{"http://localhost:8080/api/v1", "http://localhost:8080/api/v1"},
		{"http://localhost:8080/api/v1/", "http://localhost:8080/api/v1"},
		{"localhost:9000", "http://localhost:9000/api/v1"},
		{"https://gm.example.com/api/v1/", "https://gm.example.com/api/v1"},
		{"http://host/prefix", "http://host/prefix/api/v1"},
	}
	for _, c := range cases {
		got, err := normalizeBaseURL(c.in)
		if err != nil {
			t.Fatalf("normalizeBaseURL(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// callPing wires the MCP server to an in-memory client and invokes the ping
// tool, exercising the full tools/call round-trip plus the HTTP client.
func callPing(t *testing.T, api *apiClient) pingOutput {
	t.Helper()
	var out pingOutput
	mcptest.Session(t, newMCPServer(api, false), func(ctx context.Context, session *mcp.ClientSession) {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
		if err != nil {
			t.Fatalf("call ping: %v", err)
		}
		if res.IsError {
			t.Fatalf("ping returned a tool error: %+v", res.Content)
		}
		if res.StructuredContent == nil {
			t.Fatalf("ping returned no structured content")
		}
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatalf("marshal structured content: %v", err)
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode ping output: %v", err)
		}
	})
	return out
}

func clientFor(t *testing.T, serverURL, token string) *apiClient {
	t.Helper()
	api, err := newAPIClient(config{serverURL: serverURL, token: token, timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("newAPIClient: %v", err)
	}
	return api
}

// fakeAPI starts a fake gonemaster-server for the test and returns a client
// pointed at it, with no bearer token.
func fakeAPI(t *testing.T, opts apitest.Opts) *apiClient {
	t.Helper()
	return clientFor(t, apitest.New(t, opts).URL, "")
}

// tokenMode is the fake in token mode: it answers whoami as such and rejects
// every other route without the matching Bearer token.
func tokenMode(token string) apitest.Opts {
	return apitest.Opts{WhoamiMode: "token", RequireToken: token}
}

func TestPingOpenMode(t *testing.T) {
	out := callPing(t, fakeAPI(t, apitest.Opts{}))
	if !out.Reachable {
		t.Fatalf("expected reachable, got %+v", out)
	}
	if out.AuthMode != "open" || !out.Authenticated {
		t.Errorf("expected open+authenticated, got mode=%q authenticated=%v", out.AuthMode, out.Authenticated)
	}
	if out.Detail != "ok" {
		t.Errorf("expected detail ok, got %q", out.Detail)
	}
}

func TestPingTokenModeAuthenticated(t *testing.T) {
	srv := apitest.New(t, tokenMode("gm_secret"))

	out := callPing(t, clientFor(t, srv.URL, "gm_secret"))
	if !out.Reachable || out.AuthMode != "token" || !out.Authenticated {
		t.Fatalf("expected reachable token+authenticated, got %+v", out)
	}
	if out.Detail != "ok" {
		t.Errorf("expected detail ok, got %q", out.Detail)
	}
}

func TestPingTokenModeMissingToken(t *testing.T) {
	// No token configured on the bridge: the server is in token mode, so it
	// reports authenticated=false and ping should explain the missing token.
	out := callPing(t, fakeAPI(t, tokenMode("gm_secret")))
	if !out.Reachable || out.AuthMode != "token" {
		t.Fatalf("expected reachable token mode, got %+v", out)
	}
	if out.Authenticated {
		t.Errorf("expected not authenticated without a token")
	}
	if out.Detail == "ok" || out.Detail == "" {
		t.Errorf("expected a missing-token detail, got %q", out.Detail)
	}
}

func TestPingUnreachable(t *testing.T) {
	// Start a server, capture its URL, then close it so the address refuses
	// connections deterministically.
	srv := apitest.New(t, apitest.Opts{})
	url := srv.URL
	srv.Close()

	out := callPing(t, clientFor(t, url, ""))
	if out.Reachable {
		t.Fatalf("expected unreachable, got %+v", out)
	}
	if out.Detail == "" {
		t.Errorf("expected an error detail when unreachable")
	}
}
