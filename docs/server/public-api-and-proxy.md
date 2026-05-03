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

The public API uses opaque public IDs for public job lookup and does not expose
internal job or run IDs. Public analysis endpoints are read-only.

Public endpoint groups:

| Path | Purpose |
|---|---|
| `POST /pub/api/v1/jobs` | Submit a public single-domain job. |
| `GET /pub/api/v1/jobs/{public_id}` | Poll public job status. |
| `GET /pub/api/v1/jobs/{public_id}/result` | Fetch a public job result. |
| `GET /pub/api/v1/profiles` | List stored profiles marked public. |
| `GET /pub/api/v1/locales` | List available locales. |
| `GET /pub/api/v1/lookup/{domain}` | Public lookup helper. |
| `GET /pub/api/v1/version` | Public version metadata. |
| `GET /pub/api/v1/info` | Public server info. |
| `GET /pub/api/v1/analysis/*` | Read-only public analysis data. |

Public job creation can use `profile_id` only when the selected stored profile
is marked public. It rejects `profile_overrides` so public users cannot submit
arbitrary resolver profile changes.

## Rate Limiting

> **Required for internet-facing deployments.** Rate limiting is **off by
> default**. Without it, anyone can submit unlimited DNS test jobs from a
> single IP, fill the queue, and starve legitimate users. Turn it on before
> exposing `POST /pub/api/v1/jobs` to the public internet.

```sh
gonemaster-server \
  --public-api-rate-limit-enabled \
  --public-api-rate-limit-max 10 \
  --public-api-rate-limit-window 10m
```

Equivalent JSON config:

```json
"public_api": {
  "rate_limit_enabled": true,
  "rate_limit_max": 10,
  "rate_limit_window": "10m"
}
```

The limiter applies to `POST` requests on `/pub/api/v1/`. Read endpoints are
not throttled - see [Caching](#caching) below for the right tool there.

Client IP is resolved from `RemoteAddr` by default. `X-Forwarded-For` is only
honoured when the request's `RemoteAddr` falls inside one of the CIDRs listed
in `trusted_proxy_cidrs`; from there the chain is walked right-to-left and
the first untrusted hop is taken as the client.

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
By default the public API refuses IPs in loopback / link-local / private /
CGNAT / multicast / broadcast ranges. This stops a public deployment from
being used as an internal-network probe via the engine's outbound DNS.

For private/internal deployments that legitimately need to test such
targets, opt out:

```sh
gonemaster-server --public-api-allow-private-undelegated-ip
```

or set `public_api.allow_private_undelegated_ip: true` in the config file.
The toggle is also exposed live on the admin Settings page.

## Caching

Result reads are idempotent and the public ID is unguessable, so a CDN or
reverse-proxy cache absorbs repeat reads better than rate limiting does.

The application sets `Cache-Control: public, max-age=300` on
`GET /pub/api/v1/jobs/{public_id}/result` (200 responses only). Public
analysis snapshot endpoints already advertise `public, max-age=86400, immutable`
when the snapshot slug is explicit in the path. Configure your reverse proxy
or CDN to honour these headers - e.g. enable `proxy_cache` in nginx or
caching at Caddy / Cloudflare / Fastly.

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
