package nsdiscovery

import (
	"sort"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Equivalence tests document where AllNameservers (zone-view union of glue
// and apex NS) and ZoneNameservers (parent-queried delegation view) agree
// and where they differ.

// TODO: in undelegated mode both functions read the recursor's fake-address
// map, so they always agree. Covering the divergent cases (lame delegation,
// out-of-bailiwick recursion) needs a delegated zone scaffold with distinct
// parent and child NS responses; see
// TestParentNameserversSkipsOnIntermediateNoResponse for that pattern.

// TestAllNameserversVsZoneNameserversAgreeOnCleanUndelegated verifies that
// for an undelegated zone with clean glue, AllNameservers and ZoneNameservers
// return the same nameserver set.
func TestAllNameserversVsZoneNameserversAgreeOnCleanUndelegated(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".": map[string][]string{
			"a.root": {"192.0.2.1"},
		},
		"example.com": map[string][]string{
			"ns1.example.com": {"192.0.2.11"},
			"ns2.example.com": {"192.0.2.12"},
		},
	})
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns2.example.com")
	setNSHook(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns2.example.com")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	allNS, err := AllNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("AllNameservers: %v", err)
	}
	zoneItems, err := ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ZoneNameservers: %v", err)
	}

	allSet := make([]string, 0, len(allNS))
	for _, ns := range allNS {
		allSet = append(allSet, ns.Name.String()+"|"+ns.Address.String())
	}
	sort.Strings(allSet)

	zoneSet := make([]string, 0, len(zoneItems))
	for _, it := range zoneItems {
		addr := ""
		if it.HasAddress {
			addr = it.Address.String()
		}
		zoneSet = append(zoneSet, it.Name.String()+"|"+addr)
	}
	sort.Strings(zoneSet)

	if len(allSet) != len(zoneSet) {
		t.Fatalf("size mismatch: AllNameservers=%d, ZoneNameservers=%d\nall=%#v\nzone=%#v",
			len(allSet), len(zoneSet), allSet, zoneSet)
	}
	for i := range allSet {
		if allSet[i] != zoneSet[i] {
			t.Fatalf("mismatch at %d: all=%q zone=%q (full: all=%#v zone=%#v)",
				i, allSet[i], zoneSet[i], allSet, zoneSet)
		}
	}
}

// TestAllNameserversVsZoneNameserversAgreeOnOutOfBailiwickGlue verifies that
// when the zone's nameservers are out-of-bailiwick, both functions still
// return the same set in undelegated mode.
func TestAllNameserversVsZoneNameserversAgreeOnOutOfBailiwickGlue(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".": map[string][]string{
			"a.root": {"192.0.2.1"},
		},
		"example.com": map[string][]string{
			"ns1.example.net": {"192.0.2.53"},
			"ns2.example.net": {"192.0.2.54"},
		},
	})
	setNSHook(ctx, t, r, "ns1.example.net", "192.0.2.53", "example.com", "ns1.example.net", "ns2.example.net")
	setNSHook(ctx, t, r, "ns2.example.net", "192.0.2.54", "example.com", "ns1.example.net", "ns2.example.net")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	allNS, err := AllNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("AllNameservers: %v", err)
	}
	zoneItems, err := ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ZoneNameservers: %v", err)
	}

	allNames := map[string]bool{}
	for _, ns := range allNS {
		allNames[ns.Name.String()] = true
	}
	zoneNames := map[string]bool{}
	for _, it := range zoneItems {
		zoneNames[it.Name.String()] = true
	}
	for _, want := range []string{"ns1.example.net", "ns2.example.net"} {
		if !allNames[want] {
			t.Errorf("AllNameservers missing %s; got %#v", want, allNames)
		}
		if !zoneNames[want] {
			t.Errorf("ZoneNameservers missing %s; got %#v", want, zoneNames)
		}
	}
}

// TestAllNameserversVsZoneNameserversAgreeOnEmptyZone verifies that an
// undelegated zone with no fake addresses produces the same empty result
// from both functions.
func TestAllNameserversVsZoneNameserversAgreeOnEmptyZone(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{".": map[string][]string{"a.root": {"192.0.2.1"}}})
	if err := r.AddFakeAddresses("example.com", map[string][]string{}); err != nil {
		t.Fatalf("add empty: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	allNS, err := AllNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("AllNameservers: %v", err)
	}
	zoneItems, err := ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ZoneNameservers: %v", err)
	}
	if len(allNS) != 0 {
		t.Errorf("AllNameservers expected empty, got %#v", allNS)
	}
	if len(zoneItems) != 0 {
		t.Errorf("ZoneNameservers expected empty, got %#v", zoneItems)
	}
}
