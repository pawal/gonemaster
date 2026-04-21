# Public UI explanations: ADDRESS

## Testcase address01

Description:

The IP addresses of your nameservers must be globally reachable on the public internet. This check looks at every nameserver IP and flags any that fall into ranges reserved for documentation (like `192.0.2.0/24`), local networks (`10.0.0.0/8`, link-local addresses), or are otherwise not usable from the open internet. Addresses in those ranges will never be reachable from outside clients.

## Testcase address02

Description:

Every nameserver IP should have a matching reverse DNS (PTR) record. This check looks up the PTR for each nameserver address and flags addresses where the reverse lookup either returns nothing or does not answer at all. Missing reverse records are not fatal, but they make operational troubleshooting harder and can trigger anti-spam heuristics on mail that flows through the same IPs.

## Testcase address03

Description:

The reverse DNS hostname for each nameserver IP should match the name the nameserver is listed as. This check cross-references the PTR record against the forward hostname and flags mismatches, missing PTR records, and unanswered PTR queries. A mismatch does not break DNS resolution but complicates tracing and can look suspicious to monitoring tools.

## Tag A01_ADDR_NOT_GLOBALLY_REACHABLE

Header: Nameserver IP not globally reachable

Description:

A nameserver for your domain is listed with an IP address that is not routable on the public internet (for example a private-range or reserved-use address). Clients outside the local network where that address is valid will fail to connect, effectively removing this nameserver from the set of working servers.

## Tag A01_DOCUMENTATION_ADDR

Header: Nameserver uses documentation-only IP

Description:

A nameserver is configured with an IP address drawn from a range reserved for documentation examples (`192.0.2.x`, `198.51.100.x`, `203.0.113.x`, `2001:db8::/32`). These addresses are never announced on the public internet, so the nameserver is unreachable for anyone outside of a local lab that happens to be using the same example range.

## Tag A01_LOCAL_USE_ADDR

Header: Nameserver uses local-network IP

Description:

A nameserver is listed with a private or link-local address (for example `10.x`, `192.168.x`, `fe80::`). The public internet cannot reach those ranges, so the nameserver is only available to clients on the same local network and not to resolvers elsewhere.

## Tag A01_NO_GLOBALLY_REACHABLE_ADDR

Header: No globally reachable nameserver IP

Description:

None of the IP addresses listed for your nameservers are globally reachable. Every address either belongs to a reserved range or is otherwise not routable on the public internet, which means the domain has no nameserver that clients outside the local network can actually contact.

## Tag A01_NO_NAME_SERVERS_FOUND

Header: No nameserver addresses found

Description:

The check could not find any IP addresses for the nameservers listed in the delegation. Without addresses there is nothing for resolvers to connect to, so the domain is effectively unreachable regardless of what the delegation record says.

## Tag NO_RESPONSE_PTR_QUERY

Header: No response to reverse DNS query

Description:

A reverse DNS (PTR) lookup for one of your nameserver IPs did not return any response at all - the responsible server did not answer. This is usually a problem with the reverse DNS zone rather than your own zone, but it does mean the operational paper trail (what name goes with what IP) is broken from the outside.
