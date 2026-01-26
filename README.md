# Gonemaster

Gonemaster is a Go implementation of the Zonemaster engine and CLI.

## CLI

Translated output is the default. Use `--json` for JSON output.
Use `--raw` to stream raw log entries as they are produced. `--json` and `--raw` cannot be combined.
Use `--dump-profile` to print the effective profile JSON and exit.

### Usage

```
go run ./cmd/gonemaster --domain example.com
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

### Examples

Translated output (default):
```
go run ./cmd/gonemaster --locale fr --domain example.com
```

JSON output:
```
go run ./cmd/gonemaster --json --domain example.com | jq
```

Dump effective profile:
```
go run ./cmd/gonemaster --dump-profile | jq
```

Raw streaming output:
```
go run ./cmd/gonemaster --raw --min-level INFO --domain example.com
```

Write JSON to file:
```
go run ./cmd/gonemaster --json --domain example.com --output /tmp/zonemaster.json
```

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
