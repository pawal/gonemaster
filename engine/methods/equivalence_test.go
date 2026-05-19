package methods

import (
	"sort"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/methodsv2"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Equivalence tests document where AllNameservers (zone-view union of glue
// and apex NS) and methodsv2.ZoneNameservers (parent-queried delegation
// view) agree and where they differ.
//
// These tests do not drive a behavior change; they make the difference
// explicit so future readers and the post-refactor audit can reason about
// when each function is appropriate.
//
// Note: in undelegated mode (the test infra in use), both functions
// ultimately read from the recursor's fake-address map, so they agree on
// the nameserver set. Tests asserting the differ cases (lame delegation,
// out-of-bailiwick recursion) require a delegated zone scaffold with
// distinct parent and child NS responses; that scaffold is not yet present
// in either methods or methodsv2 test files (see
// TestGetParentNSNamesAndIPsSkipsOnIntermediateNoResponse for the complex
// pattern such a setup requires).

// TestAllNameserversVsZoneNameserversAgreeOnCleanUndelegated verifies that
// for an undelegated zone with clean glue, AllNameservers and
// ZoneNameservers return the same nameserver set (modulo []Nameserver vs
// []NSItem types).
func TestAllNameserversVsZoneNameserversAgreeOnCleanUndelegated(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	methodsv2.ClearParentNSCache()
	t.Cleanup(methodsv2.ClearParentNSCache)

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
		t.Fatalf("add example.com: %v", err)
	}
	// Apex servers respond authoritatively with the same NS set as the glue.
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
	zoneItems, err := methodsv2.ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ZoneNameservers: %v", err)
	}

	// Normalize both to a sorted set of "name|address" strings.
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
// when the zone's nameservers are out-of-bailiwick (e.g. example zone's
// NS records point at *.example.net), both functions still return the same
// set in undelegated mode, since the recursor's fake-address map serves as
// the single source of truth for both.
//
// The semantically interesting case - where ApexNameservers cannot resolve
// OOB names but methodsv2's recursor-driven resolution can - requires a
// delegated zone scaffold; see the file header.
func TestAllNameserversVsZoneNameserversAgreeOnOutOfBailiwickGlue(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	methodsv2.ClearParentNSCache()
	t.Cleanup(methodsv2.ClearParentNSCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root: %v", err)
	}
	// Zone "example.com" served by out-of-bailiwick nameservers in .net.
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.net": {"192.0.2.53"},
		"ns2.example.net": {"192.0.2.54"},
	}); err != nil {
		t.Fatalf("add example.com: %v", err)
	}
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
	zoneItems, err := methodsv2.ZoneNameservers(ctx, &z)
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
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	methodsv2.ClearParentNSCache()
	t.Cleanup(methodsv2.ClearParentNSCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}
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
	zoneItems, err := methodsv2.ZoneNameservers(ctx, &z)
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
