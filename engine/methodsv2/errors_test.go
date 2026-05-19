package methodsv2

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Error propagation tests.
//
// Note: most failure modes in the parent-chain walk and apex-NS resolution
// result in an empty slice + nil error rather than a returned error. The
// "ErrorWhen..." tests below exercise the explicit error paths that exist;
// the "ReturnsEmptyWhen..." tests document the silent-empty behavior so
// callers don't assume errors will surface.

// TestGetParentNSNamesAndIPsErrorWhenNilZone verifies the nil-zone guard.
func TestGetParentNSNamesAndIPsErrorWhenNilZone(t *testing.T) {
	if _, err := GetParentNSNamesAndIPs(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

// TestGetParentNSNamesAndIPsErrorWhenMissingRecursor verifies that a zone
// without a recursor produces the "missing recursor" error rather than a
// nil-pointer panic.
func TestGetParentNSNamesAndIPsErrorWhenMissingRecursor(t *testing.T) {
	z := zone.Zone{Name: dnsname.New("example.com.")}
	if _, err := GetParentNSNamesAndIPs(context.Background(), &z); err == nil {
		t.Fatalf("expected error for missing recursor")
	}
}

// TestGetParentNSNamesAndIPsReturnsEmptyWhenRootEmpty documents that a
// recursor with no root hints returns an empty result (caching a !defined
// entry on the way) rather than an error. Callers must check len() to
// detect this state.
func TestGetParentNSNamesAndIPsReturnsEmptyWhenRootEmpty(t *testing.T) {
	ClearCache()
	defer ClearCache()
	ctx, _, _ := testhelpers.Context(t)

	// Recursor has root entry but with no servers, so RootServers returns
	// empty (not an error) and the chain walk has no starting point.
	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := GetParentNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result with no root servers, got %#v", out)
	}
}

// TestGetDelNSNamesAndIPsErrorWhenNilZone verifies the nil-zone guard.
func TestGetDelNSNamesAndIPsErrorWhenNilZone(t *testing.T) {
	if _, err := GetDelNSNamesAndIPs(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

// TestGetDelNSNamesAndIPsReturnsEmptyWhenAllDelegationServersUnreachable
// documents the silent-empty behavior: when no delegation servers are
// reachable (here, no root hints to walk from), the function returns nil
// or empty rather than an error.
func TestGetDelNSNamesAndIPsReturnsEmptyWhenAllDelegationServersUnreachable(t *testing.T) {
	ClearCache()
	defer ClearCache()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := GetDelNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result, got %#v", out)
	}
}

// TestGetZoneNSNamesAndIPsErrorWhenNilZone verifies the nil-zone guard for
// the zone-apex-view function.
func TestGetZoneNSNamesAndIPsErrorWhenNilZone(t *testing.T) {
	if _, err := GetZoneNSNamesAndIPs(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

// TestGetZoneNSNamesAndIPsReturnsEmptyWhenDelegationEmpty documents the
// silent-empty path: when delegation has no servers, the zone-apex query
// step has nothing to query and the function returns empty.
func TestGetZoneNSNamesAndIPsReturnsEmptyWhenDelegationEmpty(t *testing.T) {
	ClearCache()
	defer ClearCache()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}
	// Undelegated zone with no fake addresses: delegation is empty.
	if err := r.AddFakeAddresses("example.com", map[string][]string{}); err != nil {
		t.Fatalf("add zone: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := GetZoneNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result, got %#v", out)
	}
}
