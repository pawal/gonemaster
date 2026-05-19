package methodsv2

import (
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Forwarding parity tests: each semantic-name function must return the same
// result as its deprecated Get* counterpart on a shared fixture.

// fwdFixture builds an undelegated zone with two glue NSes plus a third
// apex-reported name.
func fwdFixture(t *testing.T) (*zone.Zone, func()) {
	t.Helper()
	ClearCache()
	nameserver.EmptyCache()

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root: %v", err)
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
	}); err != nil {
		t.Fatalf("add zone: %v", err)
	}
	_ = newAuthoritativeNameserver(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns2.example.com")
	_ = newAuthoritativeNameserver(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns2.example.com")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	return &z, func() {
		ClearCache()
		nameserver.EmptyCache()
	}
}

func TestParentNameserversEqualsGetParentNSNamesAndIPs(t *testing.T) {
	z, cleanup := fwdFixture(t)
	t.Cleanup(cleanup)
	ctx, _, _ := testhelpers.Context(t)

	a, errA := ParentNameservers(ctx, z)
	// Clear cache so the second call recomputes rather than serving from the cache
	// populated by the first call (we want true parity, not cache parity).
	ClearCache()
	b, errB := GetParentNSNamesAndIPs(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}

func TestDelegationNameserversEqualsGetDelNSNamesAndIPs(t *testing.T) {
	z, cleanup := fwdFixture(t)
	t.Cleanup(cleanup)
	ctx, _, _ := testhelpers.Context(t)

	a, errA := DelegationNameservers(ctx, z)
	b, errB := GetDelNSNamesAndIPs(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}

func TestZoneNameserversEqualsGetZoneNSNamesAndIPs(t *testing.T) {
	z, cleanup := fwdFixture(t)
	t.Cleanup(cleanup)
	ctx, _, _ := testhelpers.Context(t)

	a, errA := ZoneNameservers(ctx, z)
	b, errB := GetZoneNSNamesAndIPs(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}

func TestClearParentNSCacheClearsItemsMap(t *testing.T) {
	ClearCache()
	defer ClearCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}
	seedParentCache(ctx, t, r, "example.com.", "ns.example", "203.0.113.1")

	parentCache.mu.Lock()
	before := len(parentCache.items)
	parentCache.mu.Unlock()
	if before == 0 {
		t.Fatalf("expected nonzero cache after seed")
	}

	ClearParentNSCache()

	parentCache.mu.Lock()
	after := len(parentCache.items)
	parentCache.mu.Unlock()
	if after != 0 {
		t.Fatalf("expected empty cache after ClearParentNSCache, got %d", after)
	}
}
