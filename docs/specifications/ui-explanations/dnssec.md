# Public UI explanations: DNSSEC

## Testcase dnssec01

Description:

When a zone is signed with DNSSEC, the parent zone publishes a "DS" record that commits to the key used to sign the child. The DS uses a digest algorithm to summarise the child's key. This check looks at each DS record at the parent and classifies its digest algorithm as acceptable, deprecated, reserved, or otherwise unusable.

## Testcase dnssec02

Description:

Each DS record at the parent should match a DNSKEY in the child, and the matching DNSKEY should be able to validate the child's signatures. This check cross-references every DS against the child's DNSKEYs and flags any DS that has no counterpart, any DNSKEY not backed by DS, and any signature path that fails cryptographic validation.

## Testcase dnssec03

Description:

Zones that use NSEC3 to handle authenticated denial-of-existence publish parameters (hash algorithm, iteration count, salt, flags) describing exactly how the hashing is done. These parameters must be consistent across every nameserver and must use allowed values. This check looks up NSEC3PARAM and flags any mismatch or out-of-range value.

## Testcase dnssec04

Description:

DNSSEC signatures (RRSIGs) have a validity window: they start being valid at some time and expire at another. Signatures that live too long are risky if the key is ever compromised; too short creates operational strain. This check looks at the validity windows of your signatures and flags anything that has already expired, is not yet valid, or is well outside the configured bounds.

## Testcase dnssec05

Description:

DNSKEY records use a specific signing algorithm (such as RSA/SHA-256 or ECDSA-P256). Over time, the IETF deprecates older algorithms and recommends newer ones. This check classifies every DNSKEY's algorithm as acceptable, not recommended, deprecated, or unsuitable for zone signing, so operators can see which keys may need to be rotated.

## Testcase dnssec06

Description:

When a resolver asks for your DNSKEY records, the response should include the DNSKEYs themselves and the RRSIG that signs them. This check confirms the signature accompanies the answer - without the RRSIG the DNSKEY set cannot be validated, and the zone will fail DNSSEC verification.

## Testcase dnssec07

Description:

A zone is considered "signed" when it has a DNSKEY and at least one RRSIG covering it. The parent is expected to publish DS records matching those keys. This check looks at both sides and flags the inconsistencies - for example a child zone that looks signed but has no DS at the parent, or a parent with DS for a child that does not actually publish DNSKEYs.

## Testcase dnssec08

Description:

The DNSKEY RRset in a signed zone must itself be signed, and the signature must be cryptographically valid against one of the DNSKEYs in the same set. This check fetches the DNSKEY set and its RRSIG from every nameserver and verifies the signature - its presence, timing, and cryptographic match.

## Testcase dnssec09

Description:

The SOA record in a signed zone must carry its own RRSIG, and that signature must verify against a DNSKEY from the zone. This check queries each nameserver for SOA+RRSIG and validates the signature. A failure here usually means resolvers will reject everything from the zone as a DNSSEC error.

## Testcase dnssec10

Description:

Signed zones must be able to prove that a name or type does not exist, using either NSEC or NSEC3 records. This check probes apex NSEC/NSEC3PARAM across every nameserver and verifies the zone uses one method consistently, that the structural records are present on every server, and that their signatures are intact.

## Testcase dnssec11

Description:

A zone is either signed or not signed, and the parent's DS records must agree with that state. This check compares every nameserver's signed/unsigned state with the parent's DS presence and flags any inconsistency: DS at the parent but an unsigned zone (resolvers will fail everything as bogus) or signed zones without DS at the parent (no chain of trust).

## Testcase dnssec13

Description:

Every algorithm that appears in your DNSKEY set must also appear in the signatures of the DNSKEY, SOA, and NS records. Validating resolvers that understand a particular algorithm expect to find a signature in that algorithm. This check confirms every published algorithm is actually used to sign the zone data.

## Testcase dnssec14

Description:

DNSSEC uses cryptographic keys to sign your zone so resolvers can verify that the answers they receive have not been tampered with. For RSA keys the size (in bits) decides how hard the key is to break. This check looks at every RSA DNSKEY you publish and compares its size against the allowed range and the current recommended size for that algorithm.

## Testcase dnssec15

Description:

CDS and CDNSKEY are signalling records that a child zone can publish to tell the parent which DS records it wants. This check confirms that CDS and CDNSKEY (if used) are consistent across all your nameservers and that their content agrees with each other, so that registry-side automation does not see conflicting signals.

## Testcase dnssec16

Description:

When a zone publishes a CDS record to tell its parent to update the DS, the CDS must be properly signed by a key that corresponds to it, and must make sense against the current DNSKEY set. This check validates the CDS content and signatures so the signal actually survives registry-side checks.

## Testcase dnssec17

Description:

CDNSKEY works like CDS but carries the full DNSKEY rather than a digest. This check validates the CDNSKEY's content and signatures the same way CDS is validated, since registries use either as the trigger for DS updates.

## Testcase dnssec18

Description:

When CDS or CDNSKEY is published, its signatures must be verifiable with a key that matches a DS already published at the parent - otherwise the registry has no trust path for the update request. This check verifies that relationship and flags any CDS/CDNSKEY whose chain does not close against the parent's current DS.

## Testcase dnssec19

Description:

Some cryptographic keys are known to be weak or have been generated by broken tooling (the ROCA vulnerability, the 2008 Debian OpenSSL bug, Fermat-factorable keys, and similar issues). This check screens every published DNSKEY against a list of such weaknesses, using routines ported from the `badkeys` project, and flags any key that is unsafe to rely on.

## Testcase dnssec20

Description:

NSEC and NSEC3 records carry a type bitmap telling resolvers which record types exist at a name. If the bitmap lists fewer types than actually exist, resolvers using aggressive negative caching can be fooled into caching a wrong "does not exist" answer. This check compares the bitmap at the apex with the types really present and flags any mismatch.

## Testcase dnssec22

Description:

The nameservers of a zone are often named inside the zone itself, and then their addresses are part of the data the zone signs. If those addresses do not validate, a validating resolver that looks them up gets a failure and stops using the nameserver, even while the rest of the zone is correct. This check reads the address records of every such nameserver name from each of your nameservers and verifies them against the chain of trust of the zone.

## Tag DS01_DS_ALGO_DEPRECATED

Header: DS uses deprecated digest algorithm

Description:

A DS record for your zone uses a digest algorithm that has been deprecated (for example the old SHA-1 digest). Validating resolvers that disable deprecated algorithms will treat the DS as missing, so the parent's commitment to your zone's key is not actually checked on those resolvers.

## Tag DS01_DS_ALGO_NOT_DS

Header: DS digest algorithm not a DS algorithm

Description:

A DS record uses a digest-algorithm code that is not defined for DS at all. The record cannot be used by any validating resolver, so the parent-side link in your chain of trust is effectively broken for this entry.

## Tag DS01_DS_ALGO_PRIVATE

Header: DS uses private-use algorithm

Description:

A DS record uses a digest-algorithm code from the private-use range. Private-use algorithms are not interoperable - resolvers outside your private agreement will not understand them - so the DS cannot participate in the public chain of trust.

## Tag DS01_DS_ALGO_RESERVED

Header: DS uses reserved algorithm code

Description:

A DS record uses a digest-algorithm code that IANA has reserved and not yet assigned a meaning to. No resolver knows how to use it, so the DS contributes nothing to DNSSEC validation.

## Tag DS01_DS_ALGO_UNASSIGNED

Header: DS uses unassigned algorithm code

Description:

A DS record uses a digest-algorithm code that has not been assigned by IANA. As no resolver understands the algorithm, the DS offers no validation help and is effectively dead weight at the parent.

## Tag DS01_NO_RESPONSE

Header: No DS response from parent server

Description:

A parent-side nameserver did not answer the DS query for your zone. The check could not confirm the parent's commitment to your keys through that server, which affects resolvers that happen to hit the same non-responsive path.

## Tag DS01_PARENT_SERVER_NO_DS

Header: Parent server returned no DS

Description:

A parent-side nameserver explicitly returned no DS records even though other parent servers provide them. Resolvers landing on this particular server will see your zone as unsigned and skip DNSSEC validation, which is both a security loss and a source of inconsistent behaviour.

## Tag DS02_DNSKEY_NOT_FOR_ZONE_SIGNING

Header: DNSKEY is not a zone-signing key

Description:

A DNSKEY that a DS points at is missing the "zone" flag, meaning it is not marked as being used to sign zone data. Resolvers that enforce the flag will not trust signatures made by this key, so it cannot complete the DNSSEC chain even though the DS matches it.

## Tag DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS

Header: DNSKEY not signed by any DS-matched key

Description:

The DNSKEY RRset is not signed by a DNSKEY that any DS record at the parent matches. Validating resolvers cannot bootstrap trust: they have the DS, and they have the DNSKEY set, but no key authorised by the DS signs the set.

## Tag DS02_DS_ALGO_DNSKEY_MISMATCH

Header: DS algorithm does not match DNSKEY

Description:

A DS record at the parent names a different algorithm than the DNSKEY it points to, even though the key tag and digest line up. Validating resolvers ignore a DS whose algorithm field disagrees with the key, so this DS cannot establish trust; some resolvers fail the whole zone. Replace the DS with one generated from the actual key.

## Tag DS02_NO_DNSKEY_FOR_DS

Header: No DNSKEY matches this DS

Description:

A DS record at the parent does not match any DNSKEY in the child zone. The key the parent vouched for is gone (or never existed), so this DS can never complete validation. It should be removed from the registry or balanced by publishing the referenced key.

## Tag DS02_NO_MATCHING_DNSKEY_RRSIG

Header: No DNSKEY has a matching RRSIG

Description:

The DNSKEY set contains keys that are supposed to sign zone data, but none of their RRSIGs could be matched back to a DNSKEY. Resolvers cannot validate anything and will treat the zone as bogus.

## Tag DS02_NO_MATCH_DS_DNSKEY

Header: DS does not match any DNSKEY

Description:

A DS record does not match the digest of any DNSKEY currently published by the child zone. Validating resolvers will treat the DS as stale and fail validation, so this is almost always a DS that should be removed from the registry.

## Tag DS02_NO_VALID_DNSKEY_FOR_ANY_DS

Header: No usable DNSKEY for any DS

Description:

None of the DS records at the parent correspond to a DNSKEY that can successfully sign the DNSKEY RRset. Every validation path is broken: resolvers have DS information but no way to derive trust from it.

## Tag DS02_RRSIG_NOT_VALID_BY_DNSKEY

Header: RRSIG does not validate

Description:

An RRSIG on the DNSKEY set does not verify against the DNSKEY it claims to be made by. Either the key has been rolled, the signature is corrupted, or the records were tampered with - in any case, validating resolvers will treat the zone as bogus.

## Tag DS03_ERROR_RESPONSE_NSEC_QUERY

Header: Error response to NSEC3 query

Description:

A nameserver returned an error rcode in response to a query about NSEC3 parameters. The check cannot establish what the server is doing for authenticated denial-of-existence, which can hide real DNSSEC problems on that server.

## Tag DS03_ERR_MULT_NSEC3

Header: Multiple NSEC3 records at apex

Description:

A nameserver returned more than one NSEC3 record for the zone apex. Only one NSEC3PARAM-consistent set is allowed per zone; duplicates indicate a corrupted or misconfigured server that will likely serve inconsistent denial-of-existence answers.

## Tag DS03_ILLEGAL_HASH_ALGO

Header: Illegal NSEC3 hash algorithm

Description:

The zone's NSEC3 parameters use a hash algorithm code that is not defined (the only defined value is SHA-1 / algorithm 1). Resolvers cannot compute the hashes the zone uses, so authenticated denial-of-existence will fail and the zone will look bogus.

## Tag DS03_ILLEGAL_ITERATION_VALUE

Header: Illegal NSEC3 iteration count

Description:

The NSEC3 iteration count is outside the allowed range. Too-high iteration counts also cause excessive CPU load on validating resolvers, and some modern resolvers cap the value and simply treat higher counts as bogus. Either way, validation is unreliable.

## Tag DS03_ILLEGAL_SALT_LENGTH

Header: Illegal NSEC3 salt length

Description:

The NSEC3 salt length is outside the allowed range. Salt primarily slows dictionary attacks on hashes; oversized salts bloat every NSEC3 record without security benefit and can trigger truncation problems in resolver implementations.

## Tag DS03_INCONSISTENT_HASH_ALGO

Header: Inconsistent NSEC3 hash algorithm

Description:

Nameservers published different NSEC3 hash algorithms for the same zone. Resolvers cannot compute hashes consistently, so denial-of-existence answers will verify on some servers and fail on others.

## Tag DS03_INCONSISTENT_ITERATION

Header: Inconsistent NSEC3 iteration count

Description:

Nameservers published different NSEC3 iteration counts. The hashing is different per server, so denial-of-existence records cannot be cross-validated and resolvers will see inconsistent negative answers.

## Tag DS03_INCONSISTENT_NSEC3_FLAGS

Header: Inconsistent NSEC3 flags

Description:

Nameservers published different NSEC3 flag bits for the zone. Features like opt-out are controlled by these flags; a mismatch means resolvers can see different denial-of-existence semantics depending on which server they ask.

## Tag DS03_INCONSISTENT_SALT_LENGTH

Header: Inconsistent NSEC3 salt

Description:

Nameservers published different NSEC3 salt values. Since the salt is part of the hash input, denial-of-existence records hashed with different salts cannot be cross-validated, so any cached negative answer can look invalid to a resolver that then asks a different server.

## Tag DS03_NO_RESPONSE_NSEC_QUERY

Header: No response to NSEC3 query

Description:

A nameserver did not respond at all to the check's NSEC3 probe. The check cannot confirm the server's denial-of-existence setup, and real resolvers that depend on the same queries will see the same silence.

## Tag DS03_SERVER_NO_DNSSEC_SUPPORT

Header: Nameserver lacks DNSSEC support

Description:

A nameserver listed for your zone did not return DNSSEC material even when asked with DO set. Resolvers that validate will see the server as unsigned, which - for a zone that is otherwise signed - looks like a bogus response.

## Tag DS03_SERVER_NO_NSEC3

Header: Nameserver publishes no NSEC3

Description:

A nameserver did not publish NSEC3 records even though other servers in the zone do. Resolvers that land on this server cannot get authenticated denial-of-existence, which makes non-existent names unprovable and breaks DNSSEC validation for negative answers.

## Tag DS03_UNASSIGNED_FLAG_USED

Header: Unassigned NSEC3 flag set

Description:

An NSEC3 record sets a flag bit that has not been assigned a meaning. Some resolvers refuse to trust NSEC3 with unknown flags; if yours is like that the negative answers from this zone will fail to validate for them.

## Tag DURATION_LONG

Header: RRSIG lifetime too long

Description:

An RRSIG in your zone has a validity window longer than recommended. Long-lived signatures are more exposed to key compromise, because anyone who steals the private key can continue to forge signatures until the window closes.

## Tag REMAINING_LONG

Header: RRSIG valid for too long into the future

Description:

An RRSIG's remaining validity (time until expiry) is longer than recommended. Similar to long total lifetimes, this leaves a large window during which a compromised key can be used undetected; scheduled resigning should close the window sooner.

## Tag REMAINING_SHORT

Header: RRSIG close to expiry

Description:

An RRSIG is nearing the end of its validity window. If the zone is not resigned before it expires, validating resolvers will start returning SERVFAIL for the records. Resign soon and inspect your signing schedule.

## Tag RRSIG_EXPIRED

Header: RRSIG already expired

Description:

An RRSIG is already past its expiry date. Validating resolvers treat expired signatures as bogus, so lookups returning the affected records are failing DNSSEC validation right now for every resolver that checks. The zone must be resigned immediately.

## Tag DS05_ALGO_DEPRECATED

Header: DNSKEY uses deprecated algorithm

Description:

A DNSKEY in your zone uses a signing algorithm that has been deprecated by the IETF. Resolvers that disable deprecated algorithms will not validate signatures made with it, so this key contributes nothing to your chain of trust on those clients.

## Tag DS05_ALGO_NOT_RECOMMENDED

Header: DNSKEY uses not-recommended algorithm

Description:

A DNSKEY uses an algorithm that is still permitted but no longer recommended for new deployments. The key works today, but planning a rollover to a recommended algorithm (for example ECDSA-P256 or Ed25519) avoids surprises when the algorithm eventually reaches deprecated status.

## Tag DS05_ALGO_NOT_ZONE_SIGN

Header: DNSKEY algorithm not for zone signing

Description:

A DNSKEY uses an algorithm code that is not defined for zone signing (for example GOST, which is for other contexts). Resolvers will not use such a key for validation even if it is present, so the key does not contribute to your chain of trust.

## Tag DS05_ALGO_PRIVATE

Header: DNSKEY uses private-use algorithm

Description:

A DNSKEY uses a signing algorithm code from the private-use range. Resolvers outside your private agreement will not understand it, so the key cannot contribute to the public chain of trust.

## Tag DS05_ALGO_RESERVED

Header: DNSKEY uses reserved algorithm

Description:

A DNSKEY uses an algorithm code that IANA has reserved and not yet assigned. No resolver knows how to validate signatures made with it, so the key cannot participate in DNSSEC validation.

## Tag DS05_ALGO_UNASSIGNED

Header: DNSKEY uses unassigned algorithm

Description:

A DNSKEY uses an algorithm code with no assigned meaning. No resolver understands it, so the key cannot contribute to DNSSEC validation of your zone.

## Tag DS05_NO_RESPONSE

Header: No DNSKEY response from server

Description:

A nameserver did not answer the DNSKEY probe. The check cannot look at its algorithms, and real resolvers that ask the same server will see the same silence - which, for a signed zone, causes validation to fail.

## Tag DS05_SERVER_NO_DNSSEC

Header: Nameserver does not serve DNSSEC

Description:

A nameserver for your zone returned no DNSSEC records even when DO was set in the query. For a zone that is otherwise signed, this looks to a resolver like a bogus or unsigned response from that server.

## Tag EXTRA_PROCESSING_BROKEN

Header: DNSKEY answer missing RRSIG

Description:

A nameserver returned the DNSKEY records without the accompanying RRSIG. Without the signature, the DNSKEY set cannot be validated, so resolvers will not be able to use it as the entry point for validating the rest of the zone.

## Tag DS07_INCONSISTENT_DS

Header: Inconsistent DS at parent

Description:

The parent zone's nameservers published different DS data for your zone. Since DNSSEC validation depends on the DS chain, resolvers hitting different parent servers will reach different validation conclusions, which shows up as intermittent bogus results.

## Tag DS07_INCONSISTENT_SIGNED

Header: Zone signed on some servers only

Description:

Some of your nameservers serve a signed zone and others serve the same zone unsigned. Resolvers will get contradictory signals; those that follow DNSSEC strictly will treat the unsigned answers as downgrade attempts and return bogus.

## Tag DS07_NON_AUTH_RESPONSE_DNSKEY

Header: DNSKEY response not authoritative

Description:

A nameserver returned DNSKEY records without the authoritative answer flag. Resolvers that rely on the flag for DNSSEC validation will refuse to trust the keys, and the chain of trust breaks at that server.

## Tag DS07_NOT_SIGNED

Header: Zone is not signed

Description:

No signatures are visible anywhere in the zone; publishing DNSKEYs alone does not sign it. If the parent has DS for you, resolvers currently report everything from the zone as bogus. Either complete the signing or remove the DS at the registry.

## Tag DS07_NOT_SIGNED_ON_SERVER

Header: Nameserver serves zone unsigned

Description:

This nameserver serves the zone without a signed DNSKEY RRset, either no DNSKEYs at all or DNSKEYs with no covering RRSIG. When every server is listed the whole zone is unsigned; when only some are, resolvers that land on them see an unsigned answer and, if the parent has DS, treat it as bogus.

## Tag DS07_NO_DS_FOR_SIGNED_ZONE

Header: Signed zone has no DS at parent

Description:

Your zone publishes DNSKEYs and RRSIGs but the parent has no DS records. Resolvers cannot anchor trust for your zone, so the signing provides no actual validation benefit. Publish DS at the parent through your registrar or registry to complete the chain.

## Tag DS07_NO_DS_ON_PARENT_SERVER

Header: Parent server lacks DS for signed zone

Description:

A parent-side nameserver did not return DS records for your signed zone, even though other parent servers do. Resolvers that hit this server will see an unsigned delegation, which on some strict resolvers means validation failures.

## Tag DS07_NO_RESPONSE_DNSKEY

Header: No response to DNSKEY query

Description:

A nameserver did not respond to a DNSKEY query at all. Resolvers that validate the zone have to fetch DNSKEYs; if they happen to ask this server first, validation stalls or fails.

## Tag DS07_PARENT_PROVES_NO_DELEGATION

Header: Parent proves the zone is not delegated

Description:

The parent zone carries a signed record stating that no delegation exists at this name, and the parent is itself anchored by a DS at its own parent. The zone answers today only because the servers holding it are also authoritative for the parent, so a resolver that follows the delegation never reaches it, and a validating resolver rejects the zone and every name in it. Add the NS records for this zone to the parent, or remove the zone from those servers.

## Tag DS07_UNEXP_RCODE_RESP_DNSKEY

Header: Unexpected code on DNSKEY query

Description:

A nameserver responded to a DNSKEY query with an unexpected rcode. The server is reachable but mishandling the query, which breaks DNSSEC validation for any resolver that lands on it.

## Tag DS08_DNSKEY_RRSIG_EXPIRED

Header: DNSKEY RRSIG expired

Description:

The signature covering your DNSKEY set has already expired. Validating resolvers will treat the entire zone as bogus because the validation entry point itself is no longer usable. Resign the DNSKEY set as soon as possible.

## Tag DS08_DNSKEY_RRSIG_NOT_YET_VALID

Header: DNSKEY RRSIG not yet valid

Description:

The signature covering your DNSKEY set has a "not valid before" time in the future. Resolvers see the signature as not yet active and refuse to validate against it, so validation fails until the signature becomes current or the zone is resigned with corrected times.

## Tag DS08_MISSING_RRSIG_IN_RESPONSE

Header: DNSKEY RRSIG missing from response

Description:

A nameserver returned the DNSKEY set without its RRSIG. Without the signature, validating resolvers cannot enter the chain of trust, and the zone looks unsigned or bogus depending on whether DS is present at the parent.

## Tag DS08_NO_MATCHING_DNSKEY

Header: No DNSKEY matches RRSIG signer

Description:

The RRSIG on the DNSKEY set points at a key identifier that does not match any DNSKEY in the set. Validation cannot proceed because the signer cannot be located.

## Tag DS08_RRSIG_NOT_VALID_BY_DNSKEY

Header: DNSKEY RRSIG does not verify

Description:

The RRSIG on the DNSKEY set fails cryptographic verification against the DNSKEY it claims to have been signed with. Either the data or the signature is corrupt; resolvers will mark the zone as bogus.

## Tag DS09_MISSING_RRSIG_IN_RESPONSE

Header: SOA RRSIG missing from response

Description:

A nameserver returned the SOA record without the required RRSIG. A signed zone must always include a signature on the SOA, so resolvers that validate will see the answer as bogus and refuse the data.

## Tag DS09_NO_MATCHING_DNSKEY

Header: No DNSKEY matches SOA RRSIG signer

Description:

The RRSIG on the SOA record references a key identifier that no DNSKEY in the zone matches. The signer cannot be located, so validation cannot proceed for SOA - or for anything else that depends on it.

## Tag DS09_RRSIG_NOT_VALID_BY_DNSKEY

Header: SOA RRSIG does not verify

Description:

The signature on the SOA record fails cryptographic verification against its DNSKEY. That makes SOA look bogus to a validating resolver, and many resolvers treat SOA failure as failure of the whole zone.

## Tag DS09_SOA_RRSIG_EXPIRED

Header: SOA RRSIG expired

Description:

The signature on the SOA record has already expired. Every validating resolver will reject the SOA as bogus; because the SOA underlies many queries, the zone is effectively broken for DNSSEC until it is resigned.

## Tag DS09_SOA_RRSIG_NOT_YET_VALID

Header: SOA RRSIG not yet valid

Description:

The signature on the SOA record has a "not valid before" time in the future. Resolvers treat the SOA as bogus until the time arrives or the signature is replaced with one whose inception is in the past.

## Tag DS10_ERR_MULT_NSEC

Header: Multiple NSEC records at apex

Description:

A nameserver returned more than one NSEC record for the zone apex where one is expected. Duplicates indicate a broken server or corrupted zone, and the extra records usually cause validation to fail.

## Tag DS10_ERR_MULT_NSEC3

Header: Multiple NSEC3 records at apex

Description:

A nameserver returned more than one NSEC3 record at the apex position. This normally means the server is serving inconsistent denial-of-existence data, which breaks aggressive negative caching and can cause validation failures.

## Tag DS10_EXPECTED_NSEC_NSEC3_MISSING

Header: Expected NSEC or NSEC3 missing

Description:

A query that should have been answered with NSEC or NSEC3 denial-of-existence records got neither. The zone cannot prove the absence of the queried name, which makes validating resolvers treat the answer as bogus.

## Tag DS10_INCONSISTENT_NSEC

Header: Inconsistent NSEC at apex

Description:

Different nameservers returned different NSEC records for the zone apex. Resolvers that cross-check will see conflicting denial-of-existence data, which breaks negative answer validation.

## Tag DS10_INCONSISTENT_NSEC3

Header: Inconsistent NSEC3 at apex

Description:

Different nameservers returned different NSEC3 records for the apex. Since NSEC3 is used to prove non-existence of names, inconsistent records mean some resolvers will see a valid proof and others will not.

## Tag DS10_INCONSISTENT_NSEC_NSEC3

Header: Some servers use NSEC, others NSEC3

Description:

Some nameservers for the zone publish NSEC records and others publish NSEC3. Every server must use the same denial-of-existence mechanism; a mix means resolvers get unvalidatable mixed answers.

## Tag DS10_MIXED_NSEC_NSEC3

Header: Both NSEC and NSEC3 served

Description:

A nameserver published both NSEC and NSEC3 records for the zone. Only one method is allowed at a time; the mix is a protocol error that may cause some resolvers to pick one method and ignore the other, with inconsistent results.

## Tag DS10_NONSTANDARD_NSEC_RESPONSE

Header: Non-standard NSEC response shape

Description:

A nameserver answered an NSEC query by placing the NSEC record in the authority section instead of the answer section. This is the response shape used by RFC 4470 white-lies and RFC 9824 compact denial of existence implementations (e.g. AWS Route 53, Cloudflare). Validating resolvers accept either shape; the notice is informational so the operator is aware their nameserver uses the non-standard form.

## Tag DS10_NSEC3PARAM_GIVES_ERR_ANSWER

Header: Error rcode on NSEC3PARAM query

Description:

A nameserver returned an error rcode to an NSEC3PARAM query. The zone's NSEC3 configuration cannot be inspected through that server, and resolvers that encounter the same path will not be able to validate denial-of-existence answers reliably.

## Tag DS10_NSEC3PARAM_MISMATCHES_APEX

Header: NSEC3PARAM not at apex

Description:

The NSEC3PARAM record returned by a nameserver has an owner name other than the zone apex. Only apex NSEC3PARAM is valid; a record elsewhere is a misconfiguration that resolvers will ignore.

## Tag DS10_NSEC3PARAM_QUERY_RESPONSE_ERR

Header: NSEC3PARAM response broken

Description:

A nameserver's response to the NSEC3PARAM query is malformed or has other structural issues. Resolvers that receive similar answers cannot use the parameters, so denial-of-existence validation will be unreliable.

## Tag DS10_NSEC3_ERR_TYPE_LIST

Header: Broken NSEC3 type bitmap

Description:

An NSEC3 record carries a type bitmap that is not well formed. Validating resolvers parse the bitmap to see which record types exist at a hashed name; a broken bitmap breaks that step and can cause validation failures.

## Tag DS10_NSEC3_MISMATCHES_APEX

Header: NSEC3 not at apex

Description:

The NSEC3 record returned has an owner name that does not correspond to the zone apex hash. Resolvers cannot anchor their denial-of-existence checks, so validation of negative answers fails.

## Tag DS10_NSEC3_MISSING_SIGNATURE

Header: NSEC3 missing RRSIG

Description:

An NSEC3 record returned at the apex has no accompanying RRSIG. Unsigned NSEC3 cannot be used for DNSSEC validation, so negative answers from this server will be treated as bogus.

## Tag DS10_NSEC3_NODATA_MISSING_SOA

Header: NSEC3 no-data response missing SOA

Description:

A nameserver returned an NSEC3 proof of no-data without the SOA record the proof requires. The proof is incomplete, so resolvers cannot confirm the "no-data" answer and treat it as bogus.

## Tag DS10_NSEC3_NODATA_WRONG_SOA

Header: NSEC3 no-data SOA owner wrong

Description:

An NSEC3 no-data response included an SOA, but its owner name does not match the zone. Resolvers rely on the SOA to tie the proof to the right zone; with the wrong owner the proof is rejected.

## Tag DS10_NSEC3_NO_VERIFIED_SIGNATURE

Header: NSEC3 signature does not verify

Description:

The RRSIG on an NSEC3 record did not verify cryptographically. Resolvers cannot trust the denial-of-existence proof, so any negative answer relying on it is treated as bogus.

## Tag DS10_NSEC3_RRSIG_EXPIRED

Header: NSEC3 RRSIG expired

Description:

The signature covering an NSEC3 record has expired. Validating resolvers reject the proof, so negative answers through that record fail validation until the zone is resigned.

## Tag DS10_NSEC3_RRSIG_NOT_YET_VALID

Header: NSEC3 RRSIG not yet valid

Description:

The signature on an NSEC3 record has a start time in the future. Resolvers will not use it for validation until that time arrives or a replacement signature is published.

## Tag DS10_NSEC3_RRSIG_NO_DNSKEY

Header: No DNSKEY matches NSEC3 RRSIG

Description:

An RRSIG on an NSEC3 record references a DNSKEY that is not in the zone's published key set. Resolvers cannot locate the signer, so the NSEC3 record fails validation and negative answers become bogus.

## Tag DS10_NSEC3_RRSIG_VERIFY_ERROR

Header: NSEC3 RRSIG verify error

Description:

The cryptographic check of an NSEC3 RRSIG against its DNSKEY failed with an unexpected error. Resolvers will treat the record as bogus and reject any proof of non-existence that depended on it.

## Tag DS10_NSEC_ERR_TYPE_LIST

Header: Broken NSEC type bitmap

Description:

An NSEC record carries a type bitmap that is not well formed. Validating resolvers read the bitmap to know which record types are present; a broken bitmap breaks validation of negative answers involving this record.

## Tag DS10_NSEC_GIVES_ERR_ANSWER

Header: Error rcode on NSEC query

Description:

A nameserver returned an error rcode on a query that should have produced an NSEC record. Denial-of-existence cannot be proven through that server, so resolvers validating negative answers will fail.

## Tag DS10_NSEC_MISMATCHES_APEX

Header: NSEC owner not the apex

Description:

The NSEC record returned has an owner name that is not the zone apex. Apex-probing queries must be answered with an NSEC whose owner matches; otherwise the proof is malformed and resolvers reject it.

## Tag DS10_NSEC_MISSING_SIGNATURE

Header: NSEC missing RRSIG

Description:

An NSEC record at the apex has no RRSIG. Unsigned NSEC cannot be used for validation, so any negative answer that relies on it will be rejected as bogus by validating resolvers.

## Tag DS10_NSEC_NODATA_MISSING_SOA

Header: NSEC no-data response missing SOA

Description:

A no-data response proven with NSEC is missing the SOA record required to tie the proof to the zone. Validating resolvers reject the proof because they cannot establish context.

## Tag DS10_NSEC_NODATA_WRONG_SOA

Header: NSEC no-data SOA owner wrong

Description:

The SOA accompanying an NSEC no-data response has the wrong owner name. Resolvers use that SOA to tie the proof to the correct zone; with the wrong owner the no-data proof is rejected.

## Tag DS10_NSEC_NO_VERIFIED_SIGNATURE

Header: NSEC signature does not verify

Description:

The RRSIG on an NSEC record failed cryptographic verification. Denial-of-existence answers that reference this record will be treated as bogus by validating resolvers.

## Tag DS10_NSEC_QUERY_RESPONSE_ERR

Header: NSEC response broken

Description:

A nameserver's response to an NSEC query is malformed or otherwise cannot be processed. Resolvers that land on the server cannot validate negative answers for your zone.

## Tag DS10_NSEC_RRSIG_EXPIRED

Header: NSEC RRSIG expired

Description:

The signature on an NSEC record has expired. Validating resolvers will reject the signature, and any negative answer that depends on it becomes bogus until the zone is resigned.

## Tag DS10_NSEC_RRSIG_NOT_YET_VALID

Header: NSEC RRSIG not yet valid

Description:

The signature on an NSEC record has a start time in the future. Resolvers will not accept the proof until that time arrives or the signature is replaced with one whose inception is in the past.

## Tag DS10_NSEC_RRSIG_NO_DNSKEY

Header: No DNSKEY matches NSEC RRSIG

Description:

The RRSIG on an NSEC record references a key identifier not in the DNSKEY set. Resolvers cannot locate the signer, so the NSEC record fails validation and negative answers become bogus.

## Tag DS10_NSEC_RRSIG_VERIFY_ERROR

Header: NSEC RRSIG verify error

Description:

The cryptographic verification of an NSEC RRSIG against its DNSKEY failed. The denial-of-existence record cannot be trusted, so negative answers that depend on it will be rejected by validating resolvers.

## Tag DS10_SERVER_NO_DNSSEC

Header: Nameserver serves zone unsigned

Description:

A nameserver for a signed zone returned no DNSSEC material. Resolvers hitting that server will see an unsigned delegation, which for a DS-backed zone looks like a downgrade attempt and is treated as bogus.

## Tag DS11_DS_BUT_UNSIGNED_ZONE

Header: DS at parent but child unsigned

Description:

The parent publishes DS records for your zone, but the zone itself is not signed. Every validating resolver will treat your answers as bogus because the DS commits to a signed zone that does not exist. Either complete the signing or remove the DS at the registry.

## Tag DS11_INCONSISTENT_DS

Header: Parent servers disagree on DS

Description:

Different parent-side nameservers published different DS data for your zone. Validating resolvers that cross-check the parent will see conflicting chains of trust, which causes validation to succeed or fail depending on which parent server answered.

## Tag DS11_INCONSISTENT_SIGNED_ZONE

Header: Child servers disagree on signed state

Description:

Some of your nameservers serve a signed zone and others do not, either answering without DNSSEC data or serving DNSKEYs with no signatures. Resolvers get mixed signals about whether to validate, and strict resolvers treat the unsigned answers as bogus.

## Tag DS11_NS_WITH_UNSIGNED_ZONE

Header: Specific nameserver serves unsigned

Description:

A specific nameserver for your zone does not serve a signed zone. It either returns no DNSSEC data at all, or DNSKEYs with no signatures. If the parent has DS for you, this server looks to a validating resolver like a downgrade attempt, and everything coming from it is treated as bogus.

## Tag DS11_UNDETERMINED_DS

Header: DS state could not be determined

Description:

The check could not reach a definitive conclusion about whether your zone has DS at the parent. This usually indicates inconsistent or partially broken parent responses, which will show up as intermittent validation issues for real resolvers too.

## Tag DS11_UNDETERMINED_SIGNED_ZONE

Header: Zone signed state could not be determined

Description:

The check could not determine whether your zone is signed. Responses from your nameservers did not consistently show or hide DNSSEC material, which is itself a problem because resolvers need a consistent signal to validate correctly.

## Tag DS13_ALGO_NOT_SIGNED_DNSKEY

Header: Missing DNSKEY RRSIG for algorithm

Description:

An algorithm present in your DNSKEY set is not represented in the RRSIG covering the DNSKEY RRset. Resolvers that understand that algorithm expect a signature in it; without one they have to skip the validation path, which RFC 6840 treats as bogus.

## Tag DS13_ALGO_NOT_SIGNED_NS

Header: Missing NS RRSIG for algorithm

Description:

An algorithm present in your DNSKEY set is not used to sign the NS RRset. Resolvers that understand the algorithm and require a signature in it will treat the NS answer as bogus, which breaks delegation lookups.

## Tag DS13_ALGO_NOT_SIGNED_SOA

Header: Missing SOA RRSIG for algorithm

Description:

An algorithm present in your DNSKEY set is not used to sign the SOA. Resolvers that require a signature in every published algorithm will treat the SOA answer as bogus, which cascades to most validated operations.

## Tag DNSKEY_RSA_EXPONENT_LARGE

Header: RSA DNSKEY with an unusually large public exponent

Description:

One of your RSA signing keys uses a public exponent above 2^31-1. The key is valid and gonemaster verifies its signatures, but validators built on some cryptographic libraries, Go's standard library among them, cannot use such an exponent and will fail to validate your zone. When you next roll this key, choose a common exponent such as 65537.

## Tag DNSKEY_SMALLER_THAN_REC

Header: RSA DNSKEY below recommended size

Description:

One of your RSA signing keys is within the allowed range but smaller than the currently recommended size for its algorithm. The key is not known to be insecure today, but recommended sizes go up over time as computing power grows. Plan to roll the key to a larger size before the size drops below the minimum.

## Tag DNSKEY_TOO_SMALL_FOR_ALGO

Header: RSA DNSKEY is too small

Description:

One of your RSA signing keys is smaller than the minimum size allowed for its algorithm. A key this small is considered insecure - an attacker could plausibly forge DNSSEC signatures and redirect your domain to addresses they control. Roll the key to a larger size as soon as possible.

## Tag DNSKEY_TOO_LARGE_FOR_ALGO

Header: RSA DNSKEY is too large

Description:

One of your RSA signing keys is larger than the maximum size allowed for its algorithm. Oversized keys make DNSSEC responses bigger than necessary, which can cause message truncation, TCP fallback, or resolvers refusing to validate your zone at all. Roll the key to a size within the allowed range.

## Tag NO_RESPONSE_DNSKEY

Header: No response to DNSKEY query

Description:

A nameserver did not answer the DNSKEY probe. The check cannot inspect the zone's keys through that server, and real resolvers validating your zone will fail if they hit the same silent path.

## Tag DS15_CDS_NON_MUST_DIGEST

Header: CDS uses an inert digest type

Description:

Your CDS RRset includes records whose digest type is not on the IANA mandatory list for DNSSEC delegation. Modern registries ignore those records, so they cannot trigger a DS update at the parent. Publish a CDS with SHA-256 (currently the mandatory digest type) if you want the parent to act.

## Tag DS15_INCONSISTENT_CDNSKEY

Header: Inconsistent CDNSKEY across servers

Description:

Your nameservers published different CDNSKEY records. Registries read CDNSKEY to decide what DS records to publish at the parent; if the servers disagree, the registry may act on one signal and ignore another, leaving DS state inconsistent or incorrect.

## Tag DS15_INCONSISTENT_CDS

Header: Inconsistent CDS across servers

Description:

Your nameservers published different CDS records. Registries use CDS to update the parent-side DS; conflicting CDS records can trigger unintended updates or leave the signal unresolved.

## Tag DS15_MISMATCH_CDS_CDNSKEY

Header: CDS and CDNSKEY disagree

Description:

The CDS and CDNSKEY records you publish do not match: they should represent the same set of keys. Registries will usually refuse to act on inconsistent signals and may log a failure, so the intended DS update will not happen.

## Tag DS16_CDS_INVALID_RRSIG

Header: CDS RRSIG does not validate

Description:

The signature on your CDS record fails to validate against the DNSKEY set. Registries that validate the CDS before acting will refuse to honour the request, so the intended DS change will not be applied.

## Tag DS16_CDS_MATCHES_NON_ZONE_DNSKEY

Header: CDS matches a non-zone DNSKEY

Description:

The CDS record matches a DNSKEY that does not have the zone-signing flag. CDS is meant to commit to a zone-signing DNSKEY at the parent; using a non-zone key is a protocol violation and will cause registries to reject the request.

## Tag DS16_CDS_MATCHES_NO_DNSKEY

Header: CDS matches no DNSKEY

Description:

The CDS record does not match any DNSKEY currently published in your zone. Registries have no way to verify that the key the CDS refers to actually exists, and will typically refuse to act on the request.

## Tag DS16_CDS_SIGNED_BY_UNKNOWN_DNSKEY

Header: CDS signed by unknown DNSKEY

Description:

The RRSIG on the CDS record references a DNSKEY that is not in your published key set. Registries that validate the CDS cannot locate the signer, so the CDS is rejected and the intended DS update does not happen.

## Tag DS16_CDS_UNSIGNED

Header: CDS not signed

Description:

Your CDS record has no accompanying RRSIG. Registries require CDS to be signed because an unsigned CDS could be injected by an attacker to replace the parent DS; unsigned CDS is ignored.

## Tag DS16_CDS_WITHOUT_DNSKEY

Header: CDS published but zone has no DNSKEY

Description:

You publish a CDS record but no DNSKEY set. Without a DNSKEY set the CDS has nothing to validate against, and registries will reject the request.

## Tag DS16_DNSKEY_NOT_SIGNED_BY_CDS

Header: DNSKEY not signed by CDS-indicated key

Description:

The DNSKEY set is not signed by a key that matches the CDS, even though the CDS is trying to commit the parent to that key. Registries that cross-check will see the mismatch and refuse to act on the CDS.

## Tag DS16_MIXED_DELETE_CDS

Header: Mixed delete and normal CDS

Description:

Your zone publishes both the special "delete" CDS (which asks the registry to remove DS) and regular CDS records. The two are mutually exclusive; registries will treat this as an error and may take no action at all.

## Tag DS17_CDNSKEY_INVALID_RRSIG

Header: CDNSKEY RRSIG does not validate

Description:

The signature on your CDNSKEY record fails to validate against the DNSKEY set. Registries that validate CDNSKEY before acting will refuse to honour the request, so DS will not be updated.

## Tag DS17_CDNSKEY_IS_NON_ZONE

Header: CDNSKEY is not a zone-signing key

Description:

Your CDNSKEY record does not have the zone-signing flag set. CDNSKEY is meant to carry a zone-signing key that the parent should use for DS; a non-zone key is a protocol violation and will be rejected.

## Tag DS17_CDNSKEY_MATCHES_NO_DNSKEY

Header: CDNSKEY matches no DNSKEY

Description:

Your CDNSKEY record does not match any DNSKEY in the zone. Registries cannot verify that the advertised key actually exists, so the CDNSKEY signal will be ignored.

## Tag DS17_CDNSKEY_SIGNED_BY_UNKNOWN_DNSKEY

Header: CDNSKEY signed by unknown DNSKEY

Description:

The RRSIG on the CDNSKEY references a DNSKEY not in your published set. Registries validating the CDNSKEY cannot verify the signer, so the CDNSKEY is rejected.

## Tag DS17_CDNSKEY_UNSIGNED

Header: CDNSKEY not signed

Description:

Your CDNSKEY record has no RRSIG. Registries require CDNSKEY to be signed because an unsigned record could be injected by an attacker to hijack the DS update; unsigned CDNSKEY is ignored.

## Tag DS17_CDNSKEY_WITHOUT_DNSKEY

Header: CDNSKEY published but zone has no DNSKEY

Description:

You publish a CDNSKEY but no DNSKEY set. Without a DNSKEY set the CDNSKEY has nothing to reference, and registries have no way to validate it, so the request is rejected.

## Tag DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY

Header: DNSKEY not signed by CDNSKEY-indicated key

Description:

The DNSKEY set is not signed by the key advertised in CDNSKEY, even though CDNSKEY is trying to commit the parent to that key. Registries that cross-check will see the mismatch and refuse to act on the CDNSKEY.

## Tag DS17_MIXED_DELETE_CDNSKEY

Header: Mixed delete and normal CDNSKEY

Description:

Your zone publishes both the "delete" CDNSKEY (which asks the registry to remove DS) and regular CDNSKEY records. The two are mutually exclusive and registries will treat this as an error and take no action.

## Tag DS18_NO_MATCH_CDNSKEY_RRSIG_DS

Header: CDNSKEY RRSIG chain not anchored

Description:

The CDNSKEY signatures do not chain back to a DNSKEY whose hash corresponds to a DS at the parent. Registries require the CDNSKEY to be validated against the existing DS; without that chain, they have no basis to trust the request.

## Tag DS18_NO_MATCH_CDS_RRSIG_DS

Header: CDS RRSIG chain not anchored

Description:

The CDS signatures do not chain back to a DNSKEY whose hash corresponds to a DS at the parent. Registries validate CDS against the current DS; a CDS that cannot be verified this way will be ignored.

## Tag DS19_BADKEY_BLOCKLIST

Header: DNSKEY matches known-compromised blocklist

Description:

One of your DNSKEYs matches a known-compromised public key (from research blocklists of leaked or exposed keys - for example the 2008 Debian OpenSSL bug). The private key is effectively public, so an attacker can forge DNSSEC signatures. Revoke and replace the key immediately.

## Tag DS19_BADKEY_FERMAT

Header: DNSKEY factorable via Fermat

Description:

One of your RSA DNSKEYs has primes close enough together that Fermat's factorization algorithm can recover the private key in reasonable time. The key is effectively broken; replace it as soon as possible.

## Tag DS19_BADKEY_PATTERN

Header: DNSKEY shows weak-generator pattern

Description:

One of your RSA DNSKEYs shows structural patterns typical of known-broken key generators. The key is considered insecure; roll it out and replace it with one generated by a current, well-reviewed tool.

## Tag DS19_BADKEY_ROCA

Header: DNSKEY vulnerable to ROCA

Description:

One of your RSA DNSKEYs is vulnerable to the "Return of Coppersmith's Attack" (ROCA) and can be factored by a known technique, giving an attacker your private key. Revoke and replace the key - it cannot be considered secure.

## Tag DS19_BADKEY_RSA_INVALID

Header: DNSKEY has invalid RSA parameters

Description:

One of your DNSKEYs has RSA parameters (modulus or exponent) outside the structurally valid range. It cannot be used for correct signing and indicates a broken key generation process; replace the key.

## Tag DS19_BADKEY_SMALL_D

Header: DNSKEY vulnerable to Wiener's attack

Description:

One of your RSA DNSKEYs has a small private exponent that makes it vulnerable to Wiener's attack. The private key can be recovered efficiently, which defeats DNSSEC's signing guarantees; replace the key immediately.

## Tag DS19_BADKEY_SMALL_FACTORS

Header: DNSKEY has small prime factors

Description:

One of your RSA DNSKEYs has small prime factors that an attacker can enumerate, recovering the private key. The key is not secure; revoke and replace it.

## Tag DS19_NO_RESPONSE

Header: No response during DNSKEY badkeys check

Description:

A nameserver did not answer the DNSKEY query used by the badkeys screening check. The check could not inspect the key material on that server, so the screening is incomplete for your zone and a vulnerable key could be hiding on an unchecked path.

## Tag DS20_NO_BITMAP

Header: NSEC/NSEC3 bitmap missing

Description:

An NSEC or NSEC3 record does not carry the type bitmap it should. Resolvers that use aggressive negative caching need the bitmap to know which types exist; without it, negative answers cannot be safely cached, and validation of some responses may fail.

## Tag DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE

Header: NSEC3 bitmap misses present record type

Description:

An NSEC3 record's type bitmap omits an RR type that actually exists at the hashed name. Aggressive negative caching can then wrongly treat that type as non-existent, effectively poisoning resolver caches. Correcting the signer is required to fix the bitmap.

## Tag DS20_NSEC_BITMAP_MISMATCHES_RRTYPE

Header: NSEC bitmap misses present record type

Description:

An NSEC record's type bitmap is missing an RR type that exists at the owner name. Resolvers using RFC 8198 aggressive negative caching can then serve wrong "no such type" answers from cache, so this is a correctness and security issue that must be fixed at the signer.

## Tag DS22_NS_ADDRESS_CHAIN_BROKEN

Header: Broken chain to nameserver address

Description:

The address records of one of your nameservers live in a signed zone below your own, and the link between the two zones is broken: the DS record does not match the key that signs the lower zone, or the signatures on those records do not verify. Validating resolvers cannot reach the key that signs the address, so they reject it and stop using that nameserver.

## Tag DS22_NS_ADDRESS_ORPHAN_ZONE

Header: Nameserver address in orphan zone

Description:

The address records of one of your nameservers are signed by a separate zone loaded on the same nameserver, but your zone proves no delegation to it: the NSEC or NSEC3 record matching that name carries no NS bit. Validating resolvers have no path to the signing key and treat the address as forged.

## Tag DS22_NS_ADDRESS_RRSIG_EXPIRED

Header: Nameserver address signature expired

Description:

The signature covering the address records of one of your nameservers has passed its expiration date. Validating resolvers reject expired signatures, so a resolver that looks the address up again receives a failure and stops using that nameserver. Re-sign the zone that holds the address records.

## Tag DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY

Header: Nameserver address signature invalid

Description:

The signature covering the address records of one of your nameservers does not verify against the keys of the zone that is supposed to sign that name. The signature may be corrupt, not yet valid, made with a key that is no longer published, or made by a zone that has no authority over the name. Validating resolvers treat the address as forged.

## Tag DS22_NS_ADDRESS_UNSIGNED

Header: Nameserver address not signed

Description:

The address records of one of your nameservers carry no signature, although the name lies in a signed part of your zone and no insecure delegation accounts for the gap. Validating resolvers require a signature there, reject the unsigned answer, and cannot look the nameserver up.
