package zone

import (
	"context"
	"math/rand"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

// Tests for Zone.ApexNSNames and Zone.GlueNames covering apex NS extraction,
// case folding, deduplication, error propagation, and property invariants.

func apexNsAnswerPacket(zoneName string, nsNames ...string) packet.Packet {
	return dnstest.Response(dnstest.NotAuthoritative(),
		dnstest.Answers(dnstest.NSRRs(zoneName, nsNames...)...))
}

// apexSetNSHook registers a nameserver answering NS queries at the zone apex
// with the given NS names.
func apexSetNSHook(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) {
	t.Helper()
	nstest.HookedNS(t, ctx, r, name, addr,
		nstest.AnswerHook(zoneName, "NS", apexNsAnswerPacket(zoneName, nsNames...)))
}

// apexSetHookWithPacket registers a nameserver answering NS queries at the zone
// apex with a caller-supplied packet.
func apexSetHookWithPacket(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, p packet.Packet) {
	t.Helper()
	nstest.HookedNS(t, ctx, r, name, addr, nstest.AnswerHook(zoneName, "NS", p))
}

// TestZoneGlueNamesReturnsGlueFromZone verifies that GlueNames returns the
// names registered via the recursor's fake glue for an undelegated zone.
func TestZoneGlueNamesReturnsGlueFromZone(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".": {"a.root": {"192.0.2.1"}, "b.root": {"192.0.2.2"}},
		"example.com": {
			"ns1.example.com": {"192.0.2.11"},
			"ns2.example.com": {"192.0.2.12"},
		},
	})

	z := newZone(t, "example.com", r)

	names, err := z.GlueNames(ctx)
	if err != nil {
		t.Fatalf("GlueNames: %v", err)
	}
	want := []string{"ns1.example.com", "ns2.example.com"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d: %#v", len(want), len(names), names)
	}
	for i, name := range names {
		if name.String() != want[i] {
			t.Fatalf("expected %q at %d, got %q", want[i], i, name.String())
		}
	}
}

// TestZoneApexNSNamesNoNSRecordsReturnsEmpty verifies that when authoritative
// servers reply successfully but include no NS RRs at the apex, ApexNSNames
// returns an empty slice and no error.
func TestZoneApexNSNamesNoNSRecordsReturnsEmpty(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	empty := packet.Packet{Msg: new(dns.Msg)}
	apexSetHookWithPacket(ctx, t, r, "a.root", "192.0.2.1", ".", empty)

	z := newZone(t, ".", r)

	names, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("ApexNSNames: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty slice, got %#v", names)
	}
}

// TestZoneApexNSNamesQueryAllErrorPropagates verifies that an error from
// z.QueryAll (triggered by a zone with no recursor) is returned to the caller.
func TestZoneApexNSNamesQueryAllErrorPropagates(t *testing.T) {
	z := Zone{Name: dnsname.New("example.com.")}
	if _, err := z.ApexNSNames(context.Background()); err == nil {
		t.Fatalf("expected error from QueryAll/NS, got nil")
	}
}

// TestZoneApexNSNamesSkipsNonNSRecords verifies that A and SOA records mixed
// into the apex response are ignored; only NS records contribute.
func TestZoneApexNSNamesSkipsNonNSRecords(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	apexSetHookWithPacket(ctx, t, r, "a.root", "192.0.2.1", ".",
		dnstest.MixedApexRecords(".", []string{"a.root", "b.root"}))

	z := newZone(t, ".", r)

	names, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("ApexNSNames: %v", err)
	}
	want := []string{"a.root", "b.root"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d: %#v", len(want), len(names), names)
	}
	for i, name := range names {
		if name.String() != want[i] {
			t.Fatalf("expected %q at %d, got %q", want[i], i, name.String())
		}
	}
}

// TestZoneApexNSNamesSkipsNilMsgResponses verifies that servers returning a
// packet with a nil Msg are skipped silently.
func TestZoneApexNSNamesSkipsNilMsgResponses(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	apexSetHookWithPacket(ctx, t, r, "a.root", "192.0.2.1", ".", packet.Packet{Msg: nil})
	apexSetNSHook(ctx, t, r, "b.root", "192.0.2.2", ".", "a.root", "b.root")

	z := newZone(t, ".", r)

	names, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("ApexNSNames: %v", err)
	}
	want := []string{"a.root", "b.root"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d: %#v", len(want), len(names), names)
	}
	for i, name := range names {
		if name.String() != want[i] {
			t.Fatalf("expected %q at %d, got %q", want[i], i, name.String())
		}
	}
}

// TestZoneApexNSNamesDedupesAcrossMultipleServers verifies that when two
// servers return partially overlapping NS sets, the union (deduplicated) is
// returned.
func TestZoneApexNSNamesDedupesAcrossMultipleServers(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	apexSetNSHook(ctx, t, r, "a.root", "192.0.2.1", ".", "ns1.example", "ns2.example")
	apexSetNSHook(ctx, t, r, "b.root", "192.0.2.2", ".", "ns2.example", "ns3.example")

	z := newZone(t, ".", r)

	names, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("ApexNSNames: %v", err)
	}
	want := []string{"ns1.example", "ns2.example", "ns3.example"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d: %#v", len(want), len(names), names)
	}
	for i, name := range names {
		if name.String() != want[i] {
			t.Fatalf("expected %q at %d, got %q", want[i], i, name.String())
		}
	}
}

// TestZoneApexNSNamesCaseFoldedDeduplication verifies that NS records
// differing only in letter case are collapsed to a single lowercase entry.
func TestZoneApexNSNamesCaseFoldedDeduplication(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	apexSetNSHook(ctx, t, r, "a.root", "192.0.2.1", ".", "NS1.Example.")
	apexSetNSHook(ctx, t, r, "b.root", "192.0.2.2", ".", "ns1.example.")

	z := newZone(t, ".", r)

	names, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("ApexNSNames: %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("expected 1 name (case-folded), got %d: %#v", len(names), names)
	}
	if names[0].String() != "ns1.example" {
		t.Fatalf("expected %q, got %q", "ns1.example", names[0].String())
	}
}

// TestZoneApexNSNamesDedupAndSort verifies that ApexNSNames returns
// deduplicated, sorted names.
func TestZoneApexNSNamesDedupAndSort(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	apexSetNSHook(ctx, t, r, "a.root", "192.0.2.1", ".", "a.root", "b.root")
	apexSetNSHook(ctx, t, r, "b.root", "192.0.2.2", ".", "B.ROOT", "c.root")

	z := newZone(t, ".", r)

	names, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("ApexNSNames: %v", err)
	}
	want := []string{"a.root", "b.root", "c.root"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d", len(want), len(names))
	}
	for i, name := range names {
		if name.String() != want[i] {
			t.Fatalf("expected %q at %d, got %q", want[i], i, name.String())
		}
	}
}

// TestZoneApexNSNamesUndelegatedUsesApexRecords verifies that for an
// undelegated zone with apex servers returning distinct NS sets, ApexNSNames
// returns the union while GlueNames returns only the configured glue.
func TestZoneApexNSNamesUndelegatedUsesApexRecords(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".": {"a.root": {"192.0.2.1"}, "b.root": {"192.0.2.2"}},
		"example.com": {
			"ns1.example.com": {"192.0.2.11"},
			"ns2.example.com": {"192.0.2.12"},
			"ns3.example.com": {"192.0.2.13"},
		},
	})

	apexSetNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns4.example.com")
	apexSetNSHook(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns5.example.com")
	apexSetNSHook(ctx, t, r, "ns3.example.com", "192.0.2.13", "example.com", "ns4.example.com")

	z := newZone(t, "example.com", r)

	parentNames, err := z.GlueNames(ctx)
	if err != nil {
		t.Fatalf("GlueNames: %v", err)
	}
	childNames, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("ApexNSNames: %v", err)
	}

	parentWant := []string{"ns1.example.com", "ns2.example.com", "ns3.example.com"}
	if len(parentNames) != len(parentWant) {
		t.Fatalf("expected %d parent names, got %d", len(parentWant), len(parentNames))
	}
	for i, name := range parentNames {
		if name.String() != parentWant[i] {
			t.Fatalf("expected parent %q at %d, got %q", parentWant[i], i, name.String())
		}
	}

	childWant := []string{"ns1.example.com", "ns4.example.com", "ns5.example.com"}
	if len(childNames) != len(childWant) {
		t.Fatalf("expected %d child names, got %d", len(childWant), len(childNames))
	}
	for i, name := range childNames {
		if name.String() != childWant[i] {
			t.Fatalf("expected child %q at %d, got %q", childWant[i], i, name.String())
		}
	}
}

// runZoneApexNSNamesProperty sets up a zone where one apex server returns a
// random NS-name list and returns ApexNSNames's output as a string slice.
func runZoneApexNSNamesProperty(t *testing.T, names []string) []string {
	t.Helper()

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".":           {"a.root": {"192.0.2.1"}},
		"example.com": {"ns1.example.com": {"192.0.2.11"}},
	})
	apexSetNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", names...)

	z := newZone(t, "example.com", r)

	got, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("ApexNSNames: %v", err)
	}
	out := make([]string, 0, len(got))
	for _, n := range got {
		out = append(out, n.String())
	}
	return out
}

// TestZoneApexNSNamesPropertyAlwaysSortedAndDeduped runs 30 seeded scenarios
// with random NS name lists (varying sizes, duplicates, mixed case) and
// asserts the output is always lowercase-sorted and contains no duplicates.
func TestZoneApexNSNamesPropertyAlwaysSortedAndDeduped(t *testing.T) {
	for trial := 0; trial < 30; trial++ {
		seed := int64(trial * 17)
		rng := rand.New(rand.NewSource(seed))
		names := nstest.RandomNameSet(rng, 1+rng.Intn(8), "example.com")
		t.Run("trial", func(t *testing.T) {
			out := runZoneApexNSNamesProperty(t, names)
			if !nstest.IsSortedLowercase(out) {
				t.Errorf("output not sorted: %#v (input: %#v)", out, names)
			}
			if !nstest.HasNoDuplicates(out) {
				t.Errorf("output has duplicates: %#v (input: %#v)", out, names)
			}
		})
	}
}
