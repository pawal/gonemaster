# Zone15

Status: Final

## Purpose
- Check presence and syntax of CAA records (RFC 8659) at the zone apex.
- Detect nameservers that fail the CAA query specifically, since a certificate authority that cannot complete the CAA lookup may refuse to issue.
- Detect cross-nameserver inconsistencies (mixed presence, divergent content).
- Validate record content: reserved flag bits, property tag syntax, known and unknown properties, the issuer-critical flag on properties no certificate authority can honour, `issue`/`issuewild`/`issuemail` value grammar, and `iodef` URL validity.
- Report the resulting issuance policy: a zone that forbids all issuance, and issue records that contradict each other.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - CAA responses from authoritative nameservers at the zone apex.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
3. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for query type `CAA`, then skip.
   - Send exactly one CAA query to the zone apex with default query options. No companion query of any other type is sent.
   - If there is no response, record the endpoint for consolidated `Z15_NO_RESPONSE_CAA_QUERY` and skip.
   - If the RCODE is not NOERROR, record the endpoint and RCODE for consolidated `Z15_UNEXPECTED_RCODE_CAA` and skip.
   - If the response is NOERROR but not authoritative, skip this nameserver silently.
   - Else collect the CAA RRs from the answer section (zero or more records).
4. Post-processing (sequential, over all collected outcomes):
   - Emit one consolidated `Z15_NO_RESPONSE_CAA_QUERY` with the `addresses` of all silent endpoints.
   - Emit one consolidated `Z15_UNEXPECTED_RCODE_CAA` per distinct RCODE with the `addresses` of the endpoints returning it.
   - For each nameserver that returned an authoritative NOERROR response:
     - If the CAA RRset is empty, collect the nameserver for the consolidated absence tag.
     - Else group nameservers for consolidated `Z15_CAA_FOUND` by byte-exact record content (`flags`, property tag, value), and compute an NS-level consistency key from the canonical sorted concatenation of that nameserver's record keys.
   - Emit consolidated `Z15_CAA_FOUND` for each distinct CAA record content group, with the `servers` list.
   - If the absence group is non-empty, emit `Z15_NO_CAA_TLD` when the tested apex is the root zone or a top-level domain, and `Z15_NO_CAA` otherwise, with the `servers` list.
   - If at least one nameserver has CAA and at least one has none, emit `Z15_MIXED_PRESENCE`.
   - If more than one nameserver has CAA and the NS-level consistency keys differ across them, emit `Z15_INCONSISTENT_CAA`.
5. Content validation, once over the deduplicated union of distinct records (a CAA defect is a property of the zone content, not of an individual nameserver). A single record can produce more than one finding; only the tag-classification branches are mutually exclusive:
   - If `flags & 0x7F != 0`, emit `Z15_RESERVED_FLAGS`.
   - If the property tag is empty or contains a byte outside `[A-Za-z0-9]`, emit `Z15_INVALID_PROPERTY_TAG`, and additionally emit `Z15_UNKNOWN_PROPERTY_CRITICAL` when `flags & 0x80` is set. The record's value is not validated further.
   - Else if the lowercased property tag is not in the known set, emit `Z15_UNKNOWN_PROPERTY_CRITICAL` when `flags & 0x80` is set and `Z15_UNKNOWN_PROPERTY` otherwise.
   - Else for `issue`, `issuewild` and `issuemail`, emit `Z15_INVALID_ISSUE_VALUE` when the value fails the issue-value grammar.
   - Else for `iodef`, emit `Z15_INVALID_IODEF_VALUE` when the value is not a valid `mailto:`, `http:` or `https:` URL.
   - The remaining known properties (`contactemail`, `contactphone`, `issuevmc`) are recognized but their values are not validated.
6. Policy evaluation over the distinct `issue`, `issuewild` and `issuemail` records. Skipped entirely when `Z15_INCONSISTENT_CAA` was emitted, and when the tested apex is the root zone. A record forbids issuance when its value has an empty issuer-domain-name, or when its value failed the grammar (RFC 8659 section 4.2 equates the two):
   - If `issue` records are present and every `issue` value forbids, and either no `issuewild` record exists or every `issuewild` value forbids, emit `Z15_ISSUANCE_FORBIDDEN`.
   - For each property tag independently, if both a forbidding and a permitting record are present, emit `Z15_ISSUE_CONTRADICTION` with that property tag.
7. Emit `TEST_CASE_END`.

CAA record identity (for `Z15_CAA_FOUND` consolidation) is determined by the byte-exact tuple `(Flag, Tag, Value)` as received. Case differences across nameservers are real zone-content differences and are preserved. Per-nameserver consistency identity (for `Z15_INCONSISTENT_CAA`) is determined by the canonical sorted concatenation of all record keys on that nameserver. TTLs are not compared.

### Per-NS CAA Probe and Aggregation (steps 2-7)

{{% expand "Show diagram" %}}
```
ns list = ZoneNameservers

For each nameserver (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for CAA -> IPV4_DISABLED / IPV6_DISABLED, skip
   query CAA at z.Name (exactly one query per endpoint)
    +- no response          -> collect endpoint for Z15_NO_RESPONSE_CAA_QUERY
    +- rcode != NOERROR     -> collect endpoint for Z15_UNEXPECTED_RCODE_CAA
    +- NOERROR without AA   -> skip silently
    +- otherwise            -> capture CAA RRs in answer (may be zero)

Aggregate response failures:
   any silent endpoints
      -> Z15_NO_RESPONSE_CAA_QUERY (addresses)
   per distinct rcode
      -> Z15_UNEXPECTED_RCODE_CAA (rcode, addresses)

After all queries, per ns with auth NOERROR CAA response:

   CAA RRset empty
      -> add ns to noCaaGroup
   CAA RRset non-empty:
      per record on this ns:
         group ns into contentGroup by byte-exact (flags, tag, value)
      compute ns-level consistencyKey =
         canonical sorted concat of all record keys on this ns;
         add to consistencyKeySet

Aggregate presence emissions:
   per contentGroup (distinct CAA record content)
      -> Z15_CAA_FOUND (caa_flags, caa_property, caa_value, servers)
   noCaaGroup non-empty
      -> apex is root or TLD ? Z15_NO_CAA_TLD (servers)
                             : Z15_NO_CAA (servers)
   any ns with CAA AND any ns with none
      -> Z15_MIXED_PRESENCE
   more than one ns with CAA AND consistencyKeySet has > 1 entries
      -> Z15_INCONSISTENT_CAA

Content validation over the deduplicated union of records:

   flags & 0x7F != 0
      -> Z15_RESERVED_FLAGS (caa_flags, caa_property, caa_value)
   tag empty or has a byte outside [A-Za-z0-9]
      -> Z15_INVALID_PROPERTY_TAG (caa_property, caa_value)
         and if flags & 0x80
            -> Z15_UNKNOWN_PROPERTY_CRITICAL (caa_property, caa_flags)
         no value validation for this record
   else lowercased tag not in known set
      -> flags & 0x80 ? Z15_UNKNOWN_PROPERTY_CRITICAL (caa_property, caa_flags)
                      : Z15_UNKNOWN_PROPERTY (caa_property)
   else tag in {issue, issuewild, issuemail} AND value fails grammar
      -> Z15_INVALID_ISSUE_VALUE (caa_property, caa_value)
   else tag == iodef AND value is not a mailto/http/https URL
      -> Z15_INVALID_IODEF_VALUE (caa_value)

Policy evaluation (skipped if Z15_INCONSISTENT_CAA, or apex is the root):

   a record forbids when its issuer-domain-name is empty
      or when its value failed the grammar
   issue present AND every issue forbids
      AND (no issuewild OR every issuewild forbids)
      -> Z15_ISSUANCE_FORBIDDEN
   per property tag: forbidding AND permitting records coexist
      -> Z15_ISSUE_CONTRADICTION (caa_property)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `Z15_CAA_FOUND` | CAA record found at zone apex (consolidated per distinct record content). |
| `Z15_INCONSISTENT_CAA` | CAA content differs across authoritative nameservers. |
| `Z15_INVALID_IODEF_VALUE` | An `iodef` value is not a valid `mailto:`, `http:` or `https:` URL. |
| `Z15_INVALID_ISSUE_VALUE` | An `issue`, `issuewild` or `issuemail` value does not match the RFC 8659 issue-value grammar. |
| `Z15_INVALID_PROPERTY_TAG` | A property tag is empty or contains characters outside ASCII letters and digits. |
| `Z15_ISSUANCE_FORBIDDEN` | Every `issue` value forbids issuance, and no `issuewild` value permits it. |
| `Z15_ISSUE_CONTRADICTION` | For one property tag, a forbidding record and a permitting record coexist. |
| `Z15_MIXED_PRESENCE` | CAA present on some nameservers but absent on others. |
| `Z15_NO_CAA` | No CAA record found at zone apex (consolidated across all nameservers without CAA). |
| `Z15_NO_CAA_TLD` | No CAA record found at the apex of the root zone or a top-level domain (consolidated). |
| `Z15_NO_RESPONSE_CAA_QUERY` | No response to the CAA query (consolidated across all silent endpoints). |
| `Z15_RESERVED_FLAGS` | A CAA record sets flag bits other than the issuer-critical flag. |
| `Z15_UNEXPECTED_RCODE_CAA` | The CAA query returned a non-NOERROR RCODE (consolidated per distinct RCODE). |
| `Z15_UNKNOWN_PROPERTY` | A property tag is not in the known set and the issuer-critical flag is not set. |
| `Z15_UNKNOWN_PROPERTY_CRITICAL` | A property tag is unknown or unusable and the issuer-critical flag is set. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | Query type skipped (`CAA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | Query type skipped (`CAA`). |
| `Z15_CAA_FOUND` | `servers` | `array<object>` | Structured sorted list of nameservers `{ns, address}` presenting this CAA record content. |
| `Z15_CAA_FOUND` | `caa_flags` | `uint8` | CAA `Flag` octet as received (`128` = issuer critical). |
| `Z15_CAA_FOUND` | `caa_property` | `string` | CAA property tag as received. |
| `Z15_CAA_FOUND` | `caa_value` | `string` | CAA property value as received. |
| `Z15_INCONSISTENT_CAA` | `-` | `-` | No arguments. |
| `Z15_INVALID_IODEF_VALUE` | `caa_value` | `string` | The `iodef` value that failed URL validation. |
| `Z15_INVALID_ISSUE_VALUE` | `caa_property` | `string` | The property tag carrying the invalid value (`issue`, `issuewild` or `issuemail`). |
| `Z15_INVALID_ISSUE_VALUE` | `caa_value` | `string` | The value that failed the issue-value grammar. |
| `Z15_INVALID_PROPERTY_TAG` | `caa_property` | `string` | The invalid property tag as received. |
| `Z15_INVALID_PROPERTY_TAG` | `caa_value` | `string` | The value of the record carrying the invalid tag. |
| `Z15_ISSUANCE_FORBIDDEN` | `-` | `-` | No arguments. |
| `Z15_ISSUE_CONTRADICTION` | `caa_property` | `string` | The property tag whose records contradict each other. |
| `Z15_MIXED_PRESENCE` | `-` | `-` | No arguments. |
| `Z15_NO_CAA` | `servers` | `array<object>` | Structured list of nameserver `{ns, address}` items without CAA. |
| `Z15_NO_CAA_TLD` | `servers` | `array<object>` | Structured list of nameserver `{ns, address}` items without CAA. |
| `Z15_NO_RESPONSE_CAA_QUERY` | `addresses` | `array<string>` | Nameserver IP addresses that did not answer the CAA query. |
| `Z15_RESERVED_FLAGS` | `caa_flags` | `uint8` | The `Flag` octet carrying reserved bits. |
| `Z15_RESERVED_FLAGS` | `caa_property` | `string` | Property tag of the record setting reserved bits. |
| `Z15_RESERVED_FLAGS` | `caa_value` | `string` | Value of the record setting reserved bits. |
| `Z15_UNEXPECTED_RCODE_CAA` | `rcode` | `string` | The RCODE returned for the CAA query. |
| `Z15_UNEXPECTED_RCODE_CAA` | `addresses` | `array<string>` | Nameserver IP addresses returning this RCODE. |
| `Z15_UNKNOWN_PROPERTY` | `caa_property` | `string` | The unknown property tag. |
| `Z15_UNKNOWN_PROPERTY_CRITICAL` | `caa_property` | `string` | The unknown or unusable property tag marked critical. |
| `Z15_UNKNOWN_PROPERTY_CRITICAL` | `caa_flags` | `uint8` | The `Flag` octet with the issuer-critical bit set. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone15`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone15`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z15_CAA_FOUND` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z15_INCONSISTENT_CAA` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z15_INVALID_IODEF_VALUE` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z15_INVALID_ISSUE_VALUE` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). A malformed value silently forbids issuance. |
| `Z15_INVALID_PROPERTY_TAG` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). RFC 8659 section 4.1 says tags MUST NOT contain other characters. |
| `Z15_ISSUANCE_FORBIDDEN` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). Forbidding all issuance is a legitimate deliberate policy. |
| `Z15_ISSUE_CONTRADICTION` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). The forbidding record is inert, so this is a hygiene finding. |
| `Z15_MIXED_PRESENCE` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z15_NO_CAA` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). Publishing no CAA is valid and common. |
| `Z15_NO_CAA_TLD` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z15_NO_RESPONSE_CAA_QUERY` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z15_RESERVED_FLAGS` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). Consumers must ignore reserved bits, so this is a hygiene finding. |
| `Z15_UNEXPECTED_RCODE_CAA` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z15_UNKNOWN_PROPERTY` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). Certificate authorities ignore unrecognized non-critical properties. |
| `Z15_UNKNOWN_PROPERTY_CRITICAL` | `ERROR` | Default from `share/profile.json` (`test_levels.ZONE`). Conforming certificate authorities refuse all issuance for the domain. |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- No upstream (Zonemaster) equivalent exists; this is a gonemaster-specific test case.
- References: [RFC 8659](https://datatracker.ietf.org/doc/html/rfc8659), [RFC 8657](https://datatracker.ietf.org/doc/html/rfc8657), [RFC 9495](https://datatracker.ietf.org/doc/html/rfc9495)
- Potential upstream report:
  - `no`

## Issue-Value Grammar
The `issue`, `issuewild` and `issuemail` value grammar, transcribed from RFC 8659 section 4.2. RFC 9495 section 3 restates an identical grammar for `issuemail` rather than referencing this one.

```abnf
issue-value = *WSP [issuer-domain-name *WSP]
   [";" *WSP [parameters *WSP]]

issuer-domain-name = label *("." label)
label = (ALPHA / DIGIT) *( *("-") (ALPHA / DIGIT))

parameters = (parameter *WSP ";" *WSP parameters) / parameter
parameter = tag *WSP "=" *WSP value
tag = (ALPHA / DIGIT) *( *("-") (ALPHA / DIGIT))
value = *(%x21-3A / %x3C-7E)
```

Consequences relied on by this testcase:
- Every element is optional, so an empty value and a bare `";"` both parse. Such a value is valid and requests that no certificate authority issue; it is a policy statement, not a syntax error.
- A label starts and ends with a letter or digit and may contain hyphens in the interior, including consecutive hyphens. Empty labels, a trailing dot, `*` and `_` are all outside the grammar.
- A parameter value is visible ASCII (`%x21-3A` and `%x3C-7E`), which excludes both `;` and the space character.
- Whitespace is permitted only around the separators, never inside a name, tag or value.
- Parameter names are not restricted to the RFC 8657 set (`accounturi`, `validationmethods`); RFC 8657 section 6 declines to create a registry because the namespace is defined by each certificate authority.
- Errata: [eid5934](https://www.rfc-editor.org/errata/eid5934) (verified) confirms hyphens are allowed in parameter tags but not in property tags; [eid7139](https://www.rfc-editor.org/errata/eid7139) renames the `tag` and `value` rules to `parameter-tag` and `parameter-value` without altering the grammar.

## Known Properties
Every assignable entry in the IANA "Certification Authority Restriction Properties" registry is recognized: `issue`, `issuewild`, `iodef` (RFC 8659), `issuemail` (RFC 9495), `contactemail`, `contactphone` (CA/Browser Forum), and `issuevmc` (Verified Mark Certificates). Property tag matching is case insensitive per RFC 8659 section 4.1, so `Issue` classifies as `issue` and mixed case is not itself a finding.

The registry's Reserved entries `auth`, `path` and `policy` are deliberately treated as unknown. RFC 8659 reserved them so that they can never be assigned a meaning, so a zone publishing one is publishing a property no certificate authority will act on, which is exactly what `Z15_UNKNOWN_PROPERTY` reports.

## Edge Cases And Limitations
- Publishing no CAA records is valid and common; `Z15_NO_CAA` is informational only and does not indicate a problem.
- Only the zone apex is tested. The RFC 8659 section 3 tree-climbing algorithm used by certificate authorities for names below the apex is out of scope for a delegated-domain test.
- CAA published at the root apex is never consulted: RFC 8659 section 3 climbs the name tree "up to, but not including, the DNS root". Such records are still reported and syntax-checked, but no policy verdict is emitted for the root.
- CAA published at any registry-level zone apex is consulted for every name beneath it, so a forbidding or malformed record there affects the entire subtree. RFC 8659 section 3 tree-climbing runs all the way to the root and does not stop at the registry boundary, so this applies equally to a top-level domain such as `uk` and to a multi-label public suffix such as `co.uk`. Content validation runs at such a zone exactly as for any other zone.
- Only the absence tag distinguishes the root and top-level domains, and it does so on label count rather than on the Public Suffix List. The distinction rests on one rule: publicly trusted certificates cannot be issued for dotless (single-label) names, so the root and a TLD can never be the subject of one, which is what makes `Z15_NO_CAA`'s "any certificate authority may issue" wording false there. That reasoning does not extend to public suffixes generally. `co.uk` is an ordinary fully qualified domain name that may hold a certificate, and `github.io` is a public suffix that does hold certificates, so both correctly receive `Z15_NO_CAA`. A future tree-climbing feature must likewise not use the Public Suffix List to bound the climb.
- Policy verdicts (`Z15_ISSUANCE_FORBIDDEN`, `Z15_ISSUE_CONTRADICTION`) are suppressed when `Z15_INCONSISTENT_CAA` was emitted. Syntax findings over the union of records remain valid because a malformed record is malformed wherever it is served, but a policy conclusion drawn from the union could describe an RRset that no single nameserver publishes.
- `Z15_ISSUANCE_FORBIDDEN` describes `issue` and `issuewild` scope only. Per RFC 9495 section 4, issuance of certificates for email addresses is governed solely by `issuemail`, and the absence of `issuemail` leaves it unrestricted, so a zone can forbid TLS issuance while S/MIME issuance stays open.
- Trailing-dot issuer names such as `issue "ca.example."` are reported as invalid: `issuer-domain-name = label *("." label)` cannot end in a dot. The strictness is deliberate, and per RFC 8659 section 4.2 such a value also forbids issuance.
- Unknown non-critical properties are a `NOTICE` only, because RFC 8659 section 3 states that a CAA RRset containing only unrecognized properties does not restrict issuance.
- A CNAME answer at the apex yields no CAA records for the owner name and counts as absence here; apex CNAME problems are reported by other testcases.
- Nameservers returning NOERROR without the AA bit are skipped silently, matching Zone14. Lameness is reported by the Nameserver module.
- If every nameserver fails or answers non-authoritatively, only the start and end markers plus any response-failure tags are emitted.
- DNSSEC validity of the CAA RRset is not evaluated here; that is the DNSSEC module's territory.
- Records whose RDATA the DNS library refuses to parse never reach validation; the library is the arbiter of wire-format validity.
- The values of `contactemail`, `contactphone` and `issuevmc` are recognized but not validated. Semantic validation of RFC 8657 parameters (the `validationmethods` value set, the `accounturi` shape) is likewise out of scope; only the parameter grammar is checked.
- Distinct nameserver names sharing one IP are grouped per (`ns`, `address`) pair in `servers` outputs, matching the convention of the rest of the Zone module.

## Evidence In Gonemaster
- Code paths:
  - `engine/test/zone/zone.go`
- Related tests:
  - `engine/test/zone/zone_test.go`

## Open Questions
- `Z15_UNKNOWN_PROPERTY_CRITICAL` is the only `ERROR` in this testcase. It is set that way because a critical-flagged property no certificate authority can honour blocks all issuance for the domain, which is a total outage and almost always a typo, whereas a deliberate lockdown is expressed as `issue ";"`. Downgrade to `WARNING` if that proves too sharp for a syntactically valid record.
