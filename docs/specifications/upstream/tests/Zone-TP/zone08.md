<!--
Upstream-Source: docs/public/specifications/tests/Zone-TP/zone08.md
Upstream-Commit: 73ab6f07f15ae0d7436846410653efef5d81ffe9
Upstream-Date: 2023-06-15T18:44:45+02:00
-->

## ZONE08: MX is not an alias

### Test case identifier
**ZONE08** MX is not an alias

### Objective

An MX type record for a domain must not resolve to a CNAME, following
the text in section 10.3 of [RFC 2181](https://datatracker.ietf.org/doc/html/rfc2181)
and section 2.3.5 in
[RFC 5321](https://datatracker.ietf.org/doc/html/rfc5321#section-2.3.5).

### Inputs

The domain name to be tested.

### Ordered description of steps to be taken to execute the test case

1. Query the MX record from a delegated name server for the domain.
2. If the answer from step 1 is not authoritative, iterate step 1 until there is an authoritative answer.
3. If the MX answer is a CNAME, this test case fails.

### Outcome(s)

If the MX record for the domain is pointing to a CNAME, this test case
fails.

### Special procedural requirements

None.

### Intercase dependencies

None.
