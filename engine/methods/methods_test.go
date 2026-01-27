package methods

import (
	"context"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func nsAnswerPacket(zoneName string, nsNames ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	for _, nsName := range nsNames {
		msg.Answer = append(msg.Answer, &dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsName),
		})
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

func setNSHook(t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) {
	t.Helper()
	ns, err := nameserver.New(name, addr, r.Client())
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
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = true
	profile.Effective().Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	})
	setNSHook(t, r, "a.root", "192.0.2.1", ".", "a.root", "b.root")
	setNSHook(t, r, "b.root", "192.0.2.2", ".", "B.ROOT", "c.root")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method3(context.Background(), &z)
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

func TestMethod2and3UnionSorted(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = true
	profile.Effective().Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.3"},
		"b.root": {"192.0.2.4"},
	})
	setNSHook(t, r, "a.root", "192.0.2.3", ".", "a.root", "b.root")
	setNSHook(t, r, "b.root", "192.0.2.4", ".", "c.root")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := Method2and3(context.Background(), &z)
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
	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = true
	profile.Effective().Net.IPv6 = true

	r := newRootRecursor(t, map[string][]string{
		"a.root": {"192.0.2.5"},
		"b.root": {"192.0.2.6"},
		"c.root": {"192.0.2.7"},
	})
	setNSHook(t, r, "a.root", "192.0.2.5", ".", "a.root", "b.root")
	setNSHook(t, r, "b.root", "192.0.2.6", ".", "a.root", "b.root")
	setNSHook(t, r, "c.root", "192.0.2.7", ".", "a.root", "b.root")

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	out, err := Method4and5(context.Background(), &z)
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
