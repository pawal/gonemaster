# Admin API Authentication

The admin API (`/api/v1/*`) and the admin UI can be protected with bearer
tokens. Authentication is opt-in: with no tokens configured the server runs in
open mode and behaves exactly as before. The public API (`/pub/api/v1/*`), the
public UI and the analysis UI are gated only when `auth.protect_public` is on;
see [Protect the public surfaces](#protect-the-public-surfaces).

## How it works

A token has two forms:

- the **plaintext** token (`gm_...`) - the secret, held by operators and clients
- its **hash** (`sha256:...`) - what the server stores; a hash cannot be reversed,
  so a leaked config file or backup exposes no usable credential

The same token authenticates both API clients (via `Authorization: Bearer`) and
the admin UI (via a session cookie set after you paste the token once). Without
`protect_public`, only `/api/v1/*` is gated; `healthz`, `readyz`, `whoami`, and
`session` stay open.

## Enable auth and create the first token

The first token must come from config, the environment, or a flag - the UI
cannot mint one until you are already logged in.

### Where token hashes live

A token hash is read from one of three sources (a later source wins); see
[configuration.md](configuration.md) for how configuration is loaded:

- **JSON config file** passed with `--config <path>` (for example
  `/etc/gonemaster/config.json`), under an `auth.admin_tokens` array:
  ```json
  { "auth": { "admin_tokens": [ { "label": "laptop", "hash": "sha256:..." } ] } }
  ```
  Edits are picked up on `SIGHUP` / `systemctl reload`, with no restart.
- **Environment** `GONEMASTER_ADMIN_TOKEN_HASHES` - a comma-separated list of
  `label=sha256:...` (or bare `sha256:...`). Restart to apply.
- **Flag** `--admin-token-hashes "label=sha256:..."`. Restart to apply.

The packaged systemd service runs `gonemaster-server` with no `--config` and
reads `/etc/gonemaster/server.env`, so the simplest path there is to set
`GONEMASTER_ADMIN_TOKEN_HASHES` in that env file. To use a JSON config file
instead, add `--config /etc/gonemaster/config.json` to the unit's `ExecStart`.

### Steps

```
# 1. Mint a token. The plaintext is shown once - copy it now.
gonemaster-server auth add-token --label laptop

# 2. Install the printed HASH in one of the locations above. For the packaged
#    env-file install:
echo 'GONEMASTER_ADMIN_TOKEN_HASHES=laptop=sha256:...' >> /etc/gonemaster/server.env
#    (or add it to auth.admin_tokens in your --config JSON file)

# 3. Apply:
systemctl reload gonemaster-server     # config-file edits, no downtime
systemctl restart gonemaster-server    # env or flag edits
```

The server logs a structured `auth token mode` line with a `tokens` count once
tokens are active.

## Log into the admin UI

1. Open the admin UI. In token mode it shows a "paste admin token" screen.
2. Paste the plaintext token and submit. The server sets an HttpOnly session
   cookie and the dashboard appears.
3. Use "Log out" to clear the cookie on that browser.

## Programmatic clients

```
export GONEMASTER_TOKEN=gm_...
gonemaster-client ...                  # sends Authorization: Bearer automatically
# or: gonemaster-client --token gm_... ...
```

Prometheus scraping `/api/v1/metrics` must send the token too
(`authorization` or `bearer_token_file` in the scrape config). `gonemaster-nagios`
runs the engine in-process and needs no token.

## Verify a token

Quick checks against a running server (adjust host, port, and token):

```
# whoami reports the mode without needing a token
curl -s http://localhost:8080/api/v1/whoami
# token mode, not logged in -> {"authenticated":false,"mode":"token"}

# a gated endpoint without a token is rejected
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/api/v1/locales
# -> 401

# the same endpoint with a valid token succeeds
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer gm_..." \
  http://localhost:8080/api/v1/locales
# -> 200
```

`healthz` returns 200 without a token because it is exempt, so test enforcement
against a gated endpoint such as `locales`. To check the cookie path the admin UI
uses, log in and reuse the cookie:

```
curl -s -c cookies.txt -X POST -d '{"token":"gm_..."}' \
  http://localhost:8080/api/v1/session                 # -> 200, stores the cookie
curl -s -o /dev/null -w '%{http_code}\n' -b cookies.txt \
  http://localhost:8080/api/v1/locales                 # -> 200
```

## Add or revoke tokens

- **Add:** mint another token, append its hash to `auth.admin_tokens`, then reload.
- **Revoke:** delete that hash line and reload. It stops working immediately - any
  browser cookie or client still using it gets `401`. There is no session store to
  clear; the cookie simply carries a token the server no longer recognises.

## Session cookie and request origin

The session cookie `gm_admin` holds the plaintext token. It is `HttpOnly` and
`SameSite=Strict`. It is `Secure` when the request arrives over HTTPS, directly
or through a proxy in `trusted_proxy_cidrs` that sends
`X-Forwarded-Proto: https`, and when `public_url` is an `https` URL whose host
equals the host of the request.

Log out clears the cookie in that browser only. The server keeps no session
state: a copy of the cookie stays valid until its token hash is removed, as
described under "Add or revoke tokens".

A request to `/api/v1` with a method other than `GET`, `HEAD` or `OPTIONS`
MUST carry no `Origin` header or the origin of the request itself. The server
answers any other request with `403` and the code `csrf_origin_mismatch`
before it routes the request. Clients that send no `Origin`, such as
`gonemaster-client` and MCP clients, are not affected.

## Protect the public surfaces

`auth.protect_public` puts the public UI (`/public/`), the analysis UI
(`/analysis/`) and the public API (`/pub/api/v1/*`) behind the same admin
tokens. It is off by default and requires token mode: with `protect_public` on
and no admin tokens the server refuses to start, and a reload into that state
is rejected and keeps the previous configuration.

It is read from one of three sources (a later source wins):

- **JSON config file**, next to the token list. Edits are picked up on
  `SIGHUP` / `systemctl reload`:
  ```json
  { "auth": { "admin_tokens": [ { "label": "laptop", "hash": "sha256:..." } ], "protect_public": true } }
  ```
- **Environment** `GONEMASTER_AUTH_PROTECT_PUBLIC=true`. Restart to apply.
- **Flag** `--auth-protect-public`. Restart to apply.

With `protect_public` on:

- A page under `/public/` or `/analysis/` requested without a valid token
  answers `401` with a form that asks for the admin token, at the URL
  requested. Submitting a valid token sets the `gm_admin` session cookie and
  loads that URL again. The form's language follows `?lang=`, then
  `Accept-Language`.
- A static asset under those paths requested without a token answers `401`
  with no form.
- `/pub/api/v1/*` answers `401` without a token, as `/api/v1/*` does. API
  clients send `Authorization: Bearer`. A request with a method other than
  `GET`, `HEAD` or `OPTIONS` MUST pass the same origin check as `/api/v1`.
- A response that passes the check carries `Cache-Control: private` where it
  would carry `public`, so a shared cache does not serve it to another visitor.
- `robots.txt` answers `Disallow: /` without a `Sitemap` line, and
  `sitemap.xml` answers `404`.
- `/`, `/api/v1/*` and the MCP endpoint behave as without the switch.

The form and its stylesheets, `/public/_auth/login.css` and
`/analysis/_auth/login.css`, are the only responses under those paths served
without a token, so a proxy that forwards `/public/*`, `/analysis/*` and
`/pub/api/v1/*` needs no change.

The cookie is the one the admin UI sets. On one host, a login in the admin UI
also opens the public UI and the analysis UI, and the reverse. On a separate
public host the visitor logs in there once. The public UIs have no log out
control: the cookie lasts until the browser session ends, or until "Log out" in
the admin UI on the same host.

```
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/pub/api/v1/version
# -> 401
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer gm_..." \
  http://localhost:8080/pub/api/v1/version
# -> 200
```

## MCP clients

The MCP endpoint at `POST /api/v1/mcp` uses the same tokens and accepts them
as `Authorization: Bearer` only; the admin UI cookie is rejected there. The
stdio bridge `gonemaster-mcp` reads the token from `GONEMASTER_TOKEN`. See
[../mcp/README.md](../mcp/README.md).

## Turn auth off

Empty `auth.admin_tokens` (or unset the environment variable) and turn
`protect_public` off in the same change, then reload or restart. The server
logs an `auth open mode` line and the UI stops asking for a token.

## Troubleshooting

- **Every admin call returns 401:** auth is on and no valid token was sent. Log
  in again in the UI, or check `GONEMASTER_TOKEN` for clients.
- **Startup fails with `protect_public requires admin_tokens`:** `protect_public`
  is on and no token is configured. Add a token hash or turn `protect_public`
  off.
- **Lost the token:** the plaintext cannot be recovered from the hash. Mint a new
  one, add its hash, reload, and optionally drop the old hash.
- **Locked out of the UI:** you still control the config file - mint a fresh
  token, add its hash, reload, and log in with the new plaintext.
- **Keep the config file `0600`.** It stores hashes (not plaintext), but also
  database credentials; protect it regardless.

Until tokens are configured, keep the admin API on a private network or behind a
reverse proxy. See [public-api-and-proxy.md](public-api-and-proxy.md).
