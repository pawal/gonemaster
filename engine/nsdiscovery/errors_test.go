package nsdiscovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/recursor/recursortest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Error and silent-empty path tests.
//
// Most failure modes in the parent-chain walk and apex-NS resolution result
// in an empty slice plus nil error rather than a returned error. The
// "ErrorWhen..." tests below exercise the explicit error paths that exist;
// the "ReturnsEmptyWhen..." tests document the silent-empty behavior.

func TestParentNameserversErrorWhenNilZone(t *testing.T) {
	if _, err := ParentNameservers(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

func TestParentNameserversErrorWhenMissingRecursor(t *testing.T) {
	z := zone.Zone{Name: dnsname.New("example.com.")}
	if _, err := ParentNameservers(context.Background(), &z); err == nil {
		t.Fatalf("expected error for missing recursor")
	}
}

func TestParentNameserversReturnsEmptyWhenRootEmpty(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, NewCache())

	r := nstest.Recursor(t, map[string]map[string][]string{".": map[string][]string{}})

	z := newZone(t, "example.com", r)

	out, err := ParentNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result with no root servers, got %#v", out)
	}
}

func TestDelegationNameserversErrorWhenNilZone(t *testing.T) {
	if _, err := DelegationNameservers(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

func TestDelegationNameserversReturnsEmptyWhenAllDelegationServersUnreachable(t *testing.T) {

	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, NewCache())

	r := nstest.Recursor(t, map[string]map[string][]string{".": map[string][]string{}})

	z := newZone(t, "example.com", r)

	out, err := DelegationNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result, got %#v", out)
	}
}

// TestGetOOBIPsAttachesCNAMEErrorToAddressLessItem verifies that when an
// out-of-bailiwick nameserver name fails to resolve because of a typed
// *recursor.CNAMEError, getOOBIPs returns an address-less NSItem carrying
// that error in Err (rather than silently dropping it). This is the data
// the consuming testcases turn into a CNAME_* tag.
func TestGetOOBIPsAttachesCNAMEErrorToAddressLessItem(t *testing.T) {

	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	r.SetNegativeCacheTTL(60 * time.Second)
	seedErr := &recursor.CNAMEError{
		Reason: recursor.CNAMEUnresolved,
		Name:   "ns.outside.test",
		Target: "loop.outside.test",
		Detail: "loop",
	}
	recursortest.SeedCNAMEError(r, seedErr, "ns.outside.test", []string{"A", "AAAA"})

	z := newZone(t, "example", r)

	items, err := getOOBIPs(ctx, &z, []dnsname.Name{dnsname.New("ns.outside.test")})
	if err != nil {
		t.Fatalf("getOOBIPs: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %#v", len(items), items)
	}

	item := items[0]
	if item.HasAddress {
		t.Fatalf("expected address-less item, got %#v", item)
	}
	var got *recursor.CNAMEError
	if !errors.As(item.Err, &got) {
		t.Fatalf("expected NSItem.Err to be *recursor.CNAMEError, got %#v", item.Err)
	}
	if got.Reason != recursor.CNAMEUnresolved {
		t.Fatalf("expected reason CNAMEUnresolved, got %v", got.Reason)
	}
	if got.Name != "ns.outside.test" {
		t.Fatalf("unexpected CNAMEError.Name %q", got.Name)
	}
}

func TestZoneNameserversErrorWhenNilZone(t *testing.T) {
	if _, err := ZoneNameservers(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

func TestZoneNameserversReturnsEmptyWhenDelegationEmpty(t *testing.T) {

	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, NewCache())

	r := nstest.Recursor(t, map[string]map[string][]string{
		".":           {"a.root": {"192.0.2.1"}},
		"example.com": {},
	})

	z := newZone(t, "example.com", r)

	out, err := ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result, got %#v", out)
	}
}
