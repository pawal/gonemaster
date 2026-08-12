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

## Nameserver-concentrated corpora

The corpora above are deliberately NS-diverse, which is right for throughput
work and wrong for measuring what a single operator's nameserver farm does
under our load. For that, `make_domains_by_nameserver.sh` selects every
domain in a zone that delegates to a matching nameserver set.

```bash
dig @zonedata.iis.se se SOA +short                        # cheap serial check
dig +noall +answer +onesoa @zonedata.iis.se se AXFR > se.zone

./tools/server-perf-kit/make_domains_by_nameserver.sh \
  --zone se.zone \
  --ns-pattern '^ns0[12]\.one\.com$' \
  --count 500 \
  --seed 20260812 \
  --out /tmp/onecom-se-500.txt
```

The generator writes a JSON manifest next to the list recording the zone
apex, SOA serial, zone sha256, NS pattern, seed, candidate count, output
count and output sha256. That manifest is what makes a corpus reproducible,
and it is what belongs in a measurement write-up.

### Lists are not committed

Only the generator and the manifest are committed. The domain lists
themselves are regenerable from zone data, and a checked-in list of one
operator's customer domains is a target list nobody needs published. Keep
generated lists outside the repository.

### Source data and attribution

The `.se` and `.nu` zones are available by open AXFR from `zonedata.iis.se`,
republished hourly and licensed CC BY 4.0. The published service terms ask
that experiments be coordinated by mail with hostmaster@iis.se first, and
that re-fetches be gated on the SOA serial - the provider blocks abusive
IPs, so both are requirements rather than courtesies. Carry the CC BY 4.0
attribution with any results derived from this data; the manifest already
contains it.
