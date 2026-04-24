# Server Configuration

This page owns the `gonemaster-server` configuration model. The older
[../server.md](../server.md) page still contains the full current reference
while the server docs are being split.

## Precedence

Configuration is applied in this order:

1. Command-line flags
2. `GONEMASTER_*` environment variables
3. JSON config file passed with `--config`
4. Built-in defaults

Use environment variables for secrets such as database connection strings.

## Core Settings

| Setting | Purpose |
|---|---|
| `listen_addr` | Address and port for the HTTP listener. |
| `worker_count` | Number of workers that dequeue jobs. |
| `max_concurrent_jobs` | Maximum number of engine runs at once. |
| `min_level` | Minimum log level stored and returned in results. |
| `profile_path` | Default engine profile file. |
| `debug` | Enables more verbose server logging. |

## Profiles

The server has two profile sources:

- A process-wide base profile from the built-in default plus `profile_path`.
- Stored profiles in the database, referenced by jobs, batches, public
  profiles, and tag defaults.

Stored profiles are sparse overrides. They contain only the settings that
differ from the engine default.

## Related Pages

- Database config and DSNs: [database.md](database.md)
- Throughput tuning: [performance.md](performance.md)
- Stored profile endpoints: [../reference/api.md](../reference/api.md)
