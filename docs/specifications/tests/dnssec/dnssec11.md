# DNSSEC11

Status: Final

## Purpose
- Verify that parent-side DS presence is consistent with child-side DNSKEY presence (zone signing expectation), including parent consistency and child consistency reporting.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent nameservers from `parentNameservers`.
  - Child nameservers from `GlueNameservers` and `ApexNameservers`.
  - DS, SOA, and DNSKEY query responses.
  - Optional undelegated DS records (`FakeDSRecords`) when fake-address mode is active.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel parent/child nameserver execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build parent nameserver set, deduplicate by IP.
3. Undelegated shortcut:
   - If fake-address mode is active and undelegated DS records are absent, emit `TEST_CASE_END` and return.
4. Parent DS phase (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DS` and skip.
   - Query `DS` with DNSSEC enabled over UDP; if `TC` is set, retry over TCP.
   - If response is absent, non-`NOERROR`, or non-`AA`, classify nameserver as `Undetermined DS`.
   - Else, if apex DS records are absent classify as `No DS`, otherwise `Has DS`.
5. Parent decision:
   - `Undetermined DS` only => emit `DS11_UNDETERMINED_DS`, stop before child phase.
   - `No DS` only => emit `DS11_NO_PARENT_DS`, stop before child phase.
   - Mixed `No DS` and `Has DS` => emit `DS11_INCONSISTENT_DS`, `DS11_PARENT_WITHOUT_DS`, `DS11_PARENT_WITH_DS`, then continue to child phase.
   - `Has DS` only => continue to child phase.
6. Child phase (only when parent decision allows):
   - Build child nameserver set from the union of `glueNameservers` and `apexNameservers` (deduplicated by `ns.String()`), then deduplicate by IP.
   - For each nameserver (parallelized):
     - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `SOA` and `DNSKEY` and skip.
     - Query SOA over UDP (`UseVC=false`); require usable authoritative apex SOA.
     - Query DNSKEY over UDP (`UseVC=false`), retry over TCP on truncation.
     - Classify nameserver as `Undetermined signed zone`, `No DNSKEY`, or `Has DNSKEY`.
7. Child decision:
   - `Undetermined` only => emit `DS11_UNDETERMINED_SIGNED_ZONE`.
   - `No DNSKEY` only => emit `DS11_DS_BUT_UNSIGNED_ZONE`.
   - Mixed `No DNSKEY` and `Has DNSKEY` => emit `DS11_INCONSISTENT_SIGNED_ZONE`, `DS11_NS_WITH_UNSIGNED_ZONE`, `DS11_NS_WITH_SIGNED_ZONE`.
   - `Has DNSKEY` only (no undetermined, no absent) => emit `DS11_CONSISTENT_SIGNED`.
8. Emit `TEST_CASE_END`.

### Parent DS Phase (steps 4-5)

{{% expand "Show diagram" %}}
```
parent set = parentNameservers; dedupe by IP

fake-address mode AND no undelegated DS records
   -> emit TEST_CASE_END and return

For each unique parent NS IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for DS  -> IPV4_DISABLED / IPV6_DISABLED, skip
   query DS at z.Name, DNSSEC=on, UseVC=false
      (resp.TC() -> retry with UseVC=true)
    +- resp.Msg == nil / RCODE != NOERROR / !AA -> undeterminedDS
    +- no apex DS records                      -> noDS
    +- apex DS records present                 -> hasDS

Parent decision:
   only undeterminedDS                          -> DS11_UNDETERMINED_DS; stop
   only noDS                                    -> DS11_NO_PARENT_DS;    stop
   mixed noDS and hasDS                         -> DS11_INCONSISTENT_DS
                                                   DS11_PARENT_WITHOUT_DS (addresses)
                                                   DS11_PARENT_WITH_DS    (addresses)
                                                   then continue to child phase
   only hasDS                                   -> continue to child phase
```
{{% /expand %}}

### Child DNSKEY Phase (steps 6-7)

{{% expand "Show diagram" %}}
```
child set = GlueNameservers ++ ApexNameservers; dedupe by IP

For each unique child NS IP (parallel):

   transport disabled for SOA/DNSKEY -> IPV4_DISABLED / IPV6_DISABLED, skip
   query SOA at z.Name, DNSSEC=on, UseVC=false
    +- no usable authoritative apex SOA          -> undeterminedSignedZone
   query DNSKEY at z.Name, DNSSEC=on, UseVC=false
      (resp.TC() -> retry with UseVC=true)
    +- no usable response                        -> undeterminedSignedZone
    +- no apex DNSKEY in answer                  -> noDNSKEY
    +- apex DNSKEY records in answer             -> hasDNSKEY

Child decision:
   only undeterminedSignedZone                   -> DS11_UNDETERMINED_SIGNED_ZONE
   only noDNSKEY                                 -> DS11_DS_BUT_UNSIGNED_ZONE
   mixed noDNSKEY and hasDNSKEY                  -> DS11_INCONSISTENT_SIGNED_ZONE
                                                    DS11_NS_WITH_UNSIGNED_ZONE (addresses)
                                                    DS11_NS_WITH_SIGNED_ZONE   (addresses)
   only hasDNSKEY (no undetermined, no noDNSKEY) -> DS11_CONSISTENT_SIGNED

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS11_CONSISTENT_SIGNED` | Parent has DS and all child nameservers have DNSKEY - zone is consistently signed. |
| `DS11_DS_BUT_UNSIGNED_ZONE` | Parent DS indicates signing expectation but child nameservers show no DNSKEY evidence. |
| `DS11_INCONSISTENT_DS` | Parent nameservers disagree on DS existence. |
| `DS11_INCONSISTENT_SIGNED_ZONE` | Child nameservers disagree on DNSKEY presence. |
| `DS11_NS_WITH_SIGNED_ZONE` | Child nameservers in the signed subset are listed. |
| `DS11_NS_WITH_UNSIGNED_ZONE` | Child nameservers in the unsigned subset are listed. |
| `DS11_PARENT_WITHOUT_DS` | Parent nameservers without DS are listed in mixed-DS state. |
| `DS11_PARENT_WITH_DS` | Parent nameservers with DS are listed in mixed-DS state. |
| `DS11_UNDETERMINED_DS` | Parent DS state could not be determined at all. |
| `DS11_NO_PARENT_DS` | All parent nameservers report no DS record - zone is unsigned from parent view. |
| `DS11_UNDETERMINED_SIGNED_ZONE` | Child signed state could not be determined at all. |
| `IPV4_DISABLED` | IPv4 transport is disabled for queried parent/child rrtypes. |
| `IPV6_DISABLED` | IPv6 transport is disabled for queried parent/child rrtypes. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS11_CONSISTENT_SIGNED` | `-` | `-` | No arguments. |
| `DS11_DS_BUT_UNSIGNED_ZONE` | `-` | `-` | No arguments. |
| `DS11_INCONSISTENT_DS` | `-` | `-` | No arguments. |
| `DS11_INCONSISTENT_SIGNED_ZONE` | `-` | `-` | No arguments. |
| `DS11_NS_WITH_SIGNED_ZONE` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS11_NS_WITH_UNSIGNED_ZONE` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS11_PARENT_WITHOUT_DS` | `addresses` | `array<string>` | Structured parent nameserver IP list without DS. |
| `DS11_PARENT_WITH_DS` | `addresses` | `array<string>` | Structured parent nameserver IP list with DS. |
| `DS11_NO_PARENT_DS` | `-` | `-` | No arguments. |
| `DS11_UNDETERMINED_DS` | `-` | `-` | No arguments. |
| `DS11_UNDETERMINED_SIGNED_ZONE` | `-` | `-` | No arguments. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`, `SOA`, or `DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`, `SOA`, or `DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC11`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC11`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS11_CONSISTENT_SIGNED` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_DS_BUT_UNSIGNED_ZONE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_INCONSISTENT_DS` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_INCONSISTENT_SIGNED_ZONE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_NS_WITH_SIGNED_ZONE` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_NS_WITH_UNSIGNED_ZONE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_PARENT_WITHOUT_DS` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_PARENT_WITH_DS` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_NO_PARENT_DS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_UNDETERMINED_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS11_UNDETERMINED_SIGNED_ZONE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec11.md`](../../upstream/tests/DNSSEC-TP/dnssec11.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes normal parent lookup through z.Parent(ctx) and undelegated behavior from test-type inputs. Gonemaster: uses the `parentNameservers` abstraction plus a fake-DS shortcut via nameserver `FakeDSRecords`.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameserver evaluation is deduplicated by IP; repeated names on one IP share one DS11 outcome.
- When parent evaluation yields only `No DS` (and no `Has DS`), testcase emits `DS11_NO_PARENT_DS` and exits without child checks.
- Child-side `undetermined` classification requires SOA preconditions to pass first; unusable SOA responses are skipped before DNSKEY classification.
