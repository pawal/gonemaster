package methods

import (
	"context"
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
