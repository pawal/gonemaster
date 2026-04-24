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

Public job submission can be rate limited per client IP. Enable it before
exposing public job creation to the internet.

```sh
gonemaster-server \
  --public-api-rate-limit-enabled \
  --public-api-rate-limit-max 10 \
  --public-api-rate-limit-window 10m
```

Client IP is resolved from:

1. `X-Forwarded-For`, first value
2. `X-Real-IP`
3. `RemoteAddr`

Blocked requests return `429 Too Many Requests` with `Retry-After`.

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
