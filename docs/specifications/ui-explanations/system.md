# Public UI explanations: SYSTEM

## Tag NON_GLOBAL_QUERY_BLOCKED

Header: Query skipped for a non-public address

Description:

One of your nameservers points at a private, loopback, or otherwise non-globally-reachable address, so this service did not send a query to it and could not fully check that server. A public nameserver should have a public, globally reachable address.

## Tag LIMIT_REACHED

Header: Test limit reached

Description:

Your domain, or a zone above it, exceeds a limit of this test: 32 nameserver names per zone, 16 addresses per name, 512 addresses in total, 4 DNSSEC keys sharing one key tag, 8 signatures per record set, or 50 NSEC3 hash iterations. The test checked what lies within the limit and skipped the rest, so the results say nothing about what was skipped. If the only valid signature or key was skipped, the signature is reported as invalid. This finding does not change the score.
