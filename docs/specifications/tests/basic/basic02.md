# Basic02

Status: Final

## Purpose
- Verify that the child zone has at least one working authoritative nameserver for SOA.
- Classify non-working delegation nameservers into explicit failure categories.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` is provided.
  - Recursor and nameserver query path are available.
- Required inputs:
  - Child zone name (`z.Name`).
  - Delegation NS names from [`z.GlueNames(ctx)`](../../nameserver-resolution.md#gluenames).
  - Delegation NS addresses from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and fallback glue/recursive resolution path.
- Profile/config knobs that affect behavior:
  - `net.ipv4`: enables or disables IPv4 SOA probes.
  - `net.ipv6`: enables or disables IPv6 SOA probes.
  - `resolver.defaults.parallel`: controls per-test parallel probing fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Load delegation NS names with [`z.GlueNames`](../../nameserver-resolution.md#gluenames) and delegation NS address objects with [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers).
3. If no NS names exist, emit `B02_NO_DELEGATION`, then emit `TEST_CASE_END` and return.
4. For every NS name without an address object, take the address records the parent's referral holds for that name (`z.GlueAddresses`), cut at `addresses_per_ns`; mark a name left without an address as `nsCantResolve`.
5. Probe each nameserver (parallelized) with SOA query to child zone:
   - Emit transport enable/disable tags (`IPV4_*`, `IPV6_*`) for query type `SOA`.
   - Classify each response as no response, unexpected rcode, not authoritative, broken answer, or authoritative SOA answer.
6. If at least one authoritative SOA response exists, emit `B02_AUTH_RESPONSE_SOA`.
7. Otherwise emit `B02_NO_WORKING_NS`, then emit detailed categories in this order, each sorted by nameserver name and address (`B02_NS_NO_IP_ADDR` by name):
   - `B02_NS_BROKEN`
   - `B02_NS_NOT_AUTH`
   - `B02_NS_NO_IP_ADDR`
   - `B02_NS_NO_RESPONSE`
   - `B02_UNEXPECTED_RCODE`
8. Emit `TEST_CASE_END`.

### Input and Per-NS SOA Probe (steps 2-5)

{{% expand "Show diagram" %}}
```
load z.GlueNames (NS names) + GlueNameservers (NS addresses)
 +- no NS names                       -> B02_NO_DELEGATION (domain)
 |                                       emit TEST_CASE_END and return
 +- otherwise                         -> use GlueNameservers addresses
      +- per NS name without an address -> add nameserver for each glue addr of that name
                                          (z.GlueAddresses, cut at addresses_per_ns)
      +- name still without an address  -> mark nsCantResolve[name]

For each nameserver (parallel; fan-out = resolver.defaults.parallel):

   transport check for SOA
    +- IPv6 + Net.IPv6 disabled       -> IPV6_DISABLED, mark skipped
    +- IPv4 + Net.IPv4 disabled       -> IPV4_DISABLED, mark skipped
    +- enabled                        -> IPV4_ENABLED / IPV6_ENABLED

   query SOA at z.Name
    +- error or resp.Msg absent        -> nsNoResponse[key]
    +- RCODE != NOERROR                -> unexpectedRcode[key] = rcode
    +- !AA                             -> nsNotAuth[key]
    +- SOA records for z.Name in answer -> authResponseSOA[key]
    +- otherwise (NOERROR + AA, no SOA)-> nsBroken[key]
```
{{% /expand %}}

### Aggregation and Final Emission (steps 6-8)

{{% expand "Show diagram" %}}
```
authResponseSOA non-empty
 +- yes  -> B02_AUTH_RESPONSE_SOA (domain, servers)
 +- no   -> B02_NO_WORKING_NS (domain)
            then, in this order, one tag per member of each non-empty category,
            sorted by name and address:
              nsBroken          -> B02_NS_BROKEN        (per ns)
              nsNotAuth         -> B02_NS_NOT_AUTH      (per ns)
              nsCantResolve     -> B02_NS_NO_IP_ADDR    (per nsname)
              nsNoResponse      -> B02_NS_NO_RESPONSE   (per ns)
              unexpectedRcode   -> B02_UNEXPECTED_RCODE (per ns, rcode)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `B02_AUTH_RESPONSE_SOA` | At least one nameserver returns authoritative SOA for child zone. |
| `B02_NO_DELEGATION` | No delegation NS names are found. |
| `B02_NO_WORKING_NS` | No nameserver qualifies as authoritative SOA responder. |
| `B02_NS_BROKEN` | Nameserver response is NOERROR+AA but lacks SOA answer for child name (RFC 1034 section 4.2.1). |
| `B02_NS_NOT_AUTH` | Nameserver responds but AA is not set (RFC 1912 section 2.8). |
| `B02_NS_NO_IP_ADDR` | Nameserver name could not be resolved into an IP address (RFC 2181 section 10.3). |
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
| `B02_AUTH_RESPONSE_SOA` | `servers` | `array<object>` | Structured list of nameserver identities (`{ns,address}` object). |
| `B02_NO_DELEGATION` | `domain` | `string` | Child zone name without delegation. |
| `B02_NO_WORKING_NS` | `domain` | `string` | Child zone name without working authoritative NS. |
| `B02_NS_BROKEN` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with broken SOA content. |
| `B02_NS_BROKEN` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `B02_NS_NOT_AUTH` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) missing AA. |
| `B02_NS_NOT_AUTH` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `B02_NS_NO_IP_ADDR` | `ns` | `string` | Nameserver owner name lacking resolved IP addresses. |
| `B02_NS_NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `B02_NS_NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `B02_UNEXPECTED_RCODE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) returning unexpected rcode. |
| `B02_UNEXPECTED_RCODE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `B02_UNEXPECTED_RCODE` | `rcode` | `string` | Observed non-`NOERROR` rcode name. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | Query type skipped (`SOA`). |
| `IPV4_ENABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV4_ENABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_ENABLED` | `query_type` | `string` | Query type sent (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | Query type skipped (`SOA`). |
| `IPV6_ENABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV6_ENABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_ENABLED` | `query_type` | `string` | Query type sent (`SOA`). |
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
| `IPV4_DISABLED` | `DEBUG2` | Default from `share/profile.json`. |
| `IPV4_ENABLED` | `DEBUG2` | Default from `share/profile.json`. |
| `IPV6_DISABLED` | `DEBUG2` | Default from `share/profile.json`. |
| `IPV6_ENABLED` | `DEBUG2` | Default from `share/profile.json`. |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json`. |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json`. |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: summary focuses on B02-class outcome tags. Gonemaster: also emits explicit transport debug tags (`IPV4_*`, `IPV6_*`) during SOA probes.
  - Upstream: does not describe probe execution ordering details. Gonemaster: executes probes in parallel but keeps deterministic result ordering by task index.
  - Upstream (step 3.2): an in-bailiwick NS name takes its addresses from the referral glue, any other name from a recursive lookup. Gonemaster: looks up every name recursively and takes the referral glue only for a name the lookup leaves without an address, so when the glue and the child's address records differ, the child's addresses are probed (`DIV-B02-ADDRESS-SOURCE`).
  - Upstream: `B02_NS_NO_IP_ADDR` carries `nsname`. Gonemaster: carries `ns`.
  - Upstream: follows every NS name and address. Gonemaster: cuts the NS names of the delegation at 32 and the addresses of one name at 16, and a run queries at most 512 addresses; a cut logs `LIMIT_REACHED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- An NS name that neither [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) nor the referral glue gives an address is classified via `B02_NS_NO_IP_ADDR`, whether or not other names resolve; like the other detail tags it is emitted only beside `B02_NO_WORKING_NS`.
- If all probes are skipped due transport disable, testcase can still end in `B02_NO_WORKING_NS`.
- Detailed error tags are emitted only when `B02_NO_WORKING_NS` is emitted.
- Output ordering is deterministic: probe entries are merged in task index order, and detail tags are sorted by nameserver name and address.
