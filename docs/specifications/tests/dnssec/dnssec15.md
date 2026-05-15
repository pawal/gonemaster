# DNSSEC15 (dnssec15)

Status: Final

## Purpose
- Verify presence and consistency of CDS and CDNSKEY RRsets and detect mismatches between the two at child nameservers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver sets from `methods.Method4` and `methods.Method5`.
  - CDS and CDNSKEY query responses with DNSSEC enabled.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build child nameserver set from Method4+Method5 and deduplicate by IP.
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
   - If both RRsets are non-empty, compare each CDS against CDNSKEY set and each CDNSKEY against CDS set; mark nameserver mismatch when no match is found.
6. Emit classification tags with `addresses`.
7. Emit `DS15_INCONSISTENT_CDS` when CDS RRsets differ across nameservers.
8. Emit `DS15_INCONSISTENT_CDNSKEY` when CDNSKEY RRsets differ across nameservers.
9. Emit `DS15_MISMATCH_CDS_CDNSKEY` for nameservers with CDS/CDNSKEY mismatch.
10. Emit `TEST_CASE_END`.

### Per-NS CDS/CDNSKEY Presence and Match (steps 2-10)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> probe
    probe : per-NS probe
    probe --> disabled : transport off
    probe --> query : transport on
    disabled : transport-disabled tag
    query : query CDS and CDNSKEY
    query --> emptyCheck
    emptyCheck : any non-empty?
    emptyCheck --> emptyAll : none
    emptyCheck --> classify : at least one
    emptyAll : no-CDS-CDNSKEY tag
    classify : per-NS classify
    classify --> cdsOnly : CDS no CDNSKEY
    classify --> cdnskeyOnly : CDNSKEY no CDS
    classify --> both : both present
    cdsOnly : CDS-only tag
    cdnskeyOnly : CDNSKEY-only tag
    both --> matchCheck
    matchCheck : content match?
    matchCheck --> mismatch : no match
    matchCheck --> bothOK : matches
    mismatch : mismatch tag
    bothOK : both-present tag
    mismatch --> done
    bothOK --> done
    cdsOnly --> done
    cdnskeyOnly --> done
    done : per-set consistency tags
    done --> [*]
    emptyAll --> [*]
    disabled --> [*]
{{< /mermaid >}}
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS15_HAS_CDNSKEY_NO_CDS` | Nameserver has non-empty CDNSKEY RRset and empty CDS RRset. |
| `DS15_HAS_CDS_AND_CDNSKEY` | Nameserver has both non-empty CDS and non-empty CDNSKEY RRsets. |
| `DS15_HAS_CDS_NO_CDNSKEY` | Nameserver has non-empty CDS RRset and empty CDNSKEY RRset. |
| `DS15_INCONSISTENT_CDNSKEY` | CDNSKEY RRsets are not identical across participating nameservers. |
| `DS15_INCONSISTENT_CDS` | CDS RRsets are not identical across participating nameservers. |
| `DS15_MISMATCH_CDS_CDNSKEY` | Nameserver has both RRsets but CDS/CDNSKEY matching checks failed. |
| `DS15_NO_CDS_CDNSKEY` | No non-empty CDS or CDNSKEY RRset found on participating nameservers. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`CDS`, `CDNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`CDS`, `CDNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
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
  - Upstream: CDS/CDNSKEY matching is described as "derived from the same DNSKEY" (or both delete). Gonemaster: mismatch check is based on CDS keytag vs CDNSKEY keytag equality (or both algorithm `0`), without deeper digest/material derivation checks.
  - Upstream: does not explicitly specify testcase boundary and transport-disabled debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Only nameservers that produced authoritative `NOERROR` responses are included in CDS/CDNSKEY set maps; others are silently ignored except for transport-disabled debug tags.
- RRset consistency comparison uses RR string signatures; ordering differences are normalized by sorting.
- Nameserver evaluation is deduplicated by IP.
