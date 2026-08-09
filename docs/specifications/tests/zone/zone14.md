# Zone14

Status: Draft

## Purpose
- Check existence and RFC 8976 compliance of the ZONEMD RR at the zone apex.
- Detect cross-nameserver inconsistencies (mixed presence, divergent content).
- Detect intra-RRset structural defects (duplicate `(Scheme, Hash)` pairs).
- Detect ZONEMD `Serial` mismatches against the current SOA serial of the same nameserver.
- Detect ZONEMD records that publish a hash algorithm outside the IANA-assigned set verifiable by standard tooling (`1` = SHA-384, `2` = SHA-512).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - ZONEMD and SOA responses from authoritative nameservers at the zone apex.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
3. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for query type `ZONEMD`, then skip.
   - Send ZONEMD query to the zone apex with default query options.
   - If no response, or response is not authoritative NOERROR, skip this nameserver silently.
   - Else collect the ZONEMD RRs from the answer section (zero or more records).
   - Send SOA query to the same nameserver and record the SOA serial for comparison (mark unavailable if the query fails or returns no SOA RR).
4. Post-processing (sequential, over all collected outcomes):
   - For each nameserver that returned an authoritative NOERROR response:
     - If the ZONEMD RRset is empty, collect the nameserver for consolidated `Z14_NO_ZONEMD`.
     - Else:
       - Build a `(Scheme, Hash)` multiset across the records on this nameserver. For every pair that appears more than once, emit `Z14_DUPLICATE_SCHEME_HASH` (`ns`, `address`, `scheme`, `hash`) once per distinct duplicated pair.
       - Build a hash-algorithm set across the records on this nameserver. For every distinct hash value that is not in `{1, 2}` (SHA-384, SHA-512), emit `Z14_UNSUPPORTED_HASH` (`ns`, `address`, `hash`) once per `(ns, hash)` pair.
       - For each ZONEMD record on this nameserver:
         - Group nameservers for consolidated `Z14_ZONEMD_FOUND` by record content (`serial`, `scheme`, `hash`, `digest`).
         - If the SOA serial was retrieved and `record.Serial != soa.Serial`, emit `Z14_SERIAL_MISMATCH` (`ns`, `address`, `zonemd_serial`, `soa_serial`).
       - Compute an NS-level consistency key from the canonical sorted concatenation of all `(Serial, Scheme, Hash, Digest)` tuples on this nameserver and add it to the cross-nameserver key set.
   - Emit consolidated `Z14_ZONEMD_FOUND` for each distinct ZONEMD record content group, with `servers` list, `serial`, `scheme`, `hash`, and `digest`.
   - Emit consolidated `Z14_NO_ZONEMD` once with the `servers` list of nameservers without ZONEMD.
   - If at least one nameserver has ZONEMD and at least one has no ZONEMD, emit `Z14_MIXED_PRESENCE`.
   - If more than one nameserver has ZONEMD and the NS-level consistency keys differ across them, emit `Z14_INCONSISTENT_ZONEMD`.
5. Emit `TEST_CASE_END`.

ZONEMD record identity (for `Z14_ZONEMD_FOUND` consolidation) is determined by the tuple `(Serial, Scheme, Hash, Digest)`. Per-nameserver consistency identity (for `Z14_INCONSISTENT_ZONEMD`) is determined by the canonical sorted concatenation of all `(Serial, Scheme, Hash, Digest)` tuples on that nameserver.

### Per-NS ZONEMD Probe and Aggregation (steps 2-5)

{{% expand "Show diagram" %}}
```
ns list = ZoneNameservers

For each nameserver (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for ZONEMD -> IPV4_DISABLED / IPV6_DISABLED, skip
   query ZONEMD at z.Name
    +- no resp / not authoritative NOERROR -> skip silently
    +- otherwise                            -> capture ZONEMD RRs in answer
                                               (may be zero)
   query SOA at z.Name (same NS)            -> capture SOA serial when available

After all queries, per ns with auth NOERROR ZONEMD response:

   ZONEMD RRset empty
      -> add ns to noZonemdGroup
   ZONEMD RRset non-empty:
      build (scheme, hash) multiset over this ns's records
         every pair appearing >1 times
            -> Z14_DUPLICATE_SCHEME_HASH (ns, address, scheme, hash)
               (one per distinct duplicated pair)
      build hash-algorithm set over this ns's records
         every hash NOT in {1 (SHA-384), 2 (SHA-512)}
            -> Z14_UNSUPPORTED_HASH (ns, address, hash)
               (one per distinct ns/hash pair)
      per record on this ns:
         group ns into contentGroup by (serial, scheme, hash, digest)
         SOA serial was captured AND record.Serial != soa.Serial
            -> Z14_SERIAL_MISMATCH (ns, address, zonemd_serial, soa_serial)
      compute ns-level consistencyKey =
         canonical sorted concat of all (serial, scheme, hash, digest)
         tuples on this ns; add to consistencyKeySet

Aggregate emissions:
   per contentGroup (distinct ZONEMD record content)
      -> Z14_ZONEMD_FOUND (servers, serial, scheme, hash, digest)
   noZonemdGroup non-empty
      -> Z14_NO_ZONEMD (servers)
   any ns with ZONEMD AND any ns with none
      -> Z14_MIXED_PRESENCE
   more than one ns with ZONEMD AND consistencyKeySet has > 1 entries
      -> Z14_INCONSISTENT_ZONEMD

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `Z14_DUPLICATE_SCHEME_HASH` | Two or more ZONEMD records on the same nameserver share the same `(Scheme, Hash)` pair. |
| `Z14_INCONSISTENT_ZONEMD` | ZONEMD content differs across authoritative nameservers. |
| `Z14_MIXED_PRESENCE` | ZONEMD present on some nameservers but absent on others. |
| `Z14_NO_ZONEMD` | No ZONEMD record found at zone apex (consolidated across all nameservers without ZONEMD). |
| `Z14_SERIAL_MISMATCH` | ZONEMD `Serial` differs from the current SOA serial of the same nameserver. |
| `Z14_UNSUPPORTED_HASH` | ZONEMD hash algorithm is outside `{1 = SHA-384, 2 = SHA-512}` (consolidated per `(ns, hash)` pair). |
| `Z14_ZONEMD_FOUND` | ZONEMD record found at zone apex (consolidated per distinct record content). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | Query type skipped (`ZONEMD`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | Query type skipped (`ZONEMD`). |
| `Z14_DUPLICATE_SCHEME_HASH` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `Z14_DUPLICATE_SCHEME_HASH` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `Z14_DUPLICATE_SCHEME_HASH` | `scheme` | `uint8` | The duplicated ZONEMD `Scheme` value (`1` = SIMPLE, `240-254` = private use). |
| `Z14_DUPLICATE_SCHEME_HASH` | `hash` | `uint8` | The duplicated ZONEMD hash algorithm value (`1` = SHA-384, `2` = SHA-512, `240-254` = private use). |
| `Z14_INCONSISTENT_ZONEMD` | `-` | `-` | No arguments. |
| `Z14_MIXED_PRESENCE` | `-` | `-` | No arguments. |
| `Z14_NO_ZONEMD` | `servers` | `array<object>` | Structured list of nameserver `{ns, address}` items without ZONEMD. |
| `Z14_SERIAL_MISMATCH` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `Z14_SERIAL_MISMATCH` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `Z14_SERIAL_MISMATCH` | `zonemd_serial` | `uint32` | `Serial` field carried in the ZONEMD record. |
| `Z14_SERIAL_MISMATCH` | `soa_serial` | `uint32` | Current SOA serial from the same nameserver. |
| `Z14_UNSUPPORTED_HASH` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `Z14_UNSUPPORTED_HASH` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `Z14_UNSUPPORTED_HASH` | `hash` | `uint8` | The unsupported hash algorithm value (anything not in `{1 = SHA-384, 2 = SHA-512}`; includes reserved values `0`, `255`, the unassigned range `3-239`, and the private-use range `240-254`). |
| `Z14_ZONEMD_FOUND` | `servers` | `array<object>` | Structured sorted list of nameservers `{ns, address}` presenting this ZONEMD record content. |
| `Z14_ZONEMD_FOUND` | `serial` | `uint32` | ZONEMD `Serial` field. |
| `Z14_ZONEMD_FOUND` | `scheme` | `uint8` | ZONEMD `Scheme` field (`1` = SIMPLE, `240-254` = private use). |
| `Z14_ZONEMD_FOUND` | `hash` | `uint8` | ZONEMD hash algorithm field (`1` = SHA-384, `2` = SHA-512, `240-254` = private use). |
| `Z14_ZONEMD_FOUND` | `digest` | `string` | Lowercase hex-encoded `Digest` field. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone14`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone14`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z14_DUPLICATE_SCHEME_HASH` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z14_INCONSISTENT_ZONEMD` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z14_MIXED_PRESENCE` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z14_NO_ZONEMD` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). ZONEMD is optional per RFC 8976. |
| `Z14_SERIAL_MISMATCH` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z14_UNSUPPORTED_HASH` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). RFC 8976 permits private-use values, but standard verifiers cannot process anything outside `{1, 2}`. |
| `Z14_ZONEMD_FOUND` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- No upstream (Zonemaster) equivalent exists; this is a gonemaster-specific test case.
- References: [RFC 8976](https://datatracker.ietf.org/doc/html/rfc8976)
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- ZONEMD is an optional zone apex record per RFC 8976; `Z14_NO_ZONEMD` is informational only and does not indicate a problem.
- Only authoritative NOERROR responses are evaluated. Nameservers returning non-NOERROR or non-AA responses are skipped silently.
- Multiple ZONEMD records with distinct `(Scheme, Hash)` pairs at the zone apex are valid per RFC 8976 §2.2.4 and emit one `Z14_ZONEMD_FOUND` per distinct record content; only repeated `(Scheme, Hash)` pairs produce `Z14_DUPLICATE_SCHEME_HASH`.
- Hash algorithm `1` (SHA-384) MUST be supported by verifiers; algorithm `2` (SHA-512) SHOULD be supported. Any other value (reserved `0` and `255`, unassigned `3-239`, private-use `240-254`) emits `Z14_UNSUPPORTED_HASH` once per `(ns, hash)` pair; the underlying records are still surfaced via `Z14_ZONEMD_FOUND` so the digest content remains visible. `Scheme` values outside `{1 = SIMPLE}` are not flagged separately; only the hash algorithm is gated, since `Scheme` only affects canonicalization and the digest itself is not verified by this testcase.
- `Digest` content is **not** verified. Only structural fields (`Serial`, `Scheme`, `Hash`, presence) and cross-nameserver content equality are evaluated. Full digest verification would require performing an AXFR and recomputing the digest over the canonical zone, which is out of scope for this testcase.
- `Z14_SERIAL_MISMATCH` is only evaluated when the SOA query to the same nameserver succeeds and returns a SOA record. If the SOA query fails, no mismatch is reported for that nameserver.
- The cross-nameserver consistency key includes every ZONEMD record on a nameserver (including duplicates), so a nameserver carrying duplicated `(Scheme, Hash)` pairs will appear inconsistent with peers that publish a clean RRset of the same logical content.
- Distinct nameserver names sharing one IP are grouped per (`ns`, `address`) pair in `servers` outputs, matching the convention of the rest of the Zone module.
