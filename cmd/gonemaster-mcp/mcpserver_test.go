package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

// fakeWhoami stands in for gonemaster-server. With an empty expectToken it
// behaves as an open-mode server; otherwise it is in token mode and reports
// authenticated only when the request carries the matching Bearer token.
func fakeWhoami(t *testing.T, expectToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/whoami" {
			http.NotFound(w, r)
			return
		}
		resp := whoamiResponse{}
		if expectToken == "" {
			resp.Mode = "open"
			resp.Authenticated = true
		} else {
			resp.Mode = "token"
			resp.Authenticated = r.Header.Get("Authorization") == "Bearer "+expectToken
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// callPing wires the MCP server to an in-memory client and invokes the ping
// tool, exercising the full tools/call round-trip plus the HTTP client.
func callPing(t *testing.T, api *apiClient) pingOutput {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientT, serverT := mcp.NewInMemoryTransports()
	srv := newMCPServer(api, false)
	go func() { _ = srv.Run(ctx, serverT) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

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
	var out pingOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode ping output: %v", err)
	}
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

func TestPingOpenMode(t *testing.T) {
	ts := fakeWhoami(t, "")
	defer ts.Close()

	out := callPing(t, clientFor(t, ts.URL, ""))
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
	ts := fakeWhoami(t, "gm_secret")
	defer ts.Close()

	out := callPing(t, clientFor(t, ts.URL, "gm_secret"))
	if !out.Reachable || out.AuthMode != "token" || !out.Authenticated {
		t.Fatalf("expected reachable token+authenticated, got %+v", out)
	}
	if out.Detail != "ok" {
		t.Errorf("expected detail ok, got %q", out.Detail)
	}
}

func TestPingTokenModeMissingToken(t *testing.T) {
	ts := fakeWhoami(t, "gm_secret")
	defer ts.Close()

	// No token configured on the bridge: the server is in token mode, so it
	// reports authenticated=false and ping should explain the missing token.
	out := callPing(t, clientFor(t, ts.URL, ""))
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
	ts := fakeWhoami(t, "")
	url := ts.URL
	ts.Close()

	out := callPing(t, clientFor(t, url, ""))
	if out.Reachable {
		t.Fatalf("expected unreachable, got %+v", out)
	}
	if out.Detail == "" {
		t.Errorf("expected an error detail when unreachable")
	}
}
