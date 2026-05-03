package nameserver

import (
	"context"
	"fmt"
	"math/rand"
	"net/netip"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

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

func TestNameserver01NxdomainWithAANotRecursor(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	// Server returns NXDOMAIN with AA=1 on all probes (fake root authority).
	// This should NOT be classified as a recursor.
	nsFakeRoot := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		msg.Authoritative = true
		return packet.Packet{Msg: msg}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsFakeRoot}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	if hasEntryTag(entries, "IS_A_RECURSOR") {
		t.Fatalf("did not expect IS_A_RECURSOR for server with AA+NXDOMAIN")
	}
	if !hasEntryTag(entries, "NO_RECURSOR") {
		t.Fatalf("expected NO_RECURSOR for server with AA+NXDOMAIN")
	}
}

func TestNameserver01NxdomainWithRAAndAAIsRecursor(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	// Server returns NXDOMAIN with both RA=1 and AA=1.
	// RA takes precedence - should still be classified as a recursor.
	nsRAandAA := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		msg.Authoritative = true
		msg.RecursionAvailable = true
		return packet.Packet{Msg: msg}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsRAandAA}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	if !hasEntryTag(entries, "IS_A_RECURSOR") {
		t.Fatalf("expected IS_A_RECURSOR when RA is set even with AA")
	}
}

func TestNameserver01NxdomainMixedAAIsRecursor(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	// Server returns NXDOMAIN on all probes, but only some have AA=1.
	// Since not ALL NXDOMAIN responses are authoritative, classify as recursor.
	queryCount := 0
	nsMixed := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		queryCount++
		if queryCount <= 2 {
			msg.Authoritative = true
		}
		return packet.Packet{Msg: msg}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsMixed}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	if !hasEntryTag(entries, "IS_A_RECURSOR") {
		t.Fatalf("expected IS_A_RECURSOR when not all NXDOMAIN responses have AA")
	}
}

func TestNameserver01ParallelQueries(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	profile.Effective().Resolver.Defaults.Parallel = 2

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *ens.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *ens.QueryOptions) (packet.Packet, error) {
			if strings.EqualFold(qtype, "A") && strings.EqualFold(qname, nonExistentNames[0]) {
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
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeNameError
			return packet.Packet{Msg: msg}, nil
		}
	}

	ns1, err := ens.New("ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := ens.New("ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var nsErr error
	go func() {
		entries, nsErr = Nameserver01(ctx, &z)
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
		if nsErr != nil {
			t.Fatalf("nameserver01: %v", nsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("nameserver01 did not finish")
	}

	var found *logger.Entry
	for _, entry := range entries {
		if entry == nil || entry.Tag != "IS_A_RECURSOR" {
			continue
		}
		if found != nil {
			t.Fatalf("expected single consolidated IS_A_RECURSOR entry, got multiple")
		}
		found = entry
	}
	if found == nil {
		t.Fatalf("expected IS_A_RECURSOR entry, got none")
	}
	servers, ok := found.Args["servers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected servers array in IS_A_RECURSOR args, got %#v", found.Args)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers in consolidated entry, got %d", len(servers))
	}
	if servers[0]["ns"] != "ns1.example" || servers[1]["ns"] != "ns2.example" {
		t.Fatalf("expected sorted ns1/ns2, got %v", servers)
	}
	if servers[0]["address"] != "192.0.2.1" || servers[1]["address"] != "192.0.2.2" {
		t.Fatalf("expected addresses 192.0.2.1/192.0.2.2, got %v", servers)
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

func TestNameserver05ParallelQueries(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	profile.Effective().Resolver.Defaults.Parallel = 2

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *ens.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *ens.QueryOptions) (packet.Packet, error) {
			if strings.EqualFold(qtype, "A") && strings.EqualFold(qname, "example") {
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
			return packet.Packet{}, nil
		}
	}

	ns1, err := ens.New("ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := ens.New("ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var nsErr error
	go func() {
		entries, nsErr = Nameserver05(ctx, &z)
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
		if nsErr != nil {
			t.Fatalf("nameserver05: %v", nsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("nameserver05 did not finish")
	}

	var order []string
	var addresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "NO_RESPONSE" {
			continue
		}
		if _, ok := entry.Args["arg_schema"]; ok {
			t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
		}
		if ns, ok := entry.Args["ns"].(string); ok {
			if strings.Contains(ns, "/") {
				t.Fatalf("expected nameserver-only ns argument, got %q", ns)
			}
			order = append(order, ns)
		}
		if address, ok := entry.Args["address"].(string); ok {
			addresses = append(addresses, address)
		}
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 no-response entries, got %v", order)
	}
	if order[0] != "ns1.example" || order[1] != "ns2.example" {
		t.Fatalf("expected deterministic log order, got %v", order)
	}
	if len(addresses) != 2 || addresses[0] != "192.0.2.1" || addresses[1] != "192.0.2.2" {
		t.Fatalf("expected deterministic address order, got %v", addresses)
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
	entry := firstEntryByTag(entries, "CAN_NOT_BE_RESOLVED")
	if entry == nil {
		t.Fatalf("missing CAN_NOT_BE_RESOLVED entry")
	}
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns2.example" {
		t.Fatalf("expected typed unresolved nameserver list, got %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
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
	entry := firstEntryByTag(entries, "NO_UPWARD_REFERRAL")
	if entry == nil {
		t.Fatalf("missing NO_UPWARD_REFERRAL entry")
	}
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("expected typed nameserver list for NO_UPWARD_REFERRAL, got %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
}

func TestNameserver08QNameCaseInsensitive(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	origScramble := scrambleCaseFunc
	t.Cleanup(func() {
		method4and5 = origM4and5
		scrambleCaseFunc = origScramble
	})
	fixedRand := rand.New(rand.NewSource(42))
	scrambleCaseFunc = func(s string) string { return util.ScrambleCaseWith(s, fixedRand) }

	ns1 := newNameserver(t, "ns1.example", "192.0.2.9", func(qname string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(strings.ToLower(qname)), dns.TypeSOA)
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
	origScramble := scrambleCaseFunc
	t.Cleanup(func() {
		method4and5 = origM4and5
		scrambleCaseFunc = origScramble
	})
	fixedRand := rand.New(rand.NewSource(42))
	scrambleCaseFunc = func(s string) string { return util.ScrambleCaseWith(s, fixedRand) }

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

	var entry *logger.Entry
	for _, item := range entries {
		if item != nil && item.Tag == "N10_NO_RESPONSE_EDNS1_QUERY" {
			entry = item
			break
		}
	}
	if entry == nil {
		t.Fatalf("expected N10_NO_RESPONSE_EDNS1_QUERY entry payload")
	}
	if _, ok := entry.Args["ns_ip_list"]; ok {
		t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
	}
	addresses, ok := entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 1 {
		t.Fatalf("expected one typed address for N10_NO_RESPONSE_EDNS1_QUERY, got %#v", entry.Args["addresses"])
	}
	if strings.Join(addresses, ";") != "192.0.2.11" {
		t.Fatalf("expected deterministic addresses order, got %#v", addresses)
	}
}

func TestNameserver11ReturnsUnknownOption(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.12", func(_ string, _ string, _ string, opts *ens.QueryOptions) packet.Packet {
		if opts != nil && opts.EDNSDetails != nil && len(opts.EDNSDetails.Data) > 0 {
			return soaPacketWithEdns("example", 0, 0, []dns.EDNS0{&dns.ERFC3597{EDNS0Code: 137}})
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

	var entry *logger.Entry
	for _, item := range entries {
		if item != nil && item.Tag == "N11_RETURNS_UNKNOWN_OPTION_CODE" {
			entry = item
			break
		}
	}
	if entry == nil {
		t.Fatalf("expected N11_RETURNS_UNKNOWN_OPTION_CODE entry payload")
	}
	if _, ok := entry.Args["ns_ip_list"]; ok {
		t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
	}
	addresses, ok := entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 1 {
		t.Fatalf("expected one typed address for N11_RETURNS_UNKNOWN_OPTION_CODE, got %#v", entry.Args["addresses"])
	}
	if strings.Join(addresses, ";") != "192.0.2.12" {
		t.Fatalf("expected deterministic addresses order, got %#v", addresses)
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

func TestNameserver13NoEdnsSupport(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.14", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeFormatError
		// No EDNS OPT record in response
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
	if !hasEntryTag(entries, "NO_EDNS_SUPPORT") {
		t.Fatalf("expected NO_EDNS_SUPPORT")
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

	var software *logger.Entry
	for _, item := range entries {
		if item != nil && item.Tag == "N15_SOFTWARE_VERSION" {
			software = item
			break
		}
	}
	if software == nil {
		t.Fatalf("expected N15_SOFTWARE_VERSION entry payload")
	}
	if _, ok := software.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", software.Args)
	}
	servers, ok := software.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server for N15_SOFTWARE_VERSION, got %#v", software.Args["servers"])
	}
	if servers[0]["ns"] != "ns1.example" {
		t.Fatalf("unexpected server payload for N15_SOFTWARE_VERSION: %#v", servers[0])
	}
	if servers[0]["address"] != "192.0.2.15" {
		t.Fatalf("expected server address in N15_SOFTWARE_VERSION payload, got %#v", servers[0])
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

func soaRecord(owner string) dns.RR {
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = dnsutil.Fqdn("ns1.example")
	soaRR.Mbox = dnsutil.Fqdn("hostmaster.example")
	soaRR.Serial = 1
	soaRR.Refresh = 3600
	soaRR.Retry = 600
	soaRR.Expire = 86400
	soaRR.Minttl = 60
	return soaRR
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
	msg.UDPSize = 1232
	msg.Version = version
	for _, opt := range options {
		msg.Pseudo = append(msg.Pseudo, opt)
	}
	if z != 0 {
		optRR := &dns.OPT{}
		optRR.Hdr = dns.Header{Name: "."}
		optRR.SetZ(z)
		msg.Extra = append(msg.Extra, optRR)
	}
	return packet.Packet{Msg: msg}
}

func aPacket(name string, address string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	aRR := &dns.A{}
	aRR.Hdr = dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}
	aRR.Addr = netip.MustParseAddr(address)
	msg.Answer = []dns.RR{aRR}
	return packet.Packet{Msg: msg}
}

func aaaaPacket(name string, address string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	aaaaRR := &dns.AAAA{}
	aaaaRR.Hdr = dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}
	aaaaRR.Addr = netip.MustParseAddr(address)
	msg.Answer = []dns.RR{aaaaRR}
	return packet.Packet{Msg: msg}
}

func txtPacket(name string, value string, class uint16) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	txtRR := &dns.TXT{}
	txtRR.Hdr = dns.Header{Name: dnsutil.Fqdn(name), Class: class, TTL: 60}
	txtRR.Txt = []string{value}
	msg.Answer = []dns.RR{txtRR}
	return packet.Packet{Msg: msg}
}

func TestNameserver16HasNSID(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	nsidValue := "ns1.example"
	ns1 := newNameserver(t, "ns1.example", "192.0.2.16", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) == "SOA" {
			return soaPacketWithEdns("example", 0, 0, []dns.EDNS0{
				&dns.NSID{Nsid: fmt.Sprintf("%x", nsidValue)},
			})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver16(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver16: %v", err)
	}
	if !hasEntryTag(entries, "N16_HAS_NSID") {
		t.Fatalf("expected N16_HAS_NSID")
	}
	if hasEntryTag(entries, "N16_NO_NSID_REVEALED") {
		t.Fatalf("unexpected N16_NO_NSID_REVEALED")
	}

	var hasNSID *logger.Entry
	for _, item := range entries {
		if item != nil && item.Tag == "N16_HAS_NSID" {
			hasNSID = item
			break
		}
	}
	if hasNSID == nil {
		t.Fatalf("expected N16_HAS_NSID entry payload")
	}
	if _, ok := hasNSID.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", hasNSID.Args)
	}
	servers, ok := hasNSID.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server for N16_HAS_NSID, got %#v", hasNSID.Args["servers"])
	}
	if servers[0]["ns"] != "ns1.example" {
		t.Fatalf("unexpected server payload for N16_HAS_NSID: %#v", servers[0])
	}
}

func TestNameserver16NoNSID(t *testing.T) {
	setupTest(t)

	origM4and5 := method4and5
	t.Cleanup(func() { method4and5 = origM4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.16", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) == "SOA" {
			return soaPacket("example")
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver16(context.Background(), &z)
	if err != nil {
		t.Fatalf("nameserver16: %v", err)
	}
	if hasEntryTag(entries, "N16_HAS_NSID") {
		t.Fatalf("unexpected N16_HAS_NSID")
	}
	if !hasEntryTag(entries, "N16_NO_NSID_REVEALED") {
		t.Fatalf("expected N16_NO_NSID_REVEALED")
	}

	var noNSID *logger.Entry
	for _, item := range entries {
		if item != nil && item.Tag == "N16_NO_NSID_REVEALED" {
			noNSID = item
			break
		}
	}
	if noNSID == nil {
		t.Fatalf("expected N16_NO_NSID_REVEALED entry payload")
	}
	if _, ok := noNSID.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", noNSID.Args)
	}
	servers, ok := noNSID.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server for N16_NO_NSID_REVEALED, got %#v", noNSID.Args["servers"])
	}
	if servers[0]["ns"] != "ns1.example" {
		t.Fatalf("unexpected server payload for N16_NO_NSID_REVEALED: %#v", servers[0])
	}
}
