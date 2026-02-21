# Connectivity01 (connectivity01)

Status: Draft

## Purpose
- Verify that nameservers are reachable over UDP for SOA and NS queries at the child zone name.
- Detect response-shape failures (missing records, wrong owner name, non-authoritative answers, and unexpected RCODEs).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - Child zone name (`z.Name`).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped.
  - `resolver.defaults.parallel`: per-nameserver query task parallelism.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Resolve nameserver list from `Method4and5`.
3. Build disabled-transport summary tags:
   - If any IPv4 nameservers exist while IPv4 is disabled, emit `CN01_IPV4_DISABLED` with `ns_list`.
   - If any IPv6 nameservers exist while IPv6 is disabled, emit `CN01_IPV6_DISABLED` with `ns_list`.
4. For each nameserver (parallelized):
   - If transport for this nameserver IP version is disabled:
     - Emit `IPV4_DISABLED` or `IPV6_DISABLED` for each rrtype (`SOA`, `NS`) and skip queries for that nameserver.
   - Else query SOA and NS for child zone over UDP.
   - If both responses are absent, emit `CN01_NO_RESPONSE_UDP`.
   - Otherwise evaluate SOA and NS responses independently:
     - No response -> `CN01_NO_RESPONSE_<QTYPE>_QUERY_UDP`.
     - `RCODE != NOERROR` -> `CN01_UNEXPECTED_RCODE_<QTYPE>_QUERY_UDP`.
     - No `<QTYPE>` record in answer -> `CN01_MISSING_<QTYPE>_RECORD_UDP`.
     - First answer owner name differs from child zone -> `CN01_WRONG_<QTYPE>_RECORD_UDP`.
     - AA flag unset -> `CN01_<QTYPE>_RECORD_NOT_AA_UDP`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `CN01_IPV4_DISABLED` | IPv4 is disabled and at least one IPv4 nameserver exists in Method4+Method5 set. |
| `CN01_IPV6_DISABLED` | IPv6 is disabled and at least one IPv6 nameserver exists in Method4+Method5 set. |
| `CN01_MISSING_NS_RECORD_UDP` | NS response exists with `NOERROR` but has no NS answer record. |
| `CN01_MISSING_SOA_RECORD_UDP` | SOA response exists with `NOERROR` but has no SOA answer record. |
| `CN01_NO_RESPONSE_NS_QUERY_UDP` | NS query has no response message while SOA handling continues. |
| `CN01_NO_RESPONSE_SOA_QUERY_UDP` | SOA query has no response message while NS handling continues. |
| `CN01_NO_RESPONSE_UDP` | Both SOA and NS queries have no response message. |
| `CN01_NS_RECORD_NOT_AA_UDP` | NS response has expected owner and record but AA flag is unset. |
| `CN01_SOA_RECORD_NOT_AA_UDP` | SOA response has expected owner and record but AA flag is unset. |
| `CN01_UNEXPECTED_RCODE_NS_QUERY_UDP` | NS response RCODE is not `NOERROR`. |
| `CN01_UNEXPECTED_RCODE_SOA_QUERY_UDP` | SOA response RCODE is not `NOERROR`. |
| `CN01_WRONG_NS_RECORD_UDP` | First NS answer owner name is not the child zone name. |
| `CN01_WRONG_SOA_RECORD_UDP` | First SOA answer owner name is not the child zone name. |
| `IPV4_DISABLED` | IPv4 transport is disabled for this nameserver/rrtype pair. |
| `IPV6_DISABLED` | IPv6 transport is disabled for this nameserver/rrtype pair. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `CN01_IPV4_DISABLED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) skipped due to IPv4 disable. |
| `CN01_IPV6_DISABLED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) skipped due to IPv6 disable. |
| `CN01_MISSING_NS_RECORD_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) producing the response. |
| `CN01_MISSING_SOA_RECORD_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) producing the response. |
| `CN01_NO_RESPONSE_NS_QUERY_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) with no NS response. |
| `CN01_NO_RESPONSE_SOA_QUERY_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) with no SOA response. |
| `CN01_NO_RESPONSE_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) with no SOA and NS response. |
| `CN01_NS_RECORD_NOT_AA_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) with non-AA NS response. |
| `CN01_SOA_RECORD_NOT_AA_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) with non-AA SOA response. |
| `CN01_UNEXPECTED_RCODE_NS_QUERY_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) producing unexpected NS RCODE. |
| `CN01_UNEXPECTED_RCODE_NS_QUERY_UDP` | `rcode` | `string` | Returned RCODE mnemonic. |
| `CN01_UNEXPECTED_RCODE_SOA_QUERY_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) producing unexpected SOA RCODE. |
| `CN01_UNEXPECTED_RCODE_SOA_QUERY_UDP` | `rcode` | `string` | Returned RCODE mnemonic. |
| `CN01_WRONG_NS_RECORD_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) returning wrong NS owner name. |
| `CN01_WRONG_NS_RECORD_UDP` | `domain_found` | `string` | Lowercased owner name found in first NS answer record. |
| `CN01_WRONG_NS_RECORD_UDP` | `domain_expected` | `string` | Lowercased expected child zone FQDN. |
| `CN01_WRONG_SOA_RECORD_UDP` | `ns` | `string` | Nameserver identity (`name/ip`) returning wrong SOA owner name. |
| `CN01_WRONG_SOA_RECORD_UDP` | `domain_found` | `string` | Lowercased owner name found in first SOA answer record. |
| `CN01_WRONG_SOA_RECORD_UDP` | `domain_expected` | `string` | Lowercased expected child zone FQDN. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA` or `NS`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA` or `NS`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Connectivity01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Connectivity01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `CN01_IPV4_DISABLED` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_IPV6_DISABLED` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_MISSING_NS_RECORD_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_MISSING_SOA_RECORD_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_NO_RESPONSE_NS_QUERY_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_NO_RESPONSE_SOA_QUERY_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_NO_RESPONSE_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_NS_RECORD_NOT_AA_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_SOA_RECORD_NOT_AA_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_UNEXPECTED_RCODE_NS_QUERY_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_UNEXPECTED_RCODE_SOA_QUERY_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_WRONG_NS_RECORD_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN01_WRONG_SOA_RECORD_UDP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |

## Differences From Upstream
- Upstream reference: [`connectivity01.md`](../../upstream/tests/Connectivity-TP/connectivity01.md)
- Differences:
  - Implementation emits additional per-query transport debug tags (`IPV4_DISABLED`, `IPV6_DISABLED`) in addition to summary tags (`CN01_IPV4_DISABLED`, `CN01_IPV6_DISABLED`).
  - Owner-name validation checks the first answer record owner for the queried rrtype.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If Method4+Method5 yields no nameservers, only testcase start/end tags are emitted.
- Query call errors are treated as absent response messages.
- A single nameserver can emit multiple findings in one run (for example one SOA issue and one NS issue).
