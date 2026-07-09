# Public UI explanations: ZONE

## Testcase zone01

Description:

The SOA record names a primary nameserver in its MNAME field. This check looks at that value and confirms it is a sensible, reachable, authoritative server for the zone, not an obviously bogus placeholder like `localhost` or the root label. A broken MNAME causes operations such as secondary-server refresh and DNS NOTIFY to fail.

## Testcase zone02

Description:

The SOA record carries a "refresh" timer that tells secondary nameservers how often to check for updates to the zone. This check looks up the refresh value and confirms it is above a reasonable minimum. A too-low refresh wastes query traffic; an excessive value can still leave a warning pending depending on policy.

## Testcase zone03

Description:

The SOA timer fields have an internal ordering relationship: refresh should be larger than retry. If refresh is smaller than retry, secondary servers that have just failed to contact the primary would be asked to try again later than they would normally refresh, which makes no sense.

## Testcase zone04

Description:

The SOA "retry" timer controls how long a secondary waits before trying again after a failed refresh. This check confirms that retry is at or above a reasonable minimum - too-short retries hammer the primary with repeated queries when there is a network issue.

## Testcase zone05

Description:

The SOA "expire" timer tells secondaries how long they may keep serving a zone when the primary is unreachable. This check confirms expire is above a minimum (short expires can make short outages look like zone deletions) and is not smaller than the refresh timer (which would make the zone expire before it ever got a chance to refresh).

## Testcase zone06

Description:

The SOA "minimum" field doubles as the default TTL for negative answers from the zone. This check verifies the value is within a sensible range - not so short that caches get no benefit, not so long that mistakes take a long time to recover from.

## Testcase zone07

Description:

The MNAME in the SOA names the zone's primary nameserver. That name must resolve to an IP address and must not itself be a CNAME. This check looks up A and AAAA records for MNAME and flags any problem, because tools that rely on MNAME will break if it does not resolve.

## Testcase zone08

Description:

Mail-exchange (MX) records point at hostnames where mail for the zone should be delivered. The standard forbids these target names from being CNAMEs. This check follows each MX target and flags any that turn out to be an alias, which can cause mail servers to refuse delivery.

## Testcase zone09

Description:

This check asks every authoritative nameserver for your zone's MX records and verifies that they all return the same answer. It also handles a few special cases: root and TLD zones that should not publish MX at all, and the "null MX" convention that explicitly says a domain does not accept mail. Inconsistent or conflicting MX data sends mail to the wrong place.

## Testcase zone10

Description:

This check asks each nameserver for the zone's SOA record and confirms the reply contains exactly one SOA with the correct owner name. Multiple SOAs, missing SOAs, or SOAs for a different name point at serious misconfiguration that will break tools relying on the SOA for zone identity. When a single correct SOA is present the check also looks for a CNAME or DNAME at the zone apex: a CNAME alongside the SOA is illegal, while a DNAME is legal but reported for visibility.

## Testcase zone11

Description:

SPF (Sender Policy Framework) is a TXT record at the zone apex that tells receiving mail servers which hosts may send mail claiming to be from your domain. This check verifies the SPF record is present consistently across your nameservers, that only one record is published (the standard forbids more than one), and that it parses without syntax errors.

## Testcase zone12

Description:

CSYNC is an optional record at the zone apex that lets the registry automatically pick up changes to the NS records or glue of the zone without a manual update. This check looks for CSYNC records across your nameservers and verifies they agree, that only one is published, and that they are well-formed.

## Testcase zone13

Description:

SPF records can reference other records using mechanisms such as `include` and `a`, each of which costs a DNS lookup when a receiving mail server evaluates the policy. RFC 7208 imposes a limit of ten lookups per evaluation. This check walks the SPF references and flags policies that exceed the limit, loop back on themselves, or rely on the deprecated `ptr` mechanism.

## Testcase zone14

Description:

ZONEMD (RFC 8976) is an optional record at the zone apex that publishes a cryptographic hash of the zone's contents, so a recipient can verify that data received from secondary servers, mirrors, or files on disk has not been altered or truncated. This check looks for ZONEMD records on each authoritative nameserver and verifies that the records agree across servers, that the serial matches the SOA, that there are no duplicate (Scheme, Hash) pairs, and that the hash algorithm is one of the IANA-assigned values that standard tooling can verify.

## Tag APEX_DNAME

Header: DNAME at zone apex

Description:

A nameserver returned a DNAME record at your zone apex alongside the SOA. A DNAME at the apex is legal - per RFC 6672 it redirects only names strictly below the owner, not the owner itself, so your SOA and NS records are unaffected. This notice is informational; it tells you the apex subtree is being aliased somewhere else, which is the standard "rename a whole subtree" deployment pattern.

## Tag SOA_AND_CNAME

Header: CNAME at zone apex alongside SOA

Description:

A nameserver returned both a CNAME and a SOA record at your zone apex. The DNS standard (RFC 1034 section 3.6.2) forbids a CNAME from coexisting with any other record type at the same owner name, and the apex always carries at least a SOA and NS. This is an illegal zone configuration: resolvers and DNS tools that encounter it will behave unpredictably - some will follow the CNAME and ignore the SOA, others will refuse to answer at all.

## Tag EXPIRE_LOWER_THAN_REFRESH

Header: SOA expire lower than refresh

Description:

The zone's SOA expire timer is smaller than its refresh timer, which is never correct. Refresh is how often secondaries check for updates; expire is how long they may keep serving stale data before giving up. If expire is smaller than refresh, the zone could expire on a secondary before the secondary has ever had a chance to refresh it.

## Tag EXPIRE_MINIMUM_VALUE_LOWER

Header: SOA expire below recommended minimum

Description:

The zone's SOA expire value is below the recommended minimum. Too short an expire means a brief outage of the primary nameserver can cause secondaries to stop serving the zone altogether, which looks like the domain has been deleted - a much more serious failure than the original outage.

## Tag MNAME_HAS_NO_ADDRESS

Header: SOA primary has no IP address

Description:

The MNAME in your SOA record (the name of the zone's primary nameserver) does not resolve to any A or AAAA record. Tools that use MNAME to find the primary - such as secondaries refreshing the zone, or registries running automated compliance checks - will fail because the name cannot be reached.

## Tag MULTIPLE_SOA

Header: Multiple SOA records returned

Description:

A nameserver returned more than one SOA record for your zone. A zone must have exactly one SOA at the apex; multiple SOAs point at a severely misconfigured server that is either serving more than one view of the zone or has a corrupted zone file.

## Tag MX_RECORD_IS_CNAME

Header: MX target is a CNAME

Description:

One of your MX records points at a hostname that turns out to be defined as a CNAME (alias). The mail standards forbid this - a sending mail server is not required to follow the CNAME and may refuse to deliver mail. The target of an MX must be a real hostname with its own A or AAAA records.

## Tag Z09_INCONSISTENT_MX

Header: Inconsistent MX sets across nameservers

Description:

Different nameservers returned different MX record sets for your zone, so mail delivery depends on which server a sending mail server happens to ask. Messages aimed at your domain may be routed to different destinations depending on the sender's resolver path.

## Tag Z09_INCONSISTENT_MX_DATA

Header: Inconsistent MX data

Description:

Name servers returned different MX data (mail targets and/or preference values). Each divergent group of name servers is listed with the mail targets it returned. Mail senders can therefore see different MX data depending on which name server they query, leading to unpredictable routing.

## Tag Z09_NON_AUTH_MX_RESPONSE

Header: MX answer not authoritative

Description:

A nameserver returned MX data for your zone without the "authoritative answer" flag set. Resolvers that validate the AA flag will not trust the answer, and sending mail servers may retry elsewhere or reject the delivery outright.

## Tag Z09_NO_RESPONSE_MX_QUERY

Header: No response to MX query

Description:

A nameserver did not answer a query for your zone's MX records. Sending mail servers that happen to ask this nameserver first will see timeouts and may delay or fail to deliver mail aimed at your domain.

## Tag Z09_NULL_MX_WITH_OTHER_MX

Header: Null MX published alongside normal MX

Description:

Your zone publishes a null MX record (`"."` with preference 0, meaning "this domain does not accept mail") along with other regular MX records. The two contradict each other - mail servers cannot tell whether to deliver or bounce - and most will treat it as a configuration error and refuse delivery.

## Tag Z09_TLD_EMAIL_DOMAIN

Header: MX record at TLD apex

Description:

A top-level domain (TLD) zone apex is publishing MX records. TLD zones are not meant to receive mail directly and should not publish MX at the apex. This is almost always an operational mistake rather than a real mail configuration, and it can cause problems with systems that assume TLDs are purely administrative.

## Tag Z09_UNEXPECTED_RCODE_MX

Header: Unexpected code for MX query

Description:

A nameserver returned an unusual response code to an MX query at the zone apex. The server is reachable but mishandling the query, which can lead to mail delivery problems or bounce messages for senders whose resolvers contact this particular server.

## Tag Z11_INCONSISTENT_SPF_POLICIES

Header: Inconsistent SPF policies across nameservers

Description:

Different nameservers for your zone returned different SPF (`v=spf1`) records. That means mail servers validating the sender will see different policies depending on which nameserver they asked, which can cause legitimate mail to be rejected as spoofed - or spoofed mail to be accepted as legitimate.

## Tag Z11_SPF_MULTIPLE_RECORDS

Header: More than one SPF record

Description:

Your zone publishes more than one `v=spf1` TXT record at the apex. The SPF standard forbids multiple SPF records for the same domain; when receivers see more than one they typically treat the policy as "permerror" and may reject your mail or treat it as unauthenticated.

## Tag Z11_SPF_SYNTAX_ERROR

Header: SPF record syntax error

Description:

Your zone's SPF record does not parse according to the SPF grammar defined in RFC 7208. Mail servers that try to evaluate it will return "permerror" and either reject mail claiming to come from your domain or pass it through without any authentication signal, depending on how strict the receiver is.

## Tag Z11_UNABLE_TO_CHECK_FOR_SPF

Header: Unable to retrieve SPF

Description:

The check could not retrieve the SPF TXT data from one or more of your nameservers (for example, because the server dropped the TXT query or returned an unexpected error). That same problem affects real mail receivers, who will see an SPF lookup failure and may treat your mail as unauthenticated.

## Tag Z12_INCONSISTENT_CSYNC

Header: Inconsistent CSYNC across nameservers

Description:

Your nameservers published different CSYNC records for the zone. CSYNC tells the parent registry how to sync NS and glue changes automatically; inconsistent CSYNC can trigger registry updates from stale or conflicting instructions, or cause the parent to refuse to act on the record at all.

## Tag Z12_MIXED_PRESENCE

Header: CSYNC present on some nameservers only

Description:

Some nameservers for your zone publish a CSYNC record and others do not. CSYNC is evaluated by the registry, which may fetch it from any server; intermittent presence means the registry's view of the record depends on which nameserver happens to answer at that moment.

## Tag Z12_MULTIPLE_CSYNC

Header: More than one CSYNC record

Description:

Your zone publishes more than one CSYNC record. Only one is allowed at the apex; multiple CSYNC records are a protocol error and registries will typically ignore them all rather than pick one, so the automated sync you were trying to configure will not happen.

## Tag Z12_SERIAL_MISMATCH

Header: CSYNC serial does not match SOA

Description:

The serial in your CSYNC record does not match the zone's current SOA serial. CSYNC is only meant to be actioned when the two serials agree; a registry that notices the mismatch will refuse to apply the CSYNC, so any NS or glue changes it was meant to publish will be skipped.

## Tag Z13_SPF_LOOKUP_COUNT_EXCEEDED

Header: SPF exceeds DNS lookup limit

Description:

Evaluating your SPF policy requires more than the ten DNS lookups that RFC 7208 permits. Receivers that follow the limit will stop evaluation early and return "permerror", which typically causes legitimate mail to be rejected or treated as unauthenticated.

## Tag Z13_SPF_LOOKUP_LOOP

Header: SPF evaluation loops

Description:

Your SPF policy references another SPF policy that, directly or indirectly, references itself. Receivers that try to evaluate it will detect the loop and return "permerror", which usually causes mail to be rejected or marked as unauthenticated.

## Tag Z13_SPF_PTR_DEPRECATED

Header: SPF uses deprecated "ptr" mechanism

Description:

Your SPF policy uses the `ptr` mechanism, which RFC 7208 has deprecated because it is slow, fragile, and easy to get wrong. Some receivers have started ignoring `ptr` outright, and relying on it for authorisation will break your mail from those receivers without warning.

## Tag Z13_UNABLE_TO_CHECK

Header: SPF evaluation could not complete

Description:

The check could not follow every reference in your SPF policy (for example because a referenced domain did not answer or returned a broken record). Receiving mail servers will run into the same failure and are likely to return "temperror" or "permerror", which can cause mail from your domain to be deferred or rejected.
