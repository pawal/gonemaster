# Zone11 (zone11)

Status: Final

## Purpose
- Validate SPF policy publication at zone apex:
  - ability to retrieve authoritative TXT data;
  - consistency of SPF policy sets across nameserver IPs;
  - single-policy expectation per nameserver IP;
  - SPF syntax and non-mail-domain policy handling.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - Nameserver resolution context is available for MethodsV2 calls.
- Required inputs:
  - Nameserver name/IP items from `methodsv2.GetDelNSNamesAndIPs` and `methodsv2.GetZoneNSNamesAndIPs`.
  - Apex TXT responses for SPF extraction.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: parallel nameserver query fanout.
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped.

## Algorithm And Decision Flow
1. Build nameserver set from MethodsV2 delegation+zone items, then group by distinct IP.
2. For each IP group (parallelized):
   - Skip disabled transports.
   - Query apex `TXT`.
   - Accept response only when response exists, `RCODE=NOERROR`, and `AA=true`.
   - Extract TXT records for apex, concatenate fragments per record, lowercase text, and keep only SPF records (`v=spf1` with end/space/tab boundary).
   - Store per-IP SPF policy list plus associated nameserver `name/ip` list.
3. If no IP produced an accepted authoritative response, emit `Z11_UNABLE_TO_CHECK_FOR_SPF`.
4. Else group per-IP policy sets by a normalized key:
   - If all policy keys are empty:
     - emit `Z11_NO_SPF_NON_MAIL_DOMAIN` for root/TLD/`.arpa` zones;
     - otherwise emit `Z11_NO_SPF_FOUND` (`domain`).
   - Else if more than one distinct policy-set key exists:
     - emit `Z11_INCONSISTENT_SPF_POLICIES`;
     - emit `Z11_DIFFERENT_SPF_POLICIES_FOUND` per policy-set group.
   - Else if any single IP has more than one SPF policy, emit `Z11_SPF_MULTIPLE_RECORDS`.
   - Else evaluate the single effective SPF policy text:
     - if syntax invalid, emit `Z11_SPF_SYNTAX_ERROR`;
     - if syntax valid and zone is root/TLD/`.arpa`:
       - emit `Z11_NULL_SPF_NON_MAIL_DOMAIN` for null SPF (`v=spf1 -all`);
       - else emit `Z11_NON_NULL_SPF_NON_MAIL_DOMAIN`;
     - if syntax valid and zone is regular mail domain, emit `Z11_SPF_SYNTAX_OK`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `Z11_DIFFERENT_SPF_POLICIES_FOUND` | A policy-set group is emitted during SPF inconsistency reporting. |
| `Z11_INCONSISTENT_SPF_POLICIES` | At least two distinct SPF policy-set groups exist across checked IPs. |
| `Z11_NO_SPF_FOUND` | No SPF policy found for a domain expected to carry mail policy. |
| `Z11_NO_SPF_NON_MAIL_DOMAIN` | No SPF policy found for root/TLD/`.arpa` domain class. |
| `Z11_NON_NULL_SPF_NON_MAIL_DOMAIN` | Non-null SPF policy found for root/TLD/`.arpa` domain class. |
| `Z11_NULL_SPF_NON_MAIL_DOMAIN` | Null SPF policy found for root/TLD/`.arpa` domain class. |
| `Z11_SPF_MULTIPLE_RECORDS` | At least one checked IP returned more than one SPF policy. |
| `Z11_SPF_SYNTAX_ERROR` | Effective SPF policy failed local syntax validation. |
| `Z11_SPF_SYNTAX_OK` | Effective SPF policy passed local syntax validation. |
| `Z11_UNABLE_TO_CHECK_FOR_SPF` | No nameserver IP yielded an authoritative TXT response suitable for SPF evaluation. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `Z11_DIFFERENT_SPF_POLICIES_FOUND` | `ns_list` | `string` | Semicolon-delimited nameserver `name/ip` list for one policy-set group. |
| `Z11_INCONSISTENT_SPF_POLICIES` | `-` | `-` | No arguments. |
| `Z11_NO_SPF_FOUND` | `domain` | `string` | Tested zone name. |
| `Z11_NO_SPF_NON_MAIL_DOMAIN` | `domain` | `string` | Tested zone name. |
| `Z11_NON_NULL_SPF_NON_MAIL_DOMAIN` | `domain` | `string` | Tested zone name. |
| `Z11_NULL_SPF_NON_MAIL_DOMAIN` | `domain` | `string` | Tested zone name. |
| `Z11_SPF_MULTIPLE_RECORDS` | `ns_list` | `string` | Semicolon-delimited nameserver `name/ip` list with multi-policy responses. |
| `Z11_SPF_SYNTAX_ERROR` | `ns_list` | `string` | Semicolon-delimited nameserver `name/ip` list used for evaluated policy. |
| `Z11_SPF_SYNTAX_ERROR` | `domain` | `string` | Tested zone name. |
| `Z11_SPF_SYNTAX_OK` | `domain` | `string` | Tested zone name. |
| `Z11_UNABLE_TO_CHECK_FOR_SPF` | `-` | `-` | No arguments. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `Z11_DIFFERENT_SPF_POLICIES_FOUND` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_INCONSISTENT_SPF_POLICIES` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_NO_SPF_FOUND` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_NO_SPF_NON_MAIL_DOMAIN` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_NON_NULL_SPF_NON_MAIL_DOMAIN` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_NULL_SPF_NON_MAIL_DOMAIN` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_SPF_MULTIPLE_RECORDS` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_SPF_SYNTAX_ERROR` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_SPF_SYNTAX_OK` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_UNABLE_TO_CHECK_FOR_SPF` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone11.md`](../../upstream/tests/Zone-TP/zone11.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: defines SPF syntax against RFC7208 ABNF semantics. Gonemaster: uses local `spfSyntaxOk`/`spfTermOk` checks, which are intentionally narrower and implementation-defined.
  - Upstream: does not explicitly define testcase boundary markers. Gonemaster: runtime emits shared `TEST_CASE_START`/`TEST_CASE_END`, but these markers are not part of current Zone11 metadata inventory.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Distinct nameserver names sharing one IP are grouped and represented together in `ns_list` outputs.
- TXT responses with authoritative `NOERROR` but without SPF TXT records are treated as empty-policy results.
- Runtime boundary markers (`TEST_CASE_START`/`TEST_CASE_END`) are emitted by shared testcase wrappers but omitted from current Zone11 metadata tag contract.
