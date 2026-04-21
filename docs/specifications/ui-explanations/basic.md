# Public UI explanations: BASIC

## Testcase basic01

Description:

This check walks the global DNS from the root servers down to your domain to confirm that the parent zone exists and that the child zone is delegated consistently. Problems here mean your domain cannot be located reliably on the public internet before the other tests even run.

## Tag B01_INCONSISTENT_DELEGATION

Header: Inconsistent parent delegation

Description:

The nameservers for your parent zone disagree about how your domain is delegated - different parent servers returned different answers to the same questions. Some resolvers will get one answer and others will get a different one, so the domain may be reachable from some places on the internet but not from others. The usual cause is a delegation change that was not applied consistently across all servers of the parent zone.
