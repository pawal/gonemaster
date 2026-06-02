# Gonemaster MCP Bridge

`gonemaster-mcp` exposes `gonemaster-server`'s admin API to Model Context
Protocol (MCP) clients as a set of tools. It speaks stdio MCP and forwards
requests to the server over HTTP; it does not run the engine itself.

Any MCP-compatible client or agent framework can drive it, for example Claude
Code, Cursor, Cline, Zed, or a custom agent built on an MCP SDK. The client
launches the bridge as a subprocess and talks to it over stdin/stdout.

## Build

```
make build-gonemaster-mcp     # builds bin/gonemaster-mcp
```

Or install it onto your PATH:

```
make install-gonemaster-mcp
```

## Configuration

The MCP client launches the bridge as a subprocess and passes configuration
through environment variables. The only command-line flags are `--help` (`-h`)
and `--version` (`-v`), which print and exit:

| Variable | Default | Purpose |
|---|---|---|
| `GONEMASTER_URL` | `http://localhost:8080/api/v1` | Base URL of the gonemaster-server admin API. A bare host or origin is accepted; the `/api/v1` suffix is added automatically. |
| `GONEMASTER_TOKEN` | (unset) | Admin token sent as `Authorization: Bearer`. Leave unset for an open-mode server. |
| `GONEMASTER_MCP_ALLOW_WRITE` | (unset) | Set to `1` to register the mutating tools (`batch_enqueue`, `batch_cancel`, `cancel_job`). Unset by default, so a default install is read + `test_domain` only. |

The bridge writes logs to stderr; stdout carries only the MCP protocol.

## Authentication

The bridge mirrors `gonemaster-client`: when `GONEMASTER_TOKEN` is set it sends
`Authorization: Bearer <token>` on every request; when it is unset it sends no
credential, which works against a server running in open mode. If the server is
in token mode and the token is missing or wrong, every call returns `401`. Mint
a token with `gonemaster-server auth add-token`; see
[../server/authentication.md](../server/authentication.md).

## Connecting a client

The configuration is the same shape everywhere: a command to run and an `env`
block. Point `command` at the built binary.

### Claude Code

```
claude mcp add gonemaster \
  -e GONEMASTER_URL=http://localhost:8080 \
  -e GONEMASTER_TOKEN=gm_your_token_here \
  -- /path/to/gonemaster-mcp
```

### Claude Desktop

Add an entry under `mcpServers` in `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "gonemaster": {
      "command": "/path/to/gonemaster-mcp",
      "env": {
        "GONEMASTER_URL": "http://localhost:8080",
        "GONEMASTER_TOKEN": "gm_your_token_here"
      }
    }
  }
}
```

Omit `GONEMASTER_TOKEN` when the server runs in open mode.

## Tools

Read tools (always available):

| Tool | Purpose |
|---|---|
| `ping` | Check connectivity and report the server's auth mode and whether the bridge is authenticated. |
| `test_domain` | Run a DNS test for a domain and wait for the result: grade, score, findings, and per-nameserver response times. |
| `run_get` | Fetch a stored run's result by id (grade, score, findings, nameserver response times). |
| `latest_for` | List a domain's most recent completed runs. |
| `run_search` | Search completed runs by domain, tag, status, severity, grade, or finish-time range. |
| `run_diff` | Compare two runs at the tag level (added / removed / severity-changed). |
| `spec_list_testcases` | List implemented testcases, optionally filtered to one module. |
| `spec_get_testcase` | Get a testcase's module, description, and the tags it can emit with rendered messages. |
| `batch_list` | List recent batches (cohort runs), newest first, with status and completion; optional `label` tag-substring filter. |
| `batch_get` | Poll a batch: total, per-status counts, and completion. |
| `cohort_stats` | Grade and worst-severity distribution across a batch's runs. |
| `cohort_tag_values` | Roll up the values a tag's argument takes across a batch (by count, or by mean score with `weight_by_score`), with sample domains. |
| `failures_by_tag` | Rank the tags driving failures in a batch, with example domains. |

`cohort_tag_values` is the generic primitive for "what values does argument
`<arg>` of tag `<tag>` take across this batch?". The `(tag, arg)` pair comes
from the [log-args inventory](../specifications/log-args-inventory.json); use
`spec_list_testcases` and `spec_get_testcase` to discover a module's tags. To
rank the operators behind a batch by mean score, call it with
`tag=IPV4_ONE_ASN`, `arg=asn`, `weight_by_score=true`; to also fold in ASNs that
appear only in the `*_DIFFERENT_ASN` / `*_SAME_ASN` tags, call those tags too
and union client-side.

`run_get`, `run_search`, and `latest_for` include a `batch_id` field, so an
agent can pivot from a known run to the batch (cohort) it belongs to. It is
empty for ad-hoc single-domain runs.

The same tools include a `public_id` field. Build a shareable public report
link as `https://<host>/#/result/<public_id>`. It is empty for runs that have
no public id.

Write tools (registered only when `GONEMASTER_MCP_ALLOW_WRITE=1`):

| Tool | Purpose |
|---|---|
| `batch_enqueue` | Enqueue a batch of domain tests (by `domains` or `from_tag`); returns a batch id to poll. |
| `batch_cancel` | Cancel a batch's in-flight jobs and remove the batch. |
| `cancel_job` | Cancel a single queued or running job. |

## Verifying the connection

After configuring the client, call the `ping` tool. A healthy open-mode setup
reports `reachable: true` and `auth_mode: open`. In token mode a correct token
reports `authenticated: true`; a missing or wrong token reports
`authenticated: false` with a hint to set `GONEMASTER_TOKEN`.
