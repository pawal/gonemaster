# Nameserver Resolution Reference

Testcase specifications refer to several distinct views of the nameservers
that serve a zone. The views are not interchangeable; each testcase picks
the view that matches what is actually being verified.

This page is the single source of truth for those names. Testcase specs
link symbol mentions back to the matching section below.

The functions in this page live in [`engine/nsdiscovery`](https://codeberg.org/pawal/gonemaster/src/branch/main/engine/nsdiscovery)
except `Zone.GlueNames` and `Zone.ApexNSNames`, which are methods on
`*zone.Zone` in [`engine/zone`](https://codeberg.org/pawal/gonemaster/src/branch/main/engine/zone).

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
