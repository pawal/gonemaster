# gonemaster-client 1 "2026" "gonemaster" "User Commands"

## NAME

gonemaster-client - HTTP API client for gonemaster-server

## SYNOPSIS

**gonemaster-client** [*GLOBAL OPTIONS*] *COMMAND* [*COMMAND OPTIONS*]

## DESCRIPTION

**gonemaster-client** interacts with a running **gonemaster-server** instance
via its REST API. It can submit test jobs, monitor progress, retrieve results,
manage the job queue, and query the domain/tag/run/entry analysis APIs.

## GLOBAL OPTIONS

**--server** *URL*
: Base API URL (default: http://localhost:8080/api/v1).

**--timeout** *DURATION*
: HTTP request timeout (default: 30s).

**--format** *FORMAT*
: Output format: **pretty**, **json**, or **json-stream** (default: pretty).

**--output** *PATH*
: Write output to a file instead of stdout.

**--locale** *LOCALE*
: Locale for translated messages (default: en).

**--no-color**
: Disable ANSI colors in pretty output.

**--header** *NAME:VALUE*
: Extra HTTP header (repeatable).

**--version**
: Print version and exit.

## COMMANDS

### jobs create

Submit a single test job.

**--domain** *DOMAIN*
: Zone to test (required).

**--min-level** *LEVEL*
: Minimum severity level.

**--tests** *TEST*
: Run specific test(s) (repeatable).

**--wait**
: Wait for completion and display results.

**--view** *VIEW*
: Result view: **summary**, **modules**, **raw**, **json**.

### jobs batch

Submit a batch of test jobs.

**--domain** *DOMAIN*
: Zone to test (repeatable).

**--file** *PATH*
: File with one domain per line (repeatable).

**--stdin**
: Read domains from stdin.

**--from-tag** *TAG*
: Re-run all domains currently in this tag. Mutually exclusive with **--domain**/**--file**/**--stdin**.

**--tag** *TAG*
: Apply this tag to all batch domains (repeatable; creates tag/domain records if needed).

**--wait**
: Wait for all jobs to complete.

**--per-job**
: Show per-job results when waiting.

### jobs list

List jobs with optional filters.

**--status** *STATUS*
: Filter by job status.

**--limit** *N*
: Maximum results (default: 100).

### jobs get *JOB-ID*

Get details for a single job.

### jobs watch *JOB-ID*

Watch a job until completion.

### jobs cancel *JOB-ID*

Cancel a running job.

### jobs purge

Delete completed jobs older than a given age, along with their results.

**--older-than** *N*
: Delete jobs finished more than N days ago. 0 (default) uses the server's configured retention_days. Returns an error if both are 0.

### domains list

List domains in the registry.

**--tag** *TAG*
: Filter to domains in this tag.

**--name** *SUBSTRING*
: Filter by domain name substring.

**--level** *LEVEL*
: Filter by exact latest level (e.g. ERROR).

**--limit** *N*
: Maximum results (default: 100).

### domains get *DOMAIN-NAME*

Get details for a domain (tags, run count, latest level).

### domains runs *DOMAIN-NAME*

List run history for a domain.

**--limit** *N*
: Maximum results (default: 20).

### domains tag *DOMAIN-NAME* *TAG* [*TAG*...]

Add a domain to one or more tags (creates tag records if needed).

### domains untag *DOMAIN-NAME* *TAG* [*TAG*...]

Remove a domain from one or more tags.

### tags list

List all tags.

### tags create *TAG-NAME*

Create a new tag.

**--description** *TEXT*
: Optional description.

### tags delete *TAG-NAME*

Delete a tag (domain records and runs are preserved).

### tags domains *TAG-NAME*

List domains in a tag.

**--limit** *N*
: Maximum results (default: 100).

### tags summary *TAG-NAME*

Show per-severity domain counts for a tag.

### tags add-domains *TAG-NAME* [*DOMAIN*...] [--file *PATH*] [--stdin]

Add domains to a tag in bulk. Creates domain records if needed.

**--file** *PATH*
: File with one domain per line (repeatable).

**--stdin**
: Read domains from stdin.

### runs list

List completed runs.

**--tag** *TAG*
: Filter to runs for domains in this tag.

**--domain** *SUBSTRING*
: Filter by domain name substring.

**--batch** *BATCH-ID*
: Filter by batch ID.

**--level** *LEVEL*
: Filter by worst level.

**--limit** *N*
: Maximum results (default: 100).

### runs get *RUN-ID*

Get metadata for a single run.

### runs results *RUN-ID*

Fetch and display results for a run.

**--view** *VIEW*
: Result view: **summary**, **modules**, **raw**, **json**.

### batches diff *BATCH-A* *BATCH-B*

Compare two batches of the same corpus domain by domain, pairing runs by
domain name and diffing each pair at the tag level. Reports how many domains
are identical and how many differ, a rollup of which tags appeared, cleared,
or changed severity and on how many domains, the domains present in only one
of the two batches, and the domains whose run carried no entries. Exits
**0** only when every domain was comparable and identical, **1** otherwise.
A batch holding more runs than **--limit** is an error rather than a
truncated comparison.

**--per-domain**
: Include the per-domain delta list, not only the rollup.

**--quiet**
: Print nothing; report the verdict through the exit status only.

**--limit** *N*
: Maximum runs fetched per batch (default: 5000).

### runs diff *RUN-A* *RUN-B*

Compare two runs at the tag level and report which findings appeared,
cleared, or changed severity. Each tag is collapsed to its worst level
within a run, so a tag emitted once per nameserver is compared by its
severest occurrence. Exits **0** when the two runs are identical, **1** when
they differ, and **2** on error, so it can be used as a gate in a script.

**--quiet**
: Print nothing; report the verdict through the exit status only.

### entries query

Query individual engine log entries across runs.

**--tag** *TAG*
: Filter by domain tag.

**--module** *MODULE*
: Filter by module name (e.g. DNSSEC).

**--testcase** *TESTCASE*
: Filter by testcase name.

**--entry-tag** *TAG*
: Filter by log event tag (e.g. DS_ALGO_NOT_SUPPORTED).

**--level** *LEVEL*
: Filter by severity level.

**--latest**
: Only entries from each domain's latest run.

**--limit** *N*
: Maximum results (default: 100).

Use **--format csv** to download results as CSV.

### batches get *BATCH-ID*

Get batch summary.

### batches watch *BATCH-ID*

Watch a batch until completion.

### batches cancel *BATCH-ID*

Cancel all jobs in a batch.

### batches remove *BATCH-ID*

Remove a batch. Use **--cancel-running** to cancel active jobs first.

### report [*DATASET-TAG*]

Compare two snapshots of an analysis cohort and classify every change as
engine-driven or real. The dataset tag defaults to the server's default
public cohort, **--to** to the newest snapshot, and **--from** to the
snapshot captured before it.

**--format** *FORMAT*
: **markdown** (the default, also named **pretty**) or **json**. Accepted
  before or after the command.

**--from** *SLUG*
: Baseline snapshot.

**--to** *SLUG*
: Later snapshot.

**--min-cluster** *N*
: Domains a cluster needs (server default 3).

**--max-spread** *N*
: Score spread a cluster allows (server default 3).

**--snapshots**
: List the cohort's snapshot slugs instead of reporting.

### queue pause

Pause the job queue.

### queue resume

Resume the job queue.

### results

Retrieve and display test results.

**--job-id** *ID*
: Fetch results for a job (repeatable).

**--batch-id** *ID*
: Fetch results for a batch.

**--view** *VIEW*
: Result view: **summary**, **modules**, **raw**, **json**.

**--aggregate**
: Aggregate results across jobs.

**--split-dir** *PATH*
: Write per-job results to separate files in a directory.

## EXAMPLES

Test a single domain and wait for results:

    gonemaster-client jobs create --domain example.com --wait

Batch test from a file and tag all domains:

    gonemaster-client jobs batch --file domains.txt --tag tld --wait --per-job

Re-run all domains in a tag:

    gonemaster-client jobs batch --from-tag tld --tag tld --wait

Get results for a completed job:

    gonemaster-client results --job-id abc123 --view summary

List domains with ERROR or worse:

    gonemaster-client domains list --tag tld --level ERROR

Show per-severity summary for a tag:

    gonemaster-client tags summary tld

List recent runs for a domain:

    gonemaster-client domains runs example.com

Fetch full results for a run:

    gonemaster-client runs results <run-id> --view summary

Export all DNSSEC entries for a tag as CSV:

    gonemaster-client --format csv entries query --tag tld --module DNSSEC --latest

Watch a batch in progress:

    gonemaster-client batches watch batch-456

Purge completed jobs older than 90 days:

    gonemaster-client jobs purge --older-than 90

Report the two newest snapshots of a cohort as Markdown:

    gonemaster-client report kommuner --output report.md

Use a remote server with JSON output:

    gonemaster-client --server https://gm.example.com/api/v1 \
        --format json jobs create --domain example.com --wait

## SEE ALSO

**gonemaster**(1), **gonemaster-server**(1), **gonemaster-nagios**(1)
