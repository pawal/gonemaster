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

## Batch Management

Useful commands:

- `batches get`: show batch summary.
- `batches watch`: wait for completion.
- `batches results`: fetch all results in a batch.
- `batches cancel`: cancel queued or running jobs in a batch.
- `batches remove`: remove queued jobs from a batch.

Batch jobs run at batch priority so they do not block interactive single-job
submissions.
