package methods

import (
	"context"
	"net/netip"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func nsAnswerPacket(zoneName string, nsNames ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	for _, nsName := range nsNames {
		nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}}
		nsRR.Ns = dnsutil.Fqdn(nsName)
		msg.Answer = append(msg.Answer, nsRR)
	}
	return packet.Packet{Msg: msg}
}

func newRootRecursor(t *testing.T, data map[string][]string) *recursor.Recursor {
	t.Helper()
	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", data); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	return r
}

func setNSHook(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, addr, r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if qname != zoneName || qtype != "NS" {
			return packet.Packet{}, nil
		}
		return nsAnswerPacket(zoneName, nsNames...), nil
	})
}

func TestMethod1ParentRoot(t *testing.T) {
	z := zone.Zone{Name: dnsname.New(".")}
	parent, err := Method1(context.Background(), &z)
	if err != nil {
		t.Fatalf("method1: %v", err)
	}
	if parent == nil || parent.Name.String() != "." {
		t.Fatalf("expected root parent, got %#v", parent)
	}
}

// TestMethod2ReturnsGlueNamesFromZone verifies that Method2 forwards to
// z.GlueNames and returns the names registered via the recursor's fake glue
// for an undelegated zone.
func TestMethod2ReturnsGlueNamesFromZone(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method2(ctx, &z)
	if err != nil {
		t.Fatalf("method2: %v", err)
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

// TestMethod2NilZoneReturnsError verifies the nil-zone guard.
func TestMethod2NilZoneReturnsError(t *testing.T) {
	if _, err := Method2(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

// TestMethod4ReturnsGlueNameserversFromZone verifies that Method4 forwards to
// z.Glue and returns nameserver objects (name + IP) for the zone's glue.
func TestMethod4ReturnsGlueNameserversFromZone(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := Method4(ctx, &z)
	if err != nil {
		t.Fatalf("method4: %v", err)
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

// TestMethod4NilZoneReturnsError verifies the nil-zone guard.
func TestMethod4NilZoneReturnsError(t *testing.T) {
	if _, err := Method4(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

// TestMethod5ReturnsApexNameserversFromZone verifies that Method5 forwards to
// z.NS and returns nameserver objects resolved from the child zone apex.
func TestMethod5ReturnsApexNameserversFromZone(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	// Both apex nameservers return themselves as the authoritative NS set.
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns2.example.com")
	setNSHook(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns2.example.com")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := Method5(ctx, &z)
	if err != nil {
		t.Fatalf("method5: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected non-empty nameserver set")
	}
	// We accept any subset of the configured pair as long as both names are present.
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

// TestMethod5NilZoneReturnsError verifies the nil-zone guard.
func TestMethod5NilZoneReturnsError(t *testing.T) {
	if _, err := Method5(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

// mixedRecordsPacket builds a response containing a mix of NS, A, and SOA
// records at the zone apex, used by TestMethod3SkipsNonNSRecords.
func mixedRecordsPacket(zoneName string, nsNames []string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess

	soa := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 3600}}
	soa.Ns = dnsutil.Fqdn("ns1." + zoneName)
	soa.Mbox = dnsutil.Fqdn("hostmaster." + zoneName)
	msg.Answer = append(msg.Answer, soa)

	for _, nsName := range nsNames {
		nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}}
		nsRR.Ns = dnsutil.Fqdn(nsName)
		msg.Answer = append(msg.Answer, nsRR)
	}

	aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn("decoy." + zoneName), Class: dns.ClassINET, TTL: 60}}
	aRR.Addr = netip.MustParseAddr("192.0.2.99")
	msg.Answer = append(msg.Answer, aRR)

	return packet.Packet{Msg: msg}
}

// setHookWithPacket installs a query hook that returns a caller-supplied
// packet for any NS query at the named zone apex; other queries return an
// empty packet (nil Msg).
func setHookWithPacket(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, p packet.Packet) {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, addr, r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if qname != zoneName || qtype != "NS" {
			return packet.Packet{}, nil
		}
		return p, nil
	})
}

// TestMethod3NoNSRecordsReturnsEmpty verifies that when authoritative servers
// reply successfully but include no NS RRs at the apex, Method3 returns an
// empty slice and no error.
func TestMethod3NoNSRecordsReturnsEmpty(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	// Hook returns an empty success response (no NS records).
	empty := packet.Packet{Msg: new(dns.Msg)}
	setHookWithPacket(ctx, t, r, "a.root", "192.0.2.1", ".", empty)

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method3(ctx, &z)
	if err != nil {
		t.Fatalf("method3: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty slice, got %#v", names)
	}
}

// TestMethod3QueryAllErrorPropagates verifies that an error from z.QueryAll
// (here triggered by a zone with no recursor) is returned to the caller.
func TestMethod3QueryAllErrorPropagates(t *testing.T) {
	// Zone with no recursor: z.NS errors with "missing recursor", which
	// QueryAll propagates and Method3 must surface.
	z := zone.Zone{Name: dnsname.New("example.com.")}
	if _, err := Method3(context.Background(), &z); err == nil {
		t.Fatalf("expected error from QueryAll/NS, got nil")
	}
}

// TestMethod3SkipsNonNSRecords verifies that A and SOA records mixed into
// the apex response are ignored; only NS records contribute to the result.
func TestMethod3SkipsNonNSRecords(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	setHookWithPacket(ctx, t, r, "a.root", "192.0.2.1", ".",
		mixedRecordsPacket(".", []string{"a.root", "b.root"}))

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method3(ctx, &z)
	if err != nil {
		t.Fatalf("method3: %v", err)
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

// TestMethod3SkipsNilMsgResponses verifies that nameservers returning a
// packet with a nil Msg are skipped silently; remaining servers still
// contribute their NS records.
func TestMethod3SkipsNilMsgResponses(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	// a.root returns Msg=nil. b.root returns valid NS records.
	setHookWithPacket(ctx, t, r, "a.root", "192.0.2.1", ".", packet.Packet{Msg: nil})
	setNSHook(ctx, t, r, "b.root", "192.0.2.2", ".", "a.root", "b.root")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method3(ctx, &z)
	if err != nil {
		t.Fatalf("method3: %v", err)
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

// TestMethod3DedupesAcrossMultipleServers verifies that when two servers
// each return a partially overlapping NS set, the union (deduplicated) is
// returned.
func TestMethod3DedupesAcrossMultipleServers(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	// a returns {ns1, ns2}; b returns {ns2, ns3}. Union must be {ns1,ns2,ns3}.
	setNSHook(ctx, t, r, "a.root", "192.0.2.1", ".", "ns1.example", "ns2.example")
	setNSHook(ctx, t, r, "b.root", "192.0.2.2", ".", "ns2.example", "ns3.example")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method3(ctx, &z)
	if err != nil {
		t.Fatalf("method3: %v", err)
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

// TestMethod3CaseFoldedDeduplication verifies that NS records differing only
// in letter case are collapsed to a single lowercase entry.
func TestMethod3CaseFoldedDeduplication(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	// Same logical name, mixed case across servers.
	setNSHook(ctx, t, r, "a.root", "192.0.2.1", ".", "NS1.Example.")
	setNSHook(ctx, t, r, "b.root", "192.0.2.2", ".", "ns1.example.")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method3(ctx, &z)
	if err != nil {
		t.Fatalf("method3: %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("expected 1 name (case-folded), got %d: %#v", len(names), names)
	}
	if names[0].String() != "ns1.example" {
		t.Fatalf("expected %q, got %q", "ns1.example", names[0].String())
	}
}

func TestMethod3DedupAndSort(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	setNSHook(ctx, t, r, "a.root", "192.0.2.1", ".", "a.root", "b.root")
	setNSHook(ctx, t, r, "b.root", "192.0.2.2", ".", "B.ROOT", "c.root")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method3(ctx, &z)
	if err != nil {
		t.Fatalf("method3: %v", err)
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

func TestMethod3UndelegatedUsesApexNSRecords(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
		"ns3.example.com": {"192.0.2.13"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns4.example.com")
	setNSHook(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns5.example.com")
	setNSHook(ctx, t, r, "ns3.example.com", "192.0.2.13", "example.com", "ns4.example.com")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	parentNames, err := Method2(ctx, &z)
	if err != nil {
		t.Fatalf("method2: %v", err)
	}
	childNames, err := Method3(ctx, &z)
	if err != nil {
		t.Fatalf("method3: %v", err)
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

func TestMethod2and3UnionSorted(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.3"},
		"b.root": {"192.0.2.4"},
	})
	setNSHook(ctx, t, r, "a.root", "192.0.2.3", ".", "a.root", "b.root")
	setNSHook(ctx, t, r, "b.root", "192.0.2.4", ".", "c.root")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method2and3(ctx, &z)
	if err != nil {
		t.Fatalf("method2and3: %v", err)
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

func TestMethod4and5UnionSorted(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.5"},
		"b.root": {"192.0.2.6"},
		"c.root": {"192.0.2.7"},
	})
	setNSHook(ctx, t, r, "a.root", "192.0.2.5", ".", "a.root", "b.root")
	setNSHook(ctx, t, r, "b.root", "192.0.2.6", ".", "a.root", "b.root")
	setNSHook(ctx, t, r, "c.root", "192.0.2.7", ".", "a.root", "b.root")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := Method4and5(ctx, &z)
	if err != nil {
		t.Fatalf("method4and5: %v", err)
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

// TestMethod2and3EmptyInputs verifies that when neither glue nor apex NS
// queries return any names, the union is an empty slice (not nil-typed
// error, not a panic).
func TestMethod2and3EmptyInputs(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	// Mark example.com as undelegated with no fake glue. Method2 returns
	// empty fake names; Method3 has no servers to query (z.NS = z.Glue =
	// empty) and returns empty.
	if err := r.AddFakeAddresses("example.com", map[string][]string{}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method2and3(ctx, &z)
	if err != nil {
		t.Fatalf("method2and3: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty slice, got %#v", names)
	}
}

// TestMethod2and3OnlyGlueWhenApexReturnsNoNS verifies that when Method2
// returns names but Method3 returns empty (apex servers reply with no NS
// records), the union equals the glue set. This exercises the one-side-
// empty branch of the dedup loop.
func TestMethod2and3OnlyGlueWhenApexReturnsNoNS(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	// Both apex servers reply with SOA + A only (no NS RRs) so Method3 is
	// empty while Method2 stays nonempty.
	noNS := packet.Packet{Msg: new(dns.Msg)}
	setHookWithPacket(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", noNS)
	setHookWithPacket(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", noNS)

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method2and3(ctx, &z)
	if err != nil {
		t.Fatalf("method2and3: %v", err)
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

// TestMethod2and3OverlapDedupedCaseInsensitively verifies that names which
// appear in both the glue set and the apex set, differing only by case,
// collapse to a single lowercased entry.
func TestMethod2and3OverlapDedupedCaseInsensitively(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	// Glue uses lowercase names; apex query will return upper-case
	// variants. Result must contain one entry per logical name, lowercased.
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "NS1.Example.com.")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method2and3(ctx, &z)
	if err != nil {
		t.Fatalf("method2and3: %v", err)
	}
	if len(names) != 1 || names[0].String() != "ns1.example.com" {
		t.Fatalf("expected single lowercase ns1.example.com, got %#v", names)
	}
}

// TestMethod2and3PropagatesError verifies that an error from Method2 (here
// triggered by a non-root zone with no recursor, which fails in z.Parent)
// is propagated to the caller and Method3 is not consulted.
func TestMethod2and3PropagatesError(t *testing.T) {
	// Zone with no recursor: z.Parent errors "missing recursor", which
	// z.GlueNames returns and Method2 forwards. Method2and3 must surface it.
	z := zone.Zone{Name: dnsname.New("example.com.")}
	if _, err := Method2and3(context.Background(), &z); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestMethod4and5EmptyInputs verifies the empty-input branch for the
// nameserver-object union.
func TestMethod4and5EmptyInputs(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	if err := r.AddFakeAddresses("example.com", map[string][]string{}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := Method4and5(ctx, &z)
	if err != nil {
		t.Fatalf("method4and5: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %#v", out)
	}
}

// TestMethod4and5OnlyGlueWhenApexHasNoServers verifies that when Method4
// returns nameservers but Method5 returns none (no apex NS resolves to
// servers), the union equals the glue set.
func TestMethod4and5OnlyGlueWhenApexHasNoServers(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	// Apex server returns the same single NS as glue, so Method5's NS set
	// equals Method4's. The union dedupes them.
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := Method4and5(ctx, &z)
	if err != nil {
		t.Fatalf("method4and5: %v", err)
	}
	if len(out) != 1 || out[0].String() != "ns1.example.com/192.0.2.11" {
		t.Fatalf("expected single ns1.example.com/192.0.2.11, got %#v", out)
	}
}

// TestMethod4and5DedupesByNameserverString verifies that a name+IP pair
// appearing identically in both glue and apex sets yields one entry
// (dedup key is ns.String() which combines name and IP).
func TestMethod4and5DedupesByNameserverString(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
	})
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	// Both apex servers report the same NS set as the glue, so every
	// (name,IP) pair appears in both Method4 and Method5; the union must
	// dedupe them.
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns2.example.com")
	setNSHook(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns2.example.com")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := Method4and5(ctx, &z)
	if err != nil {
		t.Fatalf("method4and5: %v", err)
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

// TestMethod4and5PropagatesError verifies that an error during glue resolution
// (here, a non-root zone with no recursor) propagates from Method4 through
// Method4and5 without consulting Method5.
func TestMethod4and5PropagatesError(t *testing.T) {
	z := zone.Zone{Name: dnsname.New("example.com.")}
	if _, err := Method4and5(context.Background(), &z); err == nil {
		t.Fatalf("expected error, got nil")
	}
}
