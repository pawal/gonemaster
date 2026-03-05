# DNSSEC19 (dnssec19)

Status: Draft

## Purpose
- Check DNSKEY records published by authoritative nameservers for known cryptographic weaknesses and membership in blocklists of compromised keys. This testcase ports DNSSEC-relevant checks from the [badkeys](https://github.com/badkeys/badkeys) project to detect vulnerable RSA keys (Fermat factorization, ROCA, pattern anomalies, invalid parameters, small factors, Wiener's attack) and keys matching the badkeys blocklist of known-compromised keys (e.g., Debian OpenSSL CVE-2008-0166, RFC example keys, firmware keys).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - DNSSEC07 has run and the zone is signed (i.e., `DS07_NOT_SIGNED` was not emitted). If the zone is unsigned, DNSSEC19 is skipped entirely.
- Required inputs:
  - Nameserver name/IP items from `methodsv2.GetDelNSNamesAndIPs` and `methodsv2.GetZoneNSNamesAndIPs`.
  - DNSKEY query responses for child apex from the collected nameservers.
  - Badkeys blocklist data (`blocklist.dat` and `badkeysdata.json`) from the filesystem or embedded fallback. The blocklist is optional; if absent, only RSA crypto checks run.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel DNSKEY-query execution fanout.
  - `badkeys.path`: explicit filesystem path to the directory containing `blocklist.dat` and `badkeysdata.json`.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Attempt to load the badkeys blocklist from the filesystem search path (CLI flag, XDG data dir, system dir) or embedded fallback. If the blocklist is not available, set `blocklist_available = false`.
3. Build nameserver set from delegation+zone NS items, grouped by IP.
4. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DNSKEY` and skip.
   - Query child apex `DNSKEY` with DNSSEC enabled.
   - If response is absent, non-`NOERROR`, or non-`AA`, classify nameserver as ignored.
   - If no apex DNSKEY records are present, classify nameserver as `Responds Without DNSKEY`.
   - Else classify nameserver as `Responds With DNSKEY`, and for each DNSKEY record:
     - Compute keytag.
     - Parse key material based on algorithm type:
       - RSA (algorithms 1, 5, 7, 8, 10): extract modulus N and exponent e per RFC 3110.
       - DSA (algorithms 3, 6): extract Y value per RFC 2536.
       - ECDSA P-256 (algorithm 13): extract X coordinate (first 32 bytes of 64-byte key) per RFC 6605.
       - ECDSA P-384 (algorithm 14): extract X coordinate (first 48 bytes of 96-byte key) per RFC 6605.
       - Ed25519 (algorithm 15): convert 32-byte raw key to integer per RFC 8080.
       - Ed448 (algorithm 16): convert 57-byte raw key to integer per RFC 8080.
     - If key material cannot be parsed, skip the key (do not emit a tag).
     - If RSA key, run the following checks:
       - **fermat**: Fermat factorization (100 rounds). Detects close-prime vulnerabilities.
       - **pattern**: scan modulus bytes for 16 consecutive identical bytes.
       - **roca**: ROCA fingerprint check (CVE-2017-15361, Infineon TPM vulnerability).
       - **rsainvalid**: check e < 3 or e >= N.
       - **smallfactors**: GCD of N with product of all primes <= 65537.
       - **smalld**: Wiener's continued fractions attack for small private exponent d.
     - If `blocklist_available`, run the blocklist check for all key types:
       - Compute BKHASH120: SHA-256 of the key-type-specific numeric value (N for RSA, X for ECDSA, Y for DSA, raw-to-int for Ed25519/Ed448), truncated to 15 bytes.
       - Binary search in `blocklist.dat`.
     - Record any findings as `(check_name, keytag, algo, ns, details)`.
5. If `blocklist_available == false`, emit `DS19_BLOCKLIST_NOT_FOUND`.
6. Group findings by `(keytag, check_name)`:
   - For each group, emit the corresponding `DS19_BADKEY_*` tag with merged `servers`.
7. For each DNSKEY with no findings, emit `DS19_KEY_OK` grouped by `(keytag)` with merged `servers`.
8. If both `Responds Without DNSKEY` and `Responds With DNSKEY` are empty, emit `DS19_NO_RESPONSE` for ignored nameservers.
9. If `Responds Without DNSKEY` is non-empty and `Responds With DNSKEY` is empty, emit `DS19_NO_DNSKEY`.
10. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS19_BADKEY_BLOCKLIST` | At least one DNSKEY matches a known-compromised key in the badkeys blocklist. |
| `DS19_BADKEY_FERMAT` | At least one RSA DNSKEY has a modulus factorable via Fermat factorization (close primes). |
| `DS19_BADKEY_PATTERN` | At least one RSA DNSKEY has a modulus with a suspicious repeating byte pattern (16 identical consecutive bytes). |
| `DS19_BADKEY_ROCA` | At least one RSA DNSKEY is vulnerable to the ROCA attack (CVE-2017-15361). |
| `DS19_BADKEY_RSA_INVALID` | At least one RSA DNSKEY has invalid parameters (exponent e < 3 or e >= modulus N). |
| `DS19_BADKEY_SMALL_FACTORS` | At least one RSA DNSKEY has a modulus with small prime factors (<= 65537). |
| `DS19_BADKEY_SMALL_D` | At least one RSA DNSKEY has a private exponent recoverable via Wiener's continued fractions attack. |
| `DS19_KEY_OK` | At least one DNSKEY passed all enabled badkey checks with no findings. |
| `DS19_BLOCKLIST_NOT_FOUND` | Blocklist data (`blocklist.dat` / `badkeysdata.json`) was not found on the filesystem or via embedded fallback; blocklist check was skipped. RSA crypto checks still ran. |
| `DS19_NO_DNSKEY` | No DNSKEY records found at zone apex despite the zone being signed. |
| `DS19_NO_RESPONSE` | No nameserver produced a usable DNSKEY response and at least one nameserver was ignored due to invalid/no response shape. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS19_BADKEY_BLOCKLIST` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this key. |
| `DS19_BADKEY_BLOCKLIST` | `keytag` | `int` | DNSKEY keytag. |
| `DS19_BADKEY_BLOCKLIST` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS19_BADKEY_BLOCKLIST` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS19_BADKEY_BLOCKLIST` | `blocklist_name` | `string` | Blocklist source name (e.g., `debianssl`). |
| `DS19_BADKEY_FERMAT` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this key. |
| `DS19_BADKEY_FERMAT` | `keytag` | `int` | DNSKEY keytag. |
| `DS19_BADKEY_FERMAT` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS19_BADKEY_FERMAT` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS19_BADKEY_PATTERN` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this key. |
| `DS19_BADKEY_PATTERN` | `keytag` | `int` | DNSKEY keytag. |
| `DS19_BADKEY_PATTERN` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS19_BADKEY_PATTERN` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS19_BADKEY_ROCA` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this key. |
| `DS19_BADKEY_ROCA` | `keytag` | `int` | DNSKEY keytag. |
| `DS19_BADKEY_ROCA` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS19_BADKEY_ROCA` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS19_BADKEY_RSA_INVALID` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this key. |
| `DS19_BADKEY_RSA_INVALID` | `keytag` | `int` | DNSKEY keytag. |
| `DS19_BADKEY_RSA_INVALID` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS19_BADKEY_RSA_INVALID` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS19_BADKEY_RSA_INVALID` | `subtest` | `string` | Specific invalid condition (`invalid_e` or `e_too_large`). |
| `DS19_BADKEY_SMALL_FACTORS` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this key. |
| `DS19_BADKEY_SMALL_FACTORS` | `keytag` | `int` | DNSKEY keytag. |
| `DS19_BADKEY_SMALL_FACTORS` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS19_BADKEY_SMALL_FACTORS` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS19_BADKEY_SMALL_D` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this key. |
| `DS19_BADKEY_SMALL_D` | `keytag` | `int` | DNSKEY keytag. |
| `DS19_BADKEY_SMALL_D` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS19_BADKEY_SMALL_D` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS19_KEY_OK` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this key. |
| `DS19_KEY_OK` | `keytag` | `int` | DNSKEY keytag. |
| `DS19_KEY_OK` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DS19_KEY_OK` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DS19_BLOCKLIST_NOT_FOUND` | | | No arguments (informational notice). |
| `DS19_NO_DNSKEY` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) that responded without DNSKEY records. |
| `DS19_NO_RESPONSE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) ignored due to invalid/no response shape. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC19`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC19`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS19_BADKEY_BLOCKLIST` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_BADKEY_FERMAT` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_BADKEY_PATTERN` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_BADKEY_ROCA` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_BADKEY_RSA_INVALID` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_BADKEY_SMALL_FACTORS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_BADKEY_SMALL_D` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_KEY_OK` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_BLOCKLIST_NOT_FOUND` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_NO_DNSKEY` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS19_NO_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: No upstream Zonemaster equivalent. DNSSEC19 is a new testcase unique to Gonemaster.
- Differences (Upstream vs Gonemaster):
  - Upstream: no testcase for DNSKEY cryptographic weakness or blocklist checking exists. Gonemaster: implements badkeys-based DNSKEY vulnerability detection covering RSA crypto checks and blocklist lookup.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameserver evaluation is deduplicated by IP; multiple names mapped to one IP are merged into one query outcome and expanded in `servers`.
- Nameservers with non-`NOERROR` or non-`AA` DNSKEY responses are treated as ignored and can only contribute to `DS19_NO_RESPONSE`.
- DNSKEY records with unparseable key material (e.g., truncated RSA key, wrong byte length for ECDSA) are silently skipped; no tag is emitted for the unparseable key.
- The blocklist check depends on external data (`blocklist.dat` and `badkeysdata.json`). If these files are absent, only the RSA crypto checks run and `DS19_BLOCKLIST_NOT_FOUND` is emitted once.
- RSA crypto checks only apply to DNSKEY algorithms 1, 5, 7, 8, and 10. For non-RSA key types (ECDSA, Ed25519, Ed448, DSA), only the blocklist check is available.
- The Fermat factorization check is bounded to 100 rounds; it detects close primes but will not find all weak factorisations.
- The ROCA check detects the specific Infineon TPM key generation vulnerability (CVE-2017-15361) and is not a general RSA weakness detector.
- Wiener's attack (`smalld`) only attempts recovery when the public exponent e has a bit length > 32; keys with standard e=65537 and properly generated primes will not trigger it.
- DSA keys (algorithms 3, 6) are largely obsolete and only checked against the blocklist.
- The testcase is skipped entirely if DNSSEC07 reports `DS07_NOT_SIGNED`, avoiding redundant DNSKEY queries for unsigned zones.
- A single DNSKEY may trigger multiple `DS19_BADKEY_*` tags if it fails more than one check (e.g., both `DS19_BADKEY_BLOCKLIST` and `DS19_BADKEY_FERMAT`).

## Evidence In Gonemaster
- Code paths:
  - `engine/test/dnssec/dnssec.go` (DNSSEC19 testcase function)
  - `engine/badkeys/badkeys.go` (public API)
  - `engine/badkeys/parse.go` (DNSKEY wire format parsing)
  - `engine/badkeys/blocklist.go` (BKHASH120 computation and binary search)
  - `engine/badkeys/rsa_checks.go` (fermat, pattern, roca, rsainvalid, smallfactors, smalld)
- Related tests:
  - `engine/badkeys/badkeys_test.go`
  - `engine/test/dnssec/dnssec_test.go`
