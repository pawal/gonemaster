package asnlookup

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"sync"
)

// Cache memoizes ASN lookup results by IP address.
type Cache struct {
	mu   sync.Mutex
	data map[string]Result
}

// NewCache creates an empty ASN result cache.
func NewCache() *Cache {
	return &Cache{data: map[string]Result{}}
}

func (c *Cache) get(ip netip.Addr) (Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.data[ip.String()]
	return r, ok
}

func (c *Cache) set(ip netip.Addr, r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[ip.String()] = r
}

// CacheEntry is a portable representation of one cached ASN result.
type CacheEntry struct {
	IP     string
	ASNs   []int
	Prefix string
	Raw    string
	Code   string
}

// ExportEntries returns a deterministic snapshot of cached results.
func (c *Cache) ExportEntries() []CacheEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	keys := make([]string, 0, len(c.data))
	for k := range c.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]CacheEntry, 0, len(keys))
	for _, k := range keys {
		r := c.data[k]
		entry := CacheEntry{
			IP:   k,
			ASNs: r.ASNs,
			Raw:  r.Raw,
			Code: r.Code,
		}
		if r.Prefix != nil {
			entry.Prefix = r.Prefix.String()
		}
		entries = append(entries, entry)
	}
	c.mu.Unlock()
	return entries
}

// ImportEntries populates the cache from the supplied entries.
func (c *Cache) ImportEntries(entries []CacheEntry) error {
	if c == nil {
		return fmt.Errorf("asn cache is nil")
	}
	for idx, e := range entries {
		ip, err := netip.ParseAddr(e.IP)
		if err != nil {
			return fmt.Errorf("entry %d: invalid ip %q: %w", idx, e.IP, err)
		}
		for i, asn := range e.ASNs {
			if asn < 0 {
				return fmt.Errorf("entry %d: asns[%d]: negative ASN %d", idx, i, asn)
			}
		}
		switch e.Code {
		case CodeFound, CodeEmpty, CodeError:
		case "":
			return fmt.Errorf("entry %d: code is required", idx)
		default:
			return fmt.Errorf("entry %d: unknown code %q", idx, e.Code)
		}
		r := Result{
			ASNs: e.ASNs,
			Raw:  e.Raw,
			Code: e.Code,
		}
		if e.Prefix != "" {
			prefix, err := netip.ParsePrefix(e.Prefix)
			if err != nil {
				return fmt.Errorf("entry %d: invalid prefix %q: %w", idx, e.Prefix, err)
			}
			r.Prefix = &prefix
		}
		c.set(ip, r)
	}
	return nil
}

type asnCacheKey struct{}

// WithCache stores an ASN cache in the context.
func WithCache(ctx context.Context, c *Cache) context.Context {
	return context.WithValue(ctx, asnCacheKey{}, c)
}

// CacheFromContext returns the ASN cache from context, or nil.
func CacheFromContext(ctx context.Context) *Cache {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(asnCacheKey{}).(*Cache)
	return c
}
