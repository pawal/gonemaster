package analysis

import (
	"context"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// AddressEnrichment carries the upstream-resolved (asn, prefix) pair for a
// single IP address. Either or both fields may be empty when the lookup
// backend returned no data.
type AddressEnrichment struct {
	ASN    *int64
	Prefix string
	Family string
}

// Enricher resolves the (asn, prefix) pair for an address and the
// human-readable label for an ASN. Implementations are expected to be
// fail-soft: ok=false on any error or missing data so the projector can
// proceed without enrichment.
type Enricher interface {
	EnrichAddress(ctx context.Context, ip string) (AddressEnrichment, bool)
	EnrichASNLabel(ctx context.Context, asn int64) (string, bool)
}

// resolverAdapter is the minimal asnlookup.Resolver surface. The real gonemaster
// recursor satisfies this already.
type resolverAdapter interface {
	Recurse(ctx context.Context, name string, qtype string, qclass string) (packet.Packet, error)
}

// AsnlookupEnricher is the default Enricher: it calls the engine's
// asnlookup package with a caller-supplied DNS recursor and caches results
// in memory for a TTL.
type AsnlookupEnricher struct {
	Resolver resolverAdapter
	TTL      time.Duration

	mu     sync.Mutex
	addrs  map[string]addrEntry
	asns   map[int64]asnEntry
	labels singleflight.Group
	now    func() time.Time
}

// Label cache lifetimes for lookups that yield no label.
const (
	asnEmptyTTL = 6 * time.Hour
	asnErrorTTL = 5 * time.Minute
)

type addrEntry struct {
	result AddressEnrichment
	until  time.Time
}

type asnEntry struct {
	label string
	until time.Time
}

// NewAsnlookupEnricher builds an enricher over the given recursor. Pass a
// zero TTL to get the default (24h).
func NewAsnlookupEnricher(resolver resolverAdapter, ttl time.Duration) *AsnlookupEnricher {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &AsnlookupEnricher{Resolver: resolver, TTL: ttl}
}

// recursionContext returns ctx carrying the fresh nameserver cache the recursor
// requires. Per-lookup scope matches the engine's per-run cache; results are
// cached separately so this stays short-lived scratch, not a stale global.
func (e *AsnlookupEnricher) recursionContext(ctx context.Context) context.Context {
	return nameserver.WithCache(ctx, nameserver.NewCacheStore())
}

// EnrichAddress looks up the prefix and ASN for an IP via cymru/ripe. Any
// lookup error is swallowed and returned as ok=false so the projector keeps
// going. The address string is expected in its engine-normalized form.
func (e *AsnlookupEnricher) EnrichAddress(ctx context.Context, ip string) (AddressEnrichment, bool) {
	if e == nil || e.Resolver == nil {
		return AddressEnrichment{}, false
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return AddressEnrichment{}, false
	}

	e.mu.Lock()
	if entry, ok := e.addrs[ip]; ok && time.Now().Before(entry.until) {
		e.mu.Unlock()
		return entry.result, entry.result.hasData()
	}
	e.mu.Unlock()

	result, err := asnlookup.GetWithPrefix(e.recursionContext(ctx), e.Resolver, addr)
	out := AddressEnrichment{}
	if err == nil {
		if result.Prefix != nil {
			out.Prefix = result.Prefix.String()
			if addr.Is4() {
				out.Family = "ipv4"
			} else if addr.Is6() {
				out.Family = "ipv6"
			}
		}
		if len(result.ASNs) > 0 {
			// Multi-origin prefixes (common for anycast DNS fleets) return
			// several origin ASes; pick the numerically smallest to keep
			// projection deterministic without losing ASN attribution.
			// A richer multi-valued model (one fact row per origin AS,
			// with a "multi_origin" marker on the per-address fact) is a
			// follow-up once the read path can render multi-AS endpoints
			// without cluttering the single-AS common case.
			smallest := result.ASNs[0]
			for _, a := range result.ASNs[1:] {
				if a < smallest {
					smallest = a
				}
			}
			asn := int64(smallest)
			out.ASN = &asn
		}
	}

	e.mu.Lock()
	if e.addrs == nil {
		e.addrs = map[string]addrEntry{}
	}
	e.addrs[ip] = addrEntry{result: out, until: time.Now().Add(e.TTL)}
	e.mu.Unlock()
	return out, out.hasData()
}

// EnrichASNLabel resolves the registry-provided description for an ASN.
// Falls back to ("", false) on any error or when the backend doesn't return
// a label (e.g. the ripe riswhois backend).
func (e *AsnlookupEnricher) EnrichASNLabel(ctx context.Context, asn int64) (string, bool) {
	if e == nil || e.Resolver == nil || asn <= 0 {
		return "", false
	}
	if label, ok := e.cachedASNLabel(asn); ok {
		return label, label != ""
	}
	v, _, _ := e.labels.Do(strconv.FormatInt(asn, 10), func() (any, error) {
		if label, ok := e.cachedASNLabel(asn); ok {
			return label, nil
		}
		return e.lookupASNLabel(ctx, asn), nil
	})
	label := v.(string)
	return label, label != ""
}

func (e *AsnlookupEnricher) cachedASNLabel(asn int64) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	entry, ok := e.asns[asn]
	if !ok || !e.clock().Before(entry.until) {
		return "", false
	}
	return entry.label, true
}

// lookupASNLabel caches by outcome; a canceled or expired ctx is not cached.
func (e *AsnlookupEnricher) lookupASNLabel(ctx context.Context, asn int64) string {
	info, err := asnlookup.LookupASNInfo(e.recursionContext(ctx), e.Resolver, int(asn))
	if ctx.Err() != nil {
		return ""
	}
	label, ttl := "", min(e.TTL, asnErrorTTL)
	switch {
	case err == nil && info.Code == asnlookup.CodeFound:
		label, ttl = info.Label, e.TTL
	case err == nil && info.Code == asnlookup.CodeEmpty:
		ttl = min(e.TTL, asnEmptyTTL)
	}

	e.mu.Lock()
	if e.asns == nil {
		e.asns = map[int64]asnEntry{}
	}
	e.asns[asn] = asnEntry{label: label, until: e.clock().Add(ttl)}
	e.mu.Unlock()
	return label
}

func (e *AsnlookupEnricher) clock() time.Time {
	if e.now != nil {
		return e.now()
	}
	return time.Now()
}

func (a AddressEnrichment) hasData() bool {
	return a.ASN != nil || a.Prefix != ""
}
