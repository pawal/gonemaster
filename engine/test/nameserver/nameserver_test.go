package nameserver

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	ens "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestNameserver01RecursorAndNoRecursor(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	nsRecursor := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		return packet.Packet{Msg: msg}
	})
	nsNoRecursor := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsRecursor, nsNoRecursor}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	if !hasEntryTag(entries, "IS_A_RECURSOR") {
		t.Fatalf("expected IS_A_RECURSOR")
	}
	if !hasEntryTag(entries, "NO_RECURSOR") {
		t.Fatalf("expected NO_RECURSOR")
	}
}

func TestNameserver02EDNS0Support(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.3", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacketWithEdns("example", 0, 0, nil)
		}
		return packet.Packet{}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver02(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver02: %v", err)
	}
	if !hasEntryTag(entries, "EDNS0_SUPPORT") {
		t.Fatalf("expected EDNS0_SUPPORT")
	}
}

func TestNameserver03AXFRAvailable(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.4", nil)
	soaRR := soaRecord("example")
	ns1.SetAXFRHook(func(_ context.Context, _ string, callback func(dns.RR) bool, _ string) error {
		if callback != nil {
			callback(soaRR)
		}
		return nil
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver03(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver03: %v", err)
	}
	if !hasEntryTag(entries, "AXFR_AVAILABLE") {
		t.Fatalf("expected AXFR_AVAILABLE")
	}
}

func TestNameserver04DifferentSourceIP(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.5", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg, AnswerFrom: "192.0.2.99:53"}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver04(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver04: %v", err)
	}
	if !hasEntryTag(entries, "DIFFERENT_SOURCE_IP") {
		t.Fatalf("expected DIFFERENT_SOURCE_IP")
	}
}

func TestNameserver05AAAAWellProcessed(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.6", func(qname string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		switch strings.ToUpper(qtype) {
		case "A":
			return aPacket(qname, "192.0.2.10")
		case "AAAA":
			return aaaaPacket(qname, "2001:db8::1")
		}
		return packet.Packet{}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver05(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver05: %v", err)
	}
	if !hasEntryTag(entries, "AAAA_WELL_PROCESSED") {
		t.Fatalf("expected AAAA_WELL_PROCESSED")
	}
}

func TestNameserver06NotResolved(t *testing.T) {
	setupTest(t)

	origM2 := method2
	origM3 := method3
	origM4and5 := method4and5
	t.Cleanup(func() {
		method2 = origM2
		method3 = origM3
		method4and5 = origM4and5
	})

	method2 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	method3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns2.example")}, nil
	}
	ns1 := newNameserver(t, "ns1.example", "192.0.2.7", nil)
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver06(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver06: %v", err)
	}
	if !hasEntryTag(entries, "CAN_NOT_BE_RESOLVED") {
		t.Fatalf("expected CAN_NOT_BE_RESOLVED")
	}
}

func TestNameserver07NoUpwardReferral(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.8", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver07(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver07: %v", err)
	}
	if !hasEntryTag(entries, "NO_UPWARD_REFERRAL") {
		t.Fatalf("expected NO_UPWARD_REFERRAL")
	}
}

func TestNameserver08QNameCaseInsensitive(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.9", func(qname string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Question = []dns.Question{{
			Name:   dns.Fqdn(strings.ToLower(qname)),
			Qtype:  dns.TypeSOA,
			Qclass: dns.ClassINET,
		}}
		return packet.Packet{Msg: msg}
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver08(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver08: %v", err)
	}
	if !hasEntryTag(entries, "QNAME_CASE_INSENSITIVE") {
		t.Fatalf("expected QNAME_CASE_INSENSITIVE")
	}
}

func TestNameserver09CaseQueriesSameAnswer(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.10", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacket("example")
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver09(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver09: %v", err)
	}
	if !hasEntryTag(entries, "CASE_QUERY_SAME_ANSWER") {
		t.Fatalf("expected CASE_QUERY_SAME_ANSWER")
	}
	if !hasEntryTag(entries, "CASE_QUERIES_RESULTS_OK") {
		t.Fatalf("expected CASE_QUERIES_RESULTS_OK")
	}
}

func TestNameserver10NoResponseEDNS1(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.11", func(_ string, _ string, _ string, opts *ens.QueryOptions) packet.Packet {
		if opts != nil && opts.EDNSDetails != nil && opts.EDNSDetails.Version != nil {
			if *opts.EDNSDetails.Version == 0 {
				return soaPacket("example")
			}
			if *opts.EDNSDetails.Version == 1 {
				return packet.Packet{}
			}
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver10(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver10: %v", err)
	}
	if !hasEntryTag(entries, "N10_NO_RESPONSE_EDNS1_QUERY") {
		t.Fatalf("expected N10_NO_RESPONSE_EDNS1_QUERY")
	}
}

func TestNameserver11ReturnsUnknownOption(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.12", func(_ string, _ string, _ string, opts *ens.QueryOptions) packet.Packet {
		if opts != nil && opts.EDNSDetails != nil && len(opts.EDNSDetails.Data) > 0 {
			return soaPacketWithEdns("example", 0, 0, []dns.EDNS0{&dns.EDNS0_LOCAL{Code: 137}})
		}
		return soaPacketWithEdns("example", 0, 0, nil)
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver11(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver11: %v", err)
	}
	if !hasEntryTag(entries, "N11_RETURNS_UNKNOWN_OPTION_CODE") {
		t.Fatalf("expected N11_RETURNS_UNKNOWN_OPTION_CODE")
	}
}

func TestNameserver12ZFlagsNotClear(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.13", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacketWithEdns("example", 0, 3, nil)
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver12(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver12: %v", err)
	}
	if !hasEntryTag(entries, "Z_FLAGS_NOTCLEAR") {
		t.Fatalf("expected Z_FLAGS_NOTCLEAR")
	}
}

func TestNameserver13MissingOptInTruncated(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.14", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Truncated = true
		return packet.Packet{Msg: msg}
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver13(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver13: %v", err)
	}
	if !hasEntryTag(entries, "MISSING_OPT_IN_TRUNCATED") {
		t.Fatalf("expected MISSING_OPT_IN_TRUNCATED")
	}
}

func TestNameserver15SoftwareVersionAndWrongClass(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.15", func(qname string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		switch strings.ToUpper(qtype) {
		case "SOA":
			return soaPacket("example")
		case "TXT":
			if strings.EqualFold(qname, "version.bind") {
				return txtPacket("version.bind", "bind 9", dns.ClassINET)
			}
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver15(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver15: %v", err)
	}
	if !hasEntryTag(entries, "N15_SOFTWARE_VERSION") {
		t.Fatalf("expected N15_SOFTWARE_VERSION")
	}
	if !hasEntryTag(entries, "N15_WRONG_CLASS") {
		t.Fatalf("expected N15_WRONG_CLASS")
	}
}

func setupTest(t *testing.T) {
	t.Helper()

	ens.EmptyCache()
	t.Cleanup(ens.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })
}

func newNameserver(t *testing.T, name string, ip string, handler func(qname string, qtype string, qclass string, opts *ens.QueryOptions) packet.Packet) ens.Nameserver {
	t.Helper()

	ns, err := ens.New(name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
			return packet.Packet{}
		}
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, qclass string, opts *ens.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype, qclass, opts), nil
	})
	return ns
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

func soaRecord(owner string) dns.RR {
	return &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(owner),
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Ns:      dns.Fqdn("ns1.example"),
		Mbox:    dns.Fqdn("hostmaster.example"),
		Serial:  1,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minttl:  60,
	}
}

func soaMsg(owner string) *dns.Msg {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{soaRecord(owner)}
	return msg
}

func soaPacket(owner string) packet.Packet {
	return packet.Packet{Msg: soaMsg(owner)}
}

func soaPacketWithEdns(owner string, version uint8, z uint16, options []dns.EDNS0) packet.Packet {
	msg := soaMsg(owner)
	msg.SetEdns0(1232, false)
	if opt := msg.IsEdns0(); opt != nil {
		opt.SetVersion(version)
		if z != 0 {
			opt.SetZ(z)
		}
		if len(options) > 0 {
			opt.Option = append(opt.Option, options...)
		}
	}
	return packet.Packet{Msg: msg}
}

func aPacket(name string, address string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: net.ParseIP(address).To4(),
		},
	}
	return packet.Packet{Msg: msg}
}

func aaaaPacket(name string, address string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			AAAA: net.ParseIP(address),
		},
	}
	return packet.Packet{Msg: msg}
}

func txtPacket(name string, value string, class uint16) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.TXT{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeTXT,
				Class:  class,
				Ttl:    60,
			},
			Txt: []string{value},
		},
	}
	return packet.Packet{Msg: msg}
}
