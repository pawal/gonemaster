package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newMCPServer builds the server and registers tools. Write tools register only
// when allowWrite is set. SDK use is confined to this file so a later SDK swap
// stays contained.
func newMCPServer(api *apiClient, allowWrite bool) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version}, nil)
	registerPing(srv, api)
	registerReadTools(srv, api)
	registerSpecTools(srv, api)
	registerHistoryTools(srv, api)
	registerBatchTools(srv, api)
	if allowWrite {
		registerWriteTools(srv, api)
	}
	return srv
}

type pingInput struct{}

type pingOutput struct {
	ServerURL     string `json:"server_url" jsonschema:"the gonemaster-server admin API URL this bridge targets"`
	Reachable     bool   `json:"reachable" jsonschema:"whether the gonemaster-server responded"`
	AuthMode      string `json:"auth_mode,omitempty" jsonschema:"server auth mode: open or token"`
	Authenticated bool   `json:"authenticated" jsonschema:"whether this bridge's credential is accepted (always true in open mode)"`
	Detail        string `json:"detail,omitempty" jsonschema:"human-readable status or error detail"`
}

// registerPing adds a connectivity + auth smoke-test tool.
func registerPing(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ping",
		Description: "Check connectivity to gonemaster-server and report its auth mode and whether this bridge is authenticated.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ pingInput) (*mcp.CallToolResult, pingOutput, error) {
		out := pingOutput{ServerURL: api.baseURL}
		who, err := api.whoami(ctx)
		if err != nil {
			out.Detail = "gonemaster-server unreachable: " + err.Error()
			return nil, out, nil
		}
		out.Reachable = true
		out.AuthMode = who.Mode
		out.Authenticated = who.Authenticated
		if who.Mode == "token" && !who.Authenticated {
			out.Detail = "server is in token mode but no valid GONEMASTER_TOKEN was provided"
		} else {
			out.Detail = "ok"
		}
		return nil, out, nil
	})
}
