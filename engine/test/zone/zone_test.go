package zone

import (
	"context"
	"net/netip"
	"testing"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/dnsname"
	"github.com/pawal/gonemaster/engine/logger"
	methodsv2 "github.com/pawal/gonemaster/engine/methodsv2"
	ens "github.com/pawal/gonemaster/engine/nameserver"
	"github.com/pawal/gonemaster/engine/packet"
	"github.com/pawal/gonemaster/engine/profile"
	"github.com/pawal/gonemaster/engine/util"
	zonepkg "github.com/pawal/gonemaster/engine/zone"
)

func TestZone02RefreshBelowMinimum(t *testing.T) {
	setupTest(t)

	origMethod5 := method5
	t.Cleanup(func() { method5 = origMethod5 })

	profile.Effective().TestCasesVars.Zone02.SOARefreshMinimumValue = 1000

	ns := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacket("example", 1, 500, 50, 7200, 60)
	})
	method5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone02(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone02: %v", err)
	}
	if !hasEntryTag(entries, "REFRESH_MINIMUM_VALUE_LOWER") {
		t.Fatalf("expected REFRESH_MINIMUM_VALUE_LOWER")
	}
}

func TestZone05ExpireLowerThanRefreshAndMinimum(t *testing.T) {
	setupTest(t)

	origMethod5 := method5
	t.Cleanup(func() { method5 = origMethod5 })

	profile.Effective().TestCasesVars.Zone05.SOAExpireMinimumValue = 2000

	ns := newNameserver(t, "ns1.example", "192.0.2.2", func(_ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacket("example", 1, 1500, 100, 1000, 60)
	})
	method5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone05(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone05: %v", err)
	}
	if !hasEntryTag(entries, "EXPIRE_MINIMUM_VALUE_LOWER") {
		t.Fatalf("expected EXPIRE_MINIMUM_VALUE_LOWER")
	}
	if !hasEntryTag(entries, "EXPIRE_LOWER_THAN_REFRESH") {
		t.Fatalf("expected EXPIRE_LOWER_THAN_REFRESH")
	}
}

func TestZone11SpfSyntaxError(t *testing.T) {
	setupTest(t)

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	z, err := zonepkg.New("example.com")
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}

	newNameserver(t, "ns1.example.com", "192.0.2.10", func(qname string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype != "TXT" {
			return packet.Packet{}
		}
		return txtPacket(qname, "v=spf1 amx-all")
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zonepkg.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{{
			Name:       dnsname.New("ns1.example.com"),
			Address:    netip.MustParseAddr("192.0.2.10"),
			HasAddress: true,
		}}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zonepkg.Zone) ([]methodsv2.NSItem, error) {
		return nil, nil
	}

	entries, err := Zone11(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}
	if !hasEntryTag(entries, "Z11_SPF_SYNTAX_ERROR") {
		t.Fatalf("expected Z11_SPF_SYNTAX_ERROR")
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

func newNameserver(t *testing.T, name string, ip string, handler func(qname string, qtype string, opts *ens.QueryOptions) packet.Packet) ens.Nameserver {
	t.Helper()

	ns, err := ens.New(name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(_ string, _ string, _ *ens.QueryOptions) packet.Packet {
			return packet.Packet{}
		}
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, opts *ens.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype, opts), nil
	})
	return ns
}

func soaPacket(owner string, serial uint32, refresh uint32, retry uint32, expire uint32, minimum uint32) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(owner), dns.TypeSOA)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns:      dns.Fqdn("ns1.example"),
			Mbox:    dns.Fqdn("hostmaster.example"),
			Serial:  serial,
			Refresh: refresh,
			Retry:   retry,
			Expire:  expire,
			Minttl:  minimum,
		},
	}
	return packet.Packet{Msg: msg}
}

func txtPacket(name string, value string) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), dns.TypeTXT)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.TXT{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Txt: []string{value},
		},
	}
	return packet.Packet{Msg: msg}
}
