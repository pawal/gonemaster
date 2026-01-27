# Gonemaster

Gonemaster is a Go implementation of the Zonemaster engine and CLI.

## Installation

```
go install codeberg.org/pawal/gonemaster/cmd/gonemaster@latest
```

Or build from source:

```
git clone https://codeberg.org/pawal/gonemaster.git
cd gonemaster
go test ./...
go build -o gonemaster ./cmd/gonemaster
sudo install -m 0755 gonemaster /usr/local/bin/gonemaster
```

## CLI

The CLI will output human readable log messages, JSON, or a raw untranskated log
stream. Use `--raw` to stream raw log entries as they are produced.

### Example usage

The default output is human readable translated log messages:

```
gonemaster --domain example.com
```

With JSON output:

```
gonemaster --json --domain example.com | jq
```

### Options

- `--domain` Zone name to test (required)
- `--module` Run a single module (optional)
- `--testcase` Run a single testcase (optional)
- `--profile` Profile JSON/YAML path (optional)
- `--min-level` Minimum log level (optional, default NOTICE)
- `--output` Write output to file (optional)
- `--raw` Stream raw log entries as they are produced (optional)
- `--json` Print JSON output instead of translated output (optional)
- `--dump-profile` Print effective profile in JSON and exit (optional)
- `--locale` Locale for translated output (optional; defaults to env or `en`)
- `--no-ipv4` Disable IPv4 queries (optional)
- `--no-ipv6` Disable IPv6 queries (optional)
- `--no-progress` Disable progress indicator (optional)
- `--list-tests` List all available test cases (optional)
- `--version` Print version and exit (optional)

Locale defaults to the first available in `LANGUAGE`, then `LC_ALL`, `LC_MESSAGES`,
`LANG`, and finally `en`.

### Exit codes

- `0` Success
- `2` Usage or runtime error (invalid args, output file errors, engine errors)
- `130` Interrupted (SIGINT/SIGTERM)

## Engine usage from Go

```
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/pawal/gonemaster/engine"
)

func main() {
	req := engine.RunRequest{
		Domain:   "example.com",
		Module:   "basic",
		MinLevel: "INFO",
	}
	entries, err := engine.Run(req)
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
}
```

If you want translated messages, use `engine/i18n`:

```
package main

import (
	"fmt"

	"github.com/pawal/gonemaster/engine"
	"github.com/pawal/gonemaster/engine/i18n"
)

func main() {
	entries, _ := engine.Run(engine.RunRequest{Domain: "example.com"})
	for _, entry := range entries {
		msg := i18n.Translate("fr", entry.Module, entry.Tag, entry.Args)
		fmt.Printf("%s %s:%s:%s %s\n", entry.Level, entry.Module, entry.Testcase, entry.Tag, msg)
	}
}
```

## Development

Run test coverage:
```
go test --cover ./...
```
