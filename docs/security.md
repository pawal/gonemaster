# Security Policy

This policy covers vulnerabilities in the gonemaster software and in any
deployment whose `/.well-known/security.txt` links to this page.

## Reporting

Report a vulnerability by email to `pawal@amplitut.de`. Do not open a
public issue or pull request for an unfixed vulnerability. Reports MAY be
written in English or Swedish.

A report SHOULD include:

- the version, from `gonemaster --version`, `gonemaster-server --version`, or
  `GET /pub/api/v1/version` on a deployment,
- the affected binary, route, or UI,
- the steps to reproduce,
- the observed impact.

## Scope

In scope:

- authentication or authorization bypass in the admin API, the admin UI, the
  public API, or the MCP endpoint,
- queries, zone transfers, or lookups from the server that reach a loopback,
  private, link-local, or other non-global address, unless the deployment
  allows private targets,
- a crash, a hang, or unbounded memory or CPU use caused by the responses of a
  nameserver under test,
- disclosure through the public API of internal job or batch IDs, private
  profiles, or runs that the requester did not start,
- script injection or a Content Security Policy bypass in the admin, public,
  or analysis UI,
- SQL injection or data corruption in either store.

Out of scope:

- the findings gonemaster reports about a tested domain; they belong to that
  domain's operator,
- denial of service, load testing, and traffic beyond a deployment's rate
  limits,
- a deployment that someone else operates; report to its operator,
- the version string, which the public API publishes,
- missing headers or deviations from best practice without a demonstrated
  impact,
- a vulnerability in a dependency without a demonstrated impact on gonemaster;
  report it to that project,
- social engineering and physical attacks.

## Testing rules

- Testing SHOULD use a local build or a deployment the reporter operates.
- Testing on a public deployment MUST use domains the reporter controls, MUST
  stay within the rate limits, and MUST NOT access, modify, or delete data
  beyond what demonstrates the issue.
- Details of a vulnerability MUST NOT be published until a fixed release ships
  or 90 days have passed since the report, whichever comes first.

## Response

- Acknowledgement within 7 days.
- An assessment and a planned fix release within 30 days.
- The fix ships in a release with an entry under `### Security` in the
  Changelog, crediting the reporter unless the reporter declines.

## Supported versions

Only the latest release receives security fixes.

## Safe harbor

The maintainer does not pursue legal action against research that follows
this policy in good faith. There is no bug bounty.
