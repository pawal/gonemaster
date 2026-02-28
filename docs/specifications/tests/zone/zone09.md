# Zone09 (zone09)

Status: Final

## Purpose
- Validate MX presence and consistency across authoritative nameservers, including null-MX and domain-class exceptions (root/TLD/.arpa).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - SOA and MX responses per nameserver.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: parallel nameserver query fanout.
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Collect nameservers from `Method4and5`, deduplicate probing list by IP address, and keep a name-to-IP grouping view for later reporting.
3. For each unique nameserver IP (parallelized):
   - Skip disabled transports.
   - Query apex `SOA`; continue only when response exists, `RCODE=NOERROR`, `AA=true`, and SOA answer exists.
   - Query apex `MX` over UDP with `fallback=false`; if truncated, retry with TCP (`usevc=true`, `fallback=false`).
   - Classify outcome into one bucket:
     - no response;
     - unexpected RCODE;
     - non-authoritative;
     - no MX RRset;
     - MX RRset data.
4. Emit aggregate operational tags:
   - `Z09_NO_RESPONSE_MX_QUERY` per no-response IP set;
   - `Z09_UNEXPECTED_RCODE_MX` per RCODE group;
   - `Z09_NON_AUTH_MX_RESPONSE` when non-authoritative responses exist.
5. If both “no MX” and “MX RRset” buckets are non-empty, emit:
   - `Z09_INCONSISTENT_MX`
   - `Z09_NO_MX_FOUND`
   - `Z09_MX_FOUND`
6. If MX RRset bucket is non-empty:
   - Compare RRset content across IPs using normalized lowercase wire-text ordering.
   - If inconsistent RRset content, emit `Z09_INCONSISTENT_MX_DATA` and emit one or more `Z09_MX_DATA` entries grouped by nameserver-name view.
   - If consistent RRset content:
     - evaluate null-MX conditions:
       - `Z09_NULL_MX_WITH_OTHER_MX` when `.` mailtarget is mixed with other MX RRs;
       - `Z09_NULL_MX_NON_ZERO_PREF` when null-MX preference is not zero;
     - if no null-MX:
       - emit `Z09_ROOT_EMAIL_DOMAIN` for root zone;
       - emit `Z09_TLD_EMAIL_DOMAIN` for TLD zone;
       - otherwise emit `Z09_MX_DATA`.
7. If MX RRset bucket is empty and “no MX” bucket is non-empty:
   - If zone is NOT root, TLD, or under `.arpa`: emit `Z09_MISSING_MAIL_TARGET`.
   - Otherwise (zone is root, TLD, or under `.arpa`): emit no tag.
8. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `Z09_INCONSISTENT_MX` | Some authoritative nameserver IPs return MX RRset while others return none. |
| `Z09_INCONSISTENT_MX_DATA` | Authoritative MX RRset content differs across responding nameserver IPs. |
| `Z09_MISSING_MAIL_TARGET` | No authoritative MX RRset was found and zone is not exempt (non-root, non-TLD, non-`.arpa`). |
| `Z09_MX_DATA` | MX mailtarget data is reported for one reporting group. |
| `Z09_MX_FOUND` | At least one authoritative nameserver IP returned MX RRset. |
| `Z09_NON_AUTH_MX_RESPONSE` | At least one nameserver IP returned non-authoritative MX response after SOA gating. |
| `Z09_NO_MX_FOUND` | At least one authoritative nameserver IP returned no MX RRset. |
| `Z09_NO_RESPONSE_MX_QUERY` | At least one nameserver IP gave no MX response after SOA gating. |
| `Z09_NULL_MX_NON_ZERO_PREF` | Null MX (`.` mailtarget) was returned with non-zero preference. |
| `Z09_NULL_MX_WITH_OTHER_MX` | Null MX (`.` mailtarget) is mixed with other MX records. |
| `Z09_ROOT_EMAIL_DOMAIN` | Root zone has non-null MX data. |
| `Z09_TLD_EMAIL_DOMAIN` | TLD zone has non-null MX data. |
| `Z09_UNEXPECTED_RCODE_MX` | At least one nameserver IP returned non-`NOERROR` RCODE for MX query. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone09`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone09`). |
| `Z09_INCONSISTENT_MX` | `-` | `-` | No arguments. |
| `Z09_INCONSISTENT_MX_DATA` | `-` | `-` | No arguments. |
| `Z09_MISSING_MAIL_TARGET` | `-` | `-` | No arguments. |
| `Z09_MX_DATA` | `ns_ip_list` | `string` | Semicolon-delimited nameserver IP list for this data group. |
| `Z09_MX_DATA` | `mailtarget_list` | `string` | Semicolon-delimited MX exchange list. |
| `Z09_MX_FOUND` | `ns_ip_list` | `string` | Semicolon-delimited nameserver IPs that returned MX RRset. |
| `Z09_NON_AUTH_MX_RESPONSE` | `ns_ip_list` | `string` | Semicolon-delimited nameserver IPs reported as non-authoritative (see limitation below). |
| `Z09_NO_MX_FOUND` | `ns_ip_list` | `string` | Semicolon-delimited nameserver IPs with no MX RRset. |
| `Z09_NO_RESPONSE_MX_QUERY` | `ns_ip_list` | `string` | Semicolon-delimited nameserver IPs with no MX response. |
| `Z09_NULL_MX_NON_ZERO_PREF` | `-` | `-` | No arguments. |
| `Z09_NULL_MX_WITH_OTHER_MX` | `-` | `-` | No arguments. |
| `Z09_ROOT_EMAIL_DOMAIN` | `-` | `-` | No arguments. |
| `Z09_TLD_EMAIL_DOMAIN` | `-` | `-` | No arguments. |
| `Z09_UNEXPECTED_RCODE_MX` | `rcode` | `string` | Unexpected RCODE text. |
| `Z09_UNEXPECTED_RCODE_MX` | `ns_ip_list` | `string` | Semicolon-delimited nameserver IPs for that RCODE. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_INCONSISTENT_MX` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_INCONSISTENT_MX_DATA` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_MISSING_MAIL_TARGET` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_MX_DATA` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_MX_FOUND` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NON_AUTH_MX_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NO_MX_FOUND` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NO_RESPONSE_MX_QUERY` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NULL_MX_NON_ZERO_PREF` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NULL_MX_WITH_OTHER_MX` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_ROOT_EMAIL_DOMAIN` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_TLD_EMAIL_DOMAIN` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_UNEXPECTED_RCODE_MX` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone09.md`](../../upstream/tests/Zone-TP/zone09.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: defines `Z09_NON_AUTH_MX_RESPONSE` from the non-authoritative MX set. Gonemaster: currently populates `ns_ip_list` for this tag from the no-response set (`noResponseMX`), not the non-authoritative set (`nonAuthoritativeMX`).
  - Upstream: describes name server IP set processing. Gonemaster: deduplicates probing by IP before classification and separately keeps name-group reporting views.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- `Z09_NON_AUTH_MX_RESPONSE` currently reports the wrong source IP list due implementation behavior described above.
- Query results for transport-disabled nameservers are skipped; helper debug tags for skipped transports are outside this testcase metadata contract.
- `Z09_MX_DATA` can be emitted multiple times in the inconsistent-data branch and once in the consistent-data branch.
