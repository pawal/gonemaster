# Migration Guide: Log Args v1.1

This guide is for users who consume gonemaster JSON results and are upgrading
from pre-coherency output to the v1.1 log-args contract.

Scope:

- CLI JSON outputs (`--json`, `--json-stream`)
- Server result payloads (`raw.entries[].args`)

The entry envelope is unchanged:

- `timestamp`, `module`, `testcase`, `tag`, `level`, `args`

The migration affects keys and value shapes inside `args`.

## Biggest Changes

1. `ns` no longer contains `name/ip`.
- Old: `ns: "ns1.example/192.0.2.53"`
- New: `ns: "ns1.example"`, `address: "192.0.2.53"`

2. Packed list strings were replaced by typed lists.
- Old: `ns_list`, `ns_ip_list`, `mailtarget_list`, `parent_addresses`, `zone_addresses`
- New: `servers`, `addresses`, `mail_targets`, `parent_servers`, `zone_servers`

3. ASN fields are normalized.
- Old: mixed usage (`asn` as string/int/list-like, `asn_list` as packed text).
- New: `asn` is singular (`int`) and `asns` is a collection (`array<int>`).

4. Query identity keys are canonical.
- New standard: `query_name`, `query_type`, `query_class`
- Legacy query aliases are not part of current runtime inventory.

5. Legacy generic aliases were removed from current runtime inventory.
- No longer emitted as runtime keys: `name`, `server`, `ip`, `type`, `class`, `rrtype`

6. Contract versioning is documentation-based.
- `arg_schema` is not emitted in runtime output.
- v1.1 is documented in specs, not carried as an `args` field.

## Key Mapping (Old -> New)

| Old key | New key | New type |
| --- | --- | --- |
| `ns` (endpoint `name/ip`) | `ns` + `address` | `string` + `string` |
| `ns_list` | `servers` | `array<object>` |
| `ns_ip_list` | `addresses` | `array<string>` |
| `mailtarget_list` | `mail_targets` | `array<string>` |
| `parent_addresses` | `parent_servers` | `array<object>` |
| `zone_addresses` | `zone_servers` | `array<object>` |
| `asn_list` | `asns` | `array<int>` |
| `asn` (collection semantics) | `asns` | `array<int>` |
| `asn` (singular semantics) | `asn` | `int` |
| `type` / `rrtype` | `query_type` | `string` |
| `class` | `query_class` | `string` |
| `name` (query context) | `query_name` | `string` |
| `server` | `address` (or `ns` + `address`, tag-specific) | `string` |
| `ip` | `address` | `string` |

`servers`-like object shape:

```json
{"ns":"ns1.example","address":"192.0.2.53"}
```

## Example Migration

Old:

```json
{
  "Tag": "DS07_SIGNED_ON_SERVER",
  "Args": {
    "ns_list": "ns1.example;ns2.example"
  }
}
```

New:

```json
{
  "Tag": "DS07_SIGNED_ON_SERVER",
  "Args": {
    "servers": [
      {"ns":"ns1.example"},
      {"ns":"ns2.example"}
    ]
  }
}
```

Old:

```json
{
  "Tag": "WRONG_SOA",
  "Args": {
    "ns": "ns1.example/192.0.2.53",
    "name": "example.com.",
    "owner": "wrong.example."
  }
}
```

New:

```json
{
  "Tag": "WRONG_SOA",
  "Args": {
    "ns": "ns1.example",
    "address": "192.0.2.53",
    "query_name": "example.com.",
    "owner": "wrong.example."
  }
}
```

## Consumer Update Checklist

1. Update deserializers to read typed collections:
- `servers` as `array<object>`
- `addresses` as `array<string>`
- role-specific lists like `parent_servers`, `zone_servers`

2. Stop splitting semicolon-delimited identity strings.
- Use typed fields directly.

3. Treat `ns` as name-only.
- Read IP from `address`.

4. Use canonical query keys.
- `query_name`, `query_type`, `query_class`

5. Normalize ASN handling in your consumer.
- Read `asns` for multi-ASN outputs.
- Treat `asn` as singular-only.

6. Remove dependency on `arg_schema`.

7. Add a temporary compatibility shim only if you must ingest historical data.
- Map old keys to new keys at ingest time.
- Prefer writing downstream state only in v1.1 key format.

## References

- Coherency contract: [docs/specifications/log-args-coherency.md](specifications/log-args-coherency.md)
- Canonical key glossary: [docs/specifications/log-args-key-glossary.md](specifications/log-args-key-glossary.md)
- Current runtime key inventory (human): [docs/specifications/log-args-inventory.md](specifications/log-args-inventory.md)
- Current runtime key inventory (machine): [docs/specifications/log-args-inventory.json](specifications/log-args-inventory.json)
