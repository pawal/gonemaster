package consistency

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestConsistency01MultipleSerials(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method4 = origM4
		method5 = origM5
	})

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 2, "ns2.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency01(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency01: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_SOA_SERIALS") {
		t.Fatalf("expected MULTIPLE_SOA_SERIALS")
	}
	if !hasEntryTag(entries, "SOA_SERIAL_VARIATION") {
		t.Fatalf("expected SOA_SERIAL_VARIATION")
	}
	if !hasEntryTag(entries, "SOA_SERIAL") {
		t.Fatalf("expected SOA_SERIAL")
	}
}

func TestConsistency02MultipleRnames(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method4 = origM4
		method5 = origM5
	})

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns2.example", "admin.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency02(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency02: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_SOA_RNAMES") {
		t.Fatalf("expected MULTIPLE_SOA_RNAMES")
	}
	if !hasEntryTag(entries, "SOA_RNAME") {
		t.Fatalf("expected SOA_RNAME")
	}
}

func TestConsistency03MultipleTimeSets(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method4 = origM4
		method5 = origM5
	})

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns2.example", "hostmaster.example", 7200, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency03(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency03: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_SOA_TIME_PARAMETER_SET") {
		t.Fatalf("expected MULTIPLE_SOA_TIME_PARAMETER_SET")
	}
}

func TestConsistency04MultipleNSSets(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method4 = origM4
		method5 = origM5
	})

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "NS") {
			return nsPacket("example", []string{"ns1.example"})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "NS") {
			return nsPacket("example", []string{"ns1.example", "ns2.example"})
		}
		return packet.Packet{}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency04(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency04: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_NS_SET") {
		t.Fatalf("expected MULTIPLE_NS_SET")
	}
	if !hasEntryTag(entries, "NS_SET") {
		t.Fatalf("expected NS_SET")
	}
}

func TestConsistency04ParallelNSQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method4 = origM4
		method5 = origM5
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string, nsNames []string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if strings.EqualFold(qtype, "NS") {
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return nsPacket(qname, nsNames), nil
			}
			return packet.Packet{}, nil
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1", []string{"ns1.example"}))

	ns2, err := nameserver.New("ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2", []string{"ns1.example", "ns2.example"}))

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var consErr error
	go func() {
		entries, consErr = Consistency04(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel NS queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if consErr != nil {
			t.Fatalf("consistency04: %v", consErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("consistency04 did not finish")
	}

	var order []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "NS_SET" {
			continue
		}
		if servers, ok := entry.Args["servers"].(string); ok {
			order = append(order, servers)
		}
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 NS_SET entries, got %v", order)
	}
	if order[0] != "ns1.example/192.0.2.1" || order[1] != "ns2.example/192.0.2.2" {
		t.Fatalf("expected deterministic log order, got %v", order)
	}
}

func TestConsistency05AddressesMatch(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := method2and3
	origM45 := method4and5
	origParent := queryParentAll
	t.Cleanup(func() {
		method2and3 = origM23
		method4and5 = origM45
		queryParentAll = origParent
	})

	method2and3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	}

	authNS := newNameserver(t, "auth.example", "192.0.2.53", func(qname string, qtype string) packet.Packet {
		switch strings.ToUpper(qtype) {
		case "A":
			switch strings.ToLower(qname) {
			case "ns1.example":
				return addrPacket(qname, "A", "192.0.2.1")
			case "ns2.example":
				return addrPacket(qname, "A", "192.0.2.2")
			}
		case "AAAA":
			switch strings.ToLower(qname) {
			case "ns1.example":
				return addrPacket(qname, "AAAA", "2001:db8::1")
			case "ns2.example":
				return addrPacket(qname, "AAAA", "2001:db8::2")
			}
		}
		return packet.Packet{}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{authNS}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "NS":
			return []packet.Packet{nsPacket(name, []string{"ns1.example", "ns2.example"})}, nil
		case "A":
			switch strings.ToLower(name) {
			case "ns1.example":
				return []packet.Packet{addrPacket(name, "A", "192.0.2.1")}, nil
			case "ns2.example":
				return []packet.Packet{addrPacket(name, "A", "192.0.2.2")}, nil
			}
		case "AAAA":
			switch strings.ToLower(name) {
			case "ns1.example":
				return []packet.Packet{addrPacket(name, "AAAA", "2001:db8::1")}, nil
			case "ns2.example":
				return []packet.Packet{addrPacket(name, "AAAA", "2001:db8::2")}, nil
			}
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if !hasEntryTag(entries, "ADDRESSES_MATCH") {
		t.Fatalf("expected ADDRESSES_MATCH")
	}
}

func TestConsistency05ChildZoneLame(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := method2and3
	origM45 := method4and5
	origParent := queryParentAll
	t.Cleanup(func() {
		method2and3 = origM23
		method4and5 = origM45
		queryParentAll = origParent
	})

	method2and3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}

	nonAAPacket := func() packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	}

	authNS := newNameserver(t, "auth.example", "192.0.2.53", func(_ string, _ string) packet.Packet {
		return nonAAPacket()
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{authNS}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, _ string, _ string) ([]packet.Packet, error) {
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if !hasEntryTag(entries, "CHILD_NS_FAILED") {
		t.Fatalf("expected CHILD_NS_FAILED")
	}
	entry := firstEntryByTag(entries, "CHILD_NS_FAILED")
	if _, ok := entry.Args["arg_schema"]; ok {
		t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
	}
	if ns, ok := entry.Args["ns"].(string); !ok || ns != "auth.example" {
		t.Fatalf("expected CHILD_NS_FAILED ns=auth.example, got %#v", entry.Args["ns"])
	}
	if ns, _ := entry.Args["ns"].(string); strings.Contains(ns, "/") {
		t.Fatalf("expected nameserver-only ns argument, got %q", ns)
	}
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.53" {
		t.Fatalf("expected CHILD_NS_FAILED address=192.0.2.53, got %#v", entry.Args["address"])
	}
	if !hasEntryTag(entries, "CHILD_ZONE_LAME") {
		t.Fatalf("expected CHILD_ZONE_LAME")
	}
}

func TestConsistency05InBailiwickMismatch(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := method2and3
	origM45 := method4and5
	origParent := queryParentAll
	t.Cleanup(func() {
		method2and3 = origM23
		method4and5 = origM45
		queryParentAll = origParent
	})

	method2and3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}

	authNS := newNameserver(t, "auth.example", "192.0.2.53", func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "A") && strings.EqualFold(qname, "ns1.example") {
			return addrPacket(qname, "A", "192.0.2.2")
		}
		return packet.Packet{}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{authNS}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "NS":
			return []packet.Packet{nsPacket(name, []string{"ns1.example"})}, nil
		case "A":
			if strings.EqualFold(name, "ns1.example") {
				return []packet.Packet{addrPacket(name, "A", "192.0.2.1")}, nil
			}
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if !hasEntryTag(entries, "IN_BAILIWICK_ADDR_MISMATCH") {
		t.Fatalf("expected IN_BAILIWICK_ADDR_MISMATCH")
	}
	if !hasEntryTag(entries, "EXTRA_ADDRESS_CHILD") {
		t.Fatalf("expected EXTRA_ADDRESS_CHILD")
	}
}

func TestConsistency05OutOfBailiwickMismatch(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := method2and3
	origM45 := method4and5
	origParent := queryParentAll
	origRecurse := recurse
	t.Cleanup(func() {
		method2and3 = origM23
		method4and5 = origM45
		queryParentAll = origParent
		recurse = origRecurse
	})

	method2and3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{}, nil
	}
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "NS":
			return []packet.Packet{nsPacket(name, []string{"ns1.other"})}, nil
		case "A":
			if strings.EqualFold(name, "ns1.other") {
				return []packet.Packet{addrPacket(name, "A", "192.0.2.1")}, nil
			}
		}
		return []packet.Packet{}, nil
	}

	recurse = func(_ context.Context, _ *zone.Zone, _ string, _ string) (packet.Packet, error) {
		return packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if !hasEntryTag(entries, "OUT_OF_BAILIWICK_ADDR_MISMATCH") {
		t.Fatalf("expected OUT_OF_BAILIWICK_ADDR_MISMATCH")
	}
}

func TestConsistency06MultipleMnames(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method4 = origM4
		method5 = origM5
	})

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "mname1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "mname2.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency06(context.Background(), &z)
	if err != nil {
		t.Fatalf("consistency06: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_SOA_MNAMES") {
		t.Fatalf("expected MULTIPLE_SOA_MNAMES")
	}
	if !hasEntryTag(entries, "SOA_MNAME") {
		t.Fatalf("expected SOA_MNAME")
	}
}

func newNameserver(t *testing.T, name string, ip string, handler func(qname string, qtype string) packet.Packet) nameserver.Nameserver {
	t.Helper()

	ns, err := nameserver.New(name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(_ string, _ string) packet.Packet {
			return packet.Packet{}
		}
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype), nil
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

func firstEntryByTag(entries []*logger.Entry, tag string) *logger.Entry {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag == tag {
			return entry
		}
	}
	return nil
}

func soaPacket(owner string, serial uint32, mname string, rname string, refresh uint32, retry uint32, expire uint32, minimum uint32) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = dnsutil.Fqdn(mname)
	soaRR.Mbox = dnsutil.Fqdn(rname)
	soaRR.Serial = serial
	soaRR.Refresh = refresh
	soaRR.Retry = retry
	soaRR.Expire = expire
	soaRR.Minttl = minimum
	msg.Answer = []dns.RR{soaRR}
	return packet.Packet{Msg: msg}
}

func nsPacket(owner string, nsNames []string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	for _, nsName := range nsNames {
		nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
		nsRR.Ns = dnsutil.Fqdn(nsName)
		msg.Answer = append(msg.Answer, nsRR)
	}
	return packet.Packet{Msg: msg}
}

func addrPacket(name string, qtype string, address string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true

	addr, err := netip.ParseAddr(address)
	if err != nil {
		return packet.Packet{}
	}
	switch strings.ToUpper(qtype) {
	case "A":
		aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}}
		aRR.Addr = addr.Unmap()
		msg.Answer = []dns.RR{aRR}
	case "AAAA":
		aaaaRR := &dns.AAAA{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}}
		aaaaRR.Addr = addr
		msg.Answer = []dns.RR{aaaaRR}
	default:
		return packet.Packet{}
	}
	return packet.Packet{Msg: msg}
}
