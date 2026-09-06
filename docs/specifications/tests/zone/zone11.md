# Zone11

Status: Final

## Purpose
- Validate SPF policy publication at zone apex:
  - ability to retrieve authoritative TXT data;
  - consistency of SPF policy sets across nameserver IPs;
  - single-policy expectation per nameserver IP;
  - SPF syntax and non-mail-domain policy handling.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - Nameserver resolution context is available for nsdiscovery calls.
- Required inputs:
  - Nameserver name/IP items from [`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers) and [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - Apex TXT responses for SPF extraction.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: parallel nameserver query fanout.
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped.

## Algorithm And Decision Flow
1. Build nameserver set from nsdiscovery delegation+zone items, then group by distinct IP.
2. For each IP group (parallelized):
   - Skip disabled transports.
   - Query apex `TXT`.
   - Accept response only when response exists, `RCODE=NOERROR`, and `AA=true`.
   - Extract TXT records for apex, concatenate fragments per record, lowercase text, and keep only SPF records (`v=spf1` with end/space/tab boundary).
   - Store per-IP SPF policy list plus associated nameserver `name/ip` list.
3. If no IP produced an accepted authoritative response, emit `Z11_UNABLE_TO_CHECK_FOR_SPF`.
4. Else group per-IP policy sets by a normalized key:
   - If all policy keys are empty:
     - emit `Z11_NO_SPF_NON_MAIL_DOMAIN` for root/TLD/`.arpa` zones;
     - otherwise emit `Z11_NO_SPF_FOUND` (`domain`).
   - Else if more than one distinct policy-set key exists:
     - emit `Z11_INCONSISTENT_SPF_POLICIES`;
     - emit `Z11_DIFFERENT_SPF_POLICIES_FOUND` per policy-set group.
   - Else if any single IP has more than one SPF policy, emit `Z11_SPF_MULTIPLE_RECORDS`.
   - Else evaluate the single effective SPF policy text against the grammar in [SPF Syntax Check](#spf-syntax-check):
     - if syntax invalid, emit `Z11_SPF_SYNTAX_ERROR`;
     - if syntax valid and zone is root/TLD/`.arpa`:
       - emit `Z11_NULL_SPF_NON_MAIL_DOMAIN` for null SPF (`v=spf1 -all`);
       - else emit `Z11_NON_NULL_SPF_NON_MAIL_DOMAIN`;
     - if syntax valid and zone is regular mail domain, emit `Z11_SPF_SYNTAX_OK`;
     - if syntax valid, emit `Z11_SPF_UNKNOWN_MODIFIER` after the verdict above, once per distinct unknown modifier name in record order, for every zone class.

### Per-IP SPF Collection and Policy Classification (steps 1-4)

{{% expand "Show diagram" %}}
```
all NS = nsdiscovery delegation + zone items; group by IP

For each unique IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for TXT -> IPV4_DISABLED / IPV6_DISABLED, skip
   query TXT at z.Name
    +- not (resp.Msg present AND RCODE == NOERROR AND AA) -> skip (no policies)
    +- accepted:
         extract apex TXT records, concat fragments per RR, lowercase
         keep only entries starting with "v=spf1" followed by end / space / tab
         outcome.checked  = true
         outcome.policies = spf entries (may be empty)

After all tasks, build:
   nsSpf[ip]        = policies     (only for checked IPs)
   ipToNS[ip]       = NS name/ip list at that IP
   spfNS[policyKey] = combined NS list across IPs sharing the same sorted
                       policy multiset (length-prefixed canonical key)

Classification:
   len(nsSpf) == 0
      -> Z11_UNABLE_TO_CHECK_FOR_SPF

   every IP has empty policy list (all spfNS keys empty):
      z.Name == "." OR nextHigherIsRoot OR z.Name ends ".arpa"
         -> Z11_NO_SPF_NON_MAIL_DOMAIN (domain)
      otherwise
         -> Z11_NO_SPF_FOUND          (domain)

   len(spfNS) > 1 (distinct policy sets across IPs):
      -> Z11_INCONSISTENT_SPF_POLICIES (no args)
         per spfNS group:
            Z11_DIFFERENT_SPF_POLICIES_FOUND (servers)

   any IP has more than one SPF policy:
      -> Z11_SPF_MULTIPLE_RECORDS (servers = NS at offending IPs)

   otherwise (single consistent policy, single record per IP):
      spfText = the one policy value
      check   = spfCheckSyntax(spfText)
      check.ok:
         z.Name is root / TLD / .arpa:
            nullSpfRegex matches  -> Z11_NULL_SPF_NON_MAIL_DOMAIN     (domain)
            otherwise             -> Z11_NON_NULL_SPF_NON_MAIL_DOMAIN (domain)
         otherwise                -> Z11_SPF_SYNTAX_OK                (domain)
         per name in check.modifiers (distinct, record order):
                                  -> Z11_SPF_UNKNOWN_MODIFIER         (domain, spf_modifier)
      !check.ok                   -> Z11_SPF_SYNTAX_ERROR  (servers = all NS with policies, domain)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `Z11_DIFFERENT_SPF_POLICIES_FOUND` | A policy-set group is emitted during SPF inconsistency reporting. |
| `Z11_INCONSISTENT_SPF_POLICIES` | At least two distinct SPF policy-set groups exist across checked IPs. |
| `Z11_NO_SPF_FOUND` | No SPF policy found for a domain expected to carry mail policy. |
| `Z11_NO_SPF_NON_MAIL_DOMAIN` | No SPF policy found for root/TLD/`.arpa` domain class. |
| `Z11_NON_NULL_SPF_NON_MAIL_DOMAIN` | Non-null SPF policy found for root/TLD/`.arpa` domain class. |
| `Z11_NULL_SPF_NON_MAIL_DOMAIN` | Null SPF policy found for root/TLD/`.arpa` domain class. |
| `Z11_SPF_MULTIPLE_RECORDS` | At least one checked IP returned more than one SPF policy. |
| `Z11_SPF_SYNTAX_ERROR` | Effective SPF policy failed local syntax validation. |
| `Z11_SPF_SYNTAX_OK` | Effective SPF policy passed local syntax validation. |
| `Z11_SPF_UNKNOWN_MODIFIER` | Effective SPF policy passed local syntax validation and carries a modifier other than `redirect` and `exp`; one entry per distinct modifier name. |
| `Z11_UNABLE_TO_CHECK_FOR_SPF` | No nameserver IP yielded an authoritative TXT response suitable for SPF evaluation. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `Z11_DIFFERENT_SPF_POLICIES_FOUND` | `servers` | `array<object>` | Structured nameserver `{ns,address}` object list for one policy-set group. |
| `Z11_INCONSISTENT_SPF_POLICIES` | `-` | `-` | No arguments. |
| `Z11_NO_SPF_FOUND` | `domain` | `string` | Tested zone name. |
| `Z11_NO_SPF_NON_MAIL_DOMAIN` | `domain` | `string` | Tested zone name. |
| `Z11_NON_NULL_SPF_NON_MAIL_DOMAIN` | `domain` | `string` | Tested zone name. |
| `Z11_NULL_SPF_NON_MAIL_DOMAIN` | `domain` | `string` | Tested zone name. |
| `Z11_SPF_MULTIPLE_RECORDS` | `servers` | `array<object>` | Structured nameserver `{ns,address}` object list with multi-policy responses. |
| `Z11_SPF_SYNTAX_ERROR` | `servers` | `array<object>` | Structured nameserver `{ns,address}` object list used for evaluated policy. |
| `Z11_SPF_SYNTAX_ERROR` | `domain` | `string` | Tested zone name. |
| `Z11_SPF_SYNTAX_OK` | `domain` | `string` | Tested zone name. |
| `Z11_SPF_UNKNOWN_MODIFIER` | `domain` | `string` | Tested zone name. |
| `Z11_SPF_UNKNOWN_MODIFIER` | `spf_modifier` | `string` | Lowercased modifier name, without the `=` and the value. |
| `Z11_UNABLE_TO_CHECK_FOR_SPF` | `-` | `-` | No arguments. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `Z11_DIFFERENT_SPF_POLICIES_FOUND` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_INCONSISTENT_SPF_POLICIES` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_NO_SPF_FOUND` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_NO_SPF_NON_MAIL_DOMAIN` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_NON_NULL_SPF_NON_MAIL_DOMAIN` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_NULL_SPF_NON_MAIL_DOMAIN` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_SPF_MULTIPLE_RECORDS` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_SPF_SYNTAX_ERROR` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_SPF_SYNTAX_OK` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z11_SPF_UNKNOWN_MODIFIER` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`); carries zero score penalty (`scoring` `TagPenalties`). The record is valid: RFC 7208 section 6 requires receivers to ignore modifiers they do not recognize. Surfaced because not every receiver does. |
| `Z11_UNABLE_TO_CHECK_FOR_SPF` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: defines SPF syntax against RFC 7208 ABNF semantics. Gonemaster: uses a local check (`spfCheckSyntax`/`spfTermOk`) that follows the RFC 7208 term grammar for mechanisms, modifiers and CIDR lengths, but validates domain targets with a permissive name check; see [SPF Syntax Check](#spf-syntax-check). Gonemaster additionally reports modifiers outside RFC 7208 with `Z11_SPF_UNKNOWN_MODIFIER`.
  - Upstream: defines no testcase boundary markers. Gonemaster: the runtime emits the shared `TEST_CASE_START` / `TEST_CASE_END`, which are not part of the Zone11 metadata inventory.
- Potential upstream report:
  - `no`

## SPF Syntax Check
The effective policy is checked against the record grammar of RFC 7208 section 12, transcribed below in the parts this check relies on. The text is lowercased before the check, so matching is case insensitive. A record is valid when it consists of `v=spf1` followed by zero or more terms separated by space or tab, and every term is a directive or a modifier.

```abnf
record           = version terms *SP
version          = "v=spf1"
terms            = *( 1*SP ( directive / modifier ) )
directive        = [ qualifier ] mechanism
qualifier        = "+" / "-" / "?" / "~"
mechanism        = ( all / include / a / mx / ptr / ip4 / ip6 / exists )
modifier         = redirect / explanation / unknown-modifier
unknown-modifier = name "=" macro-string
name             = ALPHA *( ALPHA / DIGIT / "-" / "_" / "." )
dual-cidr-length = [ ip4-cidr-length ] [ "/" ip6-cidr-length ]
ip4-cidr-length  = "/" ("0" / %x31-39 0*1DIGIT) ; 0 to 32
ip6-cidr-length  = "/" ("0" / %x31-39 0*2DIGIT) ; 0 to 128
macro-string     = *( macro-expand / macro-literal )
macro-expand     = ( "%{" macro-letter transformers *delimiter "}" )
                   / "%%" / "%_" / "%-"
macro-literal    = %x21-24 / %x26-7E ; visible characters except "%"
macro-letter     = "s" / "l" / "o" / "d" / "i" / "p" / "v" / "h" / "c" / "r" / "t"
transformers     = *DIGIT [ "r" ]
delimiter        = "." / "-" / "+" / "," / "/" / "_" / "="
```

Consequences relied on by this testcase:
- `a` and `mx` accept `dual-cidr-length`, so `a:example.com/24`, `a:example.com//64` and `a:example.com/24//64` are all valid. The IPv4 length is bounded to 32 and the IPv6 length to 128. A length with a leading zero, such as `/08`, is outside the grammar and is rejected. The same bounds and the same leading zero rule apply to the `ip4` and `ip6` lengths.
- A term containing `=` before any `:` is matched as a modifier, before any qualifier is considered, because a qualifier is only permitted on a directive. `-redirect=example.com` is therefore a syntax error, and so is a verification token appended to a directive, such as `-allfoo=bar`. `exists:foo=bar.example.com` is a mechanism, since its `:` precedes the `=`.
- `redirect` and `exp` take a domain value. Every other `name` is an unknown modifier whose value must match `macro-string`. RFC 7208 section 6 states that "Unrecognized modifiers MUST be ignored no matter where, or how often, they appear in a record", so the record stays valid and each distinct name is reported once with `Z11_SPF_UNKNOWN_MODIFIER`. The RFC 6652 modifiers `ra`, `rp` and `rr` fall in this class.
- The name `v` is not accepted as a modifier name. A second `v=spf1` among the terms results from two policies merged into one record, and such a record is reported with `Z11_SPF_SYNTAX_ERROR` rather than as an unknown modifier.
- `macro-literal` excludes `%`, so a `%` that does not begin a `macro-expand`, such as `%z` or `%{q}`, makes the record invalid.

## Edge Cases And Limitations
- Distinct nameserver names sharing one IP are grouped and represented together in `servers` outputs.
- TXT responses with authoritative `NOERROR` but without SPF TXT records are treated as empty-policy results.
- `Z11_SPF_UNKNOWN_MODIFIER` is emitted once per distinct modifier name, so a name repeated in the record yields one entry.
- Unknown modifier values are validated only as `macro-string`. The RFC 6652 values are not checked against the RFC 6652 grammar: a receiver never rejects a record on the content of a modifier it ignores, and the published `rp` grammar disagrees with its own prose (errata 6579, held for document update).
- Domain targets of `include`, `exists`, `redirect`, `exp`, `a`, `mx` and `ptr` are validated with a permissive name check. Macro expansions in a domain target are accepted without being checked against `macro-string`.
- `ptr` accepts a `dual-cidr-length` suffix although RFC 7208 defines none for it.
- Duplicate `redirect` or `exp` modifiers are not detected. RFC 7208 section 6 treats them as a permanent error.
- Runtime boundary markers (`TEST_CASE_START`/`TEST_CASE_END`) are emitted by shared testcase wrappers but omitted from current Zone11 metadata tag contract.
