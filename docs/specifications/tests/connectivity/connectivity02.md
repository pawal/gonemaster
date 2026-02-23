# Connectivity02 (connectivity02)

Status: Draft

## Purpose
- Verify that nameservers are reachable over TCP for SOA and NS queries at the child zone name.
- Detect TCP response-shape failures equivalent to Connectivity01 checks.

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
3. For each nameserver (parallelized):
   - If transport for this nameserver IP version is disabled:
     - Emit `IPV4_DISABLED` or `IPV6_DISABLED` for each rrtype (`SOA`, `NS`) and skip queries for that nameserver.
   - Else query SOA and NS for child zone over TCP (`UseVC=true`).
   - If both responses are absent, emit `CN02_NO_RESPONSE_TCP`.
   - Otherwise evaluate SOA and NS responses independently:
     - No response -> `CN02_NO_RESPONSE_<QTYPE>_QUERY_TCP`.
     - `RCODE != NOERROR` -> `CN02_UNEXPECTED_RCODE_<QTYPE>_QUERY_TCP`.
     - No `<QTYPE>` record in answer -> `CN02_MISSING_<QTYPE>_RECORD_TCP`.
     - First answer owner name differs from child zone -> `CN02_WRONG_<QTYPE>_RECORD_TCP`.
     - AA flag unset -> `CN02_<QTYPE>_RECORD_NOT_AA_TCP`.
4. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `CN02_MISSING_NS_RECORD_TCP` | NS response exists with `NOERROR` but has no NS answer record. |
| `CN02_MISSING_SOA_RECORD_TCP` | SOA response exists with `NOERROR` but has no SOA answer record. |
| `CN02_NO_RESPONSE_NS_QUERY_TCP` | NS query has no response message while SOA handling continues. |
| `CN02_NO_RESPONSE_SOA_QUERY_TCP` | SOA query has no response message while NS handling continues. |
| `CN02_NO_RESPONSE_TCP` | Both SOA and NS queries have no response message. |
| `CN02_NS_RECORD_NOT_AA_TCP` | NS response has expected owner and record but AA flag is unset. |
| `CN02_SOA_RECORD_NOT_AA_TCP` | SOA response has expected owner and record but AA flag is unset. |
| `CN02_UNEXPECTED_RCODE_NS_QUERY_TCP` | NS response RCODE is not `NOERROR`. |
| `CN02_UNEXPECTED_RCODE_SOA_QUERY_TCP` | SOA response RCODE is not `NOERROR`. |
| `CN02_WRONG_NS_RECORD_TCP` | First NS answer owner name is not the child zone name. |
| `CN02_WRONG_SOA_RECORD_TCP` | First SOA answer owner name is not the child zone name. |
| `IPV4_DISABLED` | IPv4 transport is disabled for this nameserver/rrtype pair. |
| `IPV6_DISABLED` | IPv6 transport is disabled for this nameserver/rrtype pair. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `CN02_MISSING_NS_RECORD_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) producing the response. |
| `CN02_MISSING_SOA_RECORD_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) producing the response. |
| `CN02_NO_RESPONSE_NS_QUERY_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) with no NS response. |
| `CN02_NO_RESPONSE_SOA_QUERY_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) with no SOA response. |
| `CN02_NO_RESPONSE_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) with no SOA and NS response. |
| `CN02_NS_RECORD_NOT_AA_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) with non-AA NS response. |
| `CN02_SOA_RECORD_NOT_AA_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) with non-AA SOA response. |
| `CN02_UNEXPECTED_RCODE_NS_QUERY_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) producing unexpected NS RCODE. |
| `CN02_UNEXPECTED_RCODE_NS_QUERY_TCP` | `rcode` | `string` | Returned RCODE mnemonic. |
| `CN02_UNEXPECTED_RCODE_SOA_QUERY_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) producing unexpected SOA RCODE. |
| `CN02_UNEXPECTED_RCODE_SOA_QUERY_TCP` | `rcode` | `string` | Returned RCODE mnemonic. |
| `CN02_WRONG_NS_RECORD_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) returning wrong NS owner name. |
| `CN02_WRONG_NS_RECORD_TCP` | `domain_found` | `string` | Lowercased owner name found in first NS answer record. |
| `CN02_WRONG_NS_RECORD_TCP` | `domain_expected` | `string` | Lowercased expected child zone FQDN. |
| `CN02_WRONG_SOA_RECORD_TCP` | `ns` | `string` | Nameserver identity (`name/ip`) returning wrong SOA owner name. |
| `CN02_WRONG_SOA_RECORD_TCP` | `domain_found` | `string` | Lowercased owner name found in first SOA answer record. |
| `CN02_WRONG_SOA_RECORD_TCP` | `domain_expected` | `string` | Lowercased expected child zone FQDN. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA` or `NS`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA` or `NS`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Connectivity02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Connectivity02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `CN02_MISSING_NS_RECORD_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_MISSING_SOA_RECORD_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_NO_RESPONSE_NS_QUERY_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_NO_RESPONSE_SOA_QUERY_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_NO_RESPONSE_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_NS_RECORD_NOT_AA_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_SOA_RECORD_NOT_AA_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_UNEXPECTED_RCODE_NS_QUERY_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_UNEXPECTED_RCODE_SOA_QUERY_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_WRONG_NS_RECORD_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN02_WRONG_SOA_RECORD_TCP` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |

## Differences From Upstream
- Upstream reference: [`connectivity02.md`](../../upstream/tests/Connectivity-TP/connectivity02.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly define this detail. Gonemaster: emits additional per-query transport debug tags (`IPV4_DISABLED`, `IPV6_DISABLED`) when transport is disabled.
  - Upstream: does not explicitly define this detail. Gonemaster: Owner-name validation checks the first answer record owner for the queried rrtype.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If Method4+Method5 yields no nameservers, only testcase start/end tags are emitted.
- Query call errors are treated as absent response messages.
- A single nameserver can emit multiple findings in one run (for example one SOA issue and one NS issue).
