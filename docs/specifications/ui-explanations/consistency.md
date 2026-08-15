# Public UI explanations: CONSISTENCY

## Testcase consistency01

Description:

Every authoritative nameserver for your zone should serve the same zone data, and the SOA serial number is the simplest way to tell. This check queries every nameserver for the SOA and compares serial numbers. A difference means one or more servers are lagging behind on zone updates, so clients get different data depending on which server they reach.

## Testcase consistency02

Description:

The SOA record carries a contact address (RNAME) that should be identical on every nameserver because it is part of the zone's own data. This check pulls the SOA from every nameserver and compares RNAMEs. Different values across servers mean the zone content itself is inconsistent, not just drifting on update timing.

## Testcase consistency03

Description:

The SOA record also carries four timer values (refresh, retry, expire, minimum) that control how secondary servers fetch updates and how long cached answers may live. These must match across all nameservers for the zone to behave predictably. This check compares them and flags any server that publishes different values than the others.

## Testcase consistency04

Description:

Every authoritative nameserver should return the same list of NS records for the zone. This check queries each nameserver for the NS set and compares them. If the sets disagree, resolvers will see different candidate nameservers depending on which server they asked first, leading to inconsistent referrals and harder-to-diagnose failures.

## Testcase consistency05

Description:

When the parent zone lists a nameserver that lives inside the child zone (like `ns1.example.com` for `example.com`), it must also publish that server's IP address as "glue" so resolvers can actually reach it. This check compares the glue addresses in the parent against the address data the child publishes for the same names and flags anywhere they disagree.

## Testcase consistency06

Description:

The SOA record names a primary nameserver in its MNAME field, and this name should be identical on every authoritative server for the zone because it is part of the zone data itself. This check compares MNAMEs across servers and flags any disagreement, which usually points to one server being loaded with a different version of the zone than the others.

## Tag MULTIPLE_SOA_SERIALS

Header: Different SOA serials across nameservers

Description:

Your nameservers returned different SOA serial numbers for the same zone, so they are not all serving the same version of the zone data. Clients will see different answers depending on which server a resolver picked, and changes you publish may only be visible on some of them. The usual cause is a secondary server that failed to complete a recent zone transfer.

## Tag CHILD_ZONE_LAME

Header: Nameserver cannot answer for its own address

Description:

A nameserver whose name is inside your zone (such as `ns1.example.com` for `example.com`) did not answer authoritatively for its own A or AAAA records on any of the paths tried. Resolvers that rely on the child zone to confirm the server's address will fail to reach it, so the delegation loses this nameserver as a working fallback.

## Tag IN_BAILIWICK_ADDR_MISMATCH

Header: Glue address not present in the zone

Description:

The parent zone publishes a glue address for one of your nameservers that is not in the matching A or AAAA records in your own zone. Resolvers initially use the glue, so traffic will go to the address the parent advertises - which is not the one your zone says is correct. The two sides must be brought back into agreement or some clients will hit a stale or wrong server.

## Tag MISSING_ADDRESS_CHILD

Header: Nameserver has glue but no address record in your zone

Description:

The parent zone publishes a glue address for one of your nameservers, but your own zone serves no A or AAAA record for that name at all. Resolution keeps working while the glue is served, so this is not urgent, but the glue is then the only thing that can be verified against. Add the matching address records to your zone so the nameserver's address is published where it belongs.

## Tag OUT_OF_BAILIWICK_ADDR_MISMATCH

Header: Glue address disagrees with public DNS

Description:

The parent zone's delegation for your domain includes glue address records for a nameserver whose name is outside the zone, and those addresses do not match what is visible in the public DNS for that name. Out-of-zone glue should not be used at all - and when it disagrees with reality it is guaranteed to cause inconsistent lookups depending on which resolver a client happens to use.

## Tag MULTIPLE_DELEGATION_NS_SET

Header: Parent nameservers serve different delegations

Description:

The parent zone's nameservers do not all hand out the same delegation for your zone - the set of NS names differs depending on which parent server is asked. This is normal for a short time after a delegation change; if it persists, resolvers keep being steered to different nameserver sets and some may reach servers that no longer host your zone.
