# gonemaster-nagios 1 "2026" "gonemaster" "User Commands"

## NAME

gonemaster-nagios - Nagios/Icinga plugin for DNS zone testing

## SYNOPSIS

**gonemaster-nagios** **-d** *DOMAIN* [*OPTIONS*]

## DESCRIPTION

**gonemaster-nagios** wraps the gonemaster engine as a Nagios-compatible plugin.
It maps DNS test severity levels to Nagios exit codes, making it suitable for
use with Nagios, Icinga, Sensu, and similar monitoring systems.

## OPTIONS

**-d**, **--domain** *DOMAIN*
: Zone name to test (required).

**--module** *MODULE*
: Run only the named module.

**--testcase** *TESTCASE*
: Run only the named testcase.

**--profile** *PATH*
: Load a custom profile from a JSON or YAML file.

**--no-ipv4**, **--disable-ipv4**
: Disable IPv4 queries.

**--no-ipv6**, **--disable-ipv6**
: Disable IPv6 queries.

**--ipv6**
: Force IPv6 queries.

**--sourceaddr4** *IPADDR*
: Source IPv4 address for outgoing queries.

**--sourceaddr6** *IPADDR*
: Source IPv6 address for outgoing queries.

**-v**, **--verbose**
: Increase output verbosity (use **-v**, **-vv**, or **-vvv**).

**-V**, **--version**
: Print version and exit.

## EXIT STATUS

**0** (OK)
: Highest severity was INFO, NOTICE, or lower.

**1** (WARNING)
: Highest severity was WARNING.

**2** (CRITICAL)
: Highest severity was ERROR or CRITICAL.

**3** (UNKNOWN)
: Runtime error or missing required options.

## VERBOSITY LEVELS

**-v**
: Show WARNING, ERROR, and CRITICAL results.

**-vv**
: Also show NOTICE results.

**-vvv**
: Also show INFO results.

## EXAMPLES

Basic Nagios check:

    gonemaster-nagios -d example.com

Check with verbose output for debugging:

    gonemaster-nagios -d example.com -vvv

Run only DNSSEC checks:

    gonemaster-nagios -d example.com --module dnssec

Nagios command definition:

    define command {
        command_name    check_dns_zone
        command_line    /usr/local/bin/gonemaster-nagios -d $ARG1$ -v
    }

Icinga service example:

    apply Service "dns-zone" {
        check_command = "check_dns_zone"
        vars.zone = host.vars.dns_zone
    }

## SEE ALSO

**gonemaster**(1), **gonemaster-server**(1), **gonemaster-client**(1)
