# Gonemaster

Gonemaster is a Go implementation of the Zonemaster DNS test engine, with a
local CLI, an HTTP server, a server automation client, and public analysis
views for tagged domain cohorts.

## Common Paths

### Run One Local Test

```sh
gonemaster example.com
gonemaster --json --domain example.com | jq
gonemaster --module dnssec --testcase dnssec01 example.com
```

Direct CLI documentation: [docs/cli.md](docs/cli.md)

### Start the Server

```sh
go build -o ./gonemaster-server ./cmd/gonemaster-server
./gonemaster-server
```

Server documentation: [docs/server/](docs/server/README.md)

### Automate the Server

```sh
gonemaster-client jobs create --domain example.com --wait --view summary
gonemaster-client jobs batch --file domains.txt --tag tld --wait
gonemaster-client entries query --tag tld --module DNSSEC --latest
```

Client documentation: [docs/client/](docs/client/README.md)

### Publish Analysis Cohorts

```sh
gonemaster-client tags create tld --description "Top-level domains"
gonemaster-client tags add-domains tld --file tlds.txt
gonemaster-client jobs batch --from-tag tld --tag tld --wait
```

Analysis documentation: [docs/analysis/](docs/analysis/README.md)

## Install

Install the local CLI:

```sh
go install codeberg.org/pawal/gonemaster/cmd/gonemaster@latest
```

Build from source:

```sh
git clone https://codeberg.org/pawal/gonemaster.git
cd gonemaster
make help
go test ./...
go build -o gonemaster ./cmd/gonemaster
sudo install -m 0755 gonemaster /usr/local/bin/gonemaster
```

The Makefile includes common targets such as `build`, `test`, `ui-build`, and
documentation/specification checks. Run `make help` for the current list.

## Documentation

- Documentation home: [docs/README.md](docs/README.md)
- Direct CLI: [docs/cli.md](docs/cli.md)
- Server: [docs/server/](docs/server/README.md)
- Server client: [docs/client/](docs/client/README.md)
- Tags, cohorts, and snapshots: [docs/analysis/](docs/analysis/README.md)
- API conventions: [docs/reference/api.md](docs/reference/api.md)
- OpenAPI: [docs/openapi.yaml](docs/openapi.yaml)
- Developer engine API: [docs/dev.md](docs/dev.md)
- Testcase specifications: [docs/specifications/](docs/specifications/README.md)
- Nagios plugin: [docs/nagios.md](docs/nagios.md)

## Highlights

- Parallel-safe engine runs with per-run state isolation.
- Text, JSON, JSON stream, and raw log output.
- Undelegated testing with explicit nameserver and DS input.
- HTTP server with persistent queue, batches, tags, profiles, and metrics.
- Public API and public UI that avoid exposing internal job IDs.
- Public cohort analysis with immutable snapshots.
- Stored packet cache save/restore for reproducible runs.

## Screenshots

CLI output:

![ascii animation](docs/demo.gif)

Admin UI:

![UI screenshot](docs/ui-screenshot.png)

Metrics view:

![Metrics screenshot](docs/metrics.png)
