# Address02

Status: Final

## Purpose
- Verify that every unique nameserver IP address has a usable reverse DNS PTR mapping.
- Distinguish between negative PTR results and total lack of PTR query response.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object.
- Required inputs:
  - Nameserver addresses from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) (delegation/glue view).
  - Nameserver addresses from [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) (child/authoritative view).
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: controls PTR query task parallelism.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Collect nameserver entries from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
3. Build an ordered unique list by IP string:
   - Concatenate [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) then [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
   - Keep the first `(nsname, ip)` seen for each unique IP.
4. For each unique IP, execute a PTR-check task (parallelized):
   - Compute reverse lookup owner with `dns.ReverseAddr`.
   - Send recursive PTR query.
   - If response has `NOERROR` and a CNAME in answer, follow the first CNAME target with one additional PTR query.
   - If a response message exists:
     - If RCODE is not `NOERROR` or PTR answer set is empty, emit `NAMESERVER_IP_WITHOUT_REVERSE` (`nsname`, `ns_ip`).
   - If no response message exists, emit `NO_RESPONSE_PTR_QUERY` (`domain`).
5. After all tasks complete, if at least one IP was checked and no tag besides `TEST_CASE_START` was emitted, emit `NAMESERVERS_IP_WITH_REVERSE`.
6. Emit `TEST_CASE_END`.

### Per-IP PTR Probe and Aggregation (steps 2-6)

{{% expand "Show diagram" %}}
```
collect nameserver IPs from GlueNameservers then ApexNameservers
 +- dedupe by IP string; first-seen (nsname, ip) wins
 |
 v
For each unique IP (parallel; fan-out = resolver.defaults.parallel):

   ptrQuery = dnsutil.ReverseAddr(ip)
    |
    v
   rec.Recurse(ptrQuery, PTR, IN)
    +- resp.Msg != nil, RCODE == NOERROR, CNAME in answer
    |     -> ptrQuery = first CNAME target
    |        rec.Recurse(new ptrQuery, PTR, IN)  (one hop max)
    |
    +- resp.Msg present
    |    +- RCODE != NOERROR or no PTR records   -> NAMESERVER_IP_WITHOUT_REVERSE
    |    +- otherwise                            -> (success; silent)
    +- resp.Msg absent                           -> NO_RESPONSE_PTR_QUERY

After all tasks:
  at least one IP checked AND no failure tag emitted -> NAMESERVERS_IP_WITH_REVERSE
emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `CNAME_CHAIN_TOO_LONG` | A discovered NS hostname's CNAME chain exceeds `CNAMEMaxChainLength` while resolving its address. |
| `CNAME_TARGET_UNRESOLVED` | A discovered NS hostname's CNAME chain forms a loop, breaks, or fails to resolve to an address. |
| `CNAME_TOO_MANY_RECORDS` | A single answer while resolving a discovered NS hostname carries more than `CNAMEMaxRecords` distinct CNAME RRs. |
| `NAMESERVER_IP_WITHOUT_REVERSE` | PTR response is present but not successful (`RCODE != NOERROR`) or has no PTR record. |
| `NAMESERVERS_IP_WITH_REVERSE` | All checked IPs have successful PTR answers and no PTR-query failure tag was emitted. |
| `NO_RESPONSE_PTR_QUERY` | PTR recursive query returned no response message. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `CNAME_CHAIN_TOO_LONG` | `query_name` | `string` | The NS hostname whose CNAME chain exceeded the depth bound. |
| `CNAME_TARGET_UNRESOLVED` | `query_name` | `string` | The NS hostname whose CNAME target could not be resolved. |
| `CNAME_TARGET_UNRESOLVED` | `cname_target` | `string` | The last attempted CNAME target. |
| `CNAME_TOO_MANY_RECORDS` | `query_name` | `string` | The NS hostname whose answer carried too many CNAME RRs. |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `nsname` | `string` | Nameserver name associated with the checked IP (first-seen for that IP). |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `ns_ip` | `string` | Checked nameserver IP address. |
| `NAMESERVERS_IP_WITH_REVERSE` | `-` | `-` | No arguments. |
| `NO_RESPONSE_PTR_QUERY` | `domain` | `string` | PTR owner name queried (reverse name or followed CNAME target). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Address02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Address02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `CNAME_CHAIN_TOO_LONG` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `CNAME_TARGET_UNRESOLVED` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `CNAME_TOO_MANY_RECORDS` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `WARNING` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `NAMESERVERS_IP_WITH_REVERSE` | `INFO` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `NO_RESPONSE_PTR_QUERY` | `WARNING` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: describes overall success/failure semantics only. Gonemaster: emits explicit diagnostic tags for pass, fail, and no-response PTR paths.
  - Upstream: does not specify PTR CNAME follow-up behavior. Gonemaster: follows one PTR CNAME hop before final PTR evaluation.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers)+[`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) yields no IP addresses, only `TEST_CASE_START` and `TEST_CASE_END` are emitted.
- Duplicate IPs are checked once; if multiple nameservers share an IP, the first-seen nameserver name is used in emitted arguments.
- Only one CNAME follow-up lookup is performed for PTR checks.
