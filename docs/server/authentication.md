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

The first token is minted on the command line; the UI cannot create one until
you are already logged in.

```
# 1. Mint a token. The plaintext is shown once - copy it now.
gonemaster-server auth add-token --label laptop

# 2. Put the printed HASH in your config file under auth.admin_tokens:
#    "auth": { "admin_tokens": [ { "label": "laptop", "hash": "sha256:..." } ] }

# 3. Apply without downtime:
systemctl reload gonemaster-server     # packaged / systemd installs
kill -HUP <pid>                        # generic alternative
```

The server logs `auth: token mode, 1 token` once tokens are active.

Deployments without a config file can pass hashes via the environment or flag
and restart to apply (reload re-reads the config file only):

```
GONEMASTER_ADMIN_TOKEN_HASHES=label=sha256:...,sha256:...
gonemaster-server --admin-token-hashes "label=sha256:..."
```

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

## Add or revoke tokens

- **Add:** mint another token, append its hash to `auth.admin_tokens`, then reload.
- **Revoke:** delete that hash line and reload. It stops working immediately - any
  browser cookie or client still using it gets `401`. There is no session store to
  clear; the cookie simply carries a token the server no longer recognises.

## Turn auth off

Empty `auth.admin_tokens` (or unset the environment variable) and reload or
restart. The server logs `auth: open mode` and the UI stops asking for a token.

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
