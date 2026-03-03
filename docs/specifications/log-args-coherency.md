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
| Nameserver endpoint identity | `ns` | `"<name>"` | Name-only in current `engine/test` emits (`name/ip` is CI-rejected). |
| Nameserver name | canonical `ns`, legacy `name`/`server` in non-migrated tags | `string` | `nsname` is no longer emitted in current runtime paths. |
| Nameserver address | canonical `address`, legacy `ip` in non-migrated tags | `string` IP | `ns_ip` is no longer emitted in current runtime paths. |
| Nameserver list (structured) | `servers` | `array<object>` | Items use `{ "ns": "...", "address": "..." }`. |
| Nameserver IP list (structured) | `addresses` | `array<string>` | Typed list for machine use. |
| PTR name list | `ptr_names` | `array<string>` | PTR hostname list for reverse-DNS mismatch contexts. |
| ASN (single) | `asn` | `int` | Singular ASN value for explicit one-ASN semantics. |
| ASN collection | `asns` | `array<int>` | Structured ASN list for machine use. |
| Query name/type/class | canonical `query_name`, `query_type`, `query_class` | `string` | Legacy aliases remain only in non-migrated/system paths. |

## Current Migration Status

- `args.ns` no longer uses `name/ip` in current `engine/test` emits.
- `args.address` exists on migrated singular-endpoint callsites.
- `servers` / `addresses` are now used for nameserver list identity in migrated tags.
- Legacy packed-list drift is now concentrated in non-nameserver list keys (for example `mailtarget_list`, `parent_addresses`, `zone_addresses`).

## Canonical Contract (v1.1)

`v1.1` is a documentation contract version. It is not emitted as a runtime
marker in `args`.

### Core Identity Fields

| Key | Type | Optionality | Meaning |
| --- | --- | --- | --- |
| `ns` | `string` | conditional | Nameserver name only (normalized FQDN string). Required when a singular nameserver identity is emitted. |
| `address` | `string` | conditional | Single IP address. Required when a singular endpoint address is known and emitted. |

Rules:

- `ns` MUST NOT contain `name/ip` combined values.
- If both nameserver name and address are known for a singular endpoint, emit both `ns` and `address`.

### Query Identity Fields

| Key | Type | Optionality | Meaning |
| --- | --- | --- | --- |
| `query_name` | `string` | conditional | Queried owner name. |
| `query_type` | `string` | conditional | Queried RR type (uppercase, for example `SOA`). |
| `query_class` | `string` | conditional | Queried RR class (uppercase, usually `IN`). |

Rules:

- For query/response/cache-skip style tags, `query_name`, `query_type`, and `query_class` SHOULD be emitted together.

### Structured Collection Fields

| Key | Type | Optionality | Meaning |
| --- | --- | --- | --- |
| `servers` | `array<object>` | optional | List of endpoint objects. |
| `addresses` | `array<string>` | optional | List of IP addresses. |
| `asns` | `array<int>` | optional | List of ASN integers. |
| `prefixes` | `array<string>` | optional | List of CIDR prefixes. |

`servers` object contract:

- object keys:
  - `ns` (`string`, optional)
  - `address` (`string`, optional)
- each item MUST contain at least one of `ns` or `address`.
- role-specific server lists MAY use semantic keys with same item shape:
  - for example `parent_servers`, `child_servers`, `failing_servers`.

### Legacy Compatibility Fields

Legacy keys may still exist during migration (for example `name`, `server`,
`type`, `class`, `rrtype`, `ip`), but they are non-canonical.

Rules:

- New implementations MUST prefer canonical fields in this section.
- New packed-list-only identity keys MUST NOT be introduced.

### Prohibited Canonical Patterns

- Packed semicolon/comma identity lists as the only machine-readable representation.
- Mixed endpoint encoding in `ns` (for example `name/ip`).

### Minimal Examples

Singular endpoint + query:

```json
{
  "ns": "ns1.example",
  "address": "192.0.2.53",
  "query_name": "example",
  "query_type": "SOA",
  "query_class": "IN"
}
```

Multiple endpoints:

```json
{
  "servers": [
    {"ns": "ns1.example", "address": "192.0.2.53"},
    {"ns": "ns2.example", "address": "2001:db8::53"}
  ]
}
```

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

## Immediate Authoring Rules

For new logging changes:

- Do not write `args["ns"] = ns.String()` when `ns.String()` is `name/ip`.
- Emit:
  - `args["ns"] = <name>`
  - `args["address"] = <ip>` (when available)
- Prefer typed structures for machine consumption:
  - `servers`, `addresses`, `asns`, `prefixes`.
- Do not add `arg_schema` to runtime output.
