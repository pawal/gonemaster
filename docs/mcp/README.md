# gonemaster and MCP

gonemaster exposes its testing and analysis tools to AI agents over the
Model Context Protocol (MCP). A connected agent can run a test, read and
search stored runs, diff two runs, and roll up a batch: the grade
distribution, the tags driving failures, the values an argument takes
across a cohort, and a classified comparison of two cohort snapshots. The
tools call the admin API of `gonemaster-server`; they run no engine of their
own.

The same 17 read tools and 3 write tools are served two ways:

| | `gonemaster-mcp`, the stdio bridge | `gonemaster-server`, the HTTP endpoint |
|---|---|---|
| Runs | on the client host, as a subprocess the MCP client launches | inside `gonemaster-server`, at `POST /api/v1/mcp` |
| Transport | stdio | Streamable HTTP |
| Authentication | `GONEMASTER_TOKEN` in the bridge's environment | `Authorization: Bearer` on every request, with the same admin tokens |
| Write tools | `GONEMASTER_MCP_ALLOW_WRITE=1` on the bridge | the `mcp_allow_write` setting |
| Fits | one operator's machine; clients that speak stdio only | a shared server, several agents, no binary on the client host |

- [server-endpoint.md](server-endpoint.md): enabling and operating the endpoint.
- [clients.md](clients.md): configuration for Claude Code, Claude Desktop and claude.ai, Cursor, VS Code,
  Windsurf, Zed, Gemini CLI, Codex CLI, and SDKs.
- [tools.md](tools.md): every tool with its inputs, defaults, caps, and outputs.
- [analysis-examples.md](analysis-examples.md): worked analysis sessions.

## Quick start: the server endpoint

1. Put the server in token mode and mint a token; see
   [../server/authentication.md](../server/authentication.md).
2. Enable the endpoint. In the config file:
   ```json
   { "mcp_enabled": true }
   ```
   or in the admin UI, Settings, MCP, "MCP endpoint".
3. Register it with the client, here Claude Code:
   ```
   claude mcp add --transport http gonemaster https://dns.example.com/api/v1/mcp \
     --header "Authorization: Bearer gm_your_token_here"
   ```
4. Ask the agent to call `ping`. It reports `reachable: true`,
   `auth_mode: token`, `authenticated: true`.

## Quick start: the stdio bridge

1. Build or install the binary:
   ```
   make build-gonemaster-mcp        # bin/gonemaster-mcp
   make install-gonemaster-mcp      # onto PATH
   ```
   Packaged installs place it at `/usr/bin/gonemaster-mcp`.
2. Register it with the client, here Claude Code:
   ```
   claude mcp add gonemaster \
     -e GONEMASTER_URL=https://dns.example.com \
     -e GONEMASTER_TOKEN=gm_your_token_here \
     -- /path/to/gonemaster-mcp
   ```
   Omit `GONEMASTER_TOKEN` against a server in open mode.
3. Ask the agent to call `ping`.

## Bridge configuration

The MCP client launches `gonemaster-mcp` and passes its configuration
through environment variables. The only command-line flags are `--help`
(`-h`) and `--version` (`-v`), which print and exit.

| Variable | Default | Purpose |
|---|---|---|
| `GONEMASTER_URL` | `http://localhost:8080/api/v1` | Base URL of the admin API. A bare host or origin is accepted; `/api/v1` is appended. |
| `GONEMASTER_TOKEN` | unset | Admin token sent as `Authorization: Bearer`. Unset works against an open-mode server. |
| `GONEMASTER_MCP_ALLOW_WRITE` | unset | `1`, `true`, `yes`, or `on` registers `batch_enqueue`, `batch_cancel`, and `cancel_job`. |

The bridge logs to stderr; stdout carries only the protocol.

## Verifying a connection

`ping` is the smoke test for either deployment.

| Result | Meaning |
|---|---|
| `reachable: true`, `auth_mode: open`, `authenticated: true` | Open-mode server, no token needed. |
| `reachable: true`, `auth_mode: token`, `authenticated: true` | Token accepted. |
| `reachable: true`, `auth_mode: token`, `authenticated: false` | Token missing or wrong. The detail names the fix. |
| `reachable: false`, detail "answered http 404" | Something answered at the URL, but it is not a gonemaster admin API. |
| `reachable: false`, detail "unreachable" | Nothing answered: wrong host or port, or the server is down. |

## The tools in brief

Read tools, always registered: `ping`, `test_domain`, `run_get`,
`latest_for`, `run_search`, `run_diff`, `spec_list_testcases`,
`spec_get_testcase`, `profile_list`, `domain_tag_list`, `cohort_list`,
`batch_list`, `batch_get`, `cohort_stats`, `failures_by_tag`,
`cohort_tag_values`, `cohort_report`.

Write tools, registered only when enabled: `batch_enqueue`, `batch_cancel`,
`cancel_job`.

Every tool carries MCP annotations, so a client can tell a read from a
write and a destructive write from an additive one before it asks the user
for confirmation. Tool results quote text from the tested zone's
nameservers; with write tools enabled, clients SHOULD confirm every
destructive call. See
[server-endpoint.md](server-endpoint.md#zone-controlled-text). `test_domain` reports progress while a run executes.
[tools.md](tools.md) has the full reference.

## See also

- [../analysis/querying.md](../analysis/querying.md): the same questions
  through `gonemaster-client`, the admin API, and SQL.
- [../server/authentication.md](../server/authentication.md): minting and
  installing admin tokens.
