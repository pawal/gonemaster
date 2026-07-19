# Admin API Authentication

The admin API (`/api/v1/*`) and the admin UI can be protected with bearer
tokens. Authentication is opt-in: with no tokens configured the server runs in
open mode and behaves exactly as before. The public API (`/pub/api/v1/*`) and
public UI are never gated by this mechanism.

## How it works

A token has two forms:

- the **plaintext** token (`gm_...`) - the secret, held by operators and clients
- its **hash** (`sha256:...`) - what the server stores; a hash cannot be reversed,
  so a leaked config file or backup exposes no usable credential

The same token authenticates both API clients (via `Authorization: Bearer`) and
the admin UI (via a session cookie set after you paste the token once). Only
`/api/v1/*` is gated; `healthz`, `readyz`, `whoami`, and `session` stay open.

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
2. Paste the plaintext token and submit. The server sets a secure, HttpOnly
   session cookie and the dashboard appears.
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

## Turn auth off

Empty `auth.admin_tokens` (or unset the environment variable) and reload or
restart. The server logs an `auth open mode` line and the UI stops asking for a
token.

## Troubleshooting

- **Every admin call returns 401:** auth is on and no valid token was sent. Log
  in again in the UI, or check `GONEMASTER_TOKEN` for clients.
- **Lost the token:** the plaintext cannot be recovered from the hash. Mint a new
  one, add its hash, reload, and optionally drop the old hash.
- **Locked out of the UI:** you still control the config file - mint a fresh
  token, add its hash, reload, and log in with the new plaintext.
- **Keep the config file `0600`.** It stores hashes (not plaintext), but also
  database credentials; protect it regardless.

Until tokens are configured, keep the admin API on a private network or behind a
reverse proxy. See [public-api-and-proxy.md](public-api-and-proxy.md).
