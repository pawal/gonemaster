# Zone09

Status: Final

## Purpose
- Validate MX presence and consistency across authoritative nameservers, including null-MX and domain-class exceptions (root/TLD/.arpa).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - MX responses per nameserver.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: parallel nameserver query fanout.
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Collect nameservers from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers), deduplicate probing list by IP address, and keep a name-to-IP grouping view for later reporting.
3. For each unique nameserver IP (parallelized):
   - Skip disabled transports.
   - Query apex `MX` directly over UDP with `fallback=false`; if truncated, retry with TCP (`usevc=true`, `fallback=false`). There is no SOA precondition.
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
   - `Z09_NO_MX_FOUND` (`servers`: name servers by host name and IP)
   - `Z09_MX_FOUND` (`servers`: name servers by host name and IP)
6. If MX RRset bucket is non-empty:
   - Compute a per-server key from the MX RDATA: the set of
     `preference SP lower(mail-target)` pairs, sorted. This key is the data
     compared for consistency.
   - Group the responding servers by that key.
   - If more than one distinct key exists, emit `Z09_INCONSISTENT_MX_DATA`
     once per distinct key, each with `servers` (that key's name servers by
     host name and IP) and `mail_targets` (that key's mail targets).
   - If exactly one key exists:
     - evaluate null-MX conditions:
       - `Z09_NULL_MX_WITH_OTHER_MX` when `.` mailtarget is mixed with other MX RRs;
       - `Z09_NULL_MX_NON_ZERO_PREF` when null-MX preference is not zero;
     - if no null-MX:
       - emit `Z09_ROOT_EMAIL_DOMAIN` for root zone (`mail_targets`);
       - emit `Z09_TLD_EMAIL_DOMAIN` for TLD zone (`mail_targets`);
       - emit `Z09_ARPA_EMAIL_DOMAIN` for a zone under `.arpa` (`mail_targets`);
       - otherwise emit `Z09_MX_DATA` with `servers` (name servers by host name
         and IP) and `mail_targets`.
     - if null-MX and neither `Z09_NULL_MX_WITH_OTHER_MX` nor
       `Z09_NULL_MX_NON_ZERO_PREF` fired, emit `Z09_VALID_NULL_MX` (a single
       zero-preference null-MX is a valid "no mail" statement).
7. If MX RRset bucket is empty and “no MX” bucket is non-empty:
   - If zone is root, TLD, or under `.arpa`: emit `Z09_NO_MX_FOUND_OR_EXPECTED`.
   - Otherwise: emit `Z09_MISSING_MAIL_TARGET`.
8. If both the MX RRset and “no MX” buckets are empty but at least one
   non-disabled nameserver was queried, emit `Z09_NO_SERVERS_MX_RESPONSE` (no
   server returned a usable MX response).
9. Emit `TEST_CASE_END`.

### Per-NS MX Probe and Aggregation (steps 2-8)

{{% expand "Show diagram" %}}
```
ns list = ZoneNameservers; dedupe by IP (uniqueServersByIP)

For each unique nameserver IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for MX -> IPV4_DISABLED / IPV6_DISABLED, skip

   query MX at z.Name (UseVC=false, Fallback=false); no SOA precondition
      resp.TC() -> retry with UseVC=true
    +- resp.Msg == nil                              -> noResponseMX[ip]
    +- RCODE != NOERROR                             -> unexpectedRcodeMX[rcode][ip]
    +- !AA                                          -> nonAuthoritativeMX[ip]
    +- no MX records for z.Name in answer           -> noMXSet[ip]
    +- otherwise                                    -> mxSet[ip] = MX records

Aggregate operational tags (independent of branch below):
   noResponseMX       non-empty -> Z09_NO_RESPONSE_MX_QUERY (addresses)
   unexpectedRcodeMX  non-empty -> Z09_UNEXPECTED_RCODE_MX  (rcode, addresses) per rcode
   nonAuthoritativeMX non-empty -> Z09_NON_AUTH_MX_RESPONSE (addresses)

Mixed presence:
   noMXSet non-empty AND mxSet non-empty
      -> Z09_INCONSISTENT_MX (no args)
         Z09_NO_MX_FOUND     (servers = noMXSet endpoints)
         Z09_MX_FOUND        (servers = mxSet endpoints)

mxSet non-empty (servers grouped by MX RDATA key: pref + lower(target), sorted):

   more than one distinct RDATA key
      -> per RDATA variant:
            Z09_INCONSISTENT_MX_DATA (servers = variant endpoints, mail_targets)

   exactly one RDATA key:
      examine first IP's MX records:
         any MX with target == "." -> hasNullMX = true
            len(records) > 1       -> Z09_NULL_MX_WITH_OTHER_MX (no args)
            MX.Preference > 0      -> Z09_NULL_MX_NON_ZERO_PREF (no args)
      !hasNullMX:
         z.Name == "."             -> Z09_ROOT_EMAIL_DOMAIN (mail_targets)
         nextHigherIsRoot(z.Name)  -> Z09_TLD_EMAIL_DOMAIN  (mail_targets)
         isArpaTree(z.Name)        -> Z09_ARPA_EMAIL_DOMAIN (mail_targets)
         otherwise                 -> Z09_MX_DATA (servers = mxSet endpoints, mail_targets)
      hasNullMX AND no null-MX problem tag fired
                                   -> Z09_VALID_NULL_MX (no args)

mxSet empty AND noMXSet non-empty:
   z.Name == "." OR TLD OR under .arpa
      -> Z09_NO_MX_FOUND_OR_EXPECTED (no args)
   otherwise
      -> Z09_MISSING_MAIL_TARGET (no args)

mxSet empty AND noMXSet empty AND at least one non-disabled server queried:
   -> Z09_NO_SERVERS_MX_RESPONSE (no args)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `Z09_ARPA_EMAIL_DOMAIN` | Zone under `.arpa` has non-null MX data. |
| `Z09_INCONSISTENT_MX` | Some authoritative nameserver IPs return MX RRset while others return none. |
| `Z09_INCONSISTENT_MX_DATA` | MX RDATA differs across responding name servers; emitted once per distinct RDATA variant. |
| `Z09_MISSING_MAIL_TARGET` | No authoritative MX RRset was found and zone is not exempt (non-root, non-TLD, non-`.arpa`). |
| `Z09_MX_DATA` | MX mailtarget data is reported for one reporting group. |
| `Z09_MX_FOUND` | At least one authoritative nameserver IP returned MX RRset. |
| `Z09_NON_AUTH_MX_RESPONSE` | At least one nameserver IP returned non-authoritative MX response after SOA gating. |
| `Z09_NO_MX_FOUND` | At least one authoritative nameserver IP returned no MX RRset. |
| `Z09_NO_MX_FOUND_OR_EXPECTED` | No MX RRset was found for a zone not expected to host mail (root, TLD, or under `.arpa`). |
| `Z09_NO_RESPONSE_MX_QUERY` | At least one nameserver IP gave no MX response after SOA gating. |
| `Z09_NO_SERVERS_MX_RESPONSE` | No server returned a usable MX response after SOA gating. |
| `Z09_NULL_MX_NON_ZERO_PREF` | Null MX (`.` mailtarget) was returned with non-zero preference. |
| `Z09_NULL_MX_WITH_OTHER_MX` | Null MX (`.` mailtarget) is mixed with other MX records. |
| `Z09_ROOT_EMAIL_DOMAIN` | Root zone has non-null MX data. |
| `Z09_TLD_EMAIL_DOMAIN` | TLD zone has non-null MX data. |
| `Z09_UNEXPECTED_RCODE_MX` | At least one nameserver IP returned non-`NOERROR` RCODE for MX query. |
| `Z09_VALID_NULL_MX` | Zone has a single zero-preference null MX (a valid "no mail" statement). |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone09`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone09`). |
| `Z09_ARPA_EMAIL_DOMAIN` | `mail_targets` | `array<string>` | Structured MX exchange hostname list. |
| `Z09_INCONSISTENT_MX` | `-` | `-` | No arguments. |
| `Z09_INCONSISTENT_MX_DATA` | `servers` | `array<object>` | Structured name servers (`{ns,address}`) returning this RDATA variant. |
| `Z09_INCONSISTENT_MX_DATA` | `mail_targets` | `array<string>` | Structured MX exchange hostname list for this RDATA variant. |
| `Z09_MISSING_MAIL_TARGET` | `-` | `-` | No arguments. |
| `Z09_MX_DATA` | `servers` | `array<object>` | Structured name servers (`{ns,address}`) for this data group. |
| `Z09_MX_DATA` | `mail_targets` | `array<string>` | Structured MX exchange hostname list. |
| `Z09_MX_FOUND` | `servers` | `array<object>` | Structured name servers (`{ns,address}`) that returned MX RRset. |
| `Z09_NON_AUTH_MX_RESPONSE` | `addresses` | `array<string>` | Structured nameserver IPs reported as non-authoritative. |
| `Z09_NO_MX_FOUND` | `servers` | `array<object>` | Structured name servers (`{ns,address}`) with no MX RRset. |
| `Z09_NO_MX_FOUND_OR_EXPECTED` | `-` | `-` | No arguments. |
| `Z09_NO_RESPONSE_MX_QUERY` | `addresses` | `array<string>` | Structured nameserver IPs with no MX response. |
| `Z09_NO_SERVERS_MX_RESPONSE` | `-` | `-` | No arguments. |
| `Z09_NULL_MX_NON_ZERO_PREF` | `-` | `-` | No arguments. |
| `Z09_NULL_MX_WITH_OTHER_MX` | `-` | `-` | No arguments. |
| `Z09_ROOT_EMAIL_DOMAIN` | `mail_targets` | `array<string>` | Structured MX exchange hostname list. |
| `Z09_TLD_EMAIL_DOMAIN` | `mail_targets` | `array<string>` | Structured MX exchange hostname list. |
| `Z09_UNEXPECTED_RCODE_MX` | `rcode` | `string` | Unexpected RCODE text. |
| `Z09_UNEXPECTED_RCODE_MX` | `addresses` | `array<string>` | Structured nameserver IPs for that RCODE. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_ARPA_EMAIL_DOMAIN` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_INCONSISTENT_MX` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_INCONSISTENT_MX_DATA` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_MISSING_MAIL_TARGET` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_MX_DATA` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_MX_FOUND` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NON_AUTH_MX_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NO_MX_FOUND` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NO_MX_FOUND_OR_EXPECTED` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NO_RESPONSE_MX_QUERY` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NO_SERVERS_MX_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NULL_MX_NON_ZERO_PREF` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_NULL_MX_WITH_OTHER_MX` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_ROOT_EMAIL_DOMAIN` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_TLD_EMAIL_DOMAIN` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_UNEXPECTED_RCODE_MX` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z09_VALID_NULL_MX` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: describes name server IP set processing. Gonemaster: deduplicates probing by IP before classification and separately keeps name-group reporting views.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Query results for transport-disabled nameservers are skipped; helper debug tags for skipped transports are outside this testcase metadata contract.
- `Z09_INCONSISTENT_MX_DATA` is emitted once per distinct MX RDATA variant. `Z09_MX_DATA` is emitted once, only in the consistent-data branch.
