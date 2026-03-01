# DNSSEC18 (dnssec18)

Status: Final

## Purpose
- Validate that CDS and CDNSKEY RRsets are signed by a DNSKEY that corresponds to DS information observed at the parent side.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent nameservers from `parentNameservers` and parent DS responses.
  - Child nameservers from `methods.Method4` and `methods.Method5`.
  - Child CDS, CDNSKEY, and DNSKEY responses with DNSSEC enabled.
  - CDS/CDNSKEY answer-section RRSIG records.
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
   - Track whether CDS and/or CDNSKEY RRset is present, plus their answer-section RRSIG records.
   - Track DNSKEY RRset records for that nameserver.
7. If neither CDS nor CDNSKEY RRsets are present, or DNSKEY RRsets are absent, stop DS18 findings.
8. For each nameserver with CDS RRset:
   - Search DS records for any DS whose keytag exists in nameserver DNSKEY RRset and also in CDS RRSIG keytags.
   - If no such DS/keytag match exists, mark nameserver for `DS18_NO_MATCH_CDS_RRSIG_DS`.
9. For each nameserver with CDNSKEY RRset:
   - Search DS records for any DS whose keytag exists in nameserver DNSKEY RRset and also in CDNSKEY RRSIG keytags.
   - If no such DS/keytag match exists, mark nameserver for `DS18_NO_MATCH_CDNSKEY_RRSIG_DS`.
10. Emit marked DS18 findings with `ns_ip_list`.
11. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS18_NO_MATCH_CDNSKEY_RRSIG_DS` | No DS-linked DNSKEY keytag matches any CDNSKEY RRSIG keytag for nameserver. |
| `DS18_NO_MATCH_CDS_RRSIG_DS` | No DS-linked DNSKEY keytag matches any CDS RRSIG keytag for nameserver. |
| `IPV4_DISABLED` | IPv4 transport is disabled for queried parent/child rrtypes. |
| `IPV6_DISABLED` | IPv6 transport is disabled for queried parent/child rrtypes. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS18_NO_MATCH_CDNSKEY_RRSIG_DS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS18_NO_MATCH_CDS_RRSIG_DS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`, `CDS`, `CDNSKEY`, or `DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`, `CDS`, `CDNSKEY`, or `DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC18`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC18`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS18_NO_MATCH_CDNSKEY_RRSIG_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS18_NO_MATCH_CDS_RRSIG_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec18.md`](../../upstream/tests/DNSSEC-TP/dnssec18.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: includes explicit undelegated test-type flow using provided DS data. Gonemaster: has no explicit undelegated branch in `DNSSEC18`; it always obtains parent-side DS through `parentNameservers`.
  - Upstream: objective describes trust from DS to CDS/CDNSKEY signatures via corresponding DNSKEY. Gonemaster: matching logic is keytag-based (`DS keytag` present in DNSKEY set and RRSIG keytags), without DS digest revalidation or CDS/CDNSKEY signature cryptographic verification in this testcase.
  - Upstream: does not explicitly specify testcase boundary and transport-disabled debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- DS18 findings are skipped when DS records are absent, when CDS/CDNSKEY RRsets are absent, or when DNSKEY RRsets are absent.
- Parent DS collection deduplicates by DS content fields and ignores duplicates across parent nameservers.
- Nameserver evaluation is deduplicated by IP on both parent and child sides.
