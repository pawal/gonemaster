# Frozen Domain Corpus

These files are fixed benchmark corpora for repeatable server performance runs.

- `domains-fixed-600.txt`
  - count: `600`
  - sha256: `dd770ee29637d2037dfa7498562de23dfd5202754a3a7296e7c024b3cf9cb75c`
- `domains-fixed-1000.txt`
  - count: `1000`
  - sha256: `f36f4c87960e241cec657dd96793e502d1ad2b3b36f71ccb4850b63f98a211dc`

Generation inputs:
- source: `majestic_million.csv` (in repository root)
- seed: `20260216`
- canary/outlier list: `tools/server-perf-kit/canary-domains.txt`
- generator: `tools/server-perf-kit/make_domains_from_majestic.sh`

Generation commands:

```bash
./tools/server-perf-kit/make_domains_from_majestic.sh \
  --input majestic_million.csv \
  --count 600 \
  --seed 20260216 \
  --out tools/server-perf-kit/corpus/domains-fixed-600.txt

./tools/server-perf-kit/make_domains_from_majestic.sh \
  --input majestic_million.csv \
  --count 1000 \
  --seed 20260216 \
  --out tools/server-perf-kit/corpus/domains-fixed-1000.txt
```

Normalization guarantee:
- All lines are registrable domains (public-suffix aware eTLD+1).
- Hostnames/subdomains in source are collapsed to main registrable domain.
