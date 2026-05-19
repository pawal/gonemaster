# DNSSEC21

Status: Draft

## Purpose
- Verify that the parent zone correctly signs the DS RRset that delegates the child zone.
- Each parent nameserver must return the DS RRset for the child along with at least one RRSIG that validates against a published parent DNSKEY. Catches parent-side DNSSEC failures (broken key rollovers, expired or misissued RRSIGs over DS) that resolvers experience as a SERVFAIL chain break but that other DNSSEC testcases cannot see when the *child* zone is the test target.
- Motivation: a parent zone with a broken DS RRSIG breaks resolution for every child it delegates, even when the child is configured correctly. Existing DNSSEC testcases all verify signatures *inside* the zone under test (child DNSKEY, child SOA, child NSEC/NSEC3, DS digest match); none validates the signature on the DS RRset *handed down* by the parent. DNSSEC02 reads the DS records from the parent's response but does not check the covering RRSIG. DNSSEC07 inspects whether an RRSIG covering DS is present in the parent's response but does not verify it cryptographically. DNSSEC21 closes that gap. Without it, an operator running gonemaster on their own domain can see "DNSSEC OK" while users see SERVFAIL.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - The zone is delegated (a non-root parent exists). Root zone runs of DNSSEC21 emit `TEST_CASE_END` and stop.
- Required inputs:
  - Parent zone object (resolved via `zoneParent(ctx, z)`).
  - Parent nameserver names/IPs from `ParentNameservers`.
  - Per parent NS IP: DS query response for the child name (DNSSEC enabled), and DNSKEY query response for the parent apex (DNSSEC enabled).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel parent NS query execution fanout.
  - `dnssec.signature_validity_skew`: existing clock-skew tolerance reused by `verifyRRSIG`.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Resolve parent zone:
   - If the zone has no parent (root) or `zoneParent` fails, emit `DS21_NO_PARENT_ZONE` and `TEST_CASE_END`, then stop.
3. Build parent nameserver set from `getParentNSNamesAndIPs`, deduplicate by IP.
4. For each unique parent NS IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `DS` and `DNSKEY`, mark NS ignored, and skip.
   - Query `DS` for `z.Name` with DNSSEC enabled (UDP, TCP fallback on TC).
     - No response, non-`NOERROR`, non-`AA`, non-`DO`, or no OPT -> classify NS as `Undetermined DS` and skip.
     - No DS records and no NSEC/NSEC3 proof in authority -> classify NS as `No DS`; do not emit DNSSEC21 findings (DNSSEC07/11 already cover unsigned-delegation cases).
     - DS records present -> retain DS RRset and answer-section RRSIG records covering DS.
   - Query `DNSKEY` for parent apex (`parent.Name`) with DNSSEC enabled.
     - No response, non-`NOERROR`, non-`AA`, or no apex DNSKEY -> classify NS as `No Parent DNSKEY` and emit `DS21_PARENT_DNSKEY_MISSING`.
   - For each RRSIG in the DS answer that covers `TypeDS` and whose signer matches `parent.Name`:
     - If inception is in the future, mark `DS21_DS_RRSIG_NOT_YET_VALID` by keytag.
     - Else if expiration is in the past, mark `DS21_DS_RRSIG_EXPIRED` by keytag.
     - Else select parent DNSKEY candidates by keytag from the parent DNSKEY RRset. If none, mark `DS21_NO_DNSKEY_FOR_DS_RRSIG` by keytag.
     - Else for each candidate DNSKEY:
       - If the signature algorithm is unsupported, mark `DS21_ALGO_NOT_SUPPORTED` by keytag+algo.
       - Else call `verifyRRSIG(sig, dsRRset, dnskey, testTime)`.
         - On success, mark NS as having a verified DS RRSIG (track keytag).
         - On failure, mark `DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY` by keytag.
   - If the DS RRset has at least one RRSIG but none verified against a parent DNSKEY, mark NS as `DS RRSIG not verifiable`.
   - If the DS RRset has no answer-section RRSIG covering DS, mark NS in `DS21_NO_DS_RRSIG`.
5. Aggregate results across parent NS IPs:
   - For each per-keytag finding, emit the corresponding `DS21_*` tag with merged `addresses` (parent NS IPs).
   - If at least one parent NS verified the DS RRSIG, emit `DS21_DS_RRSIG_VERIFIED` with the list of verifying parent NS IPs.
   - If no parent NS verified the DS RRSIG and at least one parent NS returned a signed DS RRset, emit `DS21_DS_RRSIG_NOT_VERIFIABLE` with the list of parent NS IPs that returned an unverifiable signed DS.
6. Emit `TEST_CASE_END`.

### Per-Parent-NS DS RRSIG Verification (steps 2-5)

{{% expand "Show diagram" %}}
```
zoneParent(ctx, z)
   z is root OR zoneParent returns error
      -> DS21_NO_PARENT_ZONE (zone = z.Name)
         emit TEST_CASE_END and stop

parent set = ParentNameservers; dedupe by IP

For each unique parent NS IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for DS/DNSKEY -> IPV4_DISABLED / IPV6_DISABLED, mark ignored, skip
   query DS at z.Name, DNSSEC=on (UDP; TC -> retry over TCP)
    +- no resp / RCODE != NOERROR / !AA / no OPT / !DO -> undeterminedDS, skip
    +- no DS records AND no NSEC/NSEC3 proof in authority -> noDS, skip (no DS21 findings)
    +- DS records present -> retain dsRRset, dsRRSIGs (RRSIGs covering DS, signer = parent.Name)

   query DNSKEY at parent.Name, DNSSEC=on
    +- no resp / RCODE != NOERROR / !AA / no apex DNSKEY
         -> DS21_PARENT_DNSKEY_MISSING (parent_zone, addresses); skip verification

   no RRSIG covering DS in answer
         -> DS21_NO_DS_RRSIG (addresses)

   For each RRSIG over DS (signer matches parent.Name; per-sig keyed by sig.KeyTag):
     sig.Inception in future          -> DS21_DS_RRSIG_NOT_YET_VALID[keytag][ns]
     sig.Expiration in past           -> DS21_DS_RRSIG_EXPIRED[keytag][ns]
     no parent DNSKEY with keytag     -> DS21_NO_DNSKEY_FOR_DS_RRSIG[keytag][ns]
     for each candidate DNSKEY:
        algorithm unsupported         -> DS21_ALGO_NOT_SUPPORTED[keytag][algo][ns]
        verifyRRSIG(sig, dsRRset, dnskey, testTime)
           success                    -> mark NS as having a verified DS RRSIG (keytag)
           failure                    -> DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY[keytag][ns]
   any RRSIG present but none verified
         -> mark NS as "DS RRSIG not verifiable"

Aggregation:
   per keytag entries -> emit corresponding DS21_* tag with merged addresses
   any parent NS verified DS RRSIG
         -> DS21_DS_RRSIG_VERIFIED (keytag, addresses)
   none verified AND at least one parent NS returned a signed DS RRset
         -> DS21_DS_RRSIG_NOT_VERIFIABLE (addresses)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS21_ALGO_NOT_SUPPORTED` | DS RRSIG verification requires an unsupported algorithm for this build/runtime. |
| `DS21_DS_RRSIG_EXPIRED` | DS-covering RRSIG has expired (signature inception/expiration is past). |
| `DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY` | DS-covering RRSIG was matched to a parent DNSKEY by keytag but cryptographic verification failed. |
| `DS21_DS_RRSIG_NOT_VERIFIABLE` | At least one parent nameserver returned a DS RRset with RRSIG, but no parent NS produced a DS RRset whose RRSIG verified against a parent DNSKEY. |
| `DS21_DS_RRSIG_NOT_YET_VALID` | DS-covering RRSIG inception time is in the future. |
| `DS21_DS_RRSIG_VERIFIED` | At least one parent nameserver returned a DS RRset whose RRSIG verifies against a published parent DNSKEY. |
| `DS21_NO_DNSKEY_FOR_DS_RRSIG` | DS-covering RRSIG references a keytag for which no parent DNSKEY is published. |
| `DS21_NO_DS_RRSIG` | Parent nameserver returned the DS RRset without any answer-section RRSIG covering DS. |
| `DS21_NO_PARENT_ZONE` | The zone under test has no resolvable parent (root zone) and the testcase cannot run. |
| `DS21_PARENT_DNSKEY_MISSING` | Parent nameserver did not return an authoritative apex DNSKEY response, so DS RRSIG verification cannot be attempted. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried parent nameserver (`DS` or `DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried parent nameserver (`DS` or `DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS21_ALGO_NOT_SUPPORTED` | `keytag` | `int` | DS RRSIG keytag whose algorithm cannot be processed. |
| `DS21_ALGO_NOT_SUPPORTED` | `algo_num` | `int` | DNSSEC algorithm number. |
| `DS21_ALGO_NOT_SUPPORTED` | `algo_mnemo` | `string` | DNSSEC algorithm mnemonic string. |
| `DS21_ALGO_NOT_SUPPORTED` | `addresses` | `array<string>` | Parent nameserver IPs that returned the unsupported-algo DS RRSIG. |
| `DS21_DS_RRSIG_EXPIRED` | `keytag` | `int` | Keytag of the expired DS-covering RRSIG. |
| `DS21_DS_RRSIG_EXPIRED` | `addresses` | `array<string>` | Parent nameserver IPs. |
| `DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY` | `keytag` | `int` | Keytag of the failing DS-covering RRSIG. |
| `DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY` | `addresses` | `array<string>` | Parent nameserver IPs. |
| `DS21_DS_RRSIG_NOT_VERIFIABLE` | `addresses` | `array<string>` | Parent nameserver IPs that returned a DS RRset with no verifying RRSIG. |
| `DS21_DS_RRSIG_NOT_YET_VALID` | `keytag` | `int` | Keytag of the not-yet-valid DS-covering RRSIG. |
| `DS21_DS_RRSIG_NOT_YET_VALID` | `addresses` | `array<string>` | Parent nameserver IPs. |
| `DS21_DS_RRSIG_VERIFIED` | `keytag` | `int` | Keytag of the DNSKEY that verified the DS RRSIG. |
| `DS21_DS_RRSIG_VERIFIED` | `addresses` | `array<string>` | Parent nameserver IPs that produced a verifying DS RRSIG. |
| `DS21_NO_DNSKEY_FOR_DS_RRSIG` | `keytag` | `int` | DS RRSIG keytag with no matching parent DNSKEY. |
| `DS21_NO_DNSKEY_FOR_DS_RRSIG` | `addresses` | `array<string>` | Parent nameserver IPs. |
| `DS21_NO_DS_RRSIG` | `addresses` | `array<string>` | Parent nameserver IPs that returned an unsigned DS RRset. |
| `DS21_NO_PARENT_ZONE` | `zone` | `string` | Zone name under test. |
| `DS21_PARENT_DNSKEY_MISSING` | `parent_zone` | `string` | Parent zone name whose DNSKEY response was missing or unauthoritative. |
| `DS21_PARENT_DNSKEY_MISSING` | `addresses` | `array<string>` | Parent nameserver IPs. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS` or `DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS` or `DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC21`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC21`). |

## Severity Levels Per Tag
The child zone operator cannot fix a parent-zone signing failure, so DNSSEC21 reports parent failures at `WARNING` rather than `ERROR`. The grade is dropped, but the failing testcase is the parent's, not the child's. Operators reading the report should pursue the parent registry/registrar.

| Tag | Level | Notes |
| --- | --- | --- |
| `DS21_ALGO_NOT_SUPPORTED` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS21_DS_RRSIG_EXPIRED` | `WARNING` | Parent operator is responsible; child cannot fix. |
| `DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY` | `WARNING` | Parent operator is responsible; child cannot fix. |
| `DS21_DS_RRSIG_NOT_VERIFIABLE` | `WARNING` | Parent operator is responsible; child cannot fix. |
| `DS21_DS_RRSIG_NOT_YET_VALID` | `WARNING` | Parent operator is responsible; child cannot fix. |
| `DS21_DS_RRSIG_VERIFIED` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS21_NO_DNSKEY_FOR_DS_RRSIG` | `WARNING` | Parent published RRSIG referring to an absent DNSKEY. |
| `DS21_NO_DS_RRSIG` | `WARNING` | Parent answered the DS query without a covering RRSIG. |
| `DS21_NO_PARENT_ZONE` | `DEBUG` | Root-zone runs cannot test parent DS signing. |
| `DS21_PARENT_DNSKEY_MISSING` | `WARNING` | Parent did not authoritatively answer DNSKEY at its apex. |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: No upstream Zonemaster equivalent. DNSSEC21 is a new testcase unique to gonemaster.
- Differences (Upstream vs Gonemaster):
  - Upstream: no testcase verifies the parent's RRSIG over the DS RRset that delegates the child. Existing upstream DNSSEC testcases cover only signatures published *inside* the zone under test (DNSKEY, SOA, NSEC/NSEC3 at child apex; DS digest match against child DNSKEY).
  - Gonemaster: implements parent-side DS RRSIG verification, surfacing parent-zone signing failures (e.g., broken key rollovers at the registry) that today are invisible when the child zone is the test target.
- Potential upstream report:
  - `yes` (see [issue draft](../../../../plans/upstream-issue-parent-ds-rrsig-validation.md))

## Edge Cases And Limitations
- Root zone (no parent): the testcase emits `DS21_NO_PARENT_ZONE` at DEBUG and exits without findings.
- Unsigned delegation (parent has no DS for the child): the testcase exits without DNSSEC21 findings; the unsigned-delegation case is already covered by DNSSEC07 and DNSSEC11.
- Undelegated runs (`hasFakeAddresses` is true): when synthetic parent data carries no real RRSIG, DNSSEC21 emits no findings; the testcase requires real parent NS responses.
- Parent NS deduplication is by IP. If multiple parent NS names share an IP, the IP is queried once and contributes once to each finding's `addresses` list.
- Verification reuses the existing `verifyRRSIG(sig, rrset, dnskey, at)` helper and the existing supported-algorithm policy. No new crypto is introduced.
- The DS query is the same query already issued by DNSSEC02 and DNSSEC07 against the parent. In a full test run the response is served from the per-nameserver query cache and adds no extra network traffic. The parent DNSKEY query is a single new query per parent NS IP (cached on subsequent testcases).
- DNSSEC21 does not validate the chain *above* the parent (parent's own DS at grandparent, root trust anchor). The goal is bounded to the parent->child link, which is the link broken in the .de incident.

## Evidence In Gonemaster
- Code paths (when implemented):
  - `engine/test/dnssec/dnssec.go` (DNSSEC21 testcase function).
  - `engine.dnssecTests` registration map.
- Related tests (when implemented):
  - `engine/test/dnssec/dnssec_test.go` (synthetic parent NS that returns a DS RRset with an unverifiable RRSIG; regression fixtures using `cachefile.Restore` against captured parent-failure traffic).
- References:
  - RFC 4033, 4034, 4035 (DNSSEC).
  - RFC 5155 (NSEC3, parent-side denial-of-DS handling).
