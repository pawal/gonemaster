# Basic02 (basic02)

Status: Draft

## Purpose
- Verify that the child zone has at least one working authoritative nameserver for SOA.
- Classify non-working delegation nameservers into explicit failure categories.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` is provided.
  - Recursor and nameserver query path are available.
- Required inputs:
  - Child zone name (`z.Name`).
  - Delegation NS names from `methods.Method2`.
  - Delegation NS addresses from `methods.Method4` and fallback glue/recursive resolution path.
- Profile/config knobs that affect behavior:
  - `net.ipv4`: enables or disables IPv4 SOA probes.
  - `net.ipv6`: enables or disables IPv6 SOA probes.
  - `resolver.defaults.parallel`: controls per-test parallel probing fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Load delegation NS names with `Method2` and delegation NS address objects with `Method4`.
3. If no NS names exist, emit `B02_NO_DELEGATION`, then emit `TEST_CASE_END` and return.
4. If NS names exist but address objects are empty, attempt to populate from glue addresses and mark unresolved names as `nsCantResolve`.
5. Probe each nameserver (parallelized) with SOA query to child zone:
   - Emit transport enable/disable tags (`IPV4_*`, `IPV6_*`) for rrtype `SOA`.
   - Classify each response as no response, unexpected rcode, not authoritative, broken answer, or authoritative SOA answer.
6. If at least one authoritative SOA response exists, emit `B02_AUTH_RESPONSE_SOA`.
7. Otherwise emit `B02_NO_WORKING_NS`, then emit detailed categories:
   - `B02_NS_BROKEN`
   - `B02_NS_NOT_AUTH`
   - `B02_NS_NO_IP_ADDR`
   - `B02_NS_NO_RESPONSE`
   - `B02_UNEXPECTED_RCODE`
8. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `B02_AUTH_RESPONSE_SOA` | At least one nameserver returns authoritative SOA for child zone. |
| `B02_NO_DELEGATION` | No delegation NS names are found. |
| `B02_NO_WORKING_NS` | No nameserver qualifies as authoritative SOA responder. |
| `B02_NS_BROKEN` | Nameserver response is NOERROR+AA but lacks SOA answer for child name. |
| `B02_NS_NOT_AUTH` | Nameserver responds but AA is not set. |
| `B02_NS_NO_IP_ADDR` | Nameserver name could not be resolved into an IP address. |
| `B02_NS_NO_RESPONSE` | Nameserver probe returns no response packet. |
| `B02_UNEXPECTED_RCODE` | Nameserver response rcode is not `NOERROR`. |
| `IPV4_DISABLED` | IPv4 transport is disabled for SOA probe. |
| `IPV4_ENABLED` | IPv4 transport is enabled for SOA probe. |
| `IPV6_DISABLED` | IPv6 transport is disabled for SOA probe. |
| `IPV6_ENABLED` | IPv6 transport is enabled for SOA probe. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `B02_AUTH_RESPONSE_SOA` | `domain` | `string` | Child zone name. |
| `B02_AUTH_RESPONSE_SOA` | `ns_list` | `string` | Semicolon-delimited list of nameserver identities (`name/ip`). |
| `B02_NO_DELEGATION` | `domain` | `string` | Child zone name without delegation. |
| `B02_NO_WORKING_NS` | `domain` | `string` | Child zone name without working authoritative NS. |
| `B02_NS_BROKEN` | `ns` | `string` | Nameserver identity (`name/ip`) with broken SOA content. |
| `B02_NS_NOT_AUTH` | `ns` | `string` | Nameserver identity (`name/ip`) missing AA. |
| `B02_NS_NO_IP_ADDR` | `nsname` | `string` | Nameserver owner name lacking resolved IP addresses. |
| `B02_NS_NO_RESPONSE` | `ns` | `string` | Nameserver identity (`name/ip`) with no response. |
| `B02_UNEXPECTED_RCODE` | `ns` | `string` | Nameserver identity (`name/ip`) returning unexpected rcode. |
| `B02_UNEXPECTED_RCODE` | `rcode` | `string` | Observed non-`NOERROR` rcode name. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`). |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV4_ENABLED` | `ns` | `string` | Nameserver identity (`name/ip`). |
| `IPV4_ENABLED` | `rrtype` | `string` | rrtype queried (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`). |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_ENABLED` | `ns` | `string` | Nameserver identity (`name/ip`). |
| `IPV6_ENABLED` | `rrtype` | `string` | rrtype queried (`SOA`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Basic02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Basic02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `B02_AUTH_RESPONSE_SOA` | `INFO` | Default from `share/profile.json`. |
| `B02_NO_DELEGATION` | `CRITICAL` | Default from `share/profile.json`. |
| `B02_NO_WORKING_NS` | `CRITICAL` | Default from `share/profile.json`. |
| `B02_NS_BROKEN` | `ERROR` | Default from `share/profile.json`. |
| `B02_NS_NOT_AUTH` | `ERROR` | Default from `share/profile.json`. |
| `B02_NS_NO_IP_ADDR` | `ERROR` | Default from `share/profile.json`. |
| `B02_NS_NO_RESPONSE` | `WARNING` | Default from `share/profile.json`. |
| `B02_UNEXPECTED_RCODE` | `ERROR` | Default from `share/profile.json`. |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV4_ENABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV6_ENABLED` | `DEBUG` | Default from `share/profile.json`. |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json`. |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json`. |

## Differences From Upstream
- Upstream reference: [`basic02.md`](../../upstream/tests/Basic-TP/basic02.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: summary focuses on B02-class outcome tags. Gonemaster: also emits explicit transport debug tags (`IPV4_*`, `IPV6_*`) during SOA probes.
  - Upstream: does not describe probe execution ordering details. Gonemaster: executes probes in parallel but keeps deterministic result ordering by task index.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If `Method2` returns names but `Method4` has no resolved addresses, unresolved names are classified via `B02_NS_NO_IP_ADDR`.
- If all probes are skipped due transport disable, testcase can still end in `B02_NO_WORKING_NS`.
- Detailed error tags are emitted only when `B02_NO_WORKING_NS` is emitted.
- Output ordering is deterministic after parallel execution because runner output is merged in task index order.
