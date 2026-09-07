# DNSSEC11

Status: Final

## Purpose
- Verify that parent-side DS presence is consistent with the child's signing state. A child nameserver counts as signed when it serves an apex DNSKEY RRset with at least one RRSIG covering DNSKEY. Parent consistency and child consistency are reported separately.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent nameservers from [`ParentNameservers`](../../nameserver-resolution.md#parentnameservers).
  - Child nameservers from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - DS, SOA, and DNSKEY (DO bit set) query responses.
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
   - Build child nameserver set from the union of [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) (deduplicated by `ns.String()`), then deduplicate by IP.
   - For each nameserver (parallelized):
     - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `SOA` and `DNSKEY` and skip.
     - Query SOA over UDP (`UseVC=false`); require usable authoritative apex SOA.
     - Query DNSKEY with DNSSEC enabled over UDP (`UseVC=false`), retry over TCP on truncation.
     - Classify the nameserver as `Undetermined signed zone` when no usable authoritative `NOERROR` DNSKEY response is available; as `Unsigned` when the apex DNSKEY RRset is absent, or present without an RRSIG covering DNSKEY in the answer section; as `Signed` when apex DNSKEY records and at least one such RRSIG are present.
7. Child decision:
   - `Undetermined` only => emit `DS11_UNDETERMINED_SIGNED_ZONE`.
   - `Unsigned` only => emit `DS11_DS_BUT_UNSIGNED_ZONE`.
   - Mixed `Unsigned` and `Signed` => emit `DS11_INCONSISTENT_SIGNED_ZONE`, `DS11_NS_WITH_UNSIGNED_ZONE` for the unsigned subset, `DS11_NS_WITH_SIGNED_ZONE` for the signed subset.
   - `Signed` only (no undetermined, no unsigned) => emit `DS11_CONSISTENT_SIGNED`.
8. Emit `TEST_CASE_END`.

### Parent DS Phase (steps 4-5)

{{% expand "Show diagram" %}}
```
parent set = ParentNameservers; dedupe by IP

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
   query SOA at z.Name, DNSSEC=off, UseVC=false
    +- no usable authoritative apex SOA          -> skipped, no child classification
   query DNSKEY at z.Name, DNSSEC=on, UseVC=false
      (resp.TC() -> retry with UseVC=true)
    +- no usable response                        -> undeterminedSignedZone
    +- no apex DNSKEY in answer                  -> unsigned
    +- apex DNSKEY, no RRSIG covering DNSKEY     -> unsigned
    +- apex DNSKEY and RRSIG covering DNSKEY     -> signed

Child decision:
   only undeterminedSignedZone                   -> DS11_UNDETERMINED_SIGNED_ZONE
   only unsigned                                 -> DS11_DS_BUT_UNSIGNED_ZONE
   mixed unsigned and signed                     -> DS11_INCONSISTENT_SIGNED_ZONE
                                                    DS11_NS_WITH_UNSIGNED_ZONE (addresses)
                                                    DS11_NS_WITH_SIGNED_ZONE   (addresses)
   only signed (no undetermined, no unsigned)    -> DS11_CONSISTENT_SIGNED

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS11_CONSISTENT_SIGNED` | Parent has DS and every child nameserver serves a signed DNSKEY RRset. |
| `DS11_DS_BUT_UNSIGNED_ZONE` | Parent has DS but no child nameserver serves a signed DNSKEY RRset, because the RRset is absent or unsigned. |
| `DS11_INCONSISTENT_DS` | Parent nameservers disagree on DS existence. |
| `DS11_INCONSISTENT_SIGNED_ZONE` | Child nameservers disagree on whether the DNSKEY RRset is signed. |
| `DS11_NS_WITH_SIGNED_ZONE` | Child nameservers serving a signed DNSKEY RRset are listed. |
| `DS11_NS_WITH_UNSIGNED_ZONE` | Child nameservers not serving a signed DNSKEY RRset are listed. |
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
- Differences (Upstream vs Gonemaster):
  - Upstream: describes normal parent lookup through z.Parent(ctx) and undelegated behavior from test-type inputs. Gonemaster: uses the [`ParentNameservers`](../../nameserver-resolution.md#parentnameservers) abstraction plus a fake-DS shortcut via nameserver `FakeDSRecords`.
  - Upstream: the child's signing state is decided by DNSKEY presence in a query without the DO bit. Gonemaster: a DNSKEY RRset must carry an RRSIG covering DNSKEY, observed with the DO bit, which is the same test DNSSEC07 applies (`DIV-DS11-UNSIGNED-DNSKEY`).
  - Upstream: emits nothing when every child nameserver serves the DNSKEY RRset. Gonemaster: emits `DS11_CONSISTENT_SIGNED`, which has no upstream counterpart.
  - Upstream: emits nothing when no parent nameserver publishes DS. Gonemaster: emits `DS11_NO_PARENT_DS`, which has no upstream counterpart.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: A child that publishes an apex DNSKEY RRset counts as signed, so a DS at the parent raises no finding even when the RRset carries no signature.
  - Gonemaster observed behavior: The child counts as signed only when the apex DNSKEY RRset carries an RRSIG covering DNSKEY, so a DS at the parent of such a zone yields `DS11_DS_BUT_UNSIGNED_ZONE` at `ERROR`.
  - evidence: `engine/test/dnssec/dnssec.go` (DNSSEC11 child DNSKEY phase). Validating resolvers return SERVFAIL for a zone in this state, so the delegation is unusable while upstream reports it as signed.
  - report status: `filed` ([zonemaster-engine#1549](https://github.com/zonemaster/zonemaster-engine/issues/1549))

## Edge Cases And Limitations
- Nameserver evaluation is deduplicated by IP; repeated names on one IP share one DS11 outcome.
- When parent evaluation yields only `No DS` (and no `Has DS`), testcase emits `DS11_NO_PARENT_DS` and exits without child checks.
- Child-side `undetermined` classification requires SOA preconditions to pass first; unusable SOA responses are skipped before DNSKEY classification.
- A zone that publishes DNSKEY records but serves no RRSIG over them is unsigned to a validating resolver. With a DS at the parent this is `DS11_DS_BUT_UNSIGNED_ZONE`.
- The child DNSKEY query shares its cache key with the DNSSEC07 DNSKEY query, so in a full run the child phase issues no additional queries.
