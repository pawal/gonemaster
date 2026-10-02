# The MCP Endpoint in gonemaster-server

`gonemaster-server` serves the MCP tools at `POST /api/v1/mcp` over
Streamable HTTP. The endpoint sits under the admin API, so the admin token
auth, request IDs, access log, panic recovery, and API metrics apply to it
unchanged, and a reverse proxy rule that exposes `/api/v1` covers it.

Each tool call is served by calling the admin API in-process under the
caller's own bearer token. The endpoint adds no capability the token does
not already have.

## Enabling

Two settings control the endpoint. Both default to `false`, both are read
on every request, and both can be changed without a restart.

| Setting | Effect |
|---|---|
| `mcp_enabled` | Serves the endpoint. When off, `POST /api/v1/mcp` returns `404`. |
| `mcp_allow_write` | Also registers `batch_enqueue`, `batch_cancel`, and `cancel_job`. |

In the config file:

```json
{
  "mcp_enabled": true,
  "mcp_allow_write": false
}
```

In the admin UI: Settings, group MCP, the toggles "MCP endpoint" and "MCP
write tools". Through the API: `PUT /api/v1/settings` with
`{"mcp_enabled": true}`. The server logs `mcp endpoint enabled=true` when
the setting changes and `mcp endpoint enabled` at startup when the config
file turns it on.

## Authentication

The endpoint uses the admin tokens described in
[../server/authentication.md](../server/authentication.md). In token mode
it accepts `Authorization: Bearer <token>` only; a request that carries the
admin UI's session cookie and no bearer header is rejected with `401` and
the message `bearer token required`. The caller's header is forwarded to
every in-process admin API call, which checks it again.

Admin tokens carry no scopes. `mcp_allow_write` therefore applies to every
token holder, not to a token.

In open mode (no tokens configured) the endpoint is open to anyone who can
reach `/api/v1`, as the admin API already is. Use token mode when
`mcp_enabled` is on.

## Protocol

- Streamable HTTP, stateless: no `Mcp-Session-Id`, no server-to-client
  requests. `GET` and `DELETE` return `405`.
- Responses are `text/event-stream` unless the client sends
  `Accept: application/json` alone. The event stream carries progress
  notifications from `test_domain` while the run executes.
- The request body is capped at 4 MiB.
- The server identifies itself as `gonemaster-server` with the engine
  version.

## Reverse proxy

`test_domain` holds the connection until the run finishes, up to its
`timeout_seconds`, at most 600 s. The server raises its own write deadline
for this endpoint. The proxy MUST allow the same: set its upstream read
timeout above the tool timeout you expect, or have clients pass a lower
`timeout_seconds` and finish with `run_get`. Event streams MUST pass
through unbuffered.

nginx:

```nginx
location /api/v1/mcp {
    proxy_pass http://127.0.0.1:8080/api/v1/mcp;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
    proxy_read_timeout 660s;
}
```

Caddy:

```caddyfile
reverse_proxy /api/v1/mcp localhost:8080 {
    flush_interval -1
    transport http {
        response_header_timeout 660s
    }
}
```

The admin API is not meant for the public internet. Keep `/api/v1/`,
including this path, on a private network or behind the proxy's own access
control in addition to the token.

## Observability

- Every tool call writes one log line, `mcp tool call`, with the tool
  name, its duration, and whether it failed.
- The admin API calls a tool makes are access-logged like any other, and
  they share the `request_id` of the `POST /api/v1/mcp` line that caused
  them.
- `GET /api/v1/metrics` exports
  `gonemaster_mcp_tool_calls_total{tool,outcome}` with `outcome` `ok` or
  `failed`. The JSON snapshot carries the same counts under
  `api.mcp_tool_calls`.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `404 mcp endpoint disabled` | `mcp_enabled` is off. |
| `401 admin token required` | Token mode and no credential at all. |
| `401 bearer token required` | Token mode and the credential arrived as a cookie. Send a bearer header. |
| `401 invalid admin token` | The token is not in `auth.admin_tokens`. |
| `405` | A `GET` or `DELETE`. The endpoint is stateless and takes `POST` only. |
| The client drops `test_domain` after about a minute | A proxy timeout. Raise it, or lower `timeout_seconds`. |
| Progress never arrives | A proxy buffers the event stream. Disable buffering for this path. |
| The write tools are missing from `tools/list` | `mcp_allow_write` is off. |
