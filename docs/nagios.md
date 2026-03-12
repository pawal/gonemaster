# Gonemaster Nagios Plugin

`gonemaster-nagios` is a Nagios-compatible wrapper that reports the highest
severity found in a full run.

Gonemaster’s Nagios plugin is useful because it turns DNS delegation testing into a standard operational check. It runs the same delegation analysis as the main tool, but returns the result in a form that monitoring systems can evaluate directly. That makes delegation health testable, repeatable, and suitable for alerting instead of relying on manual verification.

For operations, this means DNS delegation problems can be detected early and handled as a defined service state. The plugin is a practical way to integrate delegation checks into monitoring platforms such as Nagios, Icinga, Naemon, Shinken, or Sensu, while keeping the test logic consistent with the rest of Gonemaster.

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

## Exit codes
- `0` OK
- `1` WARNING
- `2` CRITICAL
- `3` UNKNOWN

Note: the Nagios wrapper uses the same profile settings as `gonemaster`. To
tweak warning levels and performance settings, change the profile and pass
`--profile`.
