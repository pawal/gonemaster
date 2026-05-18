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

// Equivalence tests document where Method4and5 (zone-view union of glue and
// apex NS) and methodsv2.GetZoneNSNamesAndIPs (parent-queried delegation
// view) agree and where they differ.
//
// These tests do not drive a behavior change; they make the difference
// explicit so future readers and the post-refactor audit can reason about
// when each function is appropriate.
//
// Note: in undelegated mode (the test infra in use), both functions
// ultimately read from the recursor's fake-address map, so they agree on
// the nameserver set. Tests asserting the *differ* cases (lame delegation,
// out-of-bailiwick recursion) require a delegated zone scaffold with
// distinct parent and child NS responses; that scaffold is not yet present
// in either methods or methodsv2 test files (see
// TestGetParentNSNamesAndIPsSkipsOnIntermediateNoResponse for the complex
// pattern such a setup requires). The differ scenarios are deferred to
// Phase 5 follow-up work where the AllNameservers semantic audit happens.

// TestMethod4and5VsZoneNameserversAgreeOnCleanUndelegated verifies that for
// an undelegated zone with clean glue, Method4and5 and GetZoneNSNamesAndIPs
// return the same nameserver set (modulo []Nameserver vs []NSItem types).
func TestMethod4and5VsZoneNameserversAgreeOnCleanUndelegated(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	methodsv2.ClearCache()
	t.Cleanup(methodsv2.ClearCache)

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

	m45, err := Method4and5(ctx, &z)
	if err != nil {
		t.Fatalf("method4and5: %v", err)
	}
	zoneItems, err := methodsv2.GetZoneNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("getZoneNSNamesAndIPs: %v", err)
	}

	// Normalize both to a sorted set of "name|address" strings.
	m45Set := make([]string, 0, len(m45))
	for _, ns := range m45 {
		m45Set = append(m45Set, ns.Name.String()+"|"+ns.Address.String())
	}
	sort.Strings(m45Set)

	v2Set := make([]string, 0, len(zoneItems))
	for _, it := range zoneItems {
		addr := ""
		if it.HasAddress {
			addr = it.Address.String()
		}
		v2Set = append(v2Set, it.Name.String()+"|"+addr)
	}
	sort.Strings(v2Set)

	if len(m45Set) != len(v2Set) {
		t.Fatalf("size mismatch: Method4and5=%d, GetZoneNSNamesAndIPs=%d\nm45=%#v\nv2 =%#v",
			len(m45Set), len(v2Set), m45Set, v2Set)
	}
	for i := range m45Set {
		if m45Set[i] != v2Set[i] {
			t.Fatalf("mismatch at %d: m45=%q v2=%q (full: m45=%#v v2=%#v)",
				i, m45Set[i], v2Set[i], m45Set, v2Set)
		}
	}
}

// TestMethod4and5VsZoneNameserversAgreeOnOutOfBailiwickGlue verifies that
// when the zone's nameservers are out-of-bailiwick (e.g. example zone's
// NS records point at *.example.net), both functions still return the same
// set in undelegated mode, since the recursor's fake-address map serves as
// the single source of truth for both.
//
// The semantically interesting case - where Method5 cannot resolve OOB
// names but methodsv2's recursor-driven resolution can - requires a
// delegated zone scaffold and is documented in the file header as
// deferred to Phase 5.
func TestMethod4and5VsZoneNameserversAgreeOnOutOfBailiwickGlue(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	methodsv2.ClearCache()
	t.Cleanup(methodsv2.ClearCache)

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

	m45, err := Method4and5(ctx, &z)
	if err != nil {
		t.Fatalf("method4and5: %v", err)
	}
	zoneItems, err := methodsv2.GetZoneNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("getZoneNSNamesAndIPs: %v", err)
	}

	m45Names := map[string]bool{}
	for _, ns := range m45 {
		m45Names[ns.Name.String()] = true
	}
	v2Names := map[string]bool{}
	for _, it := range zoneItems {
		v2Names[it.Name.String()] = true
	}
	for _, want := range []string{"ns1.example.net", "ns2.example.net"} {
		if !m45Names[want] {
			t.Errorf("Method4and5 missing %s; got %#v", want, m45Names)
		}
		if !v2Names[want] {
			t.Errorf("GetZoneNSNamesAndIPs missing %s; got %#v", want, v2Names)
		}
	}
}

// TestMethod4and5VsZoneNameserversAgreeOnEmptyZone verifies that an
// undelegated zone with no fake addresses produces the same empty result
// from both functions.
func TestMethod4and5VsZoneNameserversAgreeOnEmptyZone(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	methodsv2.ClearCache()
	t.Cleanup(methodsv2.ClearCache)

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

	m45, err := Method4and5(ctx, &z)
	if err != nil {
		t.Fatalf("method4and5: %v", err)
	}
	zoneItems, err := methodsv2.GetZoneNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("getZoneNSNamesAndIPs: %v", err)
	}
	if len(m45) != 0 {
		t.Errorf("Method4and5 expected empty, got %#v", m45)
	}
	if len(zoneItems) != 0 {
		t.Errorf("GetZoneNSNamesAndIPs expected empty, got %#v", zoneItems)
	}
}
