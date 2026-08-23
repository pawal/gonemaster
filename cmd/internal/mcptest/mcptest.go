package mcptest

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Session runs srv over in-memory transports and hands the connected client
// session to fn. The server goroutine is joined before Session returns, so a
// test that restores overridden package vars cannot race it.
func Session(t testing.TB, srv *mcp.Server, fn func(ctx context.Context, session *mcp.ClientSession)) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	defer func() { cancel(); <-done }()

	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT); close(done) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("mcp client connect: %v", err)
	}
	defer session.Close()

	fn(ctx, session)
}
