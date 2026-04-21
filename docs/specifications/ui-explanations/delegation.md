# Public UI explanations: DELEGATION

## Testcase delegation01

Description:

A domain should be served by at least two authoritative name servers so it stays reachable if one of them has a problem. Fewer than two is a resilience risk: when the single server has any outage the domain effectively disappears from the internet.

## Tag NOT_ENOUGH_NS_DEL

Header: Not enough nameservers

Description:

The parent zone's delegation lists fewer nameservers than the recommended minimum. With too few servers, a single outage can make your domain unreachable - visitors, mail, and other services that depend on DNS stop working until the server comes back.
