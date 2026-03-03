# Log Argument Key Glossary

This glossary defines canonical log argument keys for machine consumers.
It complements `docs/specifications/log-args-coherency.md`.

Contract version:

- `v1.1` (documentation version, not emitted in runtime `args`)

Scope:

- Normative for newly migrated tags following the coherency contract.
- Non-migrated tags may still use legacy keys during migration.

## Core Identity Keys

| Key | Type | Meaning | Notes |
| --- | --- | --- | --- |
| `ns` | `string` | Nameserver name (FQDN string). | Name only. Never `name/ip`. |
| `address` | `string` | Single nameserver IP address. | Use together with `ns` when both are known. |
| `domain` | `string` | Domain name in testcase-specific contexts. | Keep testcase meaning explicit. |
| `mname` | `string` | SOA MNAME hostname. | Hostname only. |

## Query Identity Keys

| Key | Type | Meaning | Notes |
| --- | --- | --- | --- |
| `query_name` | `string` | DNS owner name queried. | |
| `query_type` | `string` | DNS RR type queried. | Uppercase form (for example `SOA`). |
| `query_class` | `string` | DNS class queried. | Usually `IN`. |

## Structured Collection Keys

| Key | Type | Meaning | Notes |
| --- | --- | --- | --- |
| `servers` | `array<object>` | Endpoint list for machine use. | Items use `{ "ns": "...", "address": "..." }`. |
| `addresses` | `array<string>` | IP address list. | |
| `asns` | `array<int>` | ASN list. | Integer ASN values. |
| `prefixes` | `array<string>` | CIDR prefix list. | |
| `ptr_names` | `array<string>` | PTR hostname list from reverse-DNS checks. | Used for PTR mismatch detail payloads. |

## Temporal Keys

| Key | Type | Meaning | Notes |
| --- | --- | --- | --- |
| `date` | `string` | Human-visible timestamp argument in tag-specific contexts. | Use UTC RFC3339 (ISO 8601 profile). |

## Role-Specific Collection Variants

Use role-specific server collections where needed, with the same endpoint item
shape as `servers`:

- `parent_servers`
- `child_servers`
- `failing_servers`

## Transitional Legacy Keys

Current runtime inventory no longer emits legacy identity/query aliases.
If these keys are encountered in historical outputs, external adapters, or
in-flight branches, they are non-canonical in v1.1:

- `ip`
- `name`, `server`
- query aliases `type`, `class`, `rrtype`
- delimiter-packed identity fields (`;` or `,`) without typed counterparts

Migration rule:

- New and migrated emits must prefer canonical keys above.
- Do not introduce new packed-list-only identity fields.
