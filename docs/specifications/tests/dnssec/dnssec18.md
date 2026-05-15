# DNSSEC18 (dnssec18)

Status: Final

## Purpose
- Validate that CDS and CDNSKEY RRsets are signed by a DNSKEY that corresponds to DS information observed at the parent side.
- Detect whether CDS/CDNSKEY content matches, differs from, or is absent relative to the parent DS RRset, and surface evidence of in-progress KSK rollovers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent nameservers from `parentNameservers` and parent DS responses.
  - Child nameservers from `methods.Method4` and `methods.Method5`.
  - Child CDS, CDNSKEY, and DNSKEY responses with DNSSEC enabled.
  - CDS/CDNSKEY/DNSKEY answer-section RRSIG records.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query and validation fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build parent nameserver set, deduplicate by IP.
3. For each unique parent nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DS` and skip.
   - Query parent `DS` for child name with DNSSEC enabled.
   - Require response message, `RCODE=NOERROR`, and `AA=true`.
   - Collect matching-owner DS records and deduplicate by `(keytag,digestType,algorithm,digest)`.
4. If no DS records were collected, stop DS18 findings.
5. Build child nameserver set from Method4+Method5, deduplicate by IP.
6. For each unique child nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `CDNSKEY`, `CDS`, and `DNSKEY` and skip.
   - Query `CDS`, `CDNSKEY`, and `DNSKEY` with DNSSEC enabled; each requires authoritative `NOERROR` response for participation.
   - Track whether CDS and/or CDNSKEY RRset is present, plus their answer-section RRSIG records and actual record content.
   - Track DNSKEY RRset records and answer-section RRSIG records (TypeCovered == DNSKEY) for that nameserver.
7. If neither CDS nor CDNSKEY RRsets are present, or DNSKEY RRsets are absent, skip RRSIG-vs-DS validation (steps 8-12). Steps 13-16 are independently gated: content comparison (steps 13-14) runs when the relevant CDS/CDNSKEY RRset is present (regardless of DNSKEY presence); soft signals (step 15) and the on-demand absence check (step 16) run when DNSKEY records are available.
8. For each nameserver with CDS RRset:
   - Search DS records for any DS whose keytag exists in nameserver DNSKEY RRset and also in CDS RRSIG keytags.
   - If no such DS/keytag match exists, mark nameserver for `DS18_NO_MATCH_CDS_RRSIG_DS`.
9. For each nameserver with CDNSKEY RRset:
   - Search DS records for any DS whose keytag exists in nameserver DNSKEY RRset and also in CDNSKEY RRSIG keytags.
   - If no such DS/keytag match exists, mark nameserver for `DS18_NO_MATCH_CDNSKEY_RRSIG_DS`.
10. Emit marked DS18 findings with `addresses`.
11. For nameservers with CDS RRset that were not marked for `DS18_NO_MATCH_CDS_RRSIG_DS`, collect IPs and emit `DS18_MATCH_CDS_RRSIG_DS`.
12. For nameservers with CDNSKEY RRset that were not marked for `DS18_NO_MATCH_CDNSKEY_RRSIG_DS`, collect IPs and emit `DS18_MATCH_CDNSKEY_RRSIG_DS`.
13. **CDS-vs-DS content comparison** (using the first nameserver with at least one non-DELETE CDS record; nameservers with only DELETE sentinels where Algorithm == 0 are skipped):
    - Build `cdsKeySet`: canonical set of `(keyTag, algorithm, digestType, digest)` for non-DELETE CDS records.
    - Build `dsKeySet`: same structure from parent DS records.
    - If `cdsKeySet == dsKeySet`, emit `DS18_CDS_MATCHES_DS` with `cds_keytags` and `ds_keytags`.
    - Otherwise emit `DS18_CDS_ROLLOVER_SIGNALED` with `cds_keytags` and `ds_keytags`.
14. **CDNSKEY-vs-DS content comparison** (using the first nameserver with at least one non-DELETE CDNSKEY record):
    - Collect `digestTypes` from the parent DS RRset.
    - Build `dsKeytagSet` from parent DS records.
    - Each non-DELETE CDNSKEY's `KeyTag()` must appear in `dsKeytagSet`; otherwise the comparison is a mismatch.
    - For each non-DELETE CDNSKEY × digestType, compute a DS-equivalent digest using `DNSKEY.ToDS(digestType)`.
    - If every CDNSKEY's keytag is in `dsKeytagSet`, every parent DS entry is covered by some CDNSKEY-derived digest, and every CDNSKEY contributes at least one matching DS entry, emit `DS18_CDNSKEY_MATCHES_DS` with `cdnskey_keytags` and `ds_keytags`.
    - Otherwise emit `DS18_CDNSKEY_ROLLOVER_SIGNALED` with `cdnskey_keytags` and `ds_keytags`.
15. **Soft rollover signals** (evaluated whenever DNSKEY records are available, using first representative nameserver):
    - `sepKeytags` = keytags of DNSKEYs with SEP flag.
    - `dnskeySigners` = keytags from DNSKEY RRSIGs that appear in `sepKeytags`.
    - `dsKeytags` = keytags from parent DS records.
    - `dnskeyKeytagSet` = all DNSKEY keytags.
    - If `|sepKeytags| > 1`, emit `DS18_ROLLOVER_EVIDENCE_MULTI_KSK` with `keytags`.
    - If `|dnskeySigners| > 1`, emit `DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG` with `keytags`.
    - If `dsKeytags \ dnskeyKeytagSet` is non-empty, emit `DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY` with `keytags`.
    - If `sepKeytags \ dsKeytags` is non-empty, emit `DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS` with `keytags`.
16. **On-demand absence check**: if no CDS or CDNSKEY RRsets were found and at least one step-15 soft signal was emitted, emit `DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE`. (Steps 13-14 cannot fire when CDS/CDNSKEY are absent, so step 15 is the only contributor in practice.)
17. Emit `TEST_CASE_END`.

## Rollover Evidence And Scoring

`DS18_CDS_ROLLOVER_SIGNALED`, `DS18_CDNSKEY_ROLLOVER_SIGNALED`, and the four `DS18_ROLLOVER_EVIDENCE_*` tags are the rollover-evidence signals. `DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE` is emitted when no CDS/CDNSKEY RRsets are present but at least one step-15 soft signal fired. This is the typical pattern for on-demand CDS/CDNSKEY publication (e.g. Knot DNS outside its rollover window).

`DS18_MATCH_CDS_RRSIG_DS` and `DS18_CDS_ROLLOVER_SIGNALED` can coexist by design (and likewise for CDNSKEY): the former reports that the CDS RRSIG chains to a DS-linked DNSKEY (signature is verifiable), the latter reports that the CDS content advertises a different DS state. Mid-rollover, both are expected.

The bonus criterion `cds_cdnskey_published` in `scoring/bonus.go` treats `DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE` as nil (not-applicable), so a zone using on-demand publication is not penalised for absent CDS/CDNSKEY during or after a rollover. `DS15_HAS_*` tags take precedence: if any positive DS15 tag is present, the criterion is `true` regardless of DS18 evidence.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS18_CDS_MATCHES_DS` | CDS RRset content equals the parent DS RRset. Steady-state confirmation. |
| `DS18_CDS_ROLLOVER_SIGNALED` | CDS RRset content differs from parent DS. Parent is being asked to change its DS. |
| `DS18_CDNSKEY_MATCHES_DS` | CDNSKEY digests (using parent DS digest types) equal the parent DS RRset. |
| `DS18_CDNSKEY_ROLLOVER_SIGNALED` | CDNSKEY digests differ from parent DS. |
| `DS18_MATCH_CDNSKEY_RRSIG_DS` | CDNSKEY RRset is signed by a DS-linked DNSKEY keytag at at least one nameserver. |
| `DS18_MATCH_CDS_RRSIG_DS` | CDS RRset is signed by a DS-linked DNSKEY keytag at at least one nameserver. |
| `DS18_NO_MATCH_CDNSKEY_RRSIG_DS` | No DS-linked DNSKEY keytag matches any CDNSKEY RRSIG keytag for nameserver. |
| `DS18_NO_MATCH_CDS_RRSIG_DS` | No DS-linked DNSKEY keytag matches any CDS RRSIG keytag for nameserver. |
| `DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE` | No CDS/CDNSKEY RRset found but other rollover evidence is visible. On-demand publication model. |
| `DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG` | DNSKEY RRset is signed by more than one KSK keytag. Double-signature rollover phase. |
| `DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS` | Child has a SEP DNSKEY keytag with no matching DS at the parent. Pre-publication phase. |
| `DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY` | Parent DS keytag has no matching DNSKEY at the child. Post-removal phase. |
| `DS18_ROLLOVER_EVIDENCE_MULTI_KSK` | Child DNSKEY RRset contains more than one SEP-flagged key. |
| `IPV4_DISABLED` | IPv4 transport is disabled for queried parent/child rrtypes. |
| `IPV6_DISABLED` | IPv6 transport is disabled for queried parent/child rrtypes. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS18_CDS_MATCHES_DS` | `cds_keytags` | `array<uint16>` | Sorted keytags present in the CDS RRset. |
| `DS18_CDS_MATCHES_DS` | `ds_keytags` | `array<uint16>` | Sorted keytags present in the parent DS RRset. |
| `DS18_CDS_ROLLOVER_SIGNALED` | `cds_keytags` | `array<uint16>` | Sorted keytags present in the CDS RRset. |
| `DS18_CDS_ROLLOVER_SIGNALED` | `ds_keytags` | `array<uint16>` | Sorted keytags present in the parent DS RRset. |
| `DS18_CDNSKEY_MATCHES_DS` | `cdnskey_keytags` | `array<uint16>` | Sorted keytags present in the CDNSKEY RRset. |
| `DS18_CDNSKEY_MATCHES_DS` | `ds_keytags` | `array<uint16>` | Sorted keytags present in the parent DS RRset. |
| `DS18_CDNSKEY_ROLLOVER_SIGNALED` | `cdnskey_keytags` | `array<uint16>` | Sorted keytags present in the CDNSKEY RRset. |
| `DS18_CDNSKEY_ROLLOVER_SIGNALED` | `ds_keytags` | `array<uint16>` | Sorted keytags present in the parent DS RRset. |
| `DS18_MATCH_CDNSKEY_RRSIG_DS` | `addresses` | `array<string>` | Structured child nameserver IP list with matching CDNSKEY RRSIG. |
| `DS18_MATCH_CDS_RRSIG_DS` | `addresses` | `array<string>` | Structured child nameserver IP list with matching CDS RRSIG. |
| `DS18_NO_MATCH_CDNSKEY_RRSIG_DS` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS18_NO_MATCH_CDS_RRSIG_DS` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE` | - | - | No arguments. |
| `DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG` | `keytags` | `array<uint16>` | Sorted KSK keytags that each signed the DNSKEY RRset. |
| `DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS` | `keytags` | `array<uint16>` | Sorted SEP DNSKEY keytags without a matching parent DS. |
| `DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY` | `keytags` | `array<uint16>` | Sorted DS keytags without a matching child DNSKEY. |
| `DS18_ROLLOVER_EVIDENCE_MULTI_KSK` | `keytags` | `array<uint16>` | Sorted keytags of all SEP-flagged DNSKEYs found. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`, `CDS`, `CDNSKEY`, or `DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`, `CDS`, `CDNSKEY`, or `DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC18`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC18`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS18_CDS_MATCHES_DS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_CDS_ROLLOVER_SIGNALED` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_CDNSKEY_MATCHES_DS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_CDNSKEY_ROLLOVER_SIGNALED` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_MATCH_CDNSKEY_RRSIG_DS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_MATCH_CDS_RRSIG_DS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_NO_MATCH_CDNSKEY_RRSIG_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_NO_MATCH_CDS_RRSIG_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_ROLLOVER_EVIDENCE_MULTI_KSK` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec18.md`](../../upstream/tests/DNSSEC-TP/dnssec18.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: includes explicit undelegated test-type flow using provided DS data. Gonemaster: has no explicit undelegated branch in `DNSSEC18`; it always obtains parent-side DS through `parentNameservers`.
  - Upstream: objective describes trust from DS to CDS/CDNSKEY signatures via corresponding DNSKEY. Gonemaster: matching logic is keytag-based (`DS keytag` present in DNSKEY set and RRSIG keytags), without DS digest revalidation or CDS/CDNSKEY signature cryptographic verification in this testcase.
  - Upstream: does not specify rollover-detection logic. Gonemaster: adds CDS/CDNSKEY content comparison against parent DS and soft rollover-evidence signals (steps 13-16).
  - Upstream: does not explicitly specify testcase boundary and transport-disabled debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- All DS18 findings (RRSIG-vs-DS, content comparison, and soft signals) require the parent DS RRset to be non-empty (step 4 stops the test otherwise).
- DS18 RRSIG-vs-DS findings (steps 8-12) are additionally skipped when CDS/CDNSKEY RRsets are absent or when DNSKEY RRsets are absent.
- Content comparison (steps 13-14) and soft signals (step 15) use only the first representative nameserver per RRset type; inconsistencies across nameservers are flagged separately by DNSSEC15.
- A nameserver that publishes only DELETE-sentinel CDS/CDNSKEY records (Algorithm == 0) is treated as having no comparable content and is skipped for steps 13-14 in favour of the next nameserver. If every nameserver carries only DELETE sentinels, neither `MATCHES_DS` nor `ROLLOVER_SIGNALED` is emitted; DNSSEC16/17 covers the DELETE flow.
- Soft rollover signals (step 15) run regardless of CDS/CDNSKEY presence whenever DNSKEY records are available (and DS records are present, per the outer guard).
- CDNSKEY digest comparison reuses only digest types already present in the parent DS RRset; no new digest types are introduced. If the parent publishes DS in multiple digest types and the child's CDS RRset omits some types, the canonical sets differ and `DS18_CDS_ROLLOVER_SIGNALED` fires. Per RFC 7344 the CDS RRset is the operator's complete request, so reduced digest-type coverage is a legitimate change.
- Parent DS collection deduplicates by DS content fields and ignores duplicates across parent nameservers.
- Nameserver evaluation is deduplicated by IP on both parent and child sides.
