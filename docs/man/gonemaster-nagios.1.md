# gonemaster-nagios 1 "2026" "gonemaster" "User Commands"

## NAME

gonemaster-nagios - Nagios/Icinga plugin for DNS zone testing

## SYNOPSIS

**gonemaster-nagios** **-H** *DOMAIN* [*OPTIONS*]

## DESCRIPTION

**gonemaster-nagios** wraps the gonemaster engine as a Nagios-compatible plugin.
It maps DNS test severity levels to Nagios exit codes, making it suitable for
use with Nagios, Icinga, Sensu, and similar monitoring systems.

## OPTIONS

**-H**, **--hostname** *DOMAIN*
: Zone name to test (preferred).

**-d**, **--domain** *DOMAIN*
: DNS-specific alias for the zone name.

**-w**, **--warning** *LEVEL*
: Highest Gonemaster severity that should map to Nagios WARNING. Default:
  **WARNING**.

**-c**, **--critical** *LEVEL*
: Highest Gonemaster severity that should map to Nagios CRITICAL. Default:
  **ERROR**.

**-t**, **--timeout** *SECONDS*
: Plugin runtime deadline in seconds. A timeout returns **UNKNOWN**.

**--module** *MODULE*
: Run only the named module.

**--testcase** *TESTCASE*
: Run only the named testcase.

**--profile** *PATH*
: Load a custom profile from a JSON or YAML file.

**--no-ipv4**
: Disable IPv4 queries.

**--no-ipv6**
: Disable IPv6 queries.

**--force-ipv6**
: Force IPv6 queries.

**--source-addr4** *IPADDR*
: Source IPv4 address for outgoing queries.

**--source-addr6** *IPADDR*
: Source IPv6 address for outgoing queries.

**-v**, **--verbose**
: Increase output verbosity (use **-v**, **-vv**, or **-vvv**).

**-V**, **--version**
: Print version and exit.

**-h**, **--help**
: Show help and exit.

## COMPATIBILITY ALIASES

**--ipv6**
: Alias for **--force-ipv6**.

**--disable-ipv4**
: Alias for **--no-ipv4**.

**--disable-ipv6**
: Alias for **--no-ipv6**.

**--sourceaddr4**
: Alias for **--source-addr4**.

**--sourceaddr6**
: Alias for **--source-addr6**.

## EXIT STATUS

**0** (OK)
: Highest severity stayed below **--warning**.

**1** (WARNING)
: Highest severity reached **--warning** but stayed below **--critical**.

**2** (CRITICAL)
: Highest severity reached **--critical**.

**3** (UNKNOWN)
: Runtime error, timeout, or missing/invalid required options.

## VERBOSITY LEVELS

**-v**
: Show WARNING, ERROR, and CRITICAL results.

**-vv**
: Also show NOTICE results.

**-vvv**
: Also show INFO results.

## SEVERITY VALUES

Valid threshold values for **--warning** and **--critical** are:
**DEBUG3**, **DEBUG2**, **DEBUG**, **INFO**, **NOTICE**, **WARNING**,
**ERROR**, and **CRITICAL**.

## EXAMPLES

Basic Nagios check:

    gonemaster-nagios -H example.com

Check with verbose output for debugging:

    gonemaster-nagios -H example.com -vvv

Check with explicit Nagios thresholds and timeout:

    gonemaster-nagios -H example.com -w WARNING -c ERROR -t 15

Run only DNSSEC checks:

    gonemaster-nagios --domain example.com --module dnssec

Nagios command definition:

    define command {
        command_name    check_dns_zone
        command_line    /usr/local/bin/gonemaster-nagios -H $ARG1$ -w WARNING -c ERROR -t 15 -v
    }

Icinga service example:

    apply Service "dns-zone" {
        check_command = "check_dns_zone"
        vars.zone = host.vars.dns_zone
    }

## SEE ALSO

**gonemaster**(1), **gonemaster-server**(1), **gonemaster-client**(1)
