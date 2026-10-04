# Connecting MCP Clients

Every client needs one of two things: the path to the `gonemaster-mcp`
binary with its environment, or the URL of the server endpoint with a
bearer header. Replace `dns.example.com` and `gm_your_token_here` below.
Against an open-mode server, drop the token.

| Client | Server endpoint | Stdio bridge |
|---|---|---|
| Claude Code | yes | yes |
| Claude Desktop, claude.ai, Claude mobile apps | with request headers (beta) | Claude Desktop only |
| Cursor | yes | yes |
| VS Code, GitHub Copilot | yes | yes |
| Windsurf, Devin Desktop | yes | yes |
| Zed | yes | yes |
| Gemini CLI | yes | yes |
| Codex CLI | yes | yes |

A client that offers OAuth and no header field cannot reach the endpoint
in token mode. Use the bridge where such a client runs stdio servers.

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

## Claude Desktop, claude.ai, and the Claude mobile apps

Server endpoint, as a custom connector. Claude connects to it from
Anthropic's network, so the endpoint MUST be reachable from the internet.

1. Open Customize, Connectors, "Add custom connector". On a Team or
   Enterprise plan, an Owner adds it under Organization settings,
   Connectors, Add, Custom.
2. Enter `https://dns.example.com/api/v1/mcp` as the server URL.
3. Choose "No sign-in" as the authentication.
4. Under Request headers, add `authorization` with the value
   `Bearer gm_your_token_here`. Claude sends the value as entered, so it
   MUST include `Bearer `.

Request headers are a beta available to a limited set of organizations.
Without the Request headers section the connector cannot authenticate;
use the bridge in Claude Desktop. The header cannot be edited after the
connector is added: to rotate the token, remove the connector and add it
again. On a Team or Enterprise plan, every member who connects uses the
same token.

Stdio bridge, Claude Desktop only, in `claude_desktop_config.json`:

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

`.vscode/mcp.json`, also read by GitHub Copilot's agent mode:

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

## Windsurf and Devin Desktop

`mcp_config.json`:

```json
{
  "mcpServers": {
    "gonemaster": {
      "serverUrl": "https://dns.example.com/api/v1/mcp",
      "headers": { "Authorization": "Bearer gm_your_token_here" }
    }
  }
}
```

For the bridge, give `command` and `env` as in the Claude Desktop entry.

## Zed

Server endpoint, in `settings.json`:

```json
{
  "context_servers": {
    "gonemaster": {
      "url": "https://dns.example.com/api/v1/mcp",
      "headers": { "Authorization": "Bearer gm_your_token_here" }
    }
  }
}
```

Without an `Authorization` header, Zed starts the MCP OAuth flow, which
the endpoint does not offer.

Stdio bridge:

```json
{
  "context_servers": {
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

## Gemini CLI

`~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "gonemaster": {
      "httpUrl": "https://dns.example.com/api/v1/mcp",
      "headers": { "Authorization": "Bearer gm_your_token_here" }
    }
  }
}
```

The key MUST be `httpUrl`: `url` selects the SSE transport, which the
endpoint does not serve. For the bridge, give `command` and `env` as in
the Claude Desktop entry.

## Codex CLI

Server endpoint, in `~/.codex/config.toml`:

```toml
[mcp_servers.gonemaster]
url = "https://dns.example.com/api/v1/mcp"
bearer_token_env_var = "GONEMASTER_TOKEN"
```

Codex reads the token from the named environment variable and sends it as
`Authorization: Bearer`. Set `GONEMASTER_TOKEN=gm_your_token_here` in the
environment Codex runs in.

Stdio bridge:

```toml
[mcp_servers.gonemaster]
command = "/path/to/gonemaster-mcp"

[mcp_servers.gonemaster.env]
GONEMASTER_URL = "https://dns.example.com"
GONEMASTER_TOKEN = "gm_your_token_here"
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
answer means. `tools/list` shows 18 tools, or 21 when the write tools are
enabled.
