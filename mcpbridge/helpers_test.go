package mcpbridge

import "github.com/modelcontextprotocol/go-sdk/mcp"

// newMCPServer is NewServer with only the write gate, as the tests call it.
func newMCPServer(api *Client, allowWrite bool) *mcp.Server {
	return NewServer(api, Options{AllowWrite: allowWrite})
}
