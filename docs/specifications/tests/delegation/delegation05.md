# Delegation05

Status: Final

## Purpose
- Verify that NS names used for the tested zone are not aliases (CNAME targets).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - NS names from `AllNSNames`.
  - Addressed NS from `GlueNameservers` and `ApexNameservers`.
  - Recursive lookup function (`recurse`) for non-in-bailiwick NS names.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports emit transport-debug tags and skip per-NS-IP in-bailiwick checks.
  - `resolver.defaults.parallel`: parallel in-bailiwick NS-IP query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Get NS name list from `AllNSNames`.
3. Get delegation and child addressed NS lists (`GlueNameservers` and `ApexNameservers`), merge into a unique map keyed by `name/ip`, and sort keys.
4. For each NS name from step 2:
   - If NS name is in-bailiwick of tested zone:
     - For each merged addressed NS (`name/ip`) in sorted order (parallelized):
       - Build args `{ns, query_name, rrtype=A}`.
       - If transport is disabled for that addressed NS, emit `IPV4_DISABLED` or `IPV6_DISABLED` and skip.
       - Query addressed NS for `A` with recursion disabled (`RD=0`).
       - If no DNS message, emit `NO_RESPONSE`.
       - Else if `RCODE != NOERROR`, emit `UNEXPECTED_RCODE`.
       - Else if answer contains `CNAME`, emit `NS_IS_CNAME` (`nsname`).
       - Else if response is a referral/redirect, perform recursive retry (`RD=1`) against same addressed NS and emit `NS_IS_CNAME` if answer contains `CNAME`.
   - Else (sibling/out-of-bailiwick):
     - Perform recursive lookup via `recurse`.
     - Emit `NS_IS_CNAME` if recursive answer contains `CNAME`.
5. After all NS names, if `NS_IS_CNAME` was never emitted, emit `NO_NS_CNAME`.
6. Emit `TEST_CASE_END`.

### Per-NS CNAME Detection (step 4)

{{% expand "Show diagram" %}}
```
nsNames = AllNSNames
allNS   = GlueNameservers ++ ApexNameservers, unique by ns.String() ("name/ip"); keys sorted

For each nsName in nsNames:

  z.Name.IsInBailiwick(nsName)
    +- yes -> for each (name/ip) in allNS, in parallel:
                transport disabled for A
                   -> IPV4_DISABLED / IPV6_DISABLED (rrtype=A); skip
                query A at nsName, Recurse=false
                  +- err or resp.Msg == nil   -> NO_RESPONSE (query_name, rrtype)
                  +- RCODE != NOERROR         -> UNEXPECTED_RCODE (rcode, ...)
                  +- CNAME in answer          -> NS_IS_CNAME (ns=nsName)
                  +- IsRedirect               -> retry with Recurse=true
                                                  CNAME in answer
                                                    -> NS_IS_CNAME (ns=nsName)
    +- no  -> recurse(z, nsName, "A")
                resp.Msg present AND CNAME in answer
                  -> NS_IS_CNAME (ns=nsName)
```
{{% /expand %}}

### Final CNAME Status Emission (steps 5-6)

{{% expand "Show diagram" %}}
```
no NS_IS_CNAME tag emitted -> NO_NS_CNAME

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 in-bailiwick per-NS-IP check is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 in-bailiwick per-NS-IP check is skipped because IPv6 is disabled. |
| `NO_NS_CNAME` | No `NS_IS_CNAME` finding was produced in testcase execution. |
| `NO_RESPONSE` | In-bailiwick `A` query (`RD=0`) produced no DNS message. |
| `NS_IS_CNAME` | NS name resolves as CNAME in direct or recursive branch. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `UNEXPECTED_RCODE` | In-bailiwick `A` query (`RD=0`) returned non-`NOERROR` response code. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `NO_NS_CNAME` | `-` | `-` | No arguments. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) that did not return DNS message. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE` | `query_name` | `string` | NS name queried for type `A`. |
| `NO_RESPONSE` | `rrtype` | `string` | Queried rrtype (`A`). |
| `NS_IS_CNAME` | `nsname` | `string` | NS name found as CNAME. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Delegation05`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Delegation05`). |
| `UNEXPECTED_RCODE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) that returned unexpected RCODE. |
| `UNEXPECTED_RCODE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `UNEXPECTED_RCODE` | `query_name` | `string` | NS name queried for type `A`. |
| `UNEXPECTED_RCODE` | `rrtype` | `string` | Queried rrtype (`A`). |
| `UNEXPECTED_RCODE` | `rcode` | `string` | Returned DNS response code string. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_NS_CNAME` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NS_IS_CNAME` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `UNEXPECTED_RCODE` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Upstream reference: [`delegation05.md`](../../upstream/tests/Delegation-TP/delegation05.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: special-procedure text is generic about disabled transports. Gonemaster: explicitly emits `IPV4_DISABLED`/`IPV6_DISABLED` only in the in-bailiwick per-NS-IP branch.
  - Upstream: describes recursive sibling/out-of-bailiwick branch as DNS lookup followed by CNAME check. Gonemaster: does that CNAME check, but does not emit `NO_RESPONSE` or `UNEXPECTED_RCODE` in that branch.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- `NS_IS_CNAME` can be emitted multiple times for the same NS name from different addressed NS checks.
- In-bailiwick checks are performed against all unique `name/ip` entries from delegation+child sets, not only the tested NS name's own addresses.
- `NO_NS_CNAME` is emitted whenever no `NS_IS_CNAME` was found, even if other warning/debug tags were emitted.
