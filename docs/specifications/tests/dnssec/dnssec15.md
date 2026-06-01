# DNSSEC15

Status: Final

## Purpose
- Verify presence and consistency of CDS and CDNSKEY RRsets and detect mismatches between the two at child nameservers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver sets from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - CDS and CDNSKEY query responses with DNSSEC enabled.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build child nameserver set from the union of [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) (deduplicated by `ns.String()`), then deduplicate by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `CDS` and `CDNSKEY` and skip.
   - Query `CDS` with DNSSEC enabled; when response is authoritative `NOERROR`, store CDS answer RRset (which may contain zero records).
   - Query `CDNSKEY` with DNSSEC enabled; when response is authoritative `NOERROR`, store CDNSKEY answer RRset (which may contain zero records).
4. If no non-empty CDS/CDNSKEY RRset exists anywhere, emit `DS15_NO_CDS_CDNSKEY`.
5. Otherwise, per nameserver present in both RRset maps:
   - Classify nameserver into:
     - `Has CDS No CDNSKEY` (non-empty CDS, empty CDNSKEY),
     - `Has CDNSKEY No CDS` (non-empty CDNSKEY, empty CDS),
     - `Has CDS And CDNSKEY` (both non-empty).
   - Build a digest-filtered CDS view that retains only records whose digest type is designated MUST in the IANA "Implement for DNSSEC Delegation" column (currently SHA-256 and SHA-384), plus any RFC 8078 delete-signal record (algorithm 0, digest type 0, key tag 0). The classification step above operates on the unfiltered RRset; all later cross-server and cross-RRset consistency checks operate on the filtered view per RFC 9975.
   - If both the filtered CDS and the CDNSKEY RRsets are non-empty for a nameserver, compare each CDS against the CDNSKEY set and each CDNSKEY against the CDS set; a pair is considered to reference the same key when both the key tag and the DNSSEC algorithm match, or when both records use algorithm `0` (delete signal). Mark nameserver mismatch when no match is found.
6. Emit classification tags with `addresses`.
7. Emit `DS15_INCONSISTENT_CDS` when the digest-filtered CDS RRsets differ across nameservers.
8. Emit `DS15_INCONSISTENT_CDNSKEY` when CDNSKEY RRsets differ across nameservers.
9. Emit `DS15_MISMATCH_CDS_CDNSKEY` for nameservers with CDS/CDNSKEY mismatch.
10. Emit `DS15_CDS_NON_MUST_DIGEST` for nameservers that returned at least one CDS record with a non-MUST digest type (records that the consistency check already excluded).
11. Emit `TEST_CASE_END`.

### Per-NS CDS/CDNSKEY Presence and Match (steps 2-10)

{{% expand "Show diagram" %}}
```
child set = GlueNameservers ++ ApexNameservers; dedupe by IP

For each unique child NS IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for CDS/CDNSKEY -> IPV4_DISABLED / IPV6_DISABLED, skip
   query CDS at z.Name, DNSSEC=on
     resp.Msg present AND RCODE == NOERROR AND AA
        -> cdsByNS[ns] = CDS records in answer (may be empty)
   query CDNSKEY at z.Name, DNSSEC=on
     resp.Msg present AND RCODE == NOERROR AND AA
        -> cdnskeyByNS[ns] = CDNSKEY records in answer (may be empty)

After all tasks:
   no non-empty CDS AND no non-empty CDNSKEY across any NS
      -> DS15_NO_CDS_CDNSKEY

Per NS present in both maps:
   non-empty CDS,     empty     CDNSKEY  -> hasCDSNoCDNSKEY[ns]
   empty     CDS,     non-empty CDNSKEY  -> hasCDNSKEYNoCDS[ns]
   non-empty CDS AND non-empty CDNSKEY   -> hasCDSAndCDNSKEY[ns]
        cdsForConsistency[ns] = filter cdsByNS[ns] to MUST digest types
                                (RFC 9975), preserving the RFC 8078 delete
                                signal (0/0/0)
        for each filtered CDS, search CDNSKEY by (keytag AND algorithm)
          or both algorithm == 0
        for each CDNSKEY, search filtered CDS by (keytag AND algorithm)
          or both algorithm == 0
        any unmatched on either side    -> mismatchCDSCDNSKEY[ns]

Emit:
  hasCDSNoCDNSKEY    non-empty -> DS15_HAS_CDS_NO_CDNSKEY   (addresses)
  hasCDNSKEYNoCDS    non-empty -> DS15_HAS_CDNSKEY_NO_CDS   (addresses)
  hasCDSAndCDNSKEY   non-empty -> DS15_HAS_CDS_AND_CDNSKEY  (addresses)
  cdsForConsistency differ across NS
                               -> DS15_INCONSISTENT_CDS     (no args)
  CDNSKEY RRsets differ        -> DS15_INCONSISTENT_CDNSKEY (no args)
  mismatchCDSCDNSKEY non-empty -> DS15_MISMATCH_CDS_CDNSKEY (addresses)
  cdsNonMUSTDigest   non-empty -> DS15_CDS_NON_MUST_DIGEST  (addresses)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS15_CDS_NON_MUST_DIGEST` | Nameserver returned at least one CDS record whose digest type is not designated MUST by IANA. |
| `DS15_HAS_CDNSKEY_NO_CDS` | Nameserver has non-empty CDNSKEY RRset and empty CDS RRset. |
| `DS15_HAS_CDS_AND_CDNSKEY` | Nameserver has both non-empty CDS and non-empty CDNSKEY RRsets. |
| `DS15_HAS_CDS_NO_CDNSKEY` | Nameserver has non-empty CDS RRset and empty CDNSKEY RRset. |
| `DS15_INCONSISTENT_CDNSKEY` | CDNSKEY RRsets are not identical across participating nameservers. |
| `DS15_INCONSISTENT_CDS` | Digest-filtered CDS RRsets are not identical across participating nameservers. |
| `DS15_MISMATCH_CDS_CDNSKEY` | Nameserver has both RRsets but CDS/CDNSKEY matching checks failed. |
| `DS15_NO_CDS_CDNSKEY` | No non-empty CDS or CDNSKEY RRset found on participating nameservers. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`CDS`, `CDNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`CDS`, `CDNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS15_CDS_NON_MUST_DIGEST` | `addresses` | `array<string>` | Structured child nameserver IP list that returned at least one non-MUST CDS digest. |
| `DS15_HAS_CDNSKEY_NO_CDS` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS15_HAS_CDS_AND_CDNSKEY` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS15_HAS_CDS_NO_CDNSKEY` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS15_INCONSISTENT_CDNSKEY` | `-` | `-` | No arguments. |
| `DS15_INCONSISTENT_CDS` | `-` | `-` | No arguments. |
| `DS15_MISMATCH_CDS_CDNSKEY` | `addresses` | `array<string>` | Structured child nameserver IP list with mismatch. |
| `DS15_NO_CDS_CDNSKEY` | `-` | `-` | No arguments. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`CDS` or `CDNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`CDS` or `CDNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC15`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC15`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS15_CDS_NON_MUST_DIGEST` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS15_HAS_CDNSKEY_NO_CDS` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS15_HAS_CDS_AND_CDNSKEY` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS15_HAS_CDS_NO_CDNSKEY` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS15_INCONSISTENT_CDNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS15_INCONSISTENT_CDS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS15_MISMATCH_CDS_CDNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS15_NO_CDS_CDNSKEY` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec15.md`](../../upstream/tests/DNSSEC-TP/dnssec15.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: CDS/CDNSKEY matching is described as "derived from the same DNSKEY" (or both delete). Gonemaster: mismatch check pairs records when both the key tag and the DNSSEC algorithm match (or both records use algorithm `0`), without deeper digest/material derivation checks. Key tag alone is insufficient because two keys with different algorithms can share a key tag.
  - Upstream: does not explicitly specify testcase boundary and transport-disabled debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## RFC References
- RFC 7344: defines CDS and CDNSKEY semantics for child-to-parent DS maintenance.
- RFC 8078: defines the delete signal (algorithm `0`, digest type `0`, key tag `0`) that asks the parent to remove the DS RRset.
- RFC 9975: clarifies CDS/CDNSKEY consistency rules for parental agents. Gonemaster applies the digest-type filter from RFC 9975 to its consistency checks: only CDS records whose digest type is designated MUST in the IANA "Implement for DNSSEC Delegation" column participate in `DS15_INCONSISTENT_CDS` and `DS15_MISMATCH_CDS_CDNSKEY`. The delete signal is preserved across the filter so that mixed delete/update responses still surface as inconsistent.

## Edge Cases And Limitations
- Only nameservers that produced authoritative `NOERROR` responses are included in CDS/CDNSKEY set maps; others are silently ignored except for transport-disabled debug tags.
- RRset consistency comparison uses RR string signatures over the digest-filtered CDS view; ordering differences are normalized by sorting.
- CDS records whose digest type is not designated MUST by IANA are excluded from consistency checks per RFC 9975. They still appear in the underlying DNS responses; only the consistency comparison ignores them.
- Nameserver evaluation is deduplicated by IP.
