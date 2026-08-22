package asnlookup

import (
	"context"
	"net/netip"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestCacheHitSkipsLookup(t *testing.T) {
	c := NewCache()
	ip := netip.MustParseAddr("192.0.2.10")
	prefix := netip.MustParsePrefix("192.0.2.0/24")
	c.set(ip, Result{ASNs: []int{64496}, Prefix: &prefix, Code: CodeFound})

	ctx := context.Background()
	p := dnstest.DefaultProfile(t)
	ctx = profile.WithContext(ctx, p)
	ctx = WithCache(ctx, c)

	// Should return cached result without touching the resolver.
	result, err := GetWithPrefix(ctx, nil, ip)
	if err != nil {
		t.Fatalf("expected cached hit, got error: %v", err)
	}
	if result.Code != CodeFound || len(result.ASNs) != 1 || result.ASNs[0] != 64496 {
		t.Fatalf("unexpected cached result: %+v", result)
	}
}

func TestCacheFromContextReturnsNilWhenMissing(t *testing.T) {
	if c := CacheFromContext(context.Background()); c != nil {
		t.Fatalf("expected nil, got %v", c)
	}
	var nilCtx context.Context
	if c := CacheFromContext(nilCtx); c != nil {
		t.Fatalf("expected nil for nil ctx, got %v", c)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	c := NewCache()
	prefix := netip.MustParsePrefix("198.51.100.0/24")
	c.set(netip.MustParseAddr("198.51.100.1"), Result{
		ASNs: []int{64496, 64497}, Prefix: &prefix, Raw: "raw-line", Code: CodeFound,
	})
	c.set(netip.MustParseAddr("2001:db8::1"), Result{Code: CodeEmpty})

	entries := c.ExportEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	restored := NewCache()
	if err := restored.ImportEntries(entries); err != nil {
		t.Fatalf("import: %v", err)
	}

	r1, ok := restored.get(netip.MustParseAddr("198.51.100.1"))
	if !ok || r1.Code != CodeFound || len(r1.ASNs) != 2 {
		t.Fatalf("unexpected result after import: %+v (ok=%v)", r1, ok)
	}
	if r1.Prefix == nil || r1.Prefix.String() != "198.51.100.0/24" {
		t.Fatalf("unexpected prefix: %v", r1.Prefix)
	}
	if r1.Raw != "raw-line" {
		t.Fatalf("unexpected raw: %q", r1.Raw)
	}

	r2, ok := restored.get(netip.MustParseAddr("2001:db8::1"))
	if !ok || r2.Code != CodeEmpty {
		t.Fatalf("unexpected empty result: %+v (ok=%v)", r2, ok)
	}
}

func TestImportValidationErrors(t *testing.T) {
	tests := []struct {
		name  string
		entry CacheEntry
	}{
		{name: "invalid ip", entry: CacheEntry{IP: "bad", Code: CodeFound}},
		{name: "negative ASN", entry: CacheEntry{IP: "192.0.2.1", ASNs: []int{-1}, Code: CodeFound}},
		{name: "unknown code", entry: CacheEntry{IP: "192.0.2.1", Code: "BOGUS"}},
		{name: "missing code", entry: CacheEntry{IP: "192.0.2.1"}},
		{name: "invalid prefix", entry: CacheEntry{IP: "192.0.2.1", Prefix: "bad", Code: CodeFound}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := NewCache().ImportEntries([]CacheEntry{tc.entry}); err == nil {
				t.Fatalf("expected %s to be rejected", tc.name)
			}
		})
	}
}

func TestExportDeterministic(t *testing.T) {
	c := NewCache()
	c.set(netip.MustParseAddr("192.0.2.2"), Result{Code: CodeEmpty})
	c.set(netip.MustParseAddr("192.0.2.1"), Result{Code: CodeEmpty})

	entries := c.ExportEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].IP != "192.0.2.1" || entries[1].IP != "192.0.2.2" {
		t.Fatalf("expected sorted by IP, got %q then %q", entries[0].IP, entries[1].IP)
	}
}
