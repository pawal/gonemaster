# Gonemaster Server

`gonemaster-server` runs DNS tests through an HTTP API, stores results, and
serves the admin and public web interfaces.

## Surfaces

| Path | Audience | Purpose |
|---|---|---|
| `/` | Trusted operators | Admin UI for jobs, batches, domains, tags, cohorts, and settings. |
| `/api/v1/` | Trusted clients | Full admin API used by the admin UI, `gonemaster-client`, and scripts. |
| `/public/` | Public users | Single-domain public test UI. |
| `/analysis/` | Public users | Read-only public cohort analysis UI. |
| `/pub/api/v1/` | Public clients | Restricted API for public jobs, public profiles, and analysis views. |

Do not expose `/` or `/api/v1/` directly to untrusted clients. The public
surfaces are designed for internet exposure when rate limiting and reverse
proxy rules are configured.

## Lifecycle

Single-domain submissions create jobs. Batch submissions create one job per
domain and a batch record that groups them. Workers move jobs through:

```text
queued -> running -> succeeded | failed | canceled | expired
```

Completed jobs graduate into immutable `runs` and `entries` rows. Domain rows
store a denormalized latest result so common filters do not need to scan all
historical runs.

## Build

Build the server with the embedded UI:

```sh
go build -o ./gonemaster-server ./cmd/gonemaster-server
```

Rebuild embedded UI assets before a normal UI-enabled build:

```sh
make ui-build
```

Build an API-only binary when `npm` is not available:

```sh
make build-gonemaster-server-noui
```

or:

```sh
go build -tags nogui -o ./gonemaster-server ./cmd/gonemaster-server
```

With the `nogui` tag, API routes are unchanged. UI routes return a short
informational page or `404 ui not available`.

## Quick Start

Start the server:

```sh
./gonemaster-server
```

Submit a job:

```sh
JOB_ID=$(curl -s http://localhost:8080/api/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{"domain":"example.com"}' | jq -r .id)
```

Poll status:

```sh
while true; do
  STATUS=$(curl -s "http://localhost:8080/api/v1/jobs/$JOB_ID" | jq -r .status)
  echo "status=$STATUS"
  case "$STATUS" in
    succeeded|failed|canceled|expired) break ;;
  esac
  sleep 2
done
```

Fetch the result:

```sh
curl -s "http://localhost:8080/api/v1/jobs/$JOB_ID/result?locale=en" | jq .
```

For shell automation, prefer [../client/](../client/README.md) over hand-written
`curl` loops.

## Next Steps

- Install from Linux packages (deb/rpm): [install.md](install.md)
- Configure the server: [configuration.md](configuration.md)
- Choose a database: [database.md](database.md)
- Operate jobs and queues: [operations.md](operations.md)
- Expose public endpoints safely: [public-api-and-proxy.md](public-api-and-proxy.md)
- Tune throughput: [performance.md](performance.md)
- Use the web interfaces: [ui.md](ui.md)
