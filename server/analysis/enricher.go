package analysis

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
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

	mu    sync.Mutex
	addrs map[string]addrEntry
	asns  map[int64]asnEntry
}

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

	result, err := asnlookup.GetWithPrefix(ctx, e.Resolver, addr)
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

	e.mu.Lock()
	if entry, ok := e.asns[asn]; ok && time.Now().Before(entry.until) {
		e.mu.Unlock()
		return entry.label, entry.label != ""
	}
	e.mu.Unlock()

	info, err := asnlookup.LookupASNInfo(ctx, e.Resolver, int(asn))
	label := ""
	if err == nil && info.Code == asnlookup.CodeFound {
		label = info.Label
	}

	e.mu.Lock()
	if e.asns == nil {
		e.asns = map[int64]asnEntry{}
	}
	e.asns[asn] = asnEntry{label: label, until: time.Now().Add(e.TTL)}
	e.mu.Unlock()
	return label, label != ""
}

func (a AddressEnrichment) hasData() bool {
	return a.ASN != nil || a.Prefix != ""
}
