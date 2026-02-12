# Gonemaster Nagios Plugin

`gonemaster-nagios` is a Nagios-compatible wrapper that reports the highest
severity found in a full run.

## Install
```
go install codeberg.org/pawal/gonemaster/cmd/gonemaster-nagios@latest
```

Build from source:
```
go build -o gonemaster-nagios ./cmd/gonemaster-nagios
```

## Usage
```
gonemaster-nagios -d example.com
gonemaster-nagios -vv -d example.com
gonemaster-nagios -d example.com --module address
gonemaster-nagios -d example.com --testcase zone09
gonemaster-nagios -d example.com --profile ./profile.json
```

## Verbosity (`-v` repeatable)
- `-v` prints WARNING/ERROR/CRITICAL messages
- `-vv` adds NOTICE messages
- `-vvv` adds INFO messages

## Additional options (Nagios-friendly)
- `--module` Run a single module
- `--testcase` Run a single testcase
- `--profile` Profile JSON/YAML path
- `--no-ipv4` / `--disable-ipv4` Disable IPv4 queries
- `--no-ipv6` / `--disable-ipv6` Disable IPv6 queries
- `--job-test-parallelism N` Parallel testcase runs per domain (`>= 1`)

## Exit codes
- `0` OK
- `1` WARNING
- `2` CRITICAL
- `3` UNKNOWN

Note: the Nagios wrapper uses the same profile settings as `gonemaster`. To
tweak warning levels and performance settings, change the profile and pass
`--profile`.
