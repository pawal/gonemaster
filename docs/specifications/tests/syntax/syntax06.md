# Syntax06 (syntax06)

Status: Final

## Purpose
- Validate SOA `RNAME` as an email-like mailbox and verify that its mail domain/exchange resolution path is usable.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available with a recursor.
- Required inputs:
  - Nameserver objects from `methods.Method4` and `methods.Method5`.
  - SOA responses from those nameservers.
  - Recursive lookups for MX, A, and AAAA records.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6` can skip SOA probes (with debug tags).
  - `resolver.defaults.parallel` controls parallel processing of mail-server address checks.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build deduplicated nameserver list from `Method4` and `Method5`.
3. For each nameserver:
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` and skip.
   - Query SOA (RD=0, TCP off).
   - If no response, emit `NO_RESPONSE` and continue.
   - If no SOA answer, emit `NO_RESPONSE_SOA_QUERY` and continue.
   - Convert SOA `RNAME` to email-like form (`rnameToEmail`).
   - If invalid RFC822 mailbox, emit `RNAME_RFC822_INVALID` and continue.
   - Resolve MX for mailbox domain (with CNAME-follow behavior from packet/question chain).
   - If MX lookup is not `NOERROR`, emit `RNAME_MAIL_DOMAIN_INVALID` and continue.
   - For each deduplicated mail server target (or mailbox domain if no MX):
     - Resolve A and AAAA.
     - Emit `RNAME_MAIL_ILLEGAL_CNAME` if CNAME appears in A/AAAA answers.
     - Emit `RNAME_MAIL_DOMAIN_LOCALHOST` when loopback addresses are present.
     - Emit `RNAME_MAIL_DOMAIN_INVALID` when no usable non-loopback A/AAAA is found.
     - Mark candidate as valid when at least one usable A/AAAA is found.
4. After processing all nameservers, emit `RNAME_RFC822_VALID` for remaining valid candidates only when no invalid exchange verdicts were recorded.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 SOA probing for a nameserver is skipped by profile. |
| `IPV6_DISABLED` | IPv6 SOA probing for a nameserver is skipped by profile. |
| `NO_RESPONSE` | Nameserver SOA query yields no response packet. |
| `NO_RESPONSE_SOA_QUERY` | Response exists but has no SOA answer. |
| `RNAME_MAIL_DOMAIN_INVALID` | Mail domain/exchange resolution is unusable for delivery checks. |
| `RNAME_MAIL_DOMAIN_LOCALHOST` | Mail domain/exchange resolves to loopback (`127.0.0.1` or `::1`). |
| `RNAME_MAIL_ILLEGAL_CNAME` | A/AAAA lookup for mail domain/exchange contains CNAME in answer. |
| `RNAME_RFC822_INVALID` | Converted SOA `RNAME` does not validate as mailbox address. |
| `RNAME_RFC822_VALID` | Converted SOA `RNAME` remains valid after all exchange checks. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`). |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`). |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`name/ip`) with no response. |
| `NO_RESPONSE` | `domain` | `string` | Tested child zone name. |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `RNAME_MAIL_DOMAIN_INVALID` | `domain` | `string` | Invalid mail domain/exchange target. |
| `RNAME_MAIL_DOMAIN_LOCALHOST` | `domain` | `string` | Mail domain/exchange target resolving to loopback. |
| `RNAME_MAIL_DOMAIN_LOCALHOST` | `localhost` | `string` | Loopback address (`127.0.0.1` or `::1`). |
| `RNAME_MAIL_ILLEGAL_CNAME` | `domain` | `string` | Mail domain/exchange target with illegal CNAME in A/AAAA response. |
| `RNAME_RFC822_INVALID` | `rname` | `string` | Converted mailbox candidate from SOA `RNAME`. |
| `RNAME_RFC822_VALID` | `rname` | `string` | Converted mailbox candidate validated as usable. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Syntax06`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Syntax06`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `RNAME_MAIL_DOMAIN_INVALID` | `NOTICE` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `RNAME_MAIL_DOMAIN_LOCALHOST` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `RNAME_MAIL_ILLEGAL_CNAME` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `RNAME_RFC822_INVALID` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `RNAME_RFC822_VALID` | `INFO` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |

## Differences From Upstream
- Upstream reference: [`syntax06.md`](../../upstream/tests/Syntax-TP/syntax06.md)
- Differences (Upstream vs Gonemaster):
  - Upstream outcome table lists `RNAME_MAIL_DOMAIN_INVALID`, `RNAME_MAIL_DOMAIN_LOCALHOST`, and `RNAME_MAIL_ILLEGAL_CNAME` as `WARNING`; Gonemaster profile still maps `RNAME_MAIL_DOMAIN_INVALID` to `NOTICE`.
  - Upstream: does not explicitly define this detail. Gonemaster: Transport skip tags (`IPV4_DISABLED`, `IPV6_DISABLED`) are emitted but not listed in upstream metadata summary.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Warning-level outcomes for invalid/localhost/illegal-cname mail domain checks.
  - Gonemaster observed behavior: `NOTICE` for invalid domain and `WARNING` for localhost/illegal-cname.
  - evidence: `docs/specifications/upstream/tests/Syntax-TP/syntax06.md`, `share/profile.json`, `engine/test/syntax/syntax.go`.
  - report status: `not filed`

## Edge Cases And Limitations
- `RNAME_RFC822_VALID` is emitted only when at least one candidate survives and zero invalid exchange outcomes were recorded.
- Mail-server checks are deduplicated globally across all nameservers (`seenMailServers`), so repeated targets are checked once.
- A/AAAA CNAME detection emits warning-style tags even when another address family later yields a valid exchange.
- When running through `syntax.All`, this testcase is skipped if `syntax01` did not emit `ONLY_ALLOWED_CHARS`.
- When running through `syntax.All`, this testcase is skipped if `syntax05` emitted `NO_RESPONSE_SOA_QUERY`.
