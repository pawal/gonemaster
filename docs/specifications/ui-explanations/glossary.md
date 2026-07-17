# Public UI glossary

Plain-language definitions of recurring DNS jargon shown as hover/tap
tooltips inside finding explanations on the public results page. Authored
for a domain owner who is not a DNS operator.

Each entry has a stable slug, a `Match:` line, and a `Description:` block.
The sync script emits two keys per entry:

- `pub.glossary.<slug>` - the definition (1-2 sentences).
- `pub.glossary.<slug>.match` - comma-separated phrases the UI wraps as
  glossary tooltips. Only the first occurrence of each term per
  explanation block is linked, so common words stay readable.

All-caps phrases (DNSSEC, RRSIG, DS, ...) match case-sensitively so an
acronym is never confused with an ordinary word; mixed- or lower-case
phrases (glue, delegation, zone apex) match case-insensitively so a term
capitalised at the start of a sentence still links.

## Glossary dnssec

Match: DNSSEC

Description:

DNSSEC (DNS Security Extensions) adds digital signatures to DNS answers so resolvers can verify the reply genuinely came from your zone and was not altered on the way.

## Glossary ds

Match: DS

Description:

A DS (Delegation Signer) record lives in the parent zone and fingerprints your zone's signing key, linking your DNSSEC setup to the chain of trust above it.

## Glossary dnskey

Match: DNSKEY

Description:

A DNSKEY record holds one of the public keys your zone uses for DNSSEC, so resolvers can check the signatures on your records.

## Glossary rrsig

Match: RRSIG

Description:

An RRSIG is the DNSSEC signature attached to a set of DNS records; resolvers use it to confirm the records are authentic and unmodified.

## Glossary nsec

Match: NSEC

Description:

NSEC records let a DNSSEC-signed zone prove that a name does not exist, in a way that cannot be forged into a fake "not found" answer.

## Glossary nsec3

Match: NSEC3

Description:

NSEC3 does the same job as NSEC - proving a name does not exist - but hashes the names so the full contents of your zone cannot easily be listed.

## Glossary soa

Match: SOA

Description:

The SOA (Start of Authority) record carries a zone's core settings: its serial number, refresh timers, and administrative contact.

## Glossary cname

Match: CNAME

Description:

A CNAME record makes one name an alias for another. It is not allowed at a zone apex or on a nameserver name.

## Glossary glue

Match: glue

Description:

Glue is a nameserver's IP address published by the parent zone. It is needed when the nameserver's name lives inside the very zone it serves, because otherwise its address could never be looked up.

## Glossary zone-apex

Match: zone apex, apex

Description:

The zone apex is the top of your domain (for example example.com itself, not www.example.com). Records such as SOA and NS live there.

## Glossary delegation

Match: delegation

Description:

A delegation is the pointer in the parent zone that hands responsibility for your domain to your nameservers.

## Glossary authoritative

Match: authoritative

Description:

An authoritative nameserver holds the real, original data for a zone, as opposed to a resolver that only keeps cached copies.

## Glossary rcode

Match: rcode, response code

Description:

The response code (rcode) is the status a nameserver returns with each answer, such as NOERROR for success or NXDOMAIN for a name that does not exist.

## Glossary edns

Match: EDNS

Description:

EDNS is an extension that lets DNS carry larger responses and extra options over UDP, avoiding a fallback to slower TCP for big answers such as DNSSEC data.

## Glossary referral

Match: referral

Description:

A referral is the parent zone's response that points a resolver at your nameservers, listing their names and, where needed, their glue addresses.

## Glossary ttl

Match: TTL

Description:

The TTL (time to live) tells resolvers how many seconds they may cache a record before fetching a fresh copy.

## Glossary axfr

Match: AXFR

Description:

AXFR is a full zone transfer - a request to download every record in a zone at once - normally allowed only between a zone's own nameservers.

## Glossary digest

Match: digest

Description:

A digest is a short fixed-length fingerprint of a larger piece of data; a DS record uses one to represent your signing key compactly.

## Glossary bailiwick

Match: in-bailiwick, bailiwick

Description:

A name is in-bailiwick when it sits inside the zone being delegated. Such nameservers need glue, because their addresses can only be found within that same zone.

## Glossary chain-of-trust

Match: chain of trust, trust chain

Description:

The chain of trust is the DNSSEC link from the root zone down to yours: each level signs a record vouching for the key of the level below, so a resolver can trust your data from the top down.

## Glossary ipv4

Match: IPv4

Description:

IPv4 is the older internet addressing scheme (for example 192.0.2.1). Many clients still rely on it, so nameservers should stay reachable over it.

## Glossary ipv6

Match: IPv6

Description:

IPv6 is the newer internet addressing scheme (for example 2001:db8::1), designed to succeed IPv4 as address space runs out.
