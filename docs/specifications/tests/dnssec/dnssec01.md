# DNSSEC01 (dnssec01)

Status: Draft

## Purpose
- Validate DS digest algorithm usage for the child delegation and classify each observed DS digest type.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent nameserver names/IPs from `methodsv2.GetParentNSNamesAndIPs`.
  - Optional undelegated DS records exposed via parent nameserver `FakeDSRecords`.
  - DS query responses for child zone name from parent nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel parent-query execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Initialize classification sets for DS digest classes, plus tracking sets for:
   - parent nameservers ignored due to invalid response shape,
   - parent nameservers responding without valid DS owner,
   - parent nameservers responding with DS,
   - keytags seen with digest algorithm 2 vs non-2.
3. Resolve parent nameserver names/IPs.
4. Check undelegated DS input path:
   - If parent fake DS records are present, classify those DS records immediately with source `ns_list` value `-`.
   - Mark parent-NS query list empty so external parent DS queries are skipped.
5. For each unique parent nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DS` and skip.
   - Send DS query with DNSSEC enabled.
   - If response is absent or fails required shape checks (`NOERROR`, `OPT`, `DO`, `AA`), mark nameserver as ignored.
   - If response has no DS record with owner matching child zone name, mark nameserver in `Responds Without Valid DS`.
   - Else mark nameserver in `Responds With DS` and classify each DS digest algorithm into the corresponding DS01 tag set.
6. Emit DS classification tags (`DS01_DS_ALGO_*`) grouped by `(digest, keytag)` with merged `ns_list`.
7. Emit `DS01_DS_ALGO_2_MISSING` for keytags that have non-2 DS but no digest-2 DS on the same source nameservers.
8. If no valid/non-valid DS responders exist and ignored responders exist, emit `DS01_NO_RESPONSE`.
9. Emit informational tags based on zone type and undelegated DS status:
   - If zone is root (`.`), emit `DS01_ROOT_N_NO_UNDEL_DS` when no undelegated DS records were provided.
   - If zone is undelegated and non-root, emit `DS01_UNDEL_N_NO_UNDEL_DS` when no undelegated DS records were provided.
10. If `Responds Without Valid DS` is non-empty:
    - Emit `DS01_PARENT_ZONE_NO_DS` when no nameserver responded with DS.
    - Otherwise emit `DS01_PARENT_SERVER_NO_DS`.
11. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS01_DS_ALGO_2_MISSING` | A keytag has non-2 DS entries but no digest algorithm 2 DS entry on the same source nameservers. |
| `DS01_DS_ALGO_DEPRECATED` | A DS digest algorithm value maps to deprecated class. |
| `DS01_DS_ALGO_NOT_DS` | A DS digest algorithm value is reserved as "not DS". |
| `DS01_DS_ALGO_OK` | A DS digest algorithm value maps to acceptable class. |
| `DS01_DS_ALGO_PRIVATE` | A DS digest algorithm value maps to private-use class. |
| `DS01_DS_ALGO_RESERVED` | A DS digest algorithm value maps to reserved class. |
| `DS01_DS_ALGO_UNASSIGNED` | A DS digest algorithm value maps to unassigned class. |
| `DS01_NO_RESPONSE` | All queried parent nameservers were ignored for invalid/no response shape and no DS/non-DS responder set was produced. |
| `DS01_PARENT_SERVER_NO_DS` | At least one parent nameserver returned a valid DS and at least one returned no valid DS. |
| `DS01_PARENT_ZONE_NO_DS` | Parent nameservers returned no valid DS and none returned DS. |
| `DS01_ROOT_N_NO_UNDEL_DS` | Tested zone is root and no undelegated DS records were provided. |
| `DS01_UNDEL_N_NO_UNDEL_DS` | Tested zone is undelegated and no undelegated DS records were provided. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried parent nameserver (`DS`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried parent nameserver (`DS`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS01_DS_ALGO_2_MISSING` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) where digest-2 DS is missing, or `-` for undelegated DS source. |
| `DS01_DS_ALGO_2_MISSING` | `keytag` | `int` | DNSKEY key tag referenced by DS records. |
| `DS01_DS_ALGO_DEPRECATED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_DEPRECATED` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_DEPRECATED` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_DEPRECATED` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_NOT_DS` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_NOT_DS` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_NOT_DS` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_NOT_DS` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_OK` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_OK` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_OK` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_OK` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_PRIVATE` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_PRIVATE` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_PRIVATE` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_PRIVATE` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_RESERVED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_RESERVED` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_RESERVED` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_RESERVED` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_UNASSIGNED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_UNASSIGNED` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_UNASSIGNED` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_UNASSIGNED` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_NO_RESPONSE` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) ignored due to invalid/no DS response shape. |
| `DS01_PARENT_SERVER_NO_DS` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) that returned no valid DS owner. |
| `DS01_PARENT_ZONE_NO_DS` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) that returned no valid DS owner. |
| `DS01_ROOT_N_NO_UNDEL_DS` | `-` | `-` | No arguments. |
| `DS01_UNDEL_N_NO_UNDEL_DS` | `-` | `-` | No arguments. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS01_DS_ALGO_2_MISSING` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_DS_ALGO_DEPRECATED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_DS_ALGO_NOT_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_DS_ALGO_OK` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_DS_ALGO_PRIVATE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_DS_ALGO_RESERVED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_DS_ALGO_UNASSIGNED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_NO_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_PARENT_SERVER_NO_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_PARENT_ZONE_NO_DS` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_ROOT_N_NO_UNDEL_DS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS01_UNDEL_N_NO_UNDEL_DS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec01.md`](../../upstream/tests/DNSSEC-TP/dnssec01.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
  - Upstream: Summary argument list omits `ds_algo_descr` for some DS01 tags. Gonemaster: includes `ds_algo_descr` for all emitted `DS01_DS_ALGO_*` classification tags.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If no parent nameservers are available and no undelegated DS data exists, only the applicable root/undelegated informational tags and testcase boundary tags are emitted.
- For undelegated DS input, `ns_list` uses the sentinel source value `-`.
- DS answer processing requires at least one DS with owner matching child zone, but once accepted the testcase classifies all DS records in that answer section.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
