package mcpbridge

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newMCPServer is NewServer with only the write gate, as the tests call it.
func newMCPServer(api *Client, allowWrite bool) *mcp.Server {
	return NewServer(api, Options{AllowWrite: allowWrite})
}

// stub sets *target to value for the test and restores it in Cleanup.
func stub[T any](t testing.TB, target *T, value T) {
	t.Helper()
	orig := *target
	*target = value
	t.Cleanup(func() { *target = orig })
}
