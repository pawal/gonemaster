# gonemaster-mcp 1 "2026" "gonemaster" "User Commands"

## NAME

gonemaster-mcp - Model Context Protocol bridge for gonemaster-server

## SYNOPSIS

**gonemaster-mcp** [**-v** | **--version**] [**-h** | **--help**]

## OPTIONS

**-v**, **--version**
: Print version and exit.

**-h**, **--help**
: Show usage and exit.

## DESCRIPTION

**gonemaster-mcp** exposes a running **gonemaster-server**'s admin API to Model
Context Protocol (MCP) clients as a set of tools. It speaks stdio MCP and
forwards requests to the server over HTTP; it does not run the test engine
itself.

An MCP client (such as Claude Code or Claude Desktop) launches the bridge as a
subprocess and communicates over stdin/stdout. The bridge takes no command-line
flags; it is configured entirely through environment variables. All logs are
written to stderr so that stdout carries only the MCP protocol.

The same tools are served by **gonemaster-server** itself at **POST
/api/v1/mcp** over Streamable HTTP when its **mcp_enabled** setting is on.
Clients that speak HTTP need no bridge then; see the MCP documentation.

## ENVIRONMENT

**GONEMASTER_URL**
: Base URL of the gonemaster-server admin API (default:
  http://localhost:8080/api/v1). A bare host or origin is accepted; the
  **/api/v1** suffix is added automatically.

**GONEMASTER_TOKEN**
: Admin token sent as **Authorization: Bearer**. Leave unset for an open-mode
  server. When the server is in token mode and the token is missing or wrong,
  every call returns HTTP 401.

**GONEMASTER_MCP_ALLOW_WRITE**
: When set to **1** (or true/yes/on), registers the mutating tools
  (**batch_enqueue**, **batch_cancel**, **cancel_job**). Unset by default, so
  a default install exposes read and single-domain test tools only.

## TOOLS

**ping**
: Check connectivity and report the server's auth mode.

**test_domain**
: Run a DNS test for a domain and wait for the result (grade, score, findings,
  per-nameserver response times), reporting progress while it runs.

**run_get**, **latest_for**
: Fetch a stored run's result by id, or list a domain's most recent runs.
  **min_level** keeps only findings at or above a severity.

**run_search**, **run_diff**
: Search completed runs by filter, or diff two runs at the tag level.

**spec_list_testcases**, **spec_get_testcase**
: List implemented testcases, or get one testcase's tags and rendered messages.

**profile_list**, **domain_tag_list**, **cohort_list**
: List the test profiles, the user domain tags, and the analysis cohorts with
  their snapshots.

**batch_list**, **batch_get**, **cohort_stats**, **failures_by_tag**,
**cohort_tag_values**
: List or poll batches, get a batch's grade and severity distribution, rank
  the tags driving failures, or roll up the values a tag argument takes.

**cohort_report**
: Compare two snapshots of an analysis cohort and classify every change as
  engine-driven or real, with the movers and the clusters they form.

**batch_enqueue**, **batch_cancel**, **cancel_job**
: Write tools, registered only when **GONEMASTER_MCP_ALLOW_WRITE** is set.

## EXAMPLES

Register with Claude Code against a token-protected server:

    claude mcp add gonemaster \
        -e GONEMASTER_URL=http://localhost:8080 \
        -e GONEMASTER_TOKEN=gm_your_token_here \
        -- /usr/bin/gonemaster-mcp

Register against a local open-mode server (no token; URL defaults to
http://localhost:8080):

    claude mcp add gonemaster -- /usr/bin/gonemaster-mcp

Also enable the write tools (batch_enqueue, batch_cancel, cancel_job):

    claude mcp add gonemaster \
        -e GONEMASTER_URL=http://localhost:8080 \
        -e GONEMASTER_TOKEN=gm_your_token_here \
        -e GONEMASTER_MCP_ALLOW_WRITE=1 \
        -- /usr/bin/gonemaster-mcp

Claude Desktop entry in claude_desktop_config.json:

    {
      "mcpServers": {
        "gonemaster": {
          "command": "/usr/bin/gonemaster-mcp",
          "env": {
            "GONEMASTER_URL": "http://localhost:8080",
            "GONEMASTER_TOKEN": "gm_your_token_here"
          }
        }
      }
    }

## SEE ALSO

**gonemaster-server**(1), **gonemaster-client**(1)
