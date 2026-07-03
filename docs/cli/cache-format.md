# Gonemaster Packet Cache File Format

This document specifies the on-disk format produced by `gonemaster --save`
and consumed by `gonemaster --restore`. It covers:

- the JSON schema,
- the checksum contract,
- how each cache kind is represented,
- strict vs. lenient parsing behavior.

The format is implemented by [`engine/cachefile`](../../engine/cachefile/cachefile.go).

## Compression

Cache files can optionally be stored as a single gzip stream wrapping the
same JSON document. The CLI selects compression in two ways:

- `gonemaster --save FILE.gz` - any save path with a case-insensitive `.gz`
  suffix is gzip-compressed automatically.
- `gonemaster --save FILE --save-compress` - explicit opt-in; useful when
  the file name does not end in `.gz`.

`--restore` does not need a matching flag: it sniffs the leading two bytes
of the file (`1f 8b`, the gzip magic per RFC 1952) and decompresses
transparently. Files saved without compression are read as plain JSON.

## Limits

`gonemaster --save FILE --save-max-entries N` (or
`cachefile.WithMaxEntries(n)`) refuses to write the file if the exported
cache would contain more than `N` entries, counted across all kinds. The
check runs before the file is written, so a rejected save never leaves a
partial file behind. The default of `0` disables the guardrail.

### Performance and scaling

Save and restore are both linear in the number of entries. The
[`cachefile` benchmarks](../../engine/cachefile/cachefile_bench_test.go)
(`go test ./engine/cachefile -bench BenchmarkCachefile`) give
representative figures for a nameserver-only cache on an Apple M4:

| Entries | Save (plain) | Save (gzip) | Restore |
|---------|--------------|-------------|---------|
| 1,000   | ~5 ms        | ~5 ms       | ~8 ms   |
| 10,000  | ~30 ms       | ~42 ms      | ~63 ms  |
| 100,000 | ~180 ms      | ~240 ms     | ~405 ms |

On-disk size is roughly 150-400 bytes per entry for plain JSON (a typical
cached response base64-encoded), so 100,000 entries is on the order of
15-25 MB uncompressed. `gzip` compresses this several-fold; the exact ratio
depends on how many distinct responses the cache holds (many identical
responses compress dramatically, a diverse cache much less), and it adds
roughly a third to save time.

The practical ceiling is memory, not CPU or disk: `Restore` decodes the
entire file and materializes every packet in memory before the run starts,
so peak memory scales with the file size (restoring 100,000 entries
allocates on the order of hundreds of MB). For very large caches, prefer
`--save-compress` for the on-disk footprint and `--save-max-entries` to
bound growth, or split the cache across multiple files.

## File layout

A single JSON object:

```json
{
  "format": "gonemaster.packet-cache",
  "version": 2,
  "checksum": "<lowercase sha-256 hex>",
  "entries": [ ... ]
}
```

| Field      | Type   | Required | Description                                                         |
|------------|--------|----------|---------------------------------------------------------------------|
| `format`   | string | yes      | Always `"gonemaster.packet-cache"`. Foreign formats are rejected.   |
| `version`  | number | yes      | Schema version. Currently `2`. Unknown versions are rejected.       |
| `checksum` | string | no (\*)  | Lowercase SHA-256 hex of the file with `checksum` blanked.          |
| `entries`  | array  | yes      | Ordered list of cache records, discriminated by `kind`.             |

(\*) `checksum` is always written by `--save` and always verified when
present. It is optional only so that hand-crafted fixtures can omit it;
omission is a warning in lenient mode and an error in strict mode.

## Checksum contract

The checksum covers the entire file with the `checksum` field itself set to
the empty string, serialized using `encoding/json.Marshal` (no indentation,
Go struct field order). It is lowercase hex SHA-256.

Verification rules:

- **Present and correct** → file accepted.
- **Present and incorrect** → `Import` / `Restore` return an error ("checksum mismatch"), regardless of mode.
- **Absent** → warning in lenient mode; error in strict mode.

The pretty-printed form produced by `Save` (indented JSON with a trailing
newline) includes `checksum`, but the checksum value itself is computed
against the canonical, non-indented, checksum-stripped form. Consumers that
re-serialize an already-validated file must recompute the checksum if they
want to preserve roundtripping.

## Entries

Every entry carries a `kind` discriminator that selects the remaining
fields:

- `"nameserver"` - one cached response for a specific nameserver IP.
- `"recursor"`   - one cached response for the internal recursor.
- `"asn"`        - one cached ASN lookup result for a queried IP.

Unknown `kind` values are warnings in lenient mode (the entry is skipped)
and errors in strict mode.

### `kind: "nameserver"`

```json
{
  "kind": "nameserver",
  "address": "192.0.2.53",
  "key": "example.com./A/IN",
  "answer_from": "192.0.2.53:53",
  "message": "<base64-encoded DNS wire packet>"
}
```

A cached empty (nil) response is encoded with `"no_message": true` and no
`message`:

```json
{
  "kind": "nameserver",
  "address": "192.0.2.53",
  "key": "example.com./A/IN",
  "no_message": true
}
```

| Field         | Type    | Required                         | Description                                            |
|---------------|---------|----------------------------------|--------------------------------------------------------|
| `address`     | string  | yes                              | Nameserver IP (IPv4 or IPv6). Parsed with `netip`.     |
| `key`         | string  | yes                              | Internal cache key; kept opaque to the file format.    |
| `message`     | string  | required unless `no_message`     | Base64 (std) of the wire-format DNS response packet.   |
| `answer_from` | string  | no                               | Observed source address of the response.               |
| `no_message`  | boolean | no                               | `true` for a cached empty/nil response.                |

### `kind: "recursor"`

```json
{
  "kind": "recursor",
  "name": "example.com",
  "qtype": "A",
  "qclass": "IN",
  "nameservers": [
    { "name": "ns1.example", "address": "192.0.2.1" },
    { "name": "ns2.example", "address": "2001:db8::1" }
  ],
  "message": "<base64-encoded DNS wire packet>"
}
```

An empty `nameservers` array (or omission) denotes a lookup that started
from the root servers.

| Field         | Type                   | Required | Description                                               |
|---------------|------------------------|----------|-----------------------------------------------------------|
| `name`        | string                 | yes      | Query name (normalized, trailing dot stripped).           |
| `qtype`       | string                 | yes      | DNS record type (uppercase, e.g., `A`, `NS`, `DS`).       |
| `qclass`      | string                 | no       | DNS class (defaults to `IN`).                             |
| `nameservers` | array of `{name,address}` | no    | Nameserver set consulted. Empty ⇒ started from root.      |
| `message`     | string                 | yes      | Base64 (std) of the wire-format DNS response packet.      |

The recursor cache only stores non-empty responses; there is no
`no_message` form for recursor entries.

### `kind: "asn"`

```json
{
  "kind": "asn",
  "ip": "192.0.2.10",
  "asns": [64496, 64497],
  "prefix": "192.0.2.0/24",
  "raw": "64496 64497 | 192.0.2.0/24 | US | arin | 2001-01-01",
  "code": "AS_FOUND"
}
```

| Field    | Type     | Required | Description                                                 |
|----------|----------|----------|-------------------------------------------------------------|
| `ip`     | string   | yes      | Queried IP address (IPv4 or IPv6). Parsed with `netip`.     |
| `asns`   | array    | no       | List of AS numbers. Empty for `EMPTY_ASN_SET` / `ERROR_ASN_DATABASE`. |
| `prefix` | string   | no       | Most-specific routed prefix (e.g., `192.0.2.0/24`). Parsed with `netip`. |
| `raw`    | string   | no       | Backend response line, captured verbatim for debugging.     |
| `code`   | string   | yes      | One of `AS_FOUND`, `EMPTY_ASN_SET`, `ERROR_ASN_DATABASE`.   |

ASN entries are stored on hit and on miss (`EMPTY_ASN_SET`,
`ERROR_ASN_DATABASE`), so a restored cache avoids both successful and
failed lookups.

## Strict vs. lenient parsing

`Import`, `Restore`, and `Load` accept functional options:

- `WithStrict()` - turn every non-fatal warning into an error.
- `WithWarnf(fn)` - route non-fatal warnings to a callback.

On the CLI, `--cache-strict` selects `WithStrict()` for both `--restore` and
`--cache-stats`.

In **lenient** mode (default) the following produce warnings but keep
processing:

- Missing `checksum`.
- Unknown top-level field (e.g., a future `stats` section).
- Unknown field inside an entry.
- Entry with an empty or unknown `kind` (the entry is skipped).

In **strict** mode the same conditions are hard errors.

Any base64 decode failure, DNS unpack failure, invalid IP address, version
mismatch, or checksum mismatch is always a hard error regardless of mode.

## Minimal example

```json
{
  "format": "gonemaster.packet-cache",
  "version": 2,
  "checksum": "0000000000000000000000000000000000000000000000000000000000000000",
  "entries": [
    {
      "kind": "nameserver",
      "address": "192.0.2.53",
      "key": "example.com./A/IN",
      "answer_from": "192.0.2.53:53",
      "message": "PQ4BAAABAAEAAAAAB2V4YW1wbGUDY29tAAABAAE="
    },
    {
      "kind": "recursor",
      "name": "example.net",
      "qtype": "NS",
      "qclass": "IN",
      "nameservers": [
        { "name": "ns1.example", "address": "192.0.2.1" }
      ],
      "message": "E+6BAAABAAAAAAAAB2V4YW1wbGUDbmV0AAACAAE="
    },
    {
      "kind": "asn",
      "ip": "192.0.2.53",
      "asns": [64496],
      "prefix": "192.0.2.0/24",
      "code": "AS_FOUND"
    }
  ]
}
```

Replace the placeholder `checksum` with the real SHA-256 when writing a
file by hand, or let `gonemaster --save` produce it.
