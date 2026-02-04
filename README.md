# Gonemaster

Gonemaster is a Go implementation of the DNS test framework Zonemaster engine and CLI.

## Installation

```
go install codeberg.org/pawal/gonemaster/cmd/gonemaster@latest
```

Or build from source:

```
git clone https://codeberg.org/pawal/gonemaster.git
cd gonemaster
make help
go test ./...
go build -o gonemaster ./cmd/gonemaster
sudo install -m 0755 gonemaster /usr/local/bin/gonemaster
```

The project includes a Makefile with common targets like `build`, `test`, and
`ui-build`. Run `make help` to see the full list.

## CLI

The CLI runs Zonemaster tests and can output human‑readable text, JSON, or raw
log streams. See [docs/cli.md](docs/cli.md) for full usage, options, and examples.

Quick examples:

```
gonemaster --domain example.com
gonemaster --json --domain example.com | jq
```

![ascii animation](docs/demo.gif)

## API Server and UI

Gonemaster includes an API server with an included Web User Interface.
Please read the documentation in [docs/server.md](docs/server.md) to know more.

There is also a CLI for testing domains through the API - [gonemaster-client](docs/cli.md)
also has support for batch operations.

## Nagios plugin

Nagios documentation has moved to [docs/nagios.md](docs/nagios.md).

## Engine usage

Developer usage (engine APIs, callbacks, profiles, and localization) is covered
in [docs/dev.md](docs/dev.md).

## Development

Run test coverage:
```
go test --cover ./...
```
