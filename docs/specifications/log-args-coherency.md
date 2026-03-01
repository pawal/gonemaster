# Log Argument Coherency

This document defines shared terminology and invariants for structured log
arguments (`entry.args`) emitted by gonemaster.

It is intended for:

- engine/test implementation work
- translation template work (`share/lang/*.po`)
- API/CLI JSON consumers

## Scope

Applies to all emitted log entries consumed through:

- CLI `--json` and `--json-stream`
- server job result payloads (`raw.entries[].args`)
- localized message rendering (placeholder interpolation)

## Glossary (Current State)

| Term | Current key(s) in use | Typical current shape | Notes |
| --- | --- | --- | --- |
| Nameserver endpoint identity | `ns` | `"<name>/<ip>"` | Most frequent pattern today; semantically overloaded. |
| Nameserver name | `nsname`, sometimes `ns`, sometimes `name` | `string` | Naming is not yet coherent across modules. |
| Nameserver address | `ns_ip`, `ip`, sometimes `address` | `string` IP | Key choice depends on tag/module. |
| Nameserver list (display) | `ns_list` | `string` joined by `;` | Often contains endpoint strings (`name/ip`). |
| Nameserver IP list (display) | `ns_ip_list` | `string` joined by `;` | Machine consumers must split strings today. |
| ASN (single) | `asn` | `int` or `string` | Type varies by producer/tag. |
| ASN collection | `asn_list`, sometimes `asn` | mostly joined `string` | Shape varies (`string` vs list-like). |
| Query name/type/class | `query_name`, `rrtype`, `type`, `query_type`, `query_class` | `string` | Partially overlapping key set. |

## Canonical Schema (v1.1)

All new and migrated log entries MUST follow schema id:

- `arg_schema = "gonemaster.logargs/1.1"`

Canonical field model:

- `ns`: nameserver name only (FQDN, normalized)
- `address`: single IP address (string)
- `servers`: array of objects `{ "ns": "<fqdn>", "address": "<ip>" }`
- `addresses`: array of IP address strings
- `asns`: array of ASN integers
- `prefixes`: array of prefix strings (CIDR)

For role-specific sets, use semantic list keys with the same object shape:

- `parent_servers`, `child_servers`, `failing_servers`, etc.

Prohibited as primary machine fields:

- packed semicolon/comma list strings for server/IP identity
- mixed endpoint encoding in a single identity field (for example `name/ip` in `ns`)

## Invariants

### Entry envelope invariants

These are stable and must remain unchanged:

- Every entry has `timestamp`, `module`, `testcase`, `tag`, `level`.
- `args` is optional and, when present, is a JSON object.
- `args` values must be JSON-serializable.

### Translation invariants

- If a message template contains `{placeholder}`, that key must exist in
  `args` whenever that template is emitted.
- Placeholder names in translations must stay aligned with emitted arg keys.

### Coherency invariants

These invariants apply to all new and migrated tags:

1. `ns` is reserved for nameserver name only.
2. `address` is reserved for a single IP address.
3. If both name and IP are known, emit both fields separately.
4. Do not encode endpoint identity in `ns` as `name/ip`.
5. Use typed list fields (`servers`, `addresses`, `asns`, `prefixes`) for machine data.
6. Do not introduce new packed list strings as canonical data.

### Compatibility and versioning invariants

- `1.1` is the canonical baseline.
- Future `1.x` updates MUST be additive only.
- Renames/removals/type changes are forbidden in `1.x`.
- Any breaking schema change requires a new major schema id (`2.0`).

## Immediate authoring rules

For new logging changes:

- Do not write `args["ns"] = ns.String()` when `ns.String()` is `name/ip`.
- Emit:
  - `args["ns"] = <name>`
  - `args["address"] = <ip>` (when available)
- Prefer typed structures for machine consumption:
  - `servers`, `addresses`, `asns`, `prefixes`.
- Always set `args["arg_schema"] = "gonemaster.logargs/1.1"` for entries
  participating in the coherency migration.
