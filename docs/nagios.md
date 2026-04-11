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
gonemaster-nagios -H example.com
gonemaster-nagios -H example.com -w WARNING -c ERROR -t 15
gonemaster-nagios -H example.com -vv
gonemaster-nagios --domain example.com --module address
gonemaster-nagios --domain example.com --testcase zone09
gonemaster-nagios --domain example.com --profile ./profile.json
```

## Core Nagios-style options
- `-H`, `--hostname` Zone name to test (preferred)
- `-d`, `--domain` DNS-specific alias for the zone name
- `-w`, `--warning` Highest Gonemaster severity that should map to Nagios WARNING. Default: `WARNING`
- `-c`, `--critical` Highest Gonemaster severity that should map to Nagios CRITICAL. Default: `ERROR`
- `-t`, `--timeout` Plugin runtime deadline in seconds. A timeout returns `UNKNOWN`
- Valid severity values: `DEBUG3`, `DEBUG2`, `DEBUG`, `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`

## Gonemaster-specific options
- `--module` Run a single module
- `--testcase` Run a single testcase
- `--profile` Profile JSON/YAML path
- `--no-ipv4` Disable IPv4 queries
- `--no-ipv6` Disable IPv6 queries
- `--force-ipv6` Force IPv6 queries
- `--source-addr4` Override resolver IPv4 source address
- `--source-addr6` Override resolver IPv6 source address
- `--ns` Undelegated nameserver: `name` or `name/ip` (repeatable)
- `--ds` Undelegated DS record: `keytag,algorithm,digtype,digest` (repeatable; requires `--ns`)
- `--rrsig-warn-days` Emit WARNING if any apex RRSIG expires within N days (requires DNSSEC04 or dnssec module)
- `--grade-warning` Grade that triggers Nagios WARNING (e.g. `C`). Valid values: `A+`, `A`, `B`, `C`, `D`, `F`
- `--grade-critical` Grade that triggers Nagios CRITICAL (e.g. `F`). Valid values: `A+`, `A`, `B`, `C`, `D`, `F`

## Undelegated testing

Use `--ns` to test a zone via specific nameservers, bypassing normal DNS
delegation. This is useful for split-DNS environments, pre-delegation checks,
or when public resolution is blocked by a firewall.

```
# Test via internal nameservers with explicit glue:
gonemaster-nagios -H internal.example.com --ns ns1.internal/10.0.0.53 --ns ns2.internal/10.0.0.54

# Test via hostname only (IP resolved normally):
gonemaster-nagios -H example.com --ns ns1.example.com

# Pre-delegation DNSSEC test with DS record:
gonemaster-nagios -H example.com \
    --ns ns1.new-provider.net/203.0.113.1 \
    --ds 12345,13,2,ABCDEF0123456789...
```

`--ds` is only valid together with `--ns`; specifying it alone returns exit code 3.

## Grade-based monitoring

Use `--grade-warning` and `--grade-critical` to trigger Nagios states based on
the computed domain quality grade rather than (or in addition to) raw severity
levels. The grade is derived from the scoring engine using the same weighted
penalty model as the server and CLI.

Valid grade values from best to worst: `A+`, `A`, `B`, `C`, `D`, `F`.

When a grade threshold is set, the plugin computes the score after the run and
appends grade and score information to the output line:

```
ZONE OK - grade A (score 96)
ZONE WARNING - grade C (score 63)
ZONE CRITICAL - grade F (score 8)
```

Grade-based and severity-based checks run simultaneously; the most critical
result determines the exit code.

```
# Warn if grade drops below B, critical if grade is F:
gonemaster-nagios -H example.se --grade-warning B --grade-critical F

# Grade check only — no severity threshold change:
gonemaster-nagios -H example.se --grade-warning C --grade-critical F

# Combine grade check with a stricter severity threshold:
gonemaster-nagios -H example.se \
    --warning WARNING --critical ERROR \
    --grade-warning C --grade-critical F

# Full DNSSEC module check with grade monitoring:
gonemaster-nagios -H example.se --module dnssec \
    --grade-warning B --grade-critical D
```

The `--grade-warning` threshold must be a better grade than `--grade-critical`
(e.g. `--grade-warning C --grade-critical F` is valid; the reverse is not).

Example Icinga2 service with grade-based alerting:

```
apply Service "dns-grade" {
    import "generic-service"
    check_command   = "gonemaster-nagios"
    vars.domain     = host.vars.dns_zone
    vars.grade_warn = "C"
    vars.grade_crit = "F"
    vars.timeout    = 30
    assign where host.vars.dns_zone
}
```

## RRSIG expiry monitoring

Use `--rrsig-warn-days` to trigger a WARNING when any apex RRSIG expires within
N days. This gives early notice when re-signing is broken, long before the
engine's default 12-hour threshold fires.

```
# Alert WARNING if any apex RRSIG expires within 14 days:
gonemaster-nagios -H example.com --testcase dnssec04 --rrsig-warn-days 14

# Full DNSSEC module check + 30-day RRSIG warning:
gonemaster-nagios -H example.com --module dnssec --rrsig-warn-days 30

# Combined with undelegated testing:
gonemaster-nagios -H example.com --ns ns1.example/10.0.0.1 \
    --testcase dnssec04 --rrsig-warn-days 7
```

`--rrsig-warn-days` sets the `REMAINING_SHORT` threshold in the DNSSEC04 profile
variable. Using it without `--testcase dnssec04` or `--module dnssec` prints a
warning but proceeds normally.

Example Icinga2 service:

```
apply Service "dns-rrsig-expiry" {
    import "generic-service"
    check_command   = "gonemaster-nagios"
    vars.domain     = host.vars.dns_zone
    vars.testcase   = "dnssec04"
    vars.rrsig_warn = 14
    vars.timeout    = 30
    assign where host.vars.dns_zone
}
```

## Compatibility aliases
- `--ipv6` Alias for `--force-ipv6`
- `--disable-ipv4` Alias for `--no-ipv4`
- `--disable-ipv6` Alias for `--no-ipv6`
- `--sourceaddr4` Alias for `--source-addr4`
- `--sourceaddr6` Alias for `--source-addr6`

## Verbosity (`-v` repeatable)
- `-v` prints WARNING/ERROR/CRITICAL messages
- `-vv` adds NOTICE messages
- `-vvv` adds INFO messages

## Exit codes
- `0` OK, highest severity stayed below `--warning`
- `1` WARNING, highest severity reached `--warning` but stayed below `--critical`
- `2` CRITICAL, highest severity reached `--critical`
- `3` UNKNOWN, runtime error, timeout, or invalid required options

Note: `--warning` and `--critical` only control how the plugin maps Gonemaster
severity to Nagios states. To change Gonemaster's own severity assignments,
resolver settings, or performance behavior, adjust the profile and pass
`--profile`.
