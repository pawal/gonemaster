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
| Nameserver name | `ns` | `string` | Canonical key in current runtime inventory. |
| Nameserver address | `address` | `string` IP | Canonical key in current runtime inventory. |
| Nameserver list (structured) | `servers` | `array<object>` | Items use `{ "ns": "...", "address": "..." }`. |
| Nameserver IP list (structured) | `addresses` | `array<string>` | Typed list for machine use. |
| PTR name list | `ptr_names` | `array<string>` | PTR hostname list for reverse-DNS mismatch contexts. |
| ASN (single) | `asn` | `int` | Singular ASN value for explicit one-ASN semantics. |
| ASN collection | `asns` | `array<int>` | Structured ASN list for machine use. |
| Prefix collection | `prefixes` | `array<string>` | CIDR prefix list. |
| MX target list | `mail_targets` | `array<string>` | MX target hostname list. |
| Query name/type/class | `query_name`, `query_type`, `query_class` | `string` | Canonical query identity keys in current runtime inventory. |

## Migration Status

Migration is complete. All engine emitters use canonical keys:

- `ns` is nameserver name only (never `name/ip`).
- `address` carries a single IP address.
- `servers`, `addresses` replace `ns_list`, `ns_ip_list`, `nsname_list`.
- `asn` (singular int) and `asns` (list) replace `asn_list`.
- `prefixes` replaces `prefix` / `ip_prefix`.
- `ptr_names` replaces `names` in PTR mismatch contexts.
- `mail_targets` replaces `mailtarget_list`.
- `query_name`, `query_type`, `query_class` replace legacy query aliases.
- CI guardrails reject reintroduction of legacy patterns.

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
| `ptr_names` | `array<string>` | optional | PTR hostname list. |
| `mail_targets` | `array<string>` | optional | MX target hostname list. |

`servers` object contract:

- object keys:
  - `ns` (`string`, optional)
  - `address` (`string`, optional)
- each item MUST contain at least one of `ns` or `address`.
- role-specific server lists MAY use semantic keys with same item shape:
  - for example `parent_servers`, `child_servers`, `failing_servers`.

### Retired Legacy Fields

Legacy identity/query aliases are no longer emitted. See the
[key glossary](log-args-key-glossary.md) for the complete list of retired keys.

Rules:

- New implementations MUST use canonical fields defined above.
- New packed-list-only identity keys MUST NOT be introduced.
- CI guardrails enforce both rules.

### Prohibited Canonical Patterns

- Packed semicolon/comma identity lists as the only machine-readable representation.
- Mixed endpoint encoding in `ns` (for example `name/ip`).

### Consumer Examples

Singular endpoint with query identity:

```json
{
  "ns": "ns1.example.",
  "address": "192.0.2.53",
  "query_name": "example.",
  "query_type": "SOA",
  "query_class": "IN"
}
```

Multiple endpoints (`servers`):

```json
{
  "servers": [
    {"ns": "ns1.example.", "address": "192.0.2.53"},
    {"ns": "ns2.example.", "address": "2001:db8::53"}
  ]
}
```

Address list (`addresses`):

```json
{
  "ns": "ns1.example.",
  "addresses": ["192.0.2.53", "2001:db8::53"]
}
```

ASN collection (`asns`) with prefixes:

```json
{
  "address": "192.0.2.53",
  "asns": [64496, 64497],
  "prefixes": ["192.0.2.0/24"]
}
```

Role-specific server lists share the same item shape as `servers`:

```json
{
  "parent_servers": [
    {"ns": "ns1.parent.", "address": "192.0.2.1"}
  ],
  "zone_servers": [
    {"ns": "ns1.example.", "address": "192.0.2.53"}
  ]
}
```

## Invariants

### Entry envelope invariants

These are stable and must remain unchanged:

- Every entry has `timestamp`, `module`, `testcase`, `tag`, `level`.
- `args` is optional and, when present, is a JSON object.
- `args` values must be JSON-serializable.
- Date/time string values in `args` should use UTC RFC3339 (ISO 8601 profile).

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
