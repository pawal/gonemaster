# Client Examples

## Submit One Job

```sh
gonemaster-client jobs create --domain example.com --wait --view modules
```

## Run a Tagged Batch

```sh
gonemaster-client tags create tld --description "Top-level domains"
gonemaster-client tags add-domains tld --file tlds.txt
gonemaster-client jobs batch --from-tag tld --tag tld --wait
```

## Export Entries for Analysis

```sh
gonemaster-client entries query \
  --tag tld \
  --module DNSSEC \
  --latest \
  --format csv > dnssec-entries.csv
```

## Fetch Full Batch Results

```sh
gonemaster-client batches results batch_123 --view json --per-job
```

## Fetch Results Since a Timestamp

```sh
gonemaster-client jobs results \
  --all \
  --status succeeded \
  --created-after 2026-02-01T00:00:00Z \
  --aggregate
```

## Split Per-Job Result Files

```sh
gonemaster-client batches results \
  batch_123 \
  --view json \
  --per-job \
  --split-dir /tmp/gonemaster-results
```

## Extract Nameserver Pairs

```sh
gonemaster-client jobs results job_123 --view raw --format json \
  | jq -r '.entries[]
           | select(.args.ns and .args.address)
           | [.args.ns, .args.address] | @tsv'
```

## Fetch Translated Results

```sh
gonemaster-client jobs results job_123 --view translated --locale sv
```

## Report a Cohort's Two Newest Snapshots

```sh
gonemaster-client report kommuner --output kommuner-report.md
```

Reports the tags that appeared or cleared, every moving domain, and whether
each change came from the cohort or from the engine. See
[report.md](report.md).
