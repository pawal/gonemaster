# Migration Guide: From Zonemaster

This guide is for operators moving to gonemaster from a Zonemaster deployment,
whether that is CLI use, Backend integrations, or batch pipelines. It maps
commands, profiles, API calls, and output formats onto their gonemaster
equivalents.

For whether to migrate at all, see [Why gonemaster](why.md), in particular
[when not to use gonemaster](why.md#when-not-to-use-gonemaster). If a workflow
depends on the Zonemaster GUI or on the Backend JSON-RPC API as such, neither
has a direct replacement here; this guide describes porting, not substitution.

Scope:

- `zonemaster-cli` invocations and flags
- profile files
- Backend JSON-RPC integrations
- JSON result consumers
- monitoring wrappers

## What Stays the Same

- Testcase identity. gonemaster implements 73 of upstream's 74 testcase
  specifications under their upstream IDs; the remaining one, `dnssec12`, is a
  placeholder upstream as well. Ten further testcases have no upstream
  counterpart. See the
  [upstream testcase matrix](specifications/upstream-testcase-matrix.md).
- Message tags. Shared testcases keep upstream's tag identifiers, so results
  from the two engines can be diffed tag by tag.
- Severity levels. The ladder is unchanged: DEBUG3, DEBUG2, DEBUG, INFO,
  NOTICE, WARNING, ERROR, CRITICAL. The default reporting level is NOTICE in
  both.
- Undelegated input syntax. `--ns NAME/IP` and
  `--ds KEYTAG,ALGORITHM,DIGTYPE,DIGEST` are accepted unchanged.
- Translated output, selected with `--locale`.

## What Changes

- The Backend JSON-RPC API and the GUI. `gonemaster-server` exposes a REST API
  ([openapi.yaml](openapi.yaml)) and embeds its own web UIs.
- Stored test history. There is no importer for a Backend database; keep the
  old database readable for as long as its history matters.
- Saved DNS caches. `--save` and `--restore` exist in both, but the file
  formats are unrelated. Record new caches with gonemaster
  ([cache format](cli/cache-format.md)).
- Profile files. The concepts map, but the schema differs in parts; port the
  overrides, not the file (see [Profiles](#profiles)).
- Custom root hints. There is no `--hints` equivalent; the root hints are
  built in.

## CLI

`zonemaster-cli DOMAIN` becomes `gonemaster DOMAIN`. Both normalize IDN input
to IDNA A-labels.

| zonemaster-cli | gonemaster | Notes |
| --- | --- | --- |
| `--test MODULE` | `--module MODULE` | One module. |
| `--test TESTCASE` | `--testcase TESTCASE` | Repeatable; names are case insensitive. Upstream set modifiers are not supported. |
| `--level LEVEL` | `--min-level LEVEL` | Same level names; default NOTICE in both. |
| `--stop-level LEVEL` | `--stop-level LEVEL` | Unchanged. |
| `--locale LOCALE` | `--locale LOCALE` | Unchanged. |
| `--json` | `--json` | Argument keys and shapes differ; see [Results and Consumers](#results-and-consumers). |
| `--json-stream` | `--json-stream` | Same shape difference. |
| `--json-translate` | none | JSON output carries `tag` and `args` only; use human output for rendered messages. |
| `--raw` | `--raw` | Unchanged. |
| `--count` | `--count` | Human output only. |
| `--nstimes` | `--nstimes` | With `--json`, entries and timings are wrapped in one object. |
| `--progress` / `--no-progress` | `--no-progress` | Progress is on by default when stdout is a terminal. |
| `--ipv4` / `--no-ipv4` | `--no-ipv4` | IPv4 is on by default. |
| `--ipv6` / `--no-ipv6` | `--no-ipv6`, `--ipv6` | IPv6 is on by default. |
| `--sourceaddr4`, `--sourceaddr6` | `--sourceaddr4`, `--sourceaddr6` | Unchanged. |
| `--ns NAME/IP` | `--ns NAME/IP` | Unchanged. |
| `--ds KEYTAG,ALGORITHM,DIGTYPE,DIGEST` | `--ds KEYTAG,ALGORITHM,DIGTYPE,DIGEST` | Unchanged. |
| `--profile FILE` | `--profile PATH` | JSON or YAML; see [Profiles](#profiles). |
| `--save FILE`, `--restore FILE` | `--save PATH`, `--restore PATH` | Different file formats; not interchangeable. |
| `--list-tests` | `--list-tests` | Unchanged. |
| `--dump-profile` | `--dump-profile` | Unchanged. |
| `--version` | `--version` | Unchanged. |
| `--elapsed`, `--time`, `--show-level`, `--show-module`, `--show-testcase` | none | Human output has a fixed format. |
| `--encoding` | none | Output is UTF-8. |
| `--hints FILE` | none | Root hints are built in. |

Flags without an upstream counterpart include resolver overrides
(`--parallel`, `--timeout`, `--retry`, `--retrans`, `--fallback`), scoring
(`--score`, `--scoring-config`), `--output`, cache inspection
(`--cache-stats`), and the badkeys blocklist tools. See the
[CLI reference](cli/README.md).

## Profiles

Both engines start from a default profile and apply sparse overrides on top.
The shared concepts keep their keys: `net.ipv4`, `net.ipv6`, `test_cases`,
`test_levels`, and `resolver.defaults` with `timeout`, `retry`, `retrans`,
and `fallback`.

Differences:

- `resolver.defaults.igntc`, `recurse`, and `usevc` are ignored; the engine
  and the individual testcases decide the transport for each query.
- gonemaster adds resolver keys with no upstream equivalent: `parallel`,
  `unordered`, response caching, and bounds on slow nameservers. See the
  [profile settings reference](profile-settings.md).
- Thresholds for individual testcases live in `test_cases_vars`.
- Profiles may be YAML as well as JSON.

Port a profile by writing your overrides on top of the gonemaster default
rather than copying the file: run `gonemaster --dump-profile` for the default,
write only the keys you changed, and verify with
`gonemaster --dump-profile --profile PATH`.

## Server and API

| Zonemaster component | gonemaster equivalent |
| --- | --- |
| Engine (Perl library) | `engine` Go package ([dev.md](dev.md)) |
| CLI | `gonemaster` |
| Backend (RPCAPI and testagent) | `gonemaster-server`, with queue and workers built in |
| Backend database | SQLite, PostgreSQL, or MariaDB ([database setup](server/database-setup.md)) |
| GUI | embedded admin, public, and analysis UIs ([server/ui.md](server/ui.md)) |
| Scripted Backend clients | `gonemaster-client` ([client/](client/README.md)) |

There is no JSON-RPC compatibility layer. The REST contract is
[openapi.yaml](openapi.yaml); conventions are described in
[specifications/api.md](specifications/api.md). Public endpoints use opaque
public IDs and are meant to be exposed; admin endpoints are for trusted
clients only ([public API and proxy](server/public-api-and-proxy.md)).

Method mapping:

| Backend method | gonemaster endpoint |
| --- | --- |
| `version_info` | `GET /pub/api/v1/version` |
| `profile_names` | `GET /pub/api/v1/profiles` |
| `start_domain_test` | `POST /pub/api/v1/jobs` (public) or `POST /api/v1/jobs` (admin) |
| `test_progress` | `GET /pub/api/v1/jobs/{public_id}` or `GET /api/v1/jobs/{job_id}` |
| `get_test_results` | `GET /pub/api/v1/jobs/{public_id}/result` or `GET /api/v1/jobs/{job_id}/result`; accepts `locale` |
| `get_test_history` | `GET /api/v1/domains/{id}/runs` (admin only) |
| `get_data_from_parent_zone` | `GET /pub/api/v1/lookup/{domain}` |
| `add_batch_job` | `POST /api/v1/jobs/batch` |
| `get_batch_job_result` | `GET /api/v1/batches/{batch_id}` |
| `add_api_user` | none; see [authentication](server/authentication.md) |

Job submission accepts undelegated `nameservers` and `ds_info` for single
jobs; batches reject undelegated input.

## Results and Consumers

Each log entry is an object with `timestamp`, `module`, `testcase`, `tag`,
`level`, and `args`.

- Argument shapes are typed. Upstream packs identities into strings: `ns` as
  `"name/ip"`, lists joined with semicolons. gonemaster splits them into
  typed fields such as `ns` plus `address` and arrays of objects. The key
  mapping in [MIGRATION-1.1.md](MIGRATION-1.1.md) largely matches upstream's
  shapes, so the same table applies when porting a consumer.
- JSON output has no translated message text; render from `tag` and `args`,
  or use human output with `--locale`.
- Expect tags upstream never emits: the twelve testcases without an upstream
  counterpart report findings of their own, and shared testcases with
  documented divergences are marked in the
  [upstream testcase matrix](specifications/upstream-testcase-matrix.md).
- Scores and letter grades are gonemaster additions with no upstream
  counterpart ([scoring](scoring.md)).

## Monitoring

`gonemaster-nagios` replaces wrappers around `zonemaster-cli`: it runs a full
test and maps the worst finding severity to a Nagios or Icinga service state.
See [nagios.md](nagios.md).

## Migration Checklist

1. Install: `go install codeberg.org/pawal/gonemaster/cmd/gonemaster@latest`,
   or a release binary.
2. Run a familiar set of domains through both engines and diff the tags.
   Expect the ten extra testcases and the divergences documented in the
   matrix; everything else should agree.
3. Port profile overrides onto the gonemaster default; verify with
   `--dump-profile`.
4. Update scripts to the flag mapping above.
5. Port Backend integrations to the REST API or `gonemaster-client`.
6. Update JSON consumers using [MIGRATION-1.1.md](MIGRATION-1.1.md).
7. Save new DNS caches where reproducible runs are needed.
8. Keep the Backend database readable for as long as its history matters;
   runs do not import.

## References

- [Why gonemaster](why.md)
- [CLI reference](cli/README.md)
- [Server documentation](server/README.md)
- [Client documentation](client/README.md)
- [API conventions](specifications/api.md) and [openapi.yaml](openapi.yaml)
- [Log args migration guide](MIGRATION-1.1.md)
- [Upstream testcase matrix](specifications/upstream-testcase-matrix.md)
- [Scoring](scoring.md)
