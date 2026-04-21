# Public UI explanations: DELEGATION

## Testcase delegation01

Description:

A domain should be served by at least two authoritative name servers so it stays reachable if one of them has a problem. Fewer than two is a resilience risk: when the single server has any outage the domain effectively disappears from the internet.

## Testcase delegation02

Description:

A nameserver provides DNS service for your domain. This check looks at the IP addresses each of your nameservers resolves to and flags any case where two different nameserver names end up on the same IP. A single IP behind multiple names is a single point of failure: if that one machine goes down, every "different" server it backs disappears at the same time.

## Testcase delegation03

Description:

When a resolver first asks the parent zone about your domain, the parent replies with a referral listing your nameservers. This check builds the largest plausible referral response for your setup and confirms it still fits in one standard DNS-over-UDP packet. If it does not, resolvers have to fall back to TCP for every first lookup of your domain, which is slower and can fail against firewalls that only allow DNS on UDP.

## Testcase delegation04

Description:

Each nameserver listed for your domain must answer with the "authoritative answer" flag set. A server that does not claim authority is not really running your zone - it is probably a caching resolver that got listed by mistake, or an old record that points at a server that no longer serves the domain.

## Testcase delegation05

Description:

A nameserver name such as ns1.example.com must resolve directly to an IP address. This check confirms that none of your nameserver names are CNAME aliases pointing somewhere else. DNS rules forbid using a CNAME for a nameserver, and resolvers that follow the rules will refuse to use such a server, making your domain unreachable from them.

## Testcase delegation06

Description:

Every DNS zone has a Start-of-Authority (SOA) record that carries the zone's basic metadata - serial number, refresh timers, contact address. This check asks each of your nameservers for the SOA and expects to get one back. A nameserver that answers "no error" but has no SOA is misconfigured: it accepted the question but is not actually serving the zone's data.

## Testcase delegation07

Description:

The parent zone lists the nameservers for your domain ("delegation NS"), and your zone itself lists its nameservers ("child NS"). These two lists must match. This check compares them and flags any server name that appears on one side but not the other, which causes resolvers to sometimes contact a server that will not actually answer.

## Tag NOT_ENOUGH_NS_DEL

Header: Not enough nameservers

Description:

The parent zone's delegation lists fewer nameservers than the recommended minimum. With too few servers, a single outage can make your domain unreachable - visitors, mail, and other services that depend on DNS stop working until the server comes back.

## Tag NOT_ENOUGH_NS_CHILD

Header: Not enough nameservers (from the zone itself)

Description:

Your zone itself lists fewer authoritative nameservers than the recommended minimum when queried directly. This is the same resilience problem as too few nameservers at the parent: if the one or two you have goes down, the domain becomes unreachable until it comes back.

## Tag NOT_ENOUGH_IPV4_NS_DEL

Header: Not enough IPv4 nameservers (delegation)

Description:

The parent zone's delegation reaches too few nameservers over IPv4 to keep the domain available to IPv4-only clients if a server fails. Every domain needs redundancy on each address family it wants to support.

## Tag NOT_ENOUGH_IPV4_NS_CHILD

Header: Not enough IPv4 nameservers (zone)

Description:

Your zone itself lists too few nameservers reachable over IPv4. IPv4-only clients have a thin redundancy margin - a single server outage risks taking your domain offline for them.

## Tag NOT_ENOUGH_IPV6_NS_DEL

Header: Not enough IPv6 nameservers (delegation)

Description:

The parent zone's delegation reaches too few nameservers over IPv6 to keep the domain available to IPv6-only clients if a server fails.

## Tag NOT_ENOUGH_IPV6_NS_CHILD

Header: Not enough IPv6 nameservers (zone)

Description:

Your zone itself lists too few nameservers reachable over IPv6. IPv6-only clients and dual-stack clients that prefer IPv6 will hit this redundancy gap first.

## Tag NO_IPV4_NS_DEL

Header: No IPv4 nameservers in delegation

Description:

The parent zone's delegation for your domain does not reach any nameserver over IPv4. IPv4-only clients - which still make up a large share of the public internet - will not be able to resolve your domain from the delegation alone.

## Tag NO_IPV4_NS_CHILD

Header: No IPv4 nameservers in zone

Description:

Your zone itself does not list any nameserver reachable over IPv4. The delegation and the zone should agree on which address families are available; a mismatch here will confuse some resolvers.

## Tag CHILD_NS_SAME_IP

Header: Nameservers share an IP (zone)

Description:

Two or more of the nameservers your zone lists resolve to the same IP address. A single IP behind multiple "different" nameservers is a single point of failure: if that one machine goes down, all the nameservers listed on it go down at the same time, defeating the point of running more than one.

## Tag DEL_NS_SAME_IP

Header: Nameservers share an IP (delegation)

Description:

Two or more of the nameservers in the parent zone's delegation resolve to the same IP address. Clients that reach you through the delegation will contact what looks like multiple nameservers but is actually one machine, so any outage affects all of them at once.

## Tag SAME_IP_ADDRESS

Header: Nameservers share an IP address

Description:

Once the parent delegation and your own zone's nameserver list are combined, two or more different nameserver names end up on the same IP. That IP is a single point of failure - an outage there takes all those nameservers offline together.

## Tag REFERRAL_SIZE_TOO_LARGE

Header: Referral packet too large for UDP

Description:

The parent zone's referral response for your domain is larger than the 512-byte limit for a standard DNS-over-UDP answer. Every first lookup of your domain forces resolvers to retry over TCP, which is slower and can fail outright against firewalls that only allow DNS on UDP. Shorter nameserver names or a smaller nameserver set both help.

## Tag IS_NOT_AUTHORITATIVE

Header: Nameserver is not authoritative

Description:

One of the nameservers listed for your domain answered a query but did not set the "authoritative answer" flag, so it is not really running your zone. Usually this is an old delegation pointing at a server that no longer serves the zone, or a caching resolver mistakenly listed as authoritative.

## Tag NS_IS_CNAME

Header: Nameserver name is a CNAME

Description:

One of your nameserver names is defined as a CNAME pointing somewhere else instead of directly carrying an A or AAAA record. DNS does not allow CNAMEs on nameserver names, and resolvers that follow the rules will refuse to use this server, making parts of your domain unreachable.

## Tag UNEXPECTED_RCODE

Header: Unexpected response code from nameserver

Description:

A query to one of your nameservers came back with an unusual response code - something other than the expected NOERROR or NXDOMAIN. The server is reachable but is behaving incorrectly for this check, which usually points to a misconfiguration or a non-standard DNS implementation.

## Tag SOA_NOT_EXISTS

Header: SOA record missing from nameserver

Description:

A nameserver listed for your domain accepted the question about the zone's SOA record but returned no SOA in its answer. Every authoritative nameserver must serve the zone's SOA; its absence means that server is not actually configured with your zone data even though it claims to be reachable.

## Tag EXTRA_NAME_PARENT

Header: Parent lists a nameserver the zone does not

Description:

The parent zone's delegation includes a nameserver name that your own zone's NS records do not list. Resolvers that go through the parent will sometimes pick this extra name and query a server that is not part of your current setup, getting inconsistent or stale answers.

## Tag TOTAL_NAME_MISMATCH

Header: Parent and zone list different nameservers

Description:

The set of nameservers in the parent zone's delegation and the set of nameservers your zone itself publishes have no names in common. Resolvers and validators that compare the two will treat your domain as misconfigured; depending on which side a resolver trusts, it may pick an entirely different server than what you currently run.
