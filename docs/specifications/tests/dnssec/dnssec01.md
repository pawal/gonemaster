# DNSSEC01

Status: Final

## Purpose
- Validate DS digest algorithm usage for the child delegation and classify each observed DS digest type.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent nameserver names/IPs from [`ParentNameservers`](../../nameserver-resolution.md#parentnameservers).
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
   - If parent fake DS records are present, classify those DS records immediately with source `servers` value `-`.
   - Mark parent-NS query list empty so external parent DS queries are skipped.
5. For each unique parent nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DS` and skip.
   - Send DS query with DNSSEC enabled.
   - If response is absent or fails required shape checks (`NOERROR`, `OPT`, `DO`, `AA`), mark nameserver as ignored.
   - If response has no DS record with owner matching child zone name, mark nameserver in `Responds Without Valid DS`.
   - Else mark nameserver in `Responds With DS` and classify each DS digest algorithm into the corresponding DS01 tag set.
6. Emit DS classification tags (`DS01_DS_ALGO_*`) grouped by `(digest, keytag)` with merged `servers`.
7. Emit `DS01_DS_ALGO_2_MISSING` for keytags that have non-2 DS but no digest-2 DS on the same source nameservers.
8. If no valid/non-valid DS responders exist and ignored responders exist, emit `DS01_NO_RESPONSE`.
9. Emit informational tags based on zone type and undelegated DS status:
   - If zone is root (`.`), emit `DS01_ROOT_N_NO_UNDEL_DS` when no undelegated DS records were provided.
   - If zone is undelegated and non-root, emit `DS01_UNDEL_N_NO_UNDEL_DS` when no undelegated DS records were provided.
10. If `Responds Without Valid DS` is non-empty:
    - Emit `DS01_PARENT_ZONE_NO_DS` when no nameserver responded with DS.
    - Otherwise emit `DS01_PARENT_SERVER_NO_DS`.
11. Emit `TEST_CASE_END`.

### Per-Parent-NS DS Query (steps 4-5)

{{% expand "Show diagram" %}}
```
parentNS = ParentNameservers

undelegated DS path (any parent NS has FakeDSRecords for z):
   for each DS record in fake set:
     classify by digest -> sets[<tag>][digest][keytag] += "-"
     digest == 2 -> algo2DS[keytag] += "-"
     else        -> nonAlgo2DS[keytag] += "-"
   respondsWithDS += "-"
   skip live parent queries (parentNS = nil)

Otherwise, per unique parent NS IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for DS -> IPV4_DISABLED / IPV6_DISABLED, skip
   query DS at z.Name, DNSSEC=on
    +- resp.Msg == nil / RCODE != NOERROR / no EDNS / !DO / !AA
                                         -> ignoredParentNS += matching ns/ip
    +- no DS with owner == z.Name        -> respondsWithoutValidDS += matching ns/ip
    +- DS present
                                         -> respondsWithDS += matching ns/ip
        for each DS:
          tag = dnssec01TagForDigest(digest)
            ({DEPRECATED, RESERVED, UNASSIGNED, PRIVATE, NOT_DS, OK})
          sets[tag][digest][keytag] += ns/ip
          digest == 2 -> algo2DS[keytag]    += ns/ip
          else        -> nonAlgo2DS[keytag] += ns/ip
```
{{% /expand %}}

### Aggregation and Summary Emission (steps 6-10)

{{% expand "Show diagram" %}}
```
emit per-digest classification tags:
   for each tag in sorted DS01_DS_ALGO_* keys:
     for each digest in sorted digest keys:
       for each keytag in sorted keytag keys:
         -> <tag> (keytag, ds_algo_num, ds_algo_descr, servers)

algo-2 coverage:
   for each keytag in nonAlgo2DS:
     missing = nonAlgo2DS[keytag] minus algo2DS[keytag]
     missing non-empty -> DS01_DS_ALGO_2_MISSING (keytag, servers=missing)

no usable responders:
   respondsWithoutValidDS empty AND respondsWithDS empty AND ignoredParentNS non-empty
     -> DS01_NO_RESPONSE (servers = ignoredParentNS)

zone-type info tags:
   z.Name == "." AND no undelegated DS              -> DS01_ROOT_N_NO_UNDEL_DS
   z.Name != "." AND hasFakeAddresses AND no undel DS -> DS01_UNDEL_N_NO_UNDEL_DS

without-valid-DS responders:
   respondsWithoutValidDS non-empty:
     respondsWithDS empty -> DS01_PARENT_ZONE_NO_DS  (servers)
     otherwise            -> DS01_PARENT_SERVER_NO_DS (servers)

emit TEST_CASE_END
```
{{% /expand %}}

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
| `DS01_DS_ALGO_2_MISSING` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) where digest-2 DS is missing, or `-` for undelegated DS source. |
| `DS01_DS_ALGO_2_MISSING` | `keytag` | `int` | DNSKEY key tag referenced by DS records. |
| `DS01_DS_ALGO_DEPRECATED` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_DEPRECATED` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_DEPRECATED` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_DEPRECATED` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_NOT_DS` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_NOT_DS` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_NOT_DS` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_NOT_DS` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_OK` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_OK` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_OK` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_OK` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_PRIVATE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_PRIVATE` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_PRIVATE` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_PRIVATE` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_RESERVED` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_RESERVED` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_RESERVED` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_RESERVED` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_DS_ALGO_UNASSIGNED` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object), or `-` for undelegated DS source. |
| `DS01_DS_ALGO_UNASSIGNED` | `keytag` | `int` | DNSKEY key tag referenced by DS record. |
| `DS01_DS_ALGO_UNASSIGNED` | `ds_algo_num` | `int` | DS digest algorithm number from DS RDATA. |
| `DS01_DS_ALGO_UNASSIGNED` | `ds_algo_descr` | `string` | Text description of DS digest algorithm number. |
| `DS01_NO_RESPONSE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) ignored due to invalid/no DS response shape. |
| `DS01_PARENT_SERVER_NO_DS` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) that returned no valid DS owner. |
| `DS01_PARENT_ZONE_NO_DS` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) that returned no valid DS owner. |
| `DS01_ROOT_N_NO_UNDEL_DS` | `-` | `-` | No arguments. |
| `DS01_UNDEL_N_NO_UNDEL_DS` | `-` | `-` | No arguments. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
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
- For undelegated DS input, `servers` uses the sentinel source value `-`.
- DS answer processing requires at least one DS with owner matching child zone, but once accepted the testcase classifies all DS records in that answer section.
