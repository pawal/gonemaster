# Implemented Gonemaster Testcases

This document is the authoritative inventory of currently implemented gonemaster testcases.

Source of truth used for this inventory:
- `engine/plan.go` (`moduleTestcases`, `moduleOrder`)
- `engine/engine.go` (`basicTests`, `syntaxTests`, `addressTests`, `connectivityTests`, `consistencyTests`, `delegationTests`, `dnssecTests`, `nameserverTests`, `zoneTests`)

Notes:
- DNSSEC testcase ordering is numeric (`dnssec01` ... `dnssec18`) as returned by `dnssecTestcaseNames()`.
- `dnssec12` is currently not implemented and therefore not present in this inventory.

## Summary
- Modules: 9
- Implemented testcases: 83

## Regeneration

```sh
make spec-export-implemented
```

## Module Inventory

### basic (3)
- basic01 - Determine whether the child zone exists and whether a parent zone can be identified from iterative authoritative responses.
- basic02 - Verify that the child zone has at least one working authoritative nameserver for SOA.
- basic03 - Detect the "broken but functional" case by probing `A` records for `www.<child-zone>` against delegation nameservers.

### address (3)
- address01 - Classify authoritative nameserver IP addresses as globally reachable, documentation, local-use, or otherwise not globally reachable.
- address02 - Verify that every unique nameserver IP address has a usable reverse DNS PTR mapping.
- address03 - Verify that reverse PTR hostnames for nameserver IPs match the corresponding nameserver hostname.

### connectivity (4)
- connectivity01 - Verify that nameservers are reachable over UDP for SOA and NS queries at the child zone name.
- connectivity02 - Verify that nameservers are reachable over TCP for SOA and NS queries at the child zone name.
- connectivity03 - Evaluate ASN diversity of authoritative nameserver IP addresses.
- connectivity04 - Evaluate prefix diversity of nameserver IP addresses by IP family.

### consistency (6)
- consistency01 - Check SOA serial consistency across nameservers for the tested zone.
- consistency02 - Check SOA RNAME consistency across nameservers for the tested zone.
- consistency03 - Check consistency of SOA timer fields (`refresh`, `retry`, `expire`, `minimum`) across nameservers.
- consistency04 - Check NS RRset consistency across nameservers for the tested zone.
- consistency05 - Compare the delegation NS set (NS names plus attached glue) served by each responding parent nameserver.
- consistency06 - Check SOA MNAME consistency across nameservers for the tested zone.

### delegation (7)
- delegation01 - Validate that delegation-side and child-side nameserver sets meet minimum-count requirements overall and per IP family.
- delegation02 - Detect nameserver IP-address reuse within delegation data, within child data, and across the combined delegation+child addressed NS set.
- delegation03 - Check how large a synthesized maximal referral response is relative to the 512-byte non-EDNS UDP payload limit and the 1232-byte EDNS payload size, and grade it accordingly.
- delegation04 - Verify whether nameservers from delegation and child sources answer authoritatively for SOA queries.
- delegation05 - Verify that NS names used for the tested zone are not aliases (CNAME targets).
- delegation06 - Verify SOA RRset existence on nameservers collected from delegation and child sources.
- delegation07 - Compare parent-side and child-side NS name sets and report mismatches.

### dnssec (20)
- dnssec01 - Validate DS digest algorithm usage for the child delegation and classify each observed DS digest type.
- dnssec02 - Verify that DS records found at the parent delegation match usable DNSKEYs in the child zone and that matching DNSKEYs can validate DNSKEY RRset signatures.
- dnssec03 - Verify NSEC3 parameter consistency and policy compliance across child nameservers when DNSKEY support is present.
- dnssec04 - Evaluate DNSKEY/SOA RRSIG validity windows and flag signatures that are expired, too short-lived, too long-lived, or otherwise within configured limits.
- dnssec05 - Validate DNSKEY algorithm classes used by child-zone nameservers and report deprecated, reserved, private, unassigned, non-zone-signing, unrecommended, and acceptable algorithm usage.
- dnssec06 - Verify DNSSEC additional-processing behavior for DNSKEY responses by checking whether DNSKEY answers include accompanying RRSIG data.
- dnssec07 - Determine whether the child zone is signed (based on DNSKEY + covering RRSIG observations) and, for signed zones, whether parent-side DS data is present and consistent.
- dnssec08 - Verify that DNSKEY RRset signatures are present, time-valid, algorithm-supported, and cryptographically match DNSKEY records at child nameservers.
- dnssec09 - Verify that SOA responses are signed and that SOA RRSIG records are time-valid, algorithm-supported, and cryptographically matched by DNSKEY records from the same nameserver.
- dnssec10 - Verify that signed child-zone nameservers consistently provide NSEC or NSEC3 denial-of-existence material (including signatures and owner/type-shape checks) when querying for apex `NSEC` and `NSEC3PARAM`.
- dnssec11 - Verify that parent-side DS presence is consistent with child-side DNSKEY presence (zone signing expectation), including parent consistency and child consistency reporting.
- dnssec13 - Verify that each DNSKEY algorithm observed in the DNSKEY RRset also appears in RRSIG records for DNSKEY, SOA, and NS answer flows at child nameservers.
- dnssec14 - Validate RSA DNSKEY key sizes against per-algorithm minimum/maximum ranges and recommended size thresholds.
- dnssec15 - Verify presence and consistency of CDS and CDNSKEY RRsets and detect mismatches between the two at child nameservers.
- dnssec16 - Validate CDS RRsets against DNSKEY data and CDS signatures, including delete semantics and signature/keytag consistency checks.
- dnssec17 - Validate CDNSKEY RRsets against DNSKEY data and CDNSKEY signatures, including delete semantics and signature/keytag consistency checks.
- dnssec18 - Validate that CDS and CDNSKEY RRsets are signed by a DNSKEY that corresponds to DS information observed at the parent side.
- dnssec19 - Check DNSKEY records for known cryptographic weaknesses and membership in blocklists of compromised keys.
- dnssec20 - Verify that the NSEC/NSEC3 apex type bitmap accurately reflects the RR types actually present in the zone.
- dnssec21 - Verify that the parent zone correctly signs the DS RRset that delegates the child zone.

### nameserver (17)
- nameserver01 - Detect whether authoritative nameservers also behave as recursors.
- nameserver02 - Validate EDNS(0) handling on authoritative nameservers.
- nameserver03 - Check whether nameservers allow AXFR zone transfer.
- nameserver04 - Verify that nameserver responses come from the same IP address that was queried.
- nameserver05 - Evaluate nameserver behavior for AAAA queries after successful A-query baseline checks.
- nameserver06 - Verify that NS names discovered from delegation/child data can be resolved to at least one IP address.
- nameserver07 - Detect upward referrals (root NS records in authority section) returned by authoritative nameservers.
- nameserver08 - Check whether nameservers preserve or alter query-name case in the echoed question section.
- nameserver09 - Compare nameserver results for two differently cased query names that are equivalent under DNS case-insensitive matching.
- nameserver10 - Validate authoritative nameserver behavior for unsupported EDNS version queries (version 1).
- nameserver11 - Verify handling of an unknown EDNS option code in authoritative SOA responses.
- nameserver12 - Validate behavior when querying with unknown EDNS Z flags set.
- nameserver13 - Check truncated EDNS responses for missing OPT records.
- nameserver15 - Detect whether authoritative nameservers reveal software version data through CHAOS-class TXT queries.
- nameserver16 - Query authoritative nameservers with an EDNS NSID option request (RFC 5001, option code 3) and report which servers provide NSID values and what those values contain.
- nameserver17 - Probe each authoritative nameserver for DNS Cookie (RFC 7873 / RFC 9018) support and verify the returned Server Cookie is well-formed and accepted by the issuing server.
- nameserver18 - Query each authoritative nameserver for the zone and report any EDNS Extended DNS Error (RFC 8914, option code 15) option found in the response, classified by what the observed info-code implies about a directly-queried authoritative server.

### syntax (8)
- syntax01 - Validate that the tested domain name contains only allowed DNS hostname characters.
- syntax02 - Validate that no domain label starts or ends with a hyphen (`-`).
- syntax03 - Validate that domain labels do not contain a double hyphen in positions 3 and 4, except ACE labels (`xn--...`).
- syntax04 - Validate syntax of nameserver hostnames gathered from parent delegation and child apex NS sets.
- syntax05 - Detect misuse of `@` in SOA `RNAME` and ensure mailbox form is represented with dot-separated DNS notation.
- syntax06 - Validate SOA `RNAME` as an email-like mailbox and verify that its mail domain/exchange resolution path is usable.
- syntax07 - Validate SOA `MNAME` hostname syntax using the same hostname validator as `syntax04`.
- syntax08 - Validate syntax of MX exchange hostnames for the tested zone.

### zone (15)
- zone01 - Validate SOA MNAME handling for the child zone: name sanity, resolvability, authority behavior, and serial-based master inference.
- zone02 - Validate that SOA `refresh` is at or above the configured minimum threshold.
- zone03 - Validate ordering relationship between SOA timers: `refresh` should be greater than `retry`.
- zone04 - Validate that SOA `retry` is at or above the configured minimum threshold.
- zone05 - Validate SOA `expire` constraints
- zone06 - Validate that SOA default TTL (`minimum` field) is within configured lower and upper bounds.
- zone07 - Validate SOA MNAME alias/address behavior
- zone08 - Validate that MX exchange hostnames are not aliases (CNAME).
- zone09 - Validate MX presence and consistency across authoritative nameservers, including null-MX and domain-class exceptions (root/TLD/.arpa).
- zone10 - Validate SOA answer-shape correctness on nameservers: response presence, SOA presence, owner name correctness, and multiplicity. When a single correct SOA is present, also check for CNAME or DNAME at the zone apex.
- zone11 - Validate SPF policy publication at zone apex
- zone12 - Check existence and RFC 7477 compliance of the CSYNC RR at the zone apex.
- zone13 - Validate that the SPF policy at the zone apex does not exceed the DNS lookup limit defined in RFC 7208 Section 4.6.4.
- zone14 - Check existence and RFC 8976 compliance of the ZONEMD RR at the zone apex.
- zone15 - Check presence and syntax of CAA records (RFC 8659) at the zone apex.

