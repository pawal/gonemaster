# Public UI explanations: SYSTEM

## Tag NON_GLOBAL_QUERY_BLOCKED

Header: Query skipped for a non-public address

Description:

One of your nameservers points at a private, loopback, or otherwise non-globally-reachable address, so this service did not send a query to it and could not fully check that server. A public nameserver should have a public, globally reachable address.

## Tag LIMIT_REACHED

Header: Not every nameserver tested

Description:

Your domain, or a zone above it, lists more nameservers or more addresses than one test run follows: at most 32 nameserver names per zone, 16 addresses per name and 512 addresses in total. The run tested the first ones in sorted order and skipped the rest, so the results say nothing about the skipped servers. This finding does not change the score.
