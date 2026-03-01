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

This section is the normative contract for schema `gonemaster.logargs/1.1`.

### Contract Scope

The v1.1 contract applies to any entry where:

- `args` exists, and
- `args.arg_schema == "gonemaster.logargs/1.1"`.

Entries that do not set `arg_schema` are legacy/non-migrated and outside this contract.

### Entry-Level Fields

| Key | Type | Optionality | Meaning |
| --- | --- | --- | --- |
| `arg_schema` | `string` | required | Must be exactly `gonemaster.logargs/1.1`. |

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

Legacy keys may still exist during migration (for example `name`, `type`, `ip`, `ns_list`, `ns_ip_list`), but they are non-canonical.

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
  "arg_schema": "gonemaster.logargs/1.1",
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
  "arg_schema": "gonemaster.logargs/1.1",
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
