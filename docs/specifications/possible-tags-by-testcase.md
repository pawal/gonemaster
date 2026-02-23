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
- Unique tags: 447

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
make spec-export-tags
```

Generator source:
- `tools/specifications/export-tags/main.go`

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
