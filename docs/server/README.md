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

## Next Steps

- Configure the server: [configuration.md](configuration.md)
- Choose a database: [database.md](database.md)
- Operate jobs and queues: [operations.md](operations.md)
- Expose public endpoints safely: [public-api-and-proxy.md](public-api-and-proxy.md)
- Tune throughput: [performance.md](performance.md)
- Use the web interfaces: [ui.md](ui.md)
