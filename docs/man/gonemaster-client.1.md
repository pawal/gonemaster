# gonemaster-client 1 "2026" "gonemaster" "User Commands"

## NAME

gonemaster-client - HTTP API client for gonemaster-server

## SYNOPSIS

**gonemaster-client** [*GLOBAL OPTIONS*] *COMMAND* [*COMMAND OPTIONS*]

## DESCRIPTION

**gonemaster-client** interacts with a running **gonemaster-server** instance
via its REST API. It can submit test jobs, monitor progress, retrieve results,
and manage the job queue.

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

### batches get *BATCH-ID*

Get batch summary.

### batches watch *BATCH-ID*

Watch a batch until completion.

### batches cancel *BATCH-ID*

Cancel all jobs in a batch.

### batches remove *BATCH-ID*

Remove a batch. Use **--cancel-running** to cancel active jobs first.

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

Batch test from a file:

    gonemaster-client jobs batch --file domains.txt --wait --per-job

Get results for a completed job:

    gonemaster-client results --job-id abc123 --view summary

Watch a batch in progress:

    gonemaster-client batches watch batch-456

Use a remote server with JSON output:

    gonemaster-client --server https://gm.example.com/api/v1 \
        --format json jobs create --domain example.com --wait

## SEE ALSO

**gonemaster**(1), **gonemaster-server**(1), **gonemaster-nagios**(1)
