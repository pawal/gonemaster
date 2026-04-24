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
