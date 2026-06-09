# gonemaster 1 "2026" "gonemaster" "User Commands"

## NAME

gonemaster - DNS zone testing engine

## SYNOPSIS

**gonemaster** [*OPTIONS*] *DOMAIN*

## DESCRIPTION

**gonemaster** runs a comprehensive suite of DNS tests against a domain zone,
checking delegation, DNSSEC, nameserver behavior, zone configuration, and more.
Results are printed with severity levels and can be output in several formats.

## OPTIONS

### Target

**--domain** *DOMAIN*
: Zone name to test (also accepted as a positional argument).

**--module** *MODULE*
: Run only the named module (e.g., dnssec, nameserver).

**--testcase** *TESTCASE*
: Run only the named testcase (e.g., dnssec20). May be repeated to run several
  testcases, optionally across modules: `--testcase consistency04 --testcase
  delegation07`. Names are case-insensitive.

**--profile** *PATH*
: Load a custom profile from a JSON or YAML file.

### Output

**--min-level** *LEVEL*
: Minimum severity level to display (default: NOTICE).

**--stop-level** *LEVEL*
: Stop after the first entry at or above this level.

**--locale** *LOCALE*
: Locale for translated output messages.

**--output** *PATH*
: Write output to a file instead of stdout.

**--raw**
: Stream raw log entries.

**--json**
: Print results as a JSON array.

**--json-stream**
: Stream results as newline-delimited JSON.

**--count**
: Print a summary count by level and message tag.

**--nstimes**
: Print per-nameserver query timing statistics (max, min, avg, stddev, median, total, count),
  sorted by nameserver name, address, then median query time. When combined with **--json**,
  the output is wrapped as a JSON object with keys **entries** and **nameserver_timings**
  instead of a bare array.

**--no-progress**
: Disable the progress indicator.

### Resolver

**--no-ipv4**
: Disable IPv4 queries.

**--no-ipv6**
: Disable IPv6 queries.

**--ipv6**
: Force IPv6 queries.

**--parallel** *N*
: Number of parallel queries per nameserver.

**--timeout** *SECONDS*
: Query timeout in seconds.

**--retry** *N*
: Number of query retries.

**--retrans** *SECONDS*
: Retransmission interval in seconds.

**--fallback**
: Enable TCP fallback on UDP failure.

**--no-fallback**
: Disable TCP fallback on UDP failure.

**--sourceaddr4** *IPADDR*
: Source IPv4 address for outgoing queries.

**--sourceaddr6** *IPADDR*
: Source IPv6 address for outgoing queries.

### Cache

**--save** *PATH*
: Write DNS packet cache to file after the run. If *PATH* ends in `.gz` the file is gzip-compressed automatically.

**--save-compress**
: Gzip-compress the saved cache file. Implied when *PATH* ends in `.gz`. Has no effect without **--save**.

**--restore** *PATH*
: Prime DNS packet cache from file before the run. A gzip-compressed file is decompressed transparently (detected by magic bytes, regardless of file name).

**--error-cache-ttl** *SECONDS*
: Skip query retry after network errors for this duration.

**--positive-cache-ttl** *SECONDS*
: Cache positive DNS responses for this duration.

**--negative-cache-ttl** *SECONDS*
: Cache negative DNS responses for this duration.

### Undelegated Testing

**--ns** *NAME*[/*IP*]
: Specify an undelegated nameserver (repeatable).

**--ds** *KEYTAG,ALGORITHM,DIGTYPE,DIGEST*
: Specify undelegated DS data (repeatable).

### Utility

**--badkeys-update**
: Download the badkeys blocklist and exit.

**--badkeys-path** *PATH*
: Override the badkeys blocklist directory.

**--dump-profile**
: Print the effective profile as JSON and exit.

**--list-tests**
: List all available test cases and exit.

**--version**
: Print version information and exit.

## EXIT STATUS

**0**
: All tests passed.

**2**
: Usage or runtime error.

**130**
: Interrupted (SIGINT/SIGTERM).

## EXAMPLES

Test a domain with default settings:

    gonemaster example.com

Run only DNSSEC tests with JSON output:

    gonemaster --module dnssec --json example.com

Run a single testcase:

    gonemaster --testcase dnssec20 example.com

Run several testcases across modules:

    gonemaster --testcase consistency04 --testcase delegation07 example.com

Show all results including INFO level:

    gonemaster --min-level INFO example.com

Test an undelegated zone:

    gonemaster --ns ns1.example.com/192.0.2.1 example.com

Show per-nameserver query timing statistics:

    gonemaster --nstimes example.com

Include nameserver timing data in JSON output:

    gonemaster --json --nstimes example.com | jq .nameserver_timings

Save and restore the DNS cache for faster re-runs:

    gonemaster --save cache.bin example.com
    gonemaster --restore cache.bin --testcase dnssec20 example.com

Save a gzip-compressed cache (either form works):

    gonemaster --save cache.json.gz example.com
    gonemaster --save cache.bin --save-compress example.com
    gonemaster --restore cache.json.gz example.com

## SEE ALSO

**gonemaster-server**(1), **gonemaster-client**(1), **gonemaster-nagios**(1)
