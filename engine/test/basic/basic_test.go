package basic

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestBasic01Root(t *testing.T) {
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	z := zone.Zone{Name: dnsname.New(".")}
	entries, err := Basic01(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected log entries")
	}
	if entries[0].Tag != "TEST_CASE_START" {
		t.Fatalf("expected TEST_CASE_START, got %q", entries[0].Tag)
	}
	if entries[len(entries)-1].Tag != "TEST_CASE_END" {
		t.Fatalf("expected TEST_CASE_END, got %q", entries[len(entries)-1].Tag)
	}
	if !hasEntryTag(entries, "B01_CHILD_FOUND") {
		t.Fatalf("expected B01_CHILD_FOUND")
	}
	if !hasEntryTag(entries, "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("expected B01_ROOT_HAS_NO_PARENT")
	}
}

func TestBasic01Undelegated(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic01(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	if !hasEntryTag(entries, "B01_CHILD_FOUND") {
		t.Fatalf("expected B01_CHILD_FOUND")
	}
	if !hasEntryTag(entries, "B01_PARENT_DISREGARDED") {
		t.Fatalf("expected B01_PARENT_DISREGARDED")
	}
}

func TestBasic02NoDelegation(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic02(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	if !hasEntryTag(entries, "B02_NO_DELEGATION") {
		t.Fatalf("expected B02_NO_DELEGATION")
	}
	if entries[0].Tag != "TEST_CASE_START" || entries[len(entries)-1].Tag != "TEST_CASE_END" {
		t.Fatalf("expected test case start/end markers")
	}
}

func TestBasic02AuthResponseSOA(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	root, err := nameserver.New("a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "NS":
			return nsPacket(".", "a.root"), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic02(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	if !hasEntryTag(entries, "B02_AUTH_RESPONSE_SOA") {
		t.Fatalf("expected B02_AUTH_RESPONSE_SOA")
	}
}

func TestBasic02UnexpectedRcode(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	root, err := nameserver.New("a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "NS":
			return nsPacket(".", "a.root"), nil
		case name == "." && kind == "SOA":
			return rcodePacket(dns.RcodeServerFailure), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic02(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	if !hasEntryTag(entries, "B02_NO_WORKING_NS") {
		t.Fatalf("expected B02_NO_WORKING_NS")
	}
	if !hasEntryTag(entries, "B02_UNEXPECTED_RCODE") {
		t.Fatalf("expected B02_UNEXPECTED_RCODE")
	}
}

func TestBasic02NoIPAddress(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	root, err := nameserver.New("a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		if name == "." && kind == "NS" {
			return nsPacket(".", "b.root"), nil
		}
		return packet.Packet{}, nil
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic02(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	if !hasEntryTag(entries, "B02_NS_NO_IP_ADDR") {
		t.Fatalf("expected B02_NS_NO_IP_ADDR")
	}
}

func TestBasic03HasARecords(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	root, err := nameserver.New("a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && (kind == "SOA" || kind == "NS"):
			return referralPacket("example", "ns1.example", net.IPv4(192, 0, 2, 53)), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	ns1, err := nameserver.New("ns1.example", "192.0.2.53", r.Client())
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		case name == "www.example" && kind == "A":
			return aPacket("www.example", net.IPv4(192, 0, 2, 99)), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic03(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	if !hasEntryTag(entries, "HAS_A_RECORDS") {
		t.Fatalf("expected HAS_A_RECORDS")
	}
	if hasEntryTag(entries, "A_QUERY_NO_RESPONSES") {
		t.Fatalf("unexpected A_QUERY_NO_RESPONSES")
	}
}

func TestBasic03NoARecords(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	root, err := nameserver.New("a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && (kind == "SOA" || kind == "NS"):
			return referralPacket("example", "ns1.example", net.IPv4(192, 0, 2, 53)), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	ns1, err := nameserver.New("ns1.example", "192.0.2.53", r.Client())
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		case name == "www.example" && kind == "A":
			return emptyAnswerPacket(), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic03(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	if !hasEntryTag(entries, "NO_A_RECORDS") {
		t.Fatalf("expected NO_A_RECORDS")
	}
	if hasEntryTag(entries, "HAS_A_RECORDS") {
		t.Fatalf("unexpected HAS_A_RECORDS")
	}
}

func TestBasic03NoResponses(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	root, err := nameserver.New("a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && (kind == "SOA" || kind == "NS"):
			return referralPacket("example", "ns1.example", net.IPv4(192, 0, 2, 53)), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	ns1, err := nameserver.New("ns1.example", "192.0.2.53", r.Client())
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		if name == "example" && kind == "SOA" {
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		}
		return packet.Packet{}, nil
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic03(context.Background(), &z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	if !hasEntryTag(entries, "A_QUERY_NO_RESPONSES") {
		t.Fatalf("expected A_QUERY_NO_RESPONSES")
	}
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

func soaPacket(owner string, mname string, rname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns:      dns.Fqdn(mname),
			Mbox:    dns.Fqdn(rname),
			Serial:  1,
			Refresh: 3600,
			Retry:   600,
			Expire:  86400,
			Minttl:  60,
		},
	}
	return packet.Packet{Msg: msg}
}

func referralPacket(zoneName string, nsName string, nsAddr net.IP) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Ns = []dns.RR{
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
	msg.Extra = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(nsName),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: nsAddr,
		},
	}
	return packet.Packet{Msg: msg}
}

func aPacket(owner string, addr net.IP) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: addr,
		},
	}
	return packet.Packet{Msg: msg}
}

func nsPacket(owner string, nsname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsname),
		},
	}
	return packet.Packet{Msg: msg}
}

func rcodePacket(rcode int) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = rcode
	return packet.Packet{Msg: msg}
}

func emptyAnswerPacket() packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	return packet.Packet{Msg: msg}
}
