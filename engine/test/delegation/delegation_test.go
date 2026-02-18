package delegation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestDelegation01Counts(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM2 := method2
	origM3 := method3
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method2 = origM2
		method3 = origM3
		method4 = origM4
		method5 = origM5
	})

	method2 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	}
	method3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.1", nil)
		ns2 := newNameserver(t, "ns2.example", "2001:db8::1", nil)
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.2", nil)
		return []nameserver.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation01(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation01: %v", err)
	}
	if !hasEntryTag(entries, "ENOUGH_NS_DEL") {
		t.Fatalf("expected ENOUGH_NS_DEL")
	}
	if !hasEntryTag(entries, "NOT_ENOUGH_NS_CHILD") {
		t.Fatalf("expected NOT_ENOUGH_NS_CHILD")
	}
	if !hasEntryTag(entries, "NOT_ENOUGH_IPV4_NS_DEL") {
		t.Fatalf("expected NOT_ENOUGH_IPV4_NS_DEL")
	}
	if !hasEntryTag(entries, "NOT_ENOUGH_IPV6_NS_DEL") {
		t.Fatalf("expected NOT_ENOUGH_IPV6_NS_DEL")
	}
	if !hasEntryTag(entries, "NOT_ENOUGH_IPV4_NS_CHILD") {
		t.Fatalf("expected NOT_ENOUGH_IPV4_NS_CHILD")
	}
	if !hasEntryTag(entries, "NO_IPV6_NS_CHILD") {
		t.Fatalf("expected NO_IPV6_NS_CHILD")
	}
}

func TestDelegation02DuplicateIPs(t *testing.T) {
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

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.1", nil)
		ns2 := newNameserver(t, "ns2.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns3 := newNameserver(t, "ns3.example", "192.0.2.2", nil)
		return []nameserver.Nameserver{ns3}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation02(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation02: %v", err)
	}
	if !hasEntryTag(entries, "DEL_NS_SAME_IP") {
		t.Fatalf("expected DEL_NS_SAME_IP")
	}
	if !hasEntryTag(entries, "CHILD_DISTINCT_NS_IP") {
		t.Fatalf("expected CHILD_DISTINCT_NS_IP")
	}
	if !hasEntryTag(entries, "SAME_IP_ADDRESS") {
		t.Fatalf("expected SAME_IP_ADDRESS")
	}
}

func TestDelegation03ReferralSizeOK(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM1 := method1
	origM2 := method2
	origM4 := method4
	t.Cleanup(func() {
		method1 = origM1
		method2 = origM2
		method4 = origM4
	})

	method1 = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		parent := zone.Zone{Name: dnsname.New(".")}
		return &parent, nil
	}
	method2 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation03(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation03: %v", err)
	}
	if !hasEntryTag(entries, "REFERRAL_SIZE_OK") {
		t.Fatalf("expected REFERRAL_SIZE_OK")
	}
}

func TestDelegation04Authoritative(t *testing.T) {
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

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "SOA") {
				return soaPacket("example", true)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation04(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation04: %v", err)
	}
	if !hasEntryTag(entries, "ARE_AUTHORITATIVE") {
		t.Fatalf("expected ARE_AUTHORITATIVE")
	}
}

func TestDelegation04NotAuthoritative(t *testing.T) {
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

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "SOA") {
				return soaPacket("example", false)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation04(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation04: %v", err)
	}
	if !hasEntryTag(entries, "IS_NOT_AUTHORITATIVE") {
		t.Fatalf("expected IS_NOT_AUTHORITATIVE")
	}
}

func TestDelegation04ParallelQueries(t *testing.T) {
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

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, _ string, qtype string, _ string, opts *nameserver.QueryOptions) (packet.Packet, error) {
			if strings.EqualFold(qtype, "SOA") && opts != nil && opts.UseVC != nil && !*opts.UseVC {
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
			}
			return soaPacket("example", false), nil
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

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
	var delErr error
	go func() {
		entries, delErr = Delegation04(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel SOA queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if delErr != nil {
			t.Fatalf("delegation04: %v", delErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("delegation04 did not finish")
	}

	var order []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "IS_NOT_AUTHORITATIVE" {
			continue
		}
		ns, _ := entry.Args["ns"].(string)
		proto, _ := entry.Args["proto"].(string)
		order = append(order, ns+"|"+proto)
	}
	if len(order) != 4 {
		t.Fatalf("expected 4 not-authoritative entries, got %v", order)
	}
	if order[0] != "ns1.example/192.0.2.1|UDP" || order[1] != "ns1.example/192.0.2.1|TCP" ||
		order[2] != "ns2.example/192.0.2.2|UDP" || order[3] != "ns2.example/192.0.2.2|TCP" {
		t.Fatalf("expected deterministic log order, got %v", order)
	}
}

func TestDelegation05InBailiwickCNAME(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := method2and3
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method2and3 = origM23
		method4 = origM4
		method5 = origM5
	})

	method2and3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "A") && strings.EqualFold(qname, "ns1.example") {
				return cnamePacket(qname, "alias.example")
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation05(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation05: %v", err)
	}
	if !hasEntryTag(entries, "NS_IS_CNAME") {
		t.Fatalf("expected NS_IS_CNAME")
	}
	if hasEntryTag(entries, "NO_NS_CNAME") {
		t.Fatalf("did not expect NO_NS_CNAME")
	}
}

func TestDelegation05ParallelQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := method2and3
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		method2and3 = origM23
		method4 = origM4
		method5 = origM5
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if strings.EqualFold(qtype, "A") && strings.EqualFold(qname, "ns1.example") {
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return packet.Packet{}, nil
			}
			return packet.Packet{}, nil
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method2and3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
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
	var delErr error
	go func() {
		entries, delErr = Delegation05(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel A queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if delErr != nil {
			t.Fatalf("delegation05: %v", delErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("delegation05 did not finish")
	}

	var order []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "NO_RESPONSE" {
			continue
		}
		if ns, ok := entry.Args["ns"].(string); ok {
			order = append(order, ns)
		}
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 no-response entries, got %v", order)
	}
	if order[0] != "ns1.example/192.0.2.1" || order[1] != "ns2.example/192.0.2.2" {
		t.Fatalf("expected deterministic log order, got %v", order)
	}
}

func TestDelegation05OutOfBailiwickNoCNAME(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := method2and3
	origM4 := method4
	origM5 := method5
	origRecurse := recurse
	t.Cleanup(func() {
		method2and3 = origM23
		method4 = origM4
		method5 = origM5
		recurse = origRecurse
	})

	method2and3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.other")}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}
	recurse = func(_ context.Context, _ *zone.Zone, _ string, _ string) (packet.Packet, error) {
		return packet.Packet{Msg: new(dns.Msg)}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation05(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation05: %v", err)
	}
	if !hasEntryTag(entries, "NO_NS_CNAME") {
		t.Fatalf("expected NO_NS_CNAME")
	}
}

func TestDelegation06SOANotExists(t *testing.T) {
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

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "SOA") {
				return noAnswerPacket()
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation06(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation06: %v", err)
	}
	if !hasEntryTag(entries, "SOA_NOT_EXISTS") {
		t.Fatalf("expected SOA_NOT_EXISTS")
	}
}

func TestDelegation06SOAExists(t *testing.T) {
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

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "SOA") {
				return soaPacket("example", true)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation06(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation06: %v", err)
	}
	if !hasEntryTag(entries, "SOA_EXISTS") {
		t.Fatalf("expected SOA_EXISTS")
	}
}

func TestDelegation07NameMismatch(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM2 := method2
	origM3 := method3
	t.Cleanup(func() {
		method2 = origM2
		method3 = origM3
	})

	method2 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	}
	method3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns2.example"), dnsname.New("ns3.example")}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation07(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation07: %v", err)
	}
	if !hasEntryTag(entries, "EXTRA_NAME_PARENT") {
		t.Fatalf("expected EXTRA_NAME_PARENT")
	}
	if !hasEntryTag(entries, "EXTRA_NAME_CHILD") {
		t.Fatalf("expected EXTRA_NAME_CHILD")
	}
}

func TestDelegation07NamesMatch(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM2 := method2
	origM3 := method3
	t.Cleanup(func() {
		method2 = origM2
		method3 = origM3
	})

	method2 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	method3 = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation07(context.Background(), &z)
	if err != nil {
		t.Fatalf("delegation07: %v", err)
	}
	if !hasEntryTag(entries, "NAMES_MATCH") {
		t.Fatalf("expected NAMES_MATCH")
	}
}

func TestDelegation07UndelegatedReportsExtraNameChild(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	origM2 := method2
	origM3 := method3
	t.Cleanup(func() {
		method2 = origM2
		method3 = origM3
	})
	method2 = methods.Method2
	method3 = methods.Method3

	r, err := recursor.New()
	if err != nil {
		t.Fatalf("new recursor: %v", err)
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
		"ns3.example.com": {"192.0.2.13"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	makeNSAnswer := func(nsNames ...string) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		for _, nsName := range nsNames {
			msg.Answer = append(msg.Answer, &dns.NS{
				Hdr: dns.RR_Header{
					Name:   dns.Fqdn("example.com"),
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				Ns: dns.Fqdn(nsName),
			})
		}
		return packet.Packet{Msg: msg}
	}

	setHook := func(name, ip string, nsNames ...string) {
		t.Helper()
		ns, err := nameserver.NewWithContext(ctx, name, ip, r.Client())
		if err != nil {
			t.Fatalf("new nameserver: %v", err)
		}
		ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if !strings.EqualFold(qname, "example.com") || !strings.EqualFold(qtype, "NS") {
				return packet.Packet{}, nil
			}
			return makeNSAnswer(nsNames...), nil
		})
	}

	setHook("ns1.example.com", "192.0.2.11", "ns1.example.com", "ns4.example.com")
	setHook("ns2.example.com", "192.0.2.12", "ns1.example.com", "ns5.example.com")
	setHook("ns3.example.com", "192.0.2.13", "ns4.example.com")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Delegation07(ctx, &z)
	if err != nil {
		t.Fatalf("delegation07: %v", err)
	}
	if !hasEntryTag(entries, "EXTRA_NAME_PARENT") {
		t.Fatalf("expected EXTRA_NAME_PARENT")
	}
	if !hasEntryTag(entries, "EXTRA_NAME_CHILD") {
		t.Fatalf("expected EXTRA_NAME_CHILD")
	}
	if hasEntryTag(entries, "NAMES_MATCH") {
		t.Fatalf("did not expect NAMES_MATCH")
	}
}

func newNameserver(t *testing.T, name string, ip string, handler func(qname string, qtype string, opts *nameserver.QueryOptions) packet.Packet) nameserver.Nameserver {
	t.Helper()

	ns, err := nameserver.New(name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(_ string, _ string, _ *nameserver.QueryOptions) packet.Packet {
			return packet.Packet{}
		}
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype, opts), nil
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

func soaPacket(owner string, authoritative bool) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = authoritative
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns:      "ns1.example.",
			Mbox:    "hostmaster.example.",
			Serial:  1,
			Refresh: 3600,
			Retry:   600,
			Expire:  86400,
			Minttl:  60,
		},
	}
	return packet.Packet{Msg: msg}
}

func cnamePacket(owner string, target string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Target: dns.Fqdn(target),
		},
	}
	return packet.Packet{Msg: msg}
}

func noAnswerPacket() packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	return packet.Packet{Msg: msg}
}
