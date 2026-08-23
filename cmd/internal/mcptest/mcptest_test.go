package mcptest_test

import (
	"context"
	"sync/atomic"
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/mcptest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoIn struct {
	Text string `json:"text"`
}

type echoOut struct {
	Text string `json:"text"`
}

// echoServer registers one tool that echoes its input, and reports when the
// tool has been called.
func echoServer(called *atomic.Bool) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "echo", Version: "test"},
		&mcp.ServerOptions{Instructions: "echo server"})
	mcp.AddTool(srv, &mcp.Tool{Name: "echo", Description: "echo the input"},
		func(_ context.Context, _ *mcp.CallToolRequest, in echoIn) (*mcp.CallToolResult, echoOut, error) {
			called.Store(true)
			return nil, echoOut{Text: in.Text}, nil
		})
	return srv
}

func TestSessionCallsTheServer(t *testing.T) {
	var called atomic.Bool
	var got string

	mcptest.Session(t, echoServer(&called), func(ctx context.Context, s *mcp.ClientSession) {
		res, err := s.CallTool(ctx, &mcp.CallToolParams{
			Name:      "echo",
			Arguments: map[string]any{"text": "hello"},
		})
		if err != nil {
			t.Fatalf("call echo: %v", err)
		}
		if res.IsError {
			t.Fatalf("echo returned a tool error: %+v", res.Content)
		}
		got = res.StructuredContent.(map[string]any)["text"].(string)
	})

	if !called.Load() {
		t.Fatal("expected the tool handler to run")
	}
	if got != "hello" {
		t.Fatalf("echo returned %q, want hello", got)
	}
}

func TestSessionDeliversTheServerInstructions(t *testing.T) {
	var called atomic.Bool
	mcptest.Session(t, echoServer(&called), func(_ context.Context, s *mcp.ClientSession) {
		if got := s.InitializeResult().Instructions; got != "echo server" {
			t.Fatalf("instructions = %q, want the server's", got)
		}
	})
}

// TestSessionJoinsTheServerGoroutine is the reason the helper exists: the
// four hand-written copies it replaced included two that returned while the
// server goroutine was still running, so a test restoring an overridden
// package var raced it.
func TestSessionJoinsTheServerGoroutine(t *testing.T) {
	var called atomic.Bool
	var running atomic.Bool

	srv := mcp.NewServer(&mcp.Implementation{Name: "slow", Version: "test"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "echo"},
		func(_ context.Context, _ *mcp.CallToolRequest, in echoIn) (*mcp.CallToolResult, echoOut, error) {
			return nil, echoOut{Text: in.Text}, nil
		})

	running.Store(true)
	mcptest.Session(t, srv, func(ctx context.Context, s *mcp.ClientSession) {
		if _, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "x"}}); err != nil {
			t.Fatalf("call echo: %v", err)
		}
		called.Store(true)
	})

	// Session only returns once srv.Run has returned; a write here is what a
	// caller's deferred restore does, and it must not race the server.
	running.Store(false)
	if !called.Load() {
		t.Fatal("expected the callback to have run")
	}
}
