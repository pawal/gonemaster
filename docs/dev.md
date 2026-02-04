# Developer Guide

This document covers how to call the Gonemaster engine directly from Go. It
focuses on the core `engine` package and common workflows.

## Overview
- Primary entry point: `engine.Run(req)` returns a slice of `engine.LogEntry`.
- Optional: `req.LogCallback` streams `*logger.Entry` as tests run.
- Profiles can be inspected with `engine.EffectiveProfile(req)`.
- Planned testcases can be listed with `engine.PlannedTestcases(req)`.
- For IDN domains, normalize to A-labels with `engine/normalization`.

## Example: run a full test and print JSON
```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/normalization"
)

func main() {
	_, domain := normalization.NormalizeName("example.com")
	req := engine.RunRequest{
		Domain:   domain,
		MinLevel: "NOTICE",
	}
	entries, err := engine.Run(req)
	if err != nil {
		log.Fatal(err)
	}
	payload, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(payload))
}
```

## Example: run a single module and testcase
```go
package main

import (
	"fmt"
	"log"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/normalization"
)

func main() {
	_, domain := normalization.NormalizeName("example.com")
	req := engine.RunRequest{
		Domain:   domain,
		Module:   "basic",
		Testcase: "basic01",
		MinLevel: "INFO",
	}
	entries, err := engine.Run(req)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("entries=%d\n", len(entries))
}
```

## Example: stream results as they happen
```go
package main

import (
	"fmt"
	"log"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/normalization"
)

func main() {
	_, domain := normalization.NormalizeName("example.com")
	req := engine.RunRequest{
		Domain:   domain,
		MinLevel: "INFO",
		LogCallback: func(entry *logger.Entry) error {
			message, found := i18n.TranslateWithStatus("en", entry.Module, entry.Tag, entry.Args)
			if !found {
				message = entry.String()
			}
			fmt.Printf("%6.2f %-8s %s\n", entry.Timestamp, entry.Level(), message)
			return nil
		},
	}
	if _, err := engine.Run(req); err != nil {
		log.Fatal(err)
	}
}
```

## Example: inspect or customize the effective profile
```go
package main

import (
	"fmt"
	"log"

	"codeberg.org/pawal/gonemaster/engine"
)

func main() {
	req := engine.RunRequest{
		Domain:  "example.com",
		Profile: "./profile.yaml",
	}
	profile, err := engine.EffectiveProfile(req)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("unordered=%v\n", profile.Resolver.Defaults.Unordered)
}
```

## Example: plan the testcases before running
```go
package main

import (
	"fmt"
	"log"

	"codeberg.org/pawal/gonemaster/engine"
)

func main() {
	req := engine.RunRequest{Domain: "example.com"}
	testcases, err := engine.PlannedTestcases(req)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("planned=%d\n", len(testcases))
}
```

## Example: run simultaneous tests safely
The engine is re-entrant and safe to run in parallel. The simplest option is to
call `engine.Run` in separate goroutines; each call builds its own per-run
state internally. If you need to reuse or customize per-run state explicitly,
use `engine.NewRunner` and `engine.RunWithRunner`.

```go
package main

import (
	"context"
	"log"
	"sync"

	"codeberg.org/pawal/gonemaster/engine"
)

func main() {
	domains := []string{"example.com", "example.net"}
	var wg sync.WaitGroup

	for _, domain := range domains {
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			req := engine.RunRequest{
				Domain:  domain,
				Context: context.Background(),
			}
			if _, err := engine.Run(req); err != nil {
				log.Printf("run %s: %v", domain, err)
			}
		}(domain)
	}

	wg.Wait()
}
```

## Notes
- `engine.Run` does not normalize IDNs; normalize with
  `engine/normalization.NormalizeName` before running.
- `MinLevel` filters log entries at the engine output boundary.
- `LogCallback` receives entries before min-level filtering, which is useful for
  live progress or streaming output.

## Caching and tuning knobs
- Per-run nameserver caches (query cache + error cache) isolate concurrent runs.
- Hard network errors (host/network unreachable) are cached globally across runs
  to avoid repeated failing dials.
- Error cache TTL is controlled by `resolver.defaults.error_cache_ttl` (or the
  `--error-cache-ttl` CLI flag). The effective TTL is capped by the per-query
  timeout/retry budget.
- Positive/negative response TTLs are configured via
  `resolver.defaults.positive_cache_ttl` and
  `resolver.defaults.negative_cache_ttl` (CLI flags `--positive-cache-ttl` and
  `--negative-cache-ttl`). These settings are intended for global response
  caching and reuse across runs.
