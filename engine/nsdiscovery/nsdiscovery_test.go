package nsdiscovery

import (
	"context"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// TestGlueNameserversReturnsGlueFromZone verifies that GlueNameservers
// returns nameserver objects (name + IP) for the zone's glue.
func TestGlueNameserversReturnsGlueFromZone(t *testing.T) {

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

	out, err := GlueNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("GlueNameservers: %v", err)
	}
	want := []string{
		"ns1.example.com/192.0.2.11",
		"ns2.example.com/192.0.2.12",
	}
	got := make([]string, 0, len(out))
	for _, ns := range out {
		got = append(got, ns.String())
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d nameservers, got %d: %#v", len(want), len(got), got)
	}
	// Glue order from the recursor is not strictly guaranteed, so allow any order.
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Fatalf("missing %q in %#v", w, got)
		}
	}
}

// TestGlueNameserversNilZoneReturnsError verifies the nil-zone guard.
func TestGlueNameserversNilZoneReturnsError(t *testing.T) {
	if _, err := GlueNameservers(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

// TestApexNameserversReturnsApexNameservers verifies that ApexNameservers
// resolves nameserver objects from the child zone's apex NS RRset.
func TestApexNameserversReturnsApexNameservers(t *testing.T) {

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
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns2.example.com")
	setNSHook(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns2.example.com")

	z := newZone(t, "example.com", r)

	out, err := ApexNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ApexNameservers: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected non-empty nameserver set")
	}
	seenName := map[string]bool{}
	for _, ns := range out {
		seenName[ns.Name.String()] = true
	}
	for _, name := range []string{"ns1.example.com", "ns2.example.com"} {
		if !seenName[name] {
			t.Fatalf("missing nameserver %q in %#v", name, out)
		}
	}
}

// TestApexNameserversNilZoneReturnsError verifies the nil-zone guard.
func TestApexNameserversNilZoneReturnsError(t *testing.T) {
	if _, err := ApexNameservers(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

// TestAllNSNamesUnionSorted verifies that the union of glue names and apex
// NS names is deduplicated and sorted.
func TestAllNSNamesUnionSorted(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.3"},
		"b.root": {"192.0.2.4"},
	})
	setNSHook(ctx, t, r, "a.root", "192.0.2.3", ".", "a.root", "b.root")
	setNSHook(ctx, t, r, "b.root", "192.0.2.4", ".", "c.root")

	z := newZone(t, ".", r)

	names, err := AllNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("AllNSNames: %v", err)
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

// TestAllNSNamesEmptyInputs verifies that when neither glue nor apex NS
// queries return any names, the union is an empty slice.
func TestAllNSNamesEmptyInputs(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".":           {"a.root": {"192.0.2.1"}},
		"example.com": {},
	})

	z := newZone(t, "example.com", r)

	names, err := AllNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("AllNSNames: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty slice, got %#v", names)
	}
}

// TestAllNSNamesOnlyGlueWhenApexReturnsNoNS verifies that when glue names
// exist but apex NS lookups return empty, the union equals the glue set.
func TestAllNSNamesOnlyGlueWhenApexReturnsNoNS(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".": {"a.root": {"192.0.2.1"}},
		"example.com": {
			"ns1.example.com": {"192.0.2.11"},
			"ns2.example.com": {"192.0.2.12"},
		},
	})
	noNS := packet.Packet{Msg: new(dns.Msg)}
	setHookWithPacket(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", noNS)
	setHookWithPacket(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", noNS)

	z := newZone(t, "example.com", r)

	names, err := AllNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("AllNSNames: %v", err)
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

// TestAllNSNamesOverlapDedupedCaseInsensitively verifies that names appearing
// in both the glue set and the apex set, differing only by case, collapse to
// a single lowercased entry.
func TestAllNSNamesOverlapDedupedCaseInsensitively(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".":           {"a.root": {"192.0.2.1"}},
		"example.com": {"ns1.example.com": {"192.0.2.11"}},
	})
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "NS1.Example.com.")

	z := newZone(t, "example.com", r)

	names, err := AllNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("AllNSNames: %v", err)
	}
	if len(names) != 1 || names[0].String() != "ns1.example.com" {
		t.Fatalf("expected single lowercase ns1.example.com, got %#v", names)
	}
}

// TestAllNSNamesPropagatesError verifies that an error from glue lookup
// propagates and apex lookup is not consulted.
func TestAllNSNamesPropagatesError(t *testing.T) {
	z := zone.Zone{Name: dnsname.New("example.com.")}
	if _, err := AllNSNames(context.Background(), &z); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestAllNameserversUnionSorted verifies that AllNameservers returns the
// deduplicated, sorted union of glue and apex nameservers.
func TestAllNameserversUnionSorted(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.5"},
		"b.root": {"192.0.2.6"},
		"c.root": {"192.0.2.7"},
	})
	setNSHook(ctx, t, r, "a.root", "192.0.2.5", ".", "a.root", "b.root")
	setNSHook(ctx, t, r, "b.root", "192.0.2.6", ".", "a.root", "b.root")
	setNSHook(ctx, t, r, "c.root", "192.0.2.7", ".", "a.root", "b.root")

	z := newZone(t, ".", r)

	out, err := AllNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("AllNameservers: %v", err)
	}
	want := []string{
		"a.root/192.0.2.5",
		"b.root/192.0.2.6",
		"c.root/192.0.2.7",
	}
	if len(out) != len(want) {
		t.Fatalf("expected %d nameservers, got %d", len(want), len(out))
	}
	for i, ns := range out {
		if ns.String() != want[i] {
			t.Fatalf("expected %q at %d, got %q", want[i], i, ns.String())
		}
	}
}

// TestAllNameserversEmptyInputs verifies the empty-input branch.
func TestAllNameserversEmptyInputs(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".":           {"a.root": {"192.0.2.1"}},
		"example.com": {},
	})

	z := newZone(t, "example.com", r)

	out, err := AllNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("AllNameservers: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %#v", out)
	}
}

// TestAllNameserversOnlyGlueWhenApexHasNoServers verifies that when glue
// nameservers exist but apex returns no NS servers, the union equals the
// glue set.
func TestAllNameserversOnlyGlueWhenApexHasNoServers(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".":           {"a.root": {"192.0.2.1"}},
		"example.com": {"ns1.example.com": {"192.0.2.11"}},
	})
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com")

	z := newZone(t, "example.com", r)

	out, err := AllNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("AllNameservers: %v", err)
	}
	if len(out) != 1 || out[0].String() != "ns1.example.com/192.0.2.11" {
		t.Fatalf("expected single ns1.example.com/192.0.2.11, got %#v", out)
	}
}

// TestAllNameserversDedupesByNameserverString verifies that a name+IP pair
// appearing identically in both glue and apex sets yields one entry.
func TestAllNameserversDedupesByNameserverString(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{
		".": {"a.root": {"192.0.2.1"}},
		"example.com": {
			"ns1.example.com": {"192.0.2.11"},
			"ns2.example.com": {"192.0.2.12"},
		},
	})
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns2.example.com")
	setNSHook(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns2.example.com")

	z := newZone(t, "example.com", r)

	out, err := AllNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("AllNameservers: %v", err)
	}
	want := []string{
		"ns1.example.com/192.0.2.11",
		"ns2.example.com/192.0.2.12",
	}
	got := make([]string, 0, len(out))
	for _, ns := range out {
		got = append(got, ns.String())
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d entries (deduped), got %d: %#v", len(want), len(got), got)
	}
	for i, g := range got {
		if g != want[i] {
			t.Fatalf("expected %q at %d, got %q", want[i], i, g)
		}
	}
}

// TestAllNameserversPropagatesError verifies that an error during glue
// resolution propagates from glue lookup without consulting apex lookup.
func TestAllNameserversPropagatesError(t *testing.T) {
	z := zone.Zone{Name: dnsname.New("example.com.")}
	if _, err := AllNameservers(context.Background(), &z); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// mixedRecordsPacket exercise: verify ApexNameservers via ApexNSNames silently
// ignores non-NS records.
func TestApexNSNamesSkipsNonNSRecords(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.RootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	setHookWithPacket(ctx, t, r, "a.root", "192.0.2.1", ".",
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
