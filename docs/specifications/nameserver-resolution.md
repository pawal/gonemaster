# Nameserver Resolution Reference

Testcase specifications refer to several distinct views of the nameservers
that serve a zone. The views are not interchangeable; each testcase picks
the view that matches what is actually being verified.

This page is the single source of truth for those names. Testcase specs
link symbol mentions back to the matching section below.

The functions in this page live in [`engine/nsdiscovery`](https://codeberg.org/pawal/gonemaster/src/branch/main/engine/nsdiscovery)
except `Zone.GlueNames` and `Zone.ApexNSNames`, which are methods on
`*zone.Zone` in [`engine/zone`](https://codeberg.org/pawal/gonemaster/src/branch/main/engine/zone).

A nameserver address in IPv4-mapped IPv6 form (RFC 4291 section 2.5.5.2)
is queried as the IPv4 address it carries: the IPv4 switch of the profile
governs it, and it shares that address's cache.

## GlueNameservers

Nameservers from the parent's delegation glue, as name+address pairs.
The parent's claim about who serves the zone.

```go
func GlueNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error)
```

Equivalent to `z.Glue(ctx)` with a nil-zone guard. Returns an error if
`z` is nil; otherwise propagates errors from the underlying zone lookup.

Source: `engine/nsdiscovery/nsdiscovery.go`.

## GlueNames

Glue NS names from the parent zone, names only.

```go
func (z *Zone) GlueNames(ctx context.Context) ([]dnsname.Name, error)
```

Method on `*zone.Zone`. Returns the cached glue names for the zone or
fetches them from the parent on first call.

Source: `engine/zone/zone.go`.

## ApexNameservers

Nameservers resolved from the child zone's own apex NS RRset, as
name+address pairs. The child's claim about itself.

```go
func ApexNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error)
```

Equivalent to `z.NS(ctx)` with a nil-zone guard. For in-domain names
this resolves via the zone's own glue; not-in-domain names are
resolved via the recursor.

Source: `engine/nsdiscovery/nsdiscovery.go`.

## ApexNSNames

Apex NS names from the zone itself, names only.

```go
func (z *Zone) ApexNSNames(ctx context.Context) ([]dnsname.Name, error)
```

Method on `*zone.Zone`. Returns the union of NS names reported at the
zone apex by every authoritative server, deduplicated case-insensitively
and sorted. Differs from `NSNames`, which stops at the first successful
response.

Source: `engine/zone/zone.go`.

## AllNameservers

Deduplicated union of `GlueNameservers` and `ApexNameservers`. The
zone's combined view, as name+address pairs.

```go
func AllNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error)
```

The dedup key is the lower-cased `name/address` string, so the same
name with two different addresses is kept as two entries. Output is
sorted by that same key.

Source: `engine/nsdiscovery/nsdiscovery.go`.

## AllNSNames

Deduplicated union of `Zone.GlueNames` and `Zone.ApexNSNames`. The
zone's combined view, names only.

```go
func AllNSNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error)
```

Names are lowercased and sorted lexicographically. If either underlying
call returns an error, the error is propagated and no result is
returned.

Source: `engine/nsdiscovery/nsdiscovery.go`.

## DelegationNameservers

Parent-queried delegation view of the zone's nameservers, with
domain-aware address resolution. Appropriate for delegation
consistency checks that need the parent's authoritative answer.

```go
func DelegationNameservers(ctx context.Context, z *zone.Zone) ([]NSItem, error)
```

Returns `NSItem` values pairing names with addresses from in-domain
glue (if present) and not-in-domain recursive resolution (for
not-in-domain names). Result is sorted and deduplicated.

An unreachable delegation chain returns an empty slice with a nil
error; callers must check `len()`.

Source: `engine/nsdiscovery/delegation.go`.

## ZoneNameservers

Authoritative AA NS at the apex, paired with in-domain and
not-in-domain addresses via recursion.

```go
func ZoneNameservers(ctx context.Context, z *zone.Zone) ([]NSItem, error)
```

Queries the delegation servers for the apex NS RRset, keeps only
responses with AA set, and pairs the resulting names with addresses.
Differs from `DelegationNameservers` in that the names come from the
child zone's own apex NS RRset, not the parent's delegation. Useful for
detecting lame delegations where parent and child disagree.

Source: `engine/nsdiscovery/delegation.go`.

## ParentNameservers

The parent zone's own nameservers, found by walking the delegation
chain from the root down to the parent of `z`.

```go
func ParentNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error)
```

Results are memoised in a `Cache` attached to the context via
`WithCache` (one cache per engine run, isolated between runs). With no
cache attached, every call re-walks the chain.

For the root zone and zones whose names appear in the recursor's
fake-address map (undelegated test setups), an empty slice and nil
error are returned without caching.

If an intermediate name returns `NXDOMAIN` with AA set, the walker
probes the same server once at `z.Name` and, if it answers with a
referral (NS records for the child zone in the authority section),
accepts that server as the parent. This handles the RFC 8020
contradiction where a parent denies an empty non-terminal but still
holds a working delegation at the child. Without the probe, every NS
in such a parent would be discarded and the delegation view exposed to
downstream tests would be empty; Basic01 names the same condition with
`B01_PARENT_NXDOMAIN_HIDES_DELEGATION`.

Source: `engine/nsdiscovery/parent.go`.

## Fan-out Limits

A run bounds the nameserver names and addresses it takes from DNS
responses, after RFC 1034 section 5.3.3, which requires a resolver to bound
the packets it sends even when the data it follows is misconfigured. The
bounds have no protocol counterpart; their values are implementation
defined.

| Bound | Value | Set |
|---|---|---|
| `ns_names_per_zone` | 32 | The NS names of one zone: [GlueNames](#gluenames), [ApexNSNames](#apexnsnames), the apex NS names of `Zone.NS`, a referral the recursor follows, an NS RRset of the parent walk, and the names of [DelegationNameservers](#delegationnameservers) and [ZoneNameservers](#zonenameservers). |
| `addresses_per_ns` | 16 | The addresses of one NS name, from glue or from address resolution. |
| `addresses_per_run` | 512 | The distinct nameserver addresses one run queries. |

A set of names is lowercased and a set of addresses is taken as is; either
set is deduplicated, sorted in byte order of the presentation form, and cut
after the bound. A name or address past the cut is absent from the set and is
not queried through it.

The run bound counts an address when the run first queries it, before any
cache is read. A query to a further address is not sent and reads as no
response. When the run queries in parallel, which addresses fall past this
bound depends on the order of the queries.

When a bound cuts a set, the run logs `LIMIT_REACHED` in module `SYSTEM` at
level `NOTICE`. The entry does not change the score. It is logged at most
once per testcase, bound and name, in the testcase whose work first cuts the
set; a set computed once per run reports its cut once.

| Argument | Type | Meaning |
|---|---|---|
| `bound` | `string` | The bound, as named in the table above or in [DNSSEC Limits](#dnssec-limits). |
| `limit` | `int` | The value of the bound. |
| `count` | `int` | The size of the set before the cut; for `addresses_per_run`, the bound plus one; for `nsec3_iterations`, the iteration count; for `keys_per_tag`, the DNSKEYs with the key tag and algorithm of the RRSIG; for `rrsigs_per_rrset`, the RRSIGs over the RRset. |
| `domain` | `string` | The zone whose names, or the NS name whose addresses, the bound cut; for `addresses_per_run`, the NS name of the first query not sent; for a DNSSEC bound, the zone under test. |

Source: `engine/nameserver/limits.go`.

### DNSSEC Limits

DNSSEC testcases log `LIMIT_REACHED` for the bounds below, each defined in the
Implementation Notes of the testcases named, at most once per testcase and
bound. The Edge Cases of each testcase state the effect on its verdicts.

| Bound | Value | Testcases |
|---|---|---|
| `nsec3_iterations` | 50 | DNSSEC07, DNSSEC20, DNSSEC22 |
| `keys_per_tag` | 4 | DNSSEC02, DNSSEC07, DNSSEC08, DNSSEC09, DNSSEC10, DNSSEC16, DNSSEC17, DNSSEC21, DNSSEC22, DNSSEC24 |
| `rrsigs_per_rrset` | 8 | DNSSEC02, DNSSEC07, DNSSEC08, DNSSEC09, DNSSEC10, DNSSEC16, DNSSEC17, DNSSEC21, DNSSEC22, DNSSEC24 |
