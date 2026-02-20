# Possible Tags By Testcase

This document describes the authoritative tags-per-testcase artifact generated from
gonemaster module metadata functions.

Primary artifact:
- `docs/specifications/possible-tags-by-testcase.json`

## Scope
- Source functions:
  - `engine/test/basic/basic.go:Metadata`
  - `engine/test/address/address.go:AddressMetadata`
  - `engine/test/connectivity/connectivity.go:Metadata`
  - `engine/test/consistency/consistency.go:Metadata`
  - `engine/test/delegation/delegation.go:Metadata`
  - `engine/test/dnssec/dnssec.go:Metadata`
  - `engine/test/nameserver/nameserver.go:Metadata`
  - `engine/test/syntax/syntax.go:Metadata`
  - `engine/test/zone/zone.go:Metadata`

## Summary
- Modules: 9
- Testcases: 73
- Unique tags: 442

Module testcase counts:
- `address`: 3
- `basic`: 3
- `connectivity`: 4
- `consistency`: 6
- `delegation`: 7
- `dnssec`: 17
- `nameserver`: 14
- `syntax`: 8
- `zone`: 11

## Regeneration
Run:

```sh
GOCACHE=/tmp/go-build-cache go run ./tools/specifications/export-tags > docs/specifications/possible-tags-by-testcase.json
```

Generator source:
- `tools/specifications/export-tags/main.go`
