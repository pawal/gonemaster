# Connecting MCP Clients

Every client needs one of two things: the path to the `gonemaster-mcp`
binary with its environment, or the URL of the server endpoint with a
bearer header. Replace `dns.example.com` and `gm_your_token_here` below.
Against an open-mode server, drop the token.

## Claude Code

Server endpoint:

```
claude mcp add --transport http gonemaster https://dns.example.com/api/v1/mcp \
  --header "Authorization: Bearer gm_your_token_here"
```

Stdio bridge:

```
claude mcp add gonemaster \
  -e GONEMASTER_URL=https://dns.example.com \
  -e GONEMASTER_TOKEN=gm_your_token_here \
  -- /path/to/gonemaster-mcp
```

Add `-e GONEMASTER_MCP_ALLOW_WRITE=1` to the bridge for the write tools.
With the endpoint, the server's `mcp_allow_write` setting decides.

## Claude Desktop

Claude Desktop launches stdio servers from `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "gonemaster": {
      "command": "/path/to/gonemaster-mcp",
      "env": {
        "GONEMASTER_URL": "https://dns.example.com",
        "GONEMASTER_TOKEN": "gm_your_token_here"
      }
    }
  }
}
```

Claude Desktop's remote connectors authenticate with OAuth, which the
endpoint does not offer. Use the bridge there.

## Cursor

`.cursor/mcp.json` in the project, or the global file:

```json
{
  "mcpServers": {
    "gonemaster": {
      "url": "https://dns.example.com/api/v1/mcp",
      "headers": { "Authorization": "Bearer gm_your_token_here" }
    }
  }
}
```

For the bridge, give `command` and `env` as in the Claude Desktop entry.

## VS Code

`.vscode/mcp.json`:

```json
{
  "servers": {
    "gonemaster": {
      "type": "http",
      "url": "https://dns.example.com/api/v1/mcp",
      "headers": { "Authorization": "Bearer gm_your_token_here" }
    }
  }
}
```

For the bridge, use `"type": "stdio"` with `command` and `env`.

## Zed

Zed launches stdio servers from `settings.json`:

```json
{
  "context_servers": {
    "gonemaster": {
      "command": {
        "path": "/path/to/gonemaster-mcp",
        "env": {
          "GONEMASTER_URL": "https://dns.example.com",
          "GONEMASTER_TOKEN": "gm_your_token_here"
        }
      }
    }
  }
}
```

## An SDK client

Any MCP SDK can drive either deployment. With the Go SDK, the endpoint
takes an HTTP client that adds the bearer header:

```go
hc := &http.Client{Transport: bearer{token: os.Getenv("GONEMASTER_TOKEN"), next: http.DefaultTransport}}
tr := &mcp.StreamableClientTransport{Endpoint: "https://dns.example.com/api/v1/mcp", HTTPClient: hc}
client := mcp.NewClient(&mcp.Implementation{Name: "my-agent", Version: "1"}, nil)
session, err := client.Connect(ctx, tr, nil)
```

and the bridge takes a command transport:

```go
cmd := exec.Command("/path/to/gonemaster-mcp")
cmd.Env = append(os.Environ(), "GONEMASTER_URL=https://dns.example.com", "GONEMASTER_TOKEN=gm_...")
session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
```

Pass a progress token on `test_domain` to receive progress notifications
during the run.

## Checking the connection

Call `ping`. [README.md](README.md#verifying-a-connection) lists what each
answer means. `tools/list` shows 17 tools, or 20 when the write tools are
enabled.
