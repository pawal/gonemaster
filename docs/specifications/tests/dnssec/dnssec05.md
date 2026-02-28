# DNSSEC05 (dnssec05)

Status: Final

## Purpose
- Validate DNSKEY algorithm classes used by child-zone nameservers and report deprecated, reserved, private, unassigned, non-zone-signing, unrecommended, and acceptable algorithm usage.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver name/IP items from `methodsv2.GetDelNSNamesAndIPs` and `methodsv2.GetZoneNSNamesAndIPs`.
  - DNSKEY query responses for child apex from the collected nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel DNSKEY-query execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build nameserver set from delegation+zone NS items, grouped by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DNSKEY` and skip.
   - Query child apex `DNSKEY` with DNSSEC enabled.
   - If response is absent, non-`NOERROR`, or non-`AA`, classify nameserver as ignored.
   - If no apex DNSKEY records are present, classify nameserver as `Responds Without DNSKEY`.
   - Else classify nameserver as `Responds With DNSKEY`, and for each DNSKEY:
     - Compute keytag.
     - Classify `Algorithm` value with `dnssec05TagForAlgorithm`.
     - Store `(tag, algo, keytag, ns)` tuple.
4. Emit all `DS05_ALGO_*` tags grouped by `(algo, keytag)` with merged `ns_list`, including `algo_descr` and `algo_mnemo`.
5. If both `Responds Without DNSKEY` and `Responds With DNSKEY` are empty, emit `DS05_NO_RESPONSE` for ignored nameservers.
6. If `Responds Without DNSKEY` is non-empty:
   - Emit `DS05_ZONE_NO_DNSSEC` if `Responds With DNSKEY` is empty.
   - Otherwise emit `DS05_SERVER_NO_DNSSEC`.
7. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS05_ALGO_DEPRECATED` | At least one DNSKEY algorithm maps to deprecated class. |
| `DS05_ALGO_NOT_RECOMMENDED` | At least one DNSKEY algorithm maps to not-recommended class. |
| `DS05_ALGO_NOT_ZONE_SIGN` | At least one DNSKEY algorithm maps to not-for-zone-signing class. |
| `DS05_ALGO_OK` | At least one DNSKEY algorithm maps to acceptable class. |
| `DS05_ALGO_PRIVATE` | At least one DNSKEY algorithm maps to private-use class. |
| `DS05_ALGO_RESERVED` | At least one DNSKEY algorithm maps to reserved class. |
| `DS05_ALGO_UNASSIGNED` | At least one DNSKEY algorithm maps to unassigned class. |
| `DS05_NO_RESPONSE` | No nameserver produced usable DNSKEY/no-DNSKEY outcome and at least one nameserver was ignored due to invalid/no response shape. |
| `DS05_SERVER_NO_DNSSEC` | At least one nameserver returned a usable DNSKEY and at least one nameserver returned no usable DNSKEY. |
| `DS05_ZONE_NO_DNSSEC` | No nameserver returned usable DNSKEY and at least one returned no usable DNSKEY. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS05_ALGO_DEPRECATED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) for this `(algo,keytag)` class entry. |
| `DS05_ALGO_DEPRECATED` | `keytag` | `int` | DNSKEY keytag. |
| `DS05_ALGO_DEPRECATED` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS05_ALGO_DEPRECATED` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS05_ALGO_DEPRECATED` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic. |
| `DS05_ALGO_NOT_RECOMMENDED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) for this `(algo,keytag)` class entry. |
| `DS05_ALGO_NOT_RECOMMENDED` | `keytag` | `int` | DNSKEY keytag. |
| `DS05_ALGO_NOT_RECOMMENDED` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS05_ALGO_NOT_RECOMMENDED` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS05_ALGO_NOT_RECOMMENDED` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic. |
| `DS05_ALGO_NOT_ZONE_SIGN` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) for this `(algo,keytag)` class entry. |
| `DS05_ALGO_NOT_ZONE_SIGN` | `keytag` | `int` | DNSKEY keytag. |
| `DS05_ALGO_NOT_ZONE_SIGN` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS05_ALGO_NOT_ZONE_SIGN` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS05_ALGO_NOT_ZONE_SIGN` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic. |
| `DS05_ALGO_OK` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) for this `(algo,keytag)` class entry. |
| `DS05_ALGO_OK` | `keytag` | `int` | DNSKEY keytag. |
| `DS05_ALGO_OK` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS05_ALGO_OK` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS05_ALGO_OK` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic. |
| `DS05_ALGO_PRIVATE` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) for this `(algo,keytag)` class entry. |
| `DS05_ALGO_PRIVATE` | `keytag` | `int` | DNSKEY keytag. |
| `DS05_ALGO_PRIVATE` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS05_ALGO_PRIVATE` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS05_ALGO_PRIVATE` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic. |
| `DS05_ALGO_RESERVED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) for this `(algo,keytag)` class entry. |
| `DS05_ALGO_RESERVED` | `keytag` | `int` | DNSKEY keytag. |
| `DS05_ALGO_RESERVED` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS05_ALGO_RESERVED` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS05_ALGO_RESERVED` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic. |
| `DS05_ALGO_UNASSIGNED` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) for this `(algo,keytag)` class entry. |
| `DS05_ALGO_UNASSIGNED` | `keytag` | `int` | DNSKEY keytag. |
| `DS05_ALGO_UNASSIGNED` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS05_ALGO_UNASSIGNED` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS05_ALGO_UNASSIGNED` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic. |
| `DS05_NO_RESPONSE` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) ignored due to invalid/no response shape. |
| `DS05_SERVER_NO_DNSSEC` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) returning no usable DNSKEY while other nameservers did. |
| `DS05_ZONE_NO_DNSSEC` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) returning no usable DNSKEY. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC05`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC05`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS05_ALGO_DEPRECATED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_ALGO_NOT_RECOMMENDED` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_ALGO_NOT_ZONE_SIGN` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_ALGO_OK` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_ALGO_PRIVATE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_ALGO_RESERVED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_ALGO_UNASSIGNED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_NO_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_SERVER_NO_DNSSEC` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS05_ZONE_NO_DNSSEC` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec05.md`](../../upstream/tests/DNSSEC-TP/dnssec05.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: summary argument list omits `algo_descr` and `algo_mnemo` for some DS05 tags. Gonemaster: emits both `algo_descr` and `algo_mnemo` for all emitted `DS05_ALGO_*` tags.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameserver evaluation is deduplicated by IP; multiple names mapped to one IP are merged into one query outcome and expanded in `ns_list`.
- Nameservers with non-`NOERROR` or non-`AA` DNSKEY responses are treated as ignored for DS05 classification and can only contribute to `DS05_NO_RESPONSE`.
- If a DNSKEY algorithm maps outside explicit switch cases, Gonemaster classifies it as `DS05_ALGO_UNASSIGNED`.
