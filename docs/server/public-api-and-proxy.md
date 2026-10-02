# Public API and Reverse Proxy

This page owns the boundary between trusted admin surfaces and public
internet-facing surfaces.

## Admin Surfaces

Keep these private:

- `/`
- `/api/v1/`

They expose full control over jobs, batches, domains, tags, profiles, queue
state, cohorts, settings, and metrics.

## Public Surfaces

These are designed for public exposure:

- `/public/`
- `/analysis/`
- `/pub/api/v1/`

The public API addresses a job by its public ID and does not expose internal
job, run, or batch IDs: public results omit `job_id` and `batch_id`, and the
`exception` argument of an entry omits the server's own address and port. A
public ID is 12 characters from `[A-Za-z0-9]`, drawn from `crypto/rand`, about
71 bits. Earlier IDs have 8 characters, about 47 bits, and resolve unchanged.
Every run has a public ID, admin and batch runs included, so a public ID is a
capability for its result and MUST be shared only with those who may read it.
Public analysis endpoints are read-only.

Public endpoint groups:

| Path | Purpose |
|---|---|
| `POST /pub/api/v1/jobs` | Submit a public single-domain job. |
| `GET /pub/api/v1/jobs/{public_id}` | Poll public job status. |
| `GET /pub/api/v1/jobs/{public_id}/result` | Fetch a public job result. |
| `GET /pub/api/v1/jobs/{public_id}/dnssec-chain` | Fetch the DNSSEC chain summary for a public run. |
| `GET /pub/api/v1/jobs/{public_id}/asn-names` | Fetch the registered holders of the AS numbers in a public result. |
| `GET /pub/api/v1/profiles` | List stored profiles marked public. |
| `GET /pub/api/v1/locales` | List available locales. |
| `GET /pub/api/v1/lookup/{domain}` | Look up the NS names, their addresses, and the DS records of a domain. |
| `GET /pub/api/v1/version` | Public version metadata. |
| `GET /pub/api/v1/info` | Public server info. |
| `GET /pub/api/v1/analysis/*` | Read-only public analysis data. |

Public job creation accepts `profile_id` only for a stored profile marked
public; any other id, like one that does not exist, is answered with
`400 profile_not_found`. It rejects `profile_overrides`, so public users cannot submit
arbitrary resolver profile changes, and it rejects a `min_level` below `INFO`
with `400 invalid_min_level`.

The lookup queries the resolvers in `/etc/resolv.conf`, then 8.8.8.8 and
1.1.1.1, for the NS and DS records of the domain and for the A and AAAA records
of each NS name as an absolute name. It does not consult `/etc/hosts` or search
domains. An address the [query-time guard](#query-time-guard-for-non-global-targets)
refuses is omitted unless `allow_non_global_targets` is on.

## Rate Limiting

> **Required for internet-facing deployments.** Rate limiting is **off by
> default**. Without it, anyone can submit unlimited DNS test jobs from a
> single IP, fill the queue, and starve legitimate users. Turn it on before
> exposing `POST /pub/api/v1/jobs` to the public internet.

```sh
gonemaster-server \
  --public-api-rate-limit-enabled \
  --public-api-rate-limit-max 10 \
  --public-api-rate-limit-get-max 600 \
  --public-api-rate-limit-window 10m
```

Equivalent JSON config:

```json
"public_api": {
  "rate_limit_enabled": true,
  "rate_limit_max": 10,
  "rate_limit_get_max": 600,
  "rate_limit_window": "10m"
}
```

The limiter keeps two budgets per client and window. `rate_limit_max` counts
every `POST` on `/pub/api/v1/`. `rate_limit_get_max` counts the `GET` requests
that resolve a public ID, query DNS, or read a whole snapshot:

- `/pub/api/v1/jobs/{public_id}`, and its `/result`, `/dnssec-chain`, and
  `/asn-names`
- `/public/result/{public_id}`
- `/pub/api/v1/lookup/{domain}`
- `/pub/api/v1/analysis/cohorts/{dataset_tag}/snapshots/{slug}/domains`
- `/pub/api/v1/analysis/cohorts/{dataset_tag}/diff` and `/report`

Other reads are not counted; see [Caching](#caching). A client is one IPv4
address or one IPv6 /64. A rate limit change on the admin Settings page applies
to the next request.

Client IP is resolved from `RemoteAddr` by default. `X-Forwarded-For` is only
honoured when the request's `RemoteAddr` falls inside one of the CIDRs listed
in `trusted_proxy_cidrs`; from there the chain is walked right-to-left and
the first untrusted hop is taken as the client.

`X-Forwarded-Host` and `X-Forwarded-Proto` follow the same rule: all three
headers are removed from requests that do not come from a trusted proxy. With
`public_url` unset the server builds its canonical URL from those headers, and
that URL appears in `robots.txt`, `sitemap.xml`, and the `canonical` and
`og:url` tags of cached result pages. Set `public_url` to skip auto-detection.

### What goes in `trusted_proxy_cidrs`

The IP address that **gonemaster-server sees** when the reverse proxy connects
to it - i.e. the proxy's address at gonemaster-server's network vantage point.

| Setup | Value |
|---|---|
| nginx/Caddy on the same host, proxying to `127.0.0.1:8080` | `127.0.0.1/32` (and `::1/128` if also via IPv6) |
| Reverse proxy on another host in `10.0.0.0/8` | `10.0.0.5/32` (the proxy's IP), or the wider `10.0.0.0/8` if you trust the whole network |
| Behind a CDN that connects directly | the CDN's published edge ranges |
| Server exposed directly to the public internet (no proxy) | leave the list empty |

> **Set `trusted_proxy_cidrs` when running behind a reverse proxy.** Without
> it, every forwarded request is attributed to the proxy's IP and a single
> proxy fills the per-IP budget for all real clients. The same setting also
> lets the CSRF check honour `X-Forwarded-Proto: https` from the proxy - without
> it, browser POSTs from `https://your-domain` are rejected with 403
> `csrf_origin_mismatch` because gonemaster sees plain HTTP and assumes
> port 80. List only proxies you control; with it set too broadly,
> `X-Forwarded-*` headers become spoofable.

```sh
gonemaster-server --trusted-proxy-cidrs "127.0.0.1/32,::1/128"
```

or via the config file:

```json
"trusted_proxy_cidrs": ["127.0.0.1/32", "::1/128"]
```

When the server is exposed directly (no reverse proxy), leave the list empty
- `X-Forwarded-For` is then ignored and unspoofable.

Blocked requests return `429 Too Many Requests` with `Retry-After`.

## Undelegated Nameserver IPs

`POST /pub/api/v1/jobs` accepts `nameservers[].ip` for undelegated test mode.
By default the public API refuses, with `400 private_undelegated_ip`, every IP
the [query-time guard](#query-time-guard-for-non-global-targets) refuses. This
stops a public deployment from being used as an internal-network probe via the
engine's outbound DNS.

For private/internal deployments that legitimately need to test such
targets, opt out:

```sh
gonemaster-server --public-api-allow-private-undelegated-ip
```

or set `public_api.allow_private_undelegated_ip: true` in the config file.
The toggle is also exposed live on the admin Settings page.

## Query-time guard for non-global targets

The check above validates only the IPs a caller *submits*. It cannot see
addresses the engine learns later from parent glue or from resolving a
nameserver name. A second, deeper guard runs at query time: the engine refuses
to send a DNS query or a zone transfer to an address that is not globally
reachable, and emits a `NON_GLOBAL_QUERY_BLOCKED` notice instead. An address is
not globally reachable when it is in an IANA special-purpose range not marked
globally reachable (loopback, RFC 1918, CGNAT, link-local, ULA, documentation,
benchmarking, and similar), when it is multicast, or when it is a NAT64
well-known prefix address (`64:ff9b::/96`, RFC 6052) or an IPv4-compatible
address (`::a.b.c.d`) whose IPv4 address is not globally reachable. A blocked
query reads no answer from the cache that concurrent and recent runs share. The
same guard applies to the TCP connection to each RIPE whois source of the
profile `asn_db`: a source that resolves only to such addresses is skipped.

This guard is on by default and clamped for every public job, so a
caller-selected profile cannot relax it. Operator-pinned undelegated IPs
(accepted per the section above) are exempt. To permit non-global query targets
on a private/internal deployment:

```sh
gonemaster-server --public-api-allow-non-global-targets
```

or set `public_api.allow_non_global_targets: true` (also on the admin Settings
page). A public instance that runs private undelegated tests must enable both
`allow_private_undelegated_ip` (to accept the input) and
`allow_non_global_targets` (to permit the query).

## Caching

Result reads are idempotent, so a CDN or reverse-proxy cache absorbs repeat
reads better than rate limiting does.

The application sets `Cache-Control: public, max-age=300` on
`GET /pub/api/v1/jobs/{public_id}/result` and, on hits, on
`GET /pub/api/v1/jobs/{public_id}/dnssec-chain` (200 responses only), and
`public, max-age=86400` on a complete `GET /pub/api/v1/jobs/{public_id}/asn-names`
response. Public analysis snapshot endpoints already advertise `public, max-age=86400, immutable`
when the snapshot slug is explicit in the path. Configure your reverse proxy
or CDN to honour these headers - e.g. enable `proxy_cache` in nginx or
caching at Caddy / Cloudflare / Fastly.

## DNSSEC Chain Summary

`GET /pub/api/v1/jobs/{public_id}/dnssec-chain` serves a structural summary of
the chain of trust, extracted at run end from cached responses (no extra
queries) and stored only for public-UI runs. The `show_dnssec_chain_public`
flag (default `true`) gates it: when off or the id is unknown it returns `404`
`not_found`; a run without chain data returns `404` `no_chain_data` (uncached).
The result payload's `has_dnssec_chain` marker tells the UI when to fetch it.

The document carries its own `version`. Version 2 adds `servers_stale` on both
`parent` and `child` (the servers that answered with a signature outside its
validity window) and captures NSEC and NSEC3 signatures alongside SOA, CDS and
CDNSKEY. The roll-up `status` is `partial` when the validated path holds but
some servers serve expired or not-yet-valid signatures, so resolvers reaching
those servers may still fail. Documents stored before this change stay at
version 1 and have neither field.

Version 3 adds `ns_names`, the in-domain nameserver names of the zone with what
the run concluded about the signatures on their address records. Each entry
carries `name`, `status`, the `signer` observed, and the `servers` that showed
that status. `status` is one of `validates`, `insecure`, `unsigned`, `orphan`,
`chain_broken`, `rrsig_expired`, `rrsig_invalid` or `indeterminate`; where a
name was seen differently on different servers, the worst status is reported.
The section is absent where the run reached no such conclusion, which is not
the same as the zone having no in-domain nameserver name. The roll-up `status`
describes the chain of the zone itself and does not change when a name in
`ns_names` is bogus. A `servers` entry may name a nameserver of a zone
delegated below the apex, where names in that zone are evaluated.

Version 3 also adds the roll-up status `undelegated`, a signed zone whose
parent proves under signature that no delegation exists at the name. It is
distinct from `island`, where the parent is silent about the zone.

## AS Holder Names

`GET /pub/api/v1/jobs/{public_id}/asn-names` returns the registered holder of
each AS number that a stored result names in an `asn` or `asns` argument. The
server resolves each holder with a `TXT` query for `AS<n>.<source>` over the
Cymru sources of the effective profile's `asn_db`, which by default reach Team
Cymru's IP to ASN mapping service. It queries only AS numbers the result names.

```json
{
  "complete": true,
  "asns": [
    {
      "asn": 199973,
      "name": "Migrationsverket",
      "handle": "MIGR-AS",
      "country": "SE",
      "label": "MIGR-AS - Migrationsverket, SE"
    }
  ]
}
```

- `asns` holds at most 32 entries, in ascending AS number order. An AS number
  without a label is omitted.
- `label` is the registry label with control characters removed, at most 255
  characters. `name`, `handle` and `country` are parsed from it; `handle` and
  `country` are empty when the label carries none.
- `complete` is `false` when a lookup was still pending after 3 seconds. The
  pending lookups continue and fill the cache. A complete response carries
  `Cache-Control: public, max-age=86400`; an incomplete one carries `no-store`.

The server caches a label in memory for 30 days, an empty answer for 6 hours
and a failed lookup for 5 minutes. A lookup that exceeds 10 seconds is
abandoned and not cached.

The endpoint returns `404` `not_found` when `show_asn_names_public` is off,
when the server has no ASN resolver, when the public ID is unknown, or when the
job has no stored result. `GET /pub/api/v1/info` reports
`show_asn_names_public` as `true` only when the setting is on and the resolver
is available.

## Reverse Proxy

Configure the proxy so public paths are reachable and admin paths are blocked
or protected by authentication.

Public paths:

- `/public/`
- `/analysis/`
- `/pub/api/v1/`
- `/robots.txt`
- `/sitemap.xml`

The server enforces public and admin API separation internally, but the proxy
should still block admin paths from the public internet.

## nginx Example

Pair with `--trusted-proxy-cidrs 127.0.0.1/32` (or `::1/128` if proxying via
IPv6) so gonemaster-server honours the `X-Forwarded-For` header nginx sets
below.

```nginx
server {
    listen 443 ssl;
    server_name dns.example.com;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    location /public/ {
        proxy_pass http://127.0.0.1:8080/public/;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /analysis/ {
        proxy_pass http://127.0.0.1:8080/analysis/;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /pub/api/v1/ {
        proxy_pass http://127.0.0.1:8080/pub/api/v1/;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

`Strict-Transport-Security` belongs at the TLS-terminating proxy. Other common
security headers are set by the application.

When the MCP endpoint at `/api/v1/mcp` is exposed through the proxy, its
location needs unbuffered event streams and a long read timeout; see
[../mcp/server-endpoint.md](../mcp/server-endpoint.md#reverse-proxy) for the
nginx and Caddy blocks.

## Caddy Example

Caddy's `reverse_proxy` sets `X-Forwarded-For` automatically. Pair with
`--trusted-proxy-cidrs 127.0.0.1/32,::1/128` so gonemaster-server honours it.

```caddyfile
dns.example.com {
    reverse_proxy /public/* localhost:8080
    reverse_proxy /analysis/* localhost:8080
    reverse_proxy /pub/api/v1/* localhost:8080
}
```
