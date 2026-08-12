# Client Batches

Batches submit many server-side jobs in one request.

## Input Sources

```sh
gonemaster-client jobs batch --file domains.txt --tag tld --wait
cat domains.txt | gonemaster-client jobs batch --stdin --tag tld --wait
gonemaster-client jobs batch --domain example.com --domain example.net --wait
```

Use `--from-tag TAG` to rerun every domain currently in a tag:

```sh
gonemaster-client jobs batch --from-tag tld --tag tld --wait
```

`--from-tag` selects the input domains. `--tag` applies tags to the resulting
domain and run records.

Input options:

- `--domain DOMAIN`, repeatable
- `--file PATH`, repeatable
- `--stdin`
- `--from-tag TAG`

Batch options:

- `--tests NAME`, repeatable
- `--min-level LEVEL`
- `--profile-override KEY=VALUE`, repeatable
- `--profile-overrides-file PATH`
- `--wait`
- `--view summary|modules|raw|json`
- `--per-job`
- `--score`
- `--no-score`
- `--scoring-config PATH`

`--from-tag` is mutually exclusive with explicit domain input.

## Batch Management

Useful commands:

- `batches get`: show batch summary.
- `batches watch`: wait for completion.
- `batches results`: fetch all results in a batch.
- `batches cancel`: cancel queued or running jobs in a batch.
- `batches remove`: remove queued jobs from a batch.
- `batches diff`: compare two batches of the same corpus domain by domain.

Batch jobs run at batch priority so they do not block interactive single-job
submissions.

## Comparing two batches

`batches diff` pairs runs by domain name and diffs each pair at the tag
level, then rolls the result up: how many domains are identical, how many
differ, and which tags appeared, cleared, or changed severity on how many
domains. Use it when the same corpus was tested twice and you need to know
what moved - before and after a fix, or the same corpus under two different
server settings.

```sh
gonemaster-client batches diff batch_before batch_after
gonemaster-client batches diff --per-domain batch_before batch_after
```

Domains present in only one of the two batches are reported separately
rather than dropped, so a batch that did not complete cannot look cleaner
than one that did. The exit status is `0` when no domain differs and `1`
otherwise, which makes the command usable as a gate.

## Results

```sh
gonemaster-client batches results batch_123 --view modules --aggregate
```

Options:

- `--view summary|modules|raw|json`
- `--levels NOTICE,WARNING,ERROR,CRITICAL`
- `--aggregate`
- `--per-job`
- `--score`
- `--no-score`
- `--scoring-config PATH`

Use `jobs results --batch-id BATCH_ID` when you need the broader multi-job
selection flags. Use `batches results` when you already know the batch ID.

## Cancel and Remove

```sh
gonemaster-client batches cancel batch_123
gonemaster-client batches remove batch_123
gonemaster-client batches remove batch_123 --cancel-running
```

`batches cancel` cancels queued or running jobs in the batch. Completed jobs
are left alone. `batches remove` removes queued jobs; with `--cancel-running`,
it also cancels running jobs.
