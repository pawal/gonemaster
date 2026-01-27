package address

import (
	"context"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestAddress01DocumentationAddr(t *testing.T) {
	z := newZoneWithFakeAddresses(t, "example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	})

	entries, err := Address01(context.Background(), z)
	if err != nil {
		t.Fatalf("address01: %v", err)
	}
	if !hasEntryTag(entries, "A01_DOCUMENTATION_ADDR") {
		t.Fatalf("expected A01_DOCUMENTATION_ADDR")
	}
	if !hasEntryTag(entries, "A01_NO_GLOBALLY_REACHABLE_ADDR") {
		t.Fatalf("expected A01_NO_GLOBALLY_REACHABLE_ADDR")
	}
	if hasEntryTag(entries, "A01_GLOBALLY_REACHABLE_ADDR") {
		t.Fatalf("did not expect A01_GLOBALLY_REACHABLE_ADDR")
	}
}

func TestAddress01NoNameServersFound(t *testing.T) {
	z := newZoneWithFakeAddresses(t, "example", map[string][]string{
		"ns.other": {},
	})

	entries, err := Address01(context.Background(), z)
	if err != nil {
		t.Fatalf("address01: %v", err)
	}
	if !hasEntryTag(entries, "A01_NO_NAME_SERVERS_FOUND") {
		t.Fatalf("expected A01_NO_NAME_SERVERS_FOUND")
	}
}

func TestAddress02NameserversIPWithReverse(t *testing.T) {
	ptrName, err := dns.ReverseAddr("192.0.2.1")
	if err != nil {
		t.Fatalf("reverse addr: %v", err)
	}

	z := newRootZoneWithHook(t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(qname, ptrName) && strings.EqualFold(qtype, "PTR") {
			return ptrPacket(qname, "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Address02(context.Background(), z)
	if err != nil {
		t.Fatalf("address02: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVERS_IP_WITH_REVERSE") {
		t.Fatalf("expected NAMESERVERS_IP_WITH_REVERSE")
	}
}

func TestAddress02NameserverIPWithoutReverse(t *testing.T) {
	ptrName, err := dns.ReverseAddr("192.0.2.1")
	if err != nil {
		t.Fatalf("reverse addr: %v", err)
	}

	z := newRootZoneWithHook(t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(qname, ptrName) && strings.EqualFold(qtype, "PTR") {
			return noAnswerPacket(qname, "PTR")
		}
		return packet.Packet{}
	})

	entries, err := Address02(context.Background(), z)
	if err != nil {
		t.Fatalf("address02: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_IP_WITHOUT_REVERSE") {
		t.Fatalf("expected NAMESERVER_IP_WITHOUT_REVERSE")
	}
}

func TestAddress03PTRMatch(t *testing.T) {
	ptrName, err := dns.ReverseAddr("192.0.2.1")
	if err != nil {
		t.Fatalf("reverse addr: %v", err)
	}

	z := newRootZoneWithHook(t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(qname, ptrName) && strings.EqualFold(qtype, "PTR") {
			return ptrPacket(qname, "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Address03(context.Background(), z)
	if err != nil {
		t.Fatalf("address03: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_IP_PTR_MATCH") {
		t.Fatalf("expected NAMESERVER_IP_PTR_MATCH")
	}
}

func TestAddress03PTRMismatch(t *testing.T) {
	ptrName, err := dns.ReverseAddr("192.0.2.1")
	if err != nil {
		t.Fatalf("reverse addr: %v", err)
	}

	z := newRootZoneWithHook(t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(qname, ptrName) && strings.EqualFold(qtype, "PTR") {
			return ptrPacket(qname, "ptr.example.")
		}
		return packet.Packet{}
	})

	entries, err := Address03(context.Background(), z)
	if err != nil {
		t.Fatalf("address03: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_IP_PTR_MISMATCH") {
		t.Fatalf("expected NAMESERVER_IP_PTR_MISMATCH")
	}
}

func newRootZoneWithHook(t *testing.T, handler func(qname string, qtype string) packet.Packet) *zone.Zone {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	ns, err := nameserver.New("a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype), nil
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	return &z
}

func newZoneWithFakeAddresses(t *testing.T, zoneName string, data map[string][]string) *zone.Zone {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(zoneName, data); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z, err := zone.NewWithRecursor(zoneName, r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	return &z
}

func nsPacket(zoneName string, nsName string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsName),
		},
	}
	return packet.Packet{Msg: msg}
}

func ptrPacket(owner string, targets ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	for _, target := range targets {
		msg.Answer = append(msg.Answer, &dns.PTR{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypePTR,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ptr: dns.Fqdn(target),
		})
	}
	return packet.Packet{Msg: msg}
}

func noAnswerPacket(owner string, qtype string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	if qtype == "" {
		qtype = "A"
	}
	msg.Question = []dns.Question{{
		Name:   dns.Fqdn(owner),
		Qtype:  dns.StringToType[strings.ToUpper(qtype)],
		Qclass: dns.ClassINET,
	}}
	return packet.Packet{Msg: msg}
}

func hasEntryTag(entries []*logger.Entry, tag string) bool {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag == tag {
			return true
		}
	}
	return false
}
