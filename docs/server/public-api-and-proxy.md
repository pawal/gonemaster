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
not throttled — see [Caching](#caching) below for the right tool there.

Client IP is resolved from:

1. `X-Forwarded-For`, first value
2. `X-Real-IP`
3. `RemoteAddr`

> **Trust your proxy.** The first `X-Forwarded-For` value is trusted
> unconditionally. If the server is exposed directly (no reverse proxy) or the
> proxy does not strip incoming `X-Forwarded-For`, an attacker can rotate the
> header to bypass the per-IP budget. Always front the server with a proxy
> that overwrites these headers.

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
or CDN to honour these headers — e.g. enable `proxy_cache` in nginx or
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

```nginx
server {
    listen 443 ssl;
    server_name dns.example.com;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    location /public/ {
        proxy_pass http://127.0.0.1:8080/public/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /analysis/ {
        proxy_pass http://127.0.0.1:8080/analysis/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /pub/api/v1/ {
        proxy_pass http://127.0.0.1:8080/pub/api/v1/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

`Strict-Transport-Security` belongs at the TLS-terminating proxy. Other common
security headers are set by the application.

## Caddy Example

```caddyfile
dns.example.com {
    reverse_proxy /public/* localhost:8080
    reverse_proxy /analysis/* localhost:8080
    reverse_proxy /pub/api/v1/* localhost:8080
}
```
