package nameserver

import (
	"context"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

// Fan-out bounds of one run.
const (
	MaxNSNames      = 32
	MaxNSAddresses  = 16
	MaxRunAddresses = 512
)

// Bound names in LIMIT_REACHED.
const (
	BoundNSNames      = "ns_names_per_zone"
	BoundNSAddresses  = "addresses_per_ns"
	BoundRunAddresses = "addresses_per_run"
)

// runLimits holds the fan-out state of one run.
type runLimits struct {
	mu     sync.Mutex
	addrs  map[netip.Addr]struct{}
	logged map[limitKey]struct{}
}

type limitKey struct {
	log    *logger.Logger
	bound  string
	domain string
}

func newRunLimits() *runLimits {
	return &runLimits{addrs: map[netip.Addr]struct{}{}, logged: map[limitKey]struct{}{}}
}

// admit reports whether the run may query addr.
func (l *runLimits) admit(addr netip.Addr) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.addrs[addr]; ok {
		return true
	}
	if len(l.addrs) >= MaxRunAddresses {
		return false
	}
	l.addrs[addr] = struct{}{}
	return true
}

// first reports whether key is seen for the first time.
func (l *runLimits) first(key limitKey) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.logged[key]; ok {
		return false
	}
	l.logged[key] = struct{}{}
	return true
}

// admitAddress reports whether the run of c may query addr.
func (c *CacheStore) admitAddress(addr netip.Addr) bool {
	if c == nil || c.limits == nil {
		return true
	}
	return c.limits.admit(addr)
}

// LimitReached logs LIMIT_REACHED once per logger, bound and domain of the run.
func LimitReached(ctx context.Context, bound string, domain string, limit int, count int) {
	limitReached(loggerFromContextOrFallback(ctx, nil), CacheFromContext(ctx), bound, domain, limit, count)
}

func limitReached(log *logger.Logger, c *CacheStore, bound string, domain string, limit int, count int) {
	if log == nil {
		return
	}
	key := limitKey{log: log, bound: bound, domain: domain}
	if bound == BoundRunAddresses {
		key.domain = ""
	}
	if c != nil && c.limits != nil && !c.limits.first(key) {
		return
	}
	logSystemWithLogger(log, "LIMIT_REACHED", map[string]any{
		"bound":  bound,
		"limit":  limit,
		"count":  count,
		"domain": domain,
	})
}

// CapNames keeps the first MaxNSNames of the sorted, unique names of domain.
func CapNames[T any](ctx context.Context, domain string, names []T) []T {
	if len(names) <= MaxNSNames {
		return names
	}
	LimitReached(ctx, BoundNSNames, domain, MaxNSNames, len(names))
	return names[:MaxNSNames]
}

// CapAddresses sorts and deduplicates the addresses of ns and keeps the first MaxNSAddresses.
func CapAddresses(ctx context.Context, ns string, addrs []netip.Addr) []netip.Addr {
	out := slices.Clone(addrs)
	slices.SortFunc(out, func(a, b netip.Addr) int { return strings.Compare(a.String(), b.String()) })
	out = slices.Compact(out)
	if len(out) <= MaxNSAddresses {
		return out
	}
	LimitReached(ctx, BoundNSAddresses, ns, MaxNSAddresses, len(out))
	return out[:MaxNSAddresses]
}

// CapNSMap cuts a map of lowercased NS names of domain to their addresses to the bounds.
func CapNSMap(ctx context.Context, domain string, m map[string][]netip.Addr) {
	names := slices.Sorted(maps.Keys(m))
	for _, name := range names[len(CapNames(ctx, domain, names)):] {
		delete(m, name)
	}
	for name, addrs := range m {
		if len(addrs) > 0 {
			m[name] = CapAddresses(ctx, name, addrs)
		}
	}
}
