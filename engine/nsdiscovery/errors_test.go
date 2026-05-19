package nsdiscovery

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
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
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

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
	ClearParentNSCache()
	defer ClearParentNSCache()
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

	out, err := DelegationNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result, got %#v", out)
	}
}

func TestZoneNameserversErrorWhenNilZone(t *testing.T) {
	if _, err := ZoneNameservers(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

func TestZoneNameserversReturnsEmptyWhenDelegationEmpty(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{}); err != nil {
		t.Fatalf("add zone: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result, got %#v", out)
	}
}
