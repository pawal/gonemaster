# Public UI explanations: BASIC

## Testcase basic01

Description:

This check walks the global DNS from the root servers down to your domain to confirm that the parent zone exists and that the child zone is delegated consistently. Problems here mean your domain cannot be located reliably on the public internet before the other tests even run.

## Testcase basic02

Description:

This check contacts each nameserver that the parent zone delegates your domain to and asks it directly for the zone's SOA record - the basic marker that says "I am authoritative for this zone". At least one nameserver must answer authoritatively or the domain has no working home on the internet, no matter what the delegation says.

## Testcase basic03

Description:

When the stricter authority check in basic02 fails, this one tries a softer question: it asks your nameservers for the address of "www" under your domain. A server that cannot confirm authority but still returns an address is a sign of a misconfigured setup - usually a proxy or forwarder in place of a real authoritative server - that may appear to work in some situations and fail in others.

## Tag B01_INCONSISTENT_DELEGATION

Header: Inconsistent parent delegation

Description:

The nameservers for your parent zone disagree about how your domain is delegated - different parent servers returned different answers to the same questions. Some resolvers will get one answer and others will get a different one, so the domain may be reachable from some places on the internet but not from others. The usual cause is a delegation change that was not applied consistently across all servers of the parent zone.

## Tag B01_INCONSISTENT_ALIAS

Header: Inconsistent alias target

Description:

Your domain is configured as an alias (DNAME) that redirects queries to another name, but different servers returned different redirect targets. Resolvers will follow one target or the other unpredictably, so traffic meant for your domain ends up at conflicting destinations depending on which server answered.

## Tag B01_NO_CHILD

Header: Domain not found

Description:

The walk from the root servers found no evidence that your domain exists - neither a delegation NS record at the parent nor an authoritative SOA answer from any probed server. From the public internet the domain looks unregistered or completely unreachable.

## Tag B01_PARENT_NOT_FOUND

Header: Parent zone not identified

Description:

The check walked down from the root servers but could not identify a parent zone that delegates your domain. This usually means the walk was interrupted by missing or inconsistent answers from upper-level nameservers, or that the domain's own parent is misconfigured so its delegation cannot be found reliably.

## Tag B01_PARENT_UNDETERMINED

Header: Multiple possible parent zones

Description:

The walk from the root found more than one plausible parent zone for your domain, which should not happen in a well-configured hierarchy. Resolvers may disagree about where the cut between parent and child is, which leads to inconsistent behaviour depending on which resolver a client uses.

## Tag B02_NO_DELEGATION

Header: No delegation found

Description:

Your domain has no NS records in its parent zone, so resolvers have no way of learning where to look for the domain's own data. Without a delegation the domain effectively does not exist on the public internet, regardless of what nameservers you may actually be running.

## Tag B02_NO_WORKING_NS

Header: No working nameserver

Description:

None of the nameservers listed for your domain answered a direct SOA query as an authoritative server. The domain is effectively unreachable: resolvers can find the delegation in the parent zone but cannot get a usable answer from any server it points to.

## Tag B02_NS_BROKEN

Header: Nameserver answer is malformed

Description:

A nameserver claimed to be authoritative for your domain but its reply did not include the zone's SOA record. The server appears ready to answer yet is not actually loaded with your zone's data, which points to a configuration error on that specific server.

## Tag B02_NS_NOT_AUTH

Header: Nameserver not authoritative

Description:

A nameserver listed for your domain responded, but without the "authoritative answer" flag set. The server is reachable but is not really running your zone - most likely a caching resolver listed as a nameserver by mistake, or a leftover entry pointing at a server that has since been retired.

## Tag B02_NS_NO_IP_ADDR

Header: Nameserver name does not resolve

Description:

One of the names listed as a nameserver for your domain could not be resolved to an IP address. Resolvers that pick that name have nothing to connect to and will treat the nameserver as unavailable, reducing the domain's effective redundancy.

## Tag B02_NS_NO_RESPONSE

Header: Nameserver did not respond

Description:

A nameserver listed for your domain did not respond to the SOA query at all. The server may be offline, blocked by a firewall, or unreachable from the testing client. Either way, it is not providing any fallback if another server has a problem.

## Tag B02_UNEXPECTED_RCODE

Header: Unexpected response code

Description:

A nameserver returned a response code that an authoritative server for your zone should not produce for a direct SOA query. The server is reachable but behaving incorrectly, which usually indicates a misconfigured service or a non-DNS application answering on the same endpoint.

## Tag HAS_A_RECORDS

Header: Answer without authority

Description:

A nameserver returned an address record for "www" under your domain even though the preceding check could not confirm that server as authoritative. This "half-working" behaviour typically comes from a proxy or misconfigured resolver standing in for a proper authoritative nameserver, and the result is unpredictable answers for visitors across the internet.
