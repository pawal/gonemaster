# Server Perf Kit (Debian amd64)

This package runs repeatable server performance A/B (or A/B/C/...) benchmarks on one host.

It supports:
- deterministic domain corpus generation from Majestic Million
- public-suffix-aware registrable domain normalization (hostnames -> main domain)
- parallel build of all variant binaries
- sequential interleaved benchmark runs
- one-command all-tracks workflow (quick screen + decision-grade reruns + outliers)
- machine-readable report output
- gate evaluation for keep/drop decisions

## Requirements
- Debian amd64 host with network access suitable for DNS testing
- `git`, `go`, `jq`, `curl`, `awk`, `sed`, `shuf`, `sha256sum`, `ps`
- local clone of this repository

Install tools on Debian:
```bash
sudo apt-get update
sudo apt-get install -y git golang-go jq curl coreutils procps
```

## 1) Prepare Variants

Create a variants file (`name ref`, whitespace-separated):
```text
main main
track-a refs/heads/perf-track-a
track-b refs/heads/perf-track-b
```

Example file is provided:
- `tools/server-perf-kit/variants.example.tsv`

## 2) Domain Corpus

Use the frozen corpus files in this kit (recommended):

- `tools/server-perf-kit/corpus/domains-fixed-600.txt`
- `tools/server-perf-kit/corpus/domains-fixed-1000.txt`

Regenerate from Majestic Million when needed:

```bash
./tools/server-perf-kit/make_domains_from_majestic.sh \
  --input majestic_million.csv \
  --count 600 \
  --seed 20260216 \
  --out /tmp/domains-600.txt
```

Notes:
- Stratification defaults:
  - 70% from ranks `1..100000`
  - 20% from ranks `100001..500000`
  - 10% from ranks `500001..1000000`
- Input domains are normalized to registrable domains using the Public Suffix List.
  - Example: `mp.weixin.qq.com` becomes `qq.com`.
- Canary/outlier domains are forced in from:
  - `tools/server-perf-kit/canary-domains.txt`

For nameserver-concentrated corpora - every domain delegating to the same
farm, which is what makes an operator's rate limiter observable - use
`make_domains_by_nameserver.sh` instead. Provenance, guardrails and the
lists-are-not-committed policy are in `corpus/README.md`.

```bash
./tools/server-perf-kit/make_domains_by_nameserver.sh \
  --zone se.zone \
  --ns-pattern '^ns0[12]\.one\.com$' \
  --count 500 \
  --out /tmp/onecom-se-500.txt
```

## 3) Run Matrix

This builds all variant binaries in parallel, then runs all benchmark runs sequentially in interleaved order.

```bash
./tools/server-perf-kit/run_tracks_matrix.sh \
  --variants tools/server-perf-kit/variants.example.tsv \
  --domains tools/server-perf-kit/corpus/domains-fixed-1000.txt \
  --workers 8 \
  --max-concurrent-jobs 8 \
  --warmups 1 \
  --repeats 7 \
  --out-dir perf-runs/server/tracks-$(date -u +%Y%m%d-%H%M%S)
```

Output includes:
- `report.json`
- `SUMMARY.md`
- `runs.csv`
- per-run logs and JSON under `runs/`

Two options matter when the corpus is concentrated on one operator:

- `--inter-run-sleep N` idles N seconds between runs. A farm limiter has
  memory, so without a cool-down each variant is punished for the traffic of
  the one before it.
- The variants file takes an optional third column, a per-variant profile
  path, for knob-only sweeps. Variants sharing a ref share one worktree and
  one binary, so both arms are provably the same code and only the profile
  differs.

## 3b) Per-Nameserver Aggregation

`nsagg` collapses the stored `nameserver_timings` rows of a batch into one
line per address, which is the level the rate-limit question lives at: a
single run says nothing, while the sum over a 500-domain farm batch shows
which addresses dropped or refused what share of our queries.

```bash
go run ./tools/server-perf-kit/cmd/nsagg \
  --base-url http://127.0.0.1:18080 \
  serial=batch_1 workers4=batch_2 workers16=batch_3 > ns.csv
```

Columns: variant, nameserver, address, runs, queries, timeouts, refused,
timeout_rate, refused_rate, avg_median_ms, max_ms, unreachable_runs,
runs_with_failures, runs_engaged. Rates use answers plus timeouts as the
denominator, since a timeout produces no answer and would otherwise not be
counted in what we asked the address for.

`--engage-threshold N` replays a detector over stored data: it counts the
runs in which an address answered at least once and still burned N timeout
budgets. Use it to estimate how often a proposed backoff trigger would fire
on a corpus that is not being rate limited, which is the spurious-engagement
question. The count is per run, never summed - an address with one timeout
in each of fifty runs engages in none of them.

## 4) Evaluate Gates

```bash
latest=$(ls -1dt perf-runs/server/tracks-* | head -n 1)
./tools/server-perf-kit/evaluate_gates.sh "$latest" main track-a
```

Arguments:
1. run directory
2. baseline variant name (usually `main`)
3. candidate variant name

Defaults:
- throughput target: `+10%`
- p95 guardrail: `<= +5%`
- p99 guardrail: `<= +5%`

## 5) One-Command Track Workflow

Use `run_all_tracks.sh` to execute:
- quick matrix for all variants (600 domains)
- gate evaluation for every candidate track
- decision-grade A/B reruns (1000 domains) for survivors (or all)
- per-domain outlier tables for decision runs
- consolidated session summary

Example variants file:
```text
main main
track-a perf-track-a-profile-cache
track-b perf-track-b-engine-hot-cache
track-c perf-track-c-job-parallelism
track-d perf-track-d-concurrency-clamp
track-e perf-track-e-dnssec-skip
```

Run all:
```bash
./tools/server-perf-kit/run_all_tracks.sh \
  --variants tools/server-perf-kit/variants-all.tsv \
  --baseline main \
  --workers 8 \
  --max-concurrent-jobs 8
```

Output root:
- `perf-runs/server/all-tracks-<utc>/`
  - `quick/`
  - `decision/<candidate>/`
  - `gate-summary.csv`
  - `SUMMARY.md`

## Recommended Run Strategy
- Use one fixed 600-domain corpus for quick screening.
- Use one fixed 1000-domain corpus for decision-grade reruns.
- Keep host otherwise idle during runs.
- Keep profile/settings fixed across all variants in one matrix.

## 6) Workers Sweep (main only)

Use this to tune server capacity parameters on one host without branch comparisons.

Example (coupled mode, `max-concurrent-jobs = workers`):
```bash
./tools/server-perf-kit/run_workers_sweep.sh \
  --main-ref main \
  --domains tools/server-perf-kit/corpus/domains-fixed-600.txt \
  --workers-list 4,8,12,16,24,32,48,64 \
  --mode coupled \
  --warmups 1 \
  --repeats 3
```

Optional decoupled pass (fixed concurrency while varying workers):
```bash
./tools/server-perf-kit/run_workers_sweep.sh \
  --main-ref main \
  --domains tools/server-perf-kit/corpus/domains-fixed-600.txt \
  --workers-list 4,8,12,16,24,32,48,64 \
  --mode fixed \
  --fixed-max-concurrent-jobs 16 \
  --warmups 1 \
  --repeats 3
```

Outputs:
- `workers-sweep-summary.csv`
- `SUMMARY.md`
- one full `run_tracks_matrix` artifact per sweep point under `points/`
