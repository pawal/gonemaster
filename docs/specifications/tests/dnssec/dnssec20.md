# DNSSEC20

Status: Draft

## Purpose
- Verify that the NSEC/NSEC3 apex type bitmap accurately reflects the RR types actually present in the zone.
- An incomplete (subset) bitmap enables cache poisoning via RFC 8198 aggressive negative caching and replay attacks (see Petr Špaček, ISC, 2021-11-30, "Type Bitmap: Subset - Broken").

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver name/IP items from [`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers) and [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - DNSKEY query response (DNSSEC enabled) to confirm zone is signed. In a full test run, this response is cached from earlier test cases (e.g., DNSSEC10); when running standalone, the query is made directly.
  - NSEC or NSEC3PARAM query response to obtain the apex type bitmap. Also typically cached from DNSSEC10.
  - Query responses for probed RR types: A, AAAA, MX, TXT. Typically cached from earlier test cases (Basic, Zone, Connectivity).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: per-nameserver parallel execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build nameserver set from delegation+zone NS items, grouped by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DNSKEY` and skip.
   - Query child apex `DNSKEY` with DNSSEC enabled.
   - If response is absent, non-`NOERROR`, or non-`AA`, classify nameserver as `no_dnssec` and skip.
   - If no apex DNSKEY records are present, classify nameserver as `no_dnssec` and skip.
   - Query `NSEC` (DNSSEC enabled) to obtain the apex type bitmap:
     - If NSEC record for the apex is present in the answer section, extract its type bitmap (NSEC zone).
     - Else if NSEC3 record matching the apex hash is present in the authority section, extract its type bitmap (NSEC3 zone).
   - If no bitmap obtained from the NSEC query, try `NSEC3PARAM` query and look for an NSEC record in the authority section (NODATA response).
   - If no bitmap could be obtained, classify nameserver as `no_bitmap` and skip.
   - For each probed type (A, AAAA, MX, TXT):
     - Query the apex for that type with DNSSEC enabled.
     - If the response is `NOERROR` with at least one matching RR for the apex name in the answer section, the type exists.
     - If the type exists but is NOT present in the type bitmap, record a bitmap mismatch for that type.
4. Aggregate results across nameservers:
   - For each probed type with mismatches, emit `DS20_NSEC_BITMAP_MISMATCHES_RRTYPE` (NSEC zones) or `DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE` (NSEC3 zones) with merged `servers`.
   - If all nameservers with DNSSEC show correct bitmaps, emit `DS20_BITMAP_OK`.
   - If nameservers had DNSSEC but no bitmap could be obtained, emit `DS20_NO_BITMAP`.
   - If no nameserver had DNSSEC (and no other findings), emit `DS20_NO_DNSSEC`.
5. Emit `TEST_CASE_END`.

### Per-NS Bitmap Probe and Aggregation (steps 2-5)

{{% expand "Show diagram" %}}
```
nss = DelegationNameservers ++ ZoneNameservers;
      group by IP

For each unique nameserver IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for DNSKEY -> IPV4_DISABLED / IPV6_DISABLED, skip
   query DNSKEY at z.Name, DNSSEC=on
    +- resp.Msg == nil / RCODE != NOERROR / !AA -> classify ns as no_dnssec
    +- no apex DNSKEY in answer                 -> classify ns as no_dnssec
    +- apex DNSKEY present                      -> continue

   obtain apex type bitmap:
     query NSEC at z.Name, DNSSEC=on
       NSEC for apex in answer    -> bitmap from NSEC (mark zone style NSEC)
       NSEC3 for apex in authority -> bitmap from NSEC3 (mark zone style NSEC3)
     no bitmap yet:
       query NSEC3PARAM at z.Name, DNSSEC=on
         NSEC for apex in authority (NODATA) -> bitmap from NSEC

   no bitmap obtained                          -> classify ns as no_bitmap
   bitmap obtained:
     for each probed type in {A, AAAA, MX, TXT}:
       query type at z.Name, DNSSEC=on
         RCODE NOERROR with matching apex RR in answer -> type exists
         type exists AND type missing from bitmap
            -> record mismatch (kind, query_type, ns)

Aggregate:
   per (kind, query_type), mismatches non-empty
      -> DS20_NSEC_BITMAP_MISMATCHES_RRTYPE  (query_type, servers)
         or
         DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE (query_type, servers)
   NS with bitmap AND zero mismatches non-empty
      -> DS20_BITMAP_OK (servers)
   NS classified no_bitmap non-empty
      -> DS20_NO_BITMAP (servers)
   no NS with DNSSEC observed
      -> DS20_NO_DNSSEC (servers = no_dnssec)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS20_BITMAP_OK` | All probed types present in the zone are correctly represented in the NSEC/NSEC3 type bitmap on all responding nameservers. |
| `DS20_NO_BITMAP` | Nameserver has DNSKEY but no NSEC/NSEC3 bitmap could be obtained for the apex. |
| `DS20_NO_DNSSEC` | No nameserver returned a usable DNSKEY; bitmap check skipped. |
| `DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE` | An existing RR type at the apex is missing from the NSEC3 type bitmap. Enables cache poisoning via RFC 8198. |
| `DS20_NSEC_BITMAP_MISMATCHES_RRTYPE` | An existing RR type at the apex is missing from the NSEC type bitmap. Enables cache poisoning via RFC 8198. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS20_BITMAP_OK` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with correct bitmaps. |
| `DS20_NO_BITMAP` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) where no bitmap was obtained. |
| `DS20_NO_DNSSEC` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) without DNSKEY. |
| `DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE` | `query_type` | `string` | RR type name missing from the NSEC3 bitmap (e.g., `A`, `AAAA`). |
| `DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) exhibiting the mismatch. |
| `DS20_NSEC_BITMAP_MISMATCHES_RRTYPE` | `query_type` | `string` | RR type name missing from the NSEC bitmap (e.g., `A`, `AAAA`). |
| `DS20_NSEC_BITMAP_MISMATCHES_RRTYPE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) exhibiting the mismatch. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | rrtype skipped (`DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | rrtype skipped (`DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC20`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC20`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS20_BITMAP_OK` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS20_NO_BITMAP` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS20_NO_DNSSEC` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS20_NSEC_BITMAP_MISMATCHES_RRTYPE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: No upstream Zonemaster equivalent. DNSSEC20 is a new testcase unique to Gonemaster.
- Differences (Upstream vs Gonemaster):
  - Upstream: no testcase verifies NSEC/NSEC3 type bitmap accuracy against actual RR types present. DNSSEC10 checks structural correctness of the bitmap (mandatory/forbidden types) but not completeness against observed data. Gonemaster: implements dynamic bitmap accuracy verification by probing common apex RR types and comparing against the bitmap.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameserver evaluation is deduplicated by IP; multiple names mapped to one IP are merged into one query outcome and expanded in `servers`.
- Nameservers with non-`NOERROR` or non-`AA` DNSKEY responses are treated as `no_dnssec` and can only contribute to `DS20_NO_DNSSEC`.
- The set of probed types (A, AAAA, MX, TXT) is intentionally limited to common apex types. DNSKEY, SOA, NS, RRSIG, and NSEC/NSEC3 are already validated structurally by DNSSEC10 and are not re-checked here.
- Super-set bitmaps (types listed in the bitmap but not actually present) are not flagged - only subset bitmaps (existing types missing from the bitmap) are checked, as these are the security-relevant case per the ISC presentation.
- For NSEC3 zones, the NSEC3 record must have an owner hash matching the apex for the bitmap to be used; non-matching NSEC3 records are ignored.
- All queries in DNSSEC20 benefit from the per-nameserver query cache. In a full test run (after DNSSEC10, Basic, Zone, etc.), most queries are cache hits with zero network overhead.

## Evidence In Gonemaster
- Code paths:
  - `engine/test/dnssec/dnssec.go` (DNSSEC20 testcase function)
- Related tests:
  - `engine/test/dnssec/dnssec_test.go` (TestDNSSEC20BitmapOK, TestDNSSEC20NSECSubsetBitmap, TestDNSSEC20NSEC3SubsetBitmap, TestDNSSEC20NoDNSSEC)
- References:
  - RFC 4034 (NSEC type bitmap format)
  - RFC 5155 (NSEC3 type bitmap format)
  - RFC 8198 (Aggressive Use of DNSSEC-Validated Cache)
  - Petr Špaček, "NSEC* type bitmap discrepancies", OARC 35, 2021-11-30
