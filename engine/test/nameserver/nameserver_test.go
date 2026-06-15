package nameserver

import (
	"context"
	"fmt"
	"math/rand"
	"net/netip"
	"strings"
	"sync"
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	nsRecursor := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		return packet.Packet{Msg: msg}
	})
	nsNoRecursor := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsRecursor, nsNoRecursor}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	// Server returns NXDOMAIN with AA=1 on all probes (fake root authority).
	// This should NOT be classified as a recursor.
	nsFakeRoot := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		msg.Authoritative = true
		return packet.Packet{Msg: msg}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsFakeRoot}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
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

func TestNameserver01RAWithAnswerIsRecursor(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	// Server returns NOERROR with RA=1 and a real ANSWER record for the
	// out-of-bailiwick probe. That is recursion: the server resolved a name
	// it has no authority over and returned data.
	nsRAAnswer := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qname string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.RecursionAvailable = true
		a := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
		a.Addr = netip.MustParseAddr("203.0.113.1")
		msg.Answer = []dns.RR{a}
		return packet.Packet{Msg: msg}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsRAAnswer}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	if !hasEntryTag(entries, "IS_A_RECURSOR") {
		t.Fatalf("expected IS_A_RECURSOR when RA is set and ANSWER section is non-empty")
	}
}

func TestNameserver01RAReferralIsNotRecursor(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	// Authoritative-only server that returns NOERROR + RA=1 + empty
	// ANSWER + non-empty AUTHORITY (a referral). This is the leaked-RA
	// pattern observed against ns1.kptc.kp in the cohort: a referral,
	// not recursion. The new rule must NOT flag this as a recursor.
	nsRAReferral := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qname string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.RecursionAvailable = true
		_ = qname
		nsRR := &dns.NS{Hdr: dns.Header{Name: ".", Class: dns.ClassINET, TTL: 3600}}
		nsRR.Ns = "a.root-servers.net."
		msg.Ns = []dns.RR{nsRR}
		return packet.Packet{Msg: msg}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsRAReferral}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	if hasEntryTag(entries, "IS_A_RECURSOR") {
		t.Fatalf("did not expect IS_A_RECURSOR for RA-only referral response")
	}
	if !hasEntryTag(entries, "NO_RECURSOR") {
		t.Fatalf("expected NO_RECURSOR for RA-only referral response")
	}
}

func TestNameserver01RAOnSomeAnswerIsRecursor(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	// One probe gets a recursive answer (RA=1 + real ANSWER record); the
	// other two get authoritative-style NXDOMAIN responses. A single
	// recursive response is enough to classify the server as a recursor.
	queries := 0
	nsMixed := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qname string, _ string, _ *ens.QueryOptions) packet.Packet {
		queries++
		msg := new(dns.Msg)
		if queries == 1 {
			msg.Rcode = dns.RcodeSuccess
			msg.RecursionAvailable = true
			a := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			a.Addr = netip.MustParseAddr("203.0.113.1")
			msg.Answer = []dns.RR{a}
			return packet.Packet{Msg: msg}
		}
		msg.Rcode = dns.RcodeNameError
		msg.Authoritative = true
		return packet.Packet{Msg: msg}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsMixed}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	if !hasEntryTag(entries, "IS_A_RECURSOR") {
		t.Fatalf("expected IS_A_RECURSOR when one probe response has RA=1 and ANSWER records")
	}
}

func TestNameserver01NxdomainMixedAAIsRecursor(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	// Server returns NXDOMAIN on all probes, but only some have AA=1.
	// Since not ALL NXDOMAIN responses are authoritative, classify as recursor.
	queryCount := 0
	nsMixed := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		queryCount++
		if queryCount <= 2 {
			msg.Authoritative = true
		}
		return packet.Packet{Msg: msg}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsMixed}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	if !hasEntryTag(entries, "IS_A_RECURSOR") {
		t.Fatalf("expected IS_A_RECURSOR when not all NXDOMAIN responses have AA")
	}
}

func TestNameserver01ParallelQueries(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

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

	ns1, err := ens.NewWithContext(ctx, "ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := ens.NewWithContext(ctx, "ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.3", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacketWithEdns("example", 0, 0, nil)
		}
		return packet.Packet{}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver02(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver02: %v", err)
	}
	if !hasEntryTag(entries, "EDNS0_SUPPORT") {
		t.Fatalf("expected EDNS0_SUPPORT")
	}
}

func TestNameserver03AXFRAvailable(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.4", nil)
	soaRR := soaRecord("example")
	ns1.SetAXFRHook(func(_ context.Context, _ string, callback func(dns.RR) bool, _ string) error {
		if callback != nil {
			callback(soaRR)
		}
		return nil
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver03(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver03: %v", err)
	}
	if !hasEntryTag(entries, "AXFR_AVAILABLE") {
		t.Fatalf("expected AXFR_AVAILABLE")
	}
}

func TestNameserver04DifferentSourceIP(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.5", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg, AnswerFrom: "192.0.2.99:53"}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver04(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver04: %v", err)
	}
	if !hasEntryTag(entries, "DIFFERENT_SOURCE_IP") {
		t.Fatalf("expected DIFFERENT_SOURCE_IP")
	}
}

func TestNameserver05AAAAWellProcessed(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.6", func(qname string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		switch strings.ToUpper(qtype) {
		case "A":
			return aPacket(qname, "192.0.2.10")
		case "AAAA":
			return aaaaPacket(qname, "2001:db8::1")
		}
		return packet.Packet{}
	})

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver05(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver05: %v", err)
	}
	if !hasEntryTag(entries, "AAAA_WELL_PROCESSED") {
		t.Fatalf("expected AAAA_WELL_PROCESSED")
	}
}

func TestNameserver05ParallelQueries(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

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

	ns1, err := ens.NewWithContext(ctx, "ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := ens.NewWithContext(ctx, "ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
	ctx := setupTest(t)

	origM2 := glueNames
	origM3 := apexNSNames
	origM4and5 := allNameservers
	t.Cleanup(func() {
		glueNames = origM2
		apexNSNames = origM3
		allNameservers = origM4and5
	})

	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	apexNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns2.example")}, nil
	}
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.7", nil)
	allNameservers = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver06(ctx, &z)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.8", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver07(ctx, &z)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	origScramble := scrambleCaseFunc
	t.Cleanup(func() {
		authoritativeNS = origM4and5
		scrambleCaseFunc = origScramble
	})
	fixedRand := rand.New(rand.NewSource(42))
	scrambleCaseFunc = func(s string) string { return util.ScrambleCaseWith(s, fixedRand) }

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.9", func(qname string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(strings.ToLower(qname)), dns.TypeSOA)
		return packet.Packet{Msg: msg}
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver08(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver08: %v", err)
	}
	if !hasEntryTag(entries, "QNAME_CASE_INSENSITIVE") {
		t.Fatalf("expected QNAME_CASE_INSENSITIVE")
	}
}

func TestNameserver08DoesNotReuseDifferentCaseCachedPacket(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	origScramble := scrambleCaseFunc
	t.Cleanup(func() {
		authoritativeNS = origM4and5
		scrambleCaseFunc = origScramble
	})

	var calls int
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.98", func(qname string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		calls++
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.StringToType[qtype])
		msg.Authoritative = true
		msg.Rcode = dns.RcodeSuccess
		cname := &dns.CNAME{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 3600}}
		cname.Target = "alias.example."
		msg.Answer = []dns.RR{cname}
		return packet.Packet{Msg: msg}
	})

	if _, err := ns1.QueryWithOptions(ctx, "www.example", "SOA", nil); err != nil {
		t.Fatalf("prime lower-case cache entry: %v", err)
	}

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}
	scrambleCaseFunc = func(string) string { return "wWw.eXaMpLe" }

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver08(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver08: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected mixed-case query to bypass differently cased cached packet, got %d network calls", calls)
	}
	if !hasEntryTag(entries, "QNAME_CASE_SENSITIVE") {
		t.Fatalf("expected QNAME_CASE_SENSITIVE")
	}
	if hasEntryTag(entries, "QNAME_CASE_INSENSITIVE") {
		t.Fatalf("unexpected QNAME_CASE_INSENSITIVE from differently cased cached packet")
	}
}

func TestNameserver09CaseQueriesSameAnswer(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	origScramble := scrambleCaseFunc
	t.Cleanup(func() {
		authoritativeNS = origM4and5
		scrambleCaseFunc = origScramble
	})
	fixedRand := rand.New(rand.NewSource(42))
	scrambleCaseFunc = func(s string) string { return util.ScrambleCaseWith(s, fixedRand) }

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.10", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacket("example")
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver09(ctx, &z)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.11", func(_ string, _ string, _ string, opts *ens.QueryOptions) packet.Packet {
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
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver10(ctx, &z)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.12", func(_ string, _ string, _ string, opts *ens.QueryOptions) packet.Packet {
		if opts != nil && opts.EDNSDetails != nil && len(opts.EDNSDetails.Data) > 0 {
			return soaPacketWithEdns("example", 0, 0, []dns.EDNS0{&dns.ERFC3597{EDNS0Code: 137}})
		}
		return soaPacketWithEdns("example", 0, 0, nil)
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver11(ctx, &z)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.13", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacketWithEdns("example", 0, 3, nil)
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver12(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver12: %v", err)
	}
	if !hasEntryTag(entries, "Z_FLAGS_NOTCLEAR") {
		t.Fatalf("expected Z_FLAGS_NOTCLEAR")
	}
}

func TestNameserver13MissingOptInTruncated(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.14", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Truncated = true
		return packet.Packet{Msg: msg}
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver13(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver13: %v", err)
	}
	if !hasEntryTag(entries, "MISSING_OPT_IN_TRUNCATED") {
		t.Fatalf("expected MISSING_OPT_IN_TRUNCATED")
	}
}

func TestNameserver13NoEdnsSupport(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.14", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeFormatError
		// No EDNS OPT record in response
		return packet.Packet{Msg: msg}
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver13(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver13: %v", err)
	}
	if !hasEntryTag(entries, "NO_EDNS_SUPPORT") {
		t.Fatalf("expected NO_EDNS_SUPPORT")
	}
}

func TestNameserver15SoftwareVersionAndWrongClass(t *testing.T) {
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.15", func(qname string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
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
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver15(ctx, &z)
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

func setupTest(t *testing.T) context.Context {
	t.Helper()

	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })
	return ens.WithCache(context.Background(), ens.NewCacheStore())
}

func newNameserver(t *testing.T, ctx context.Context, name string, ip string, handler func(qname string, qtype string, qclass string, opts *ens.QueryOptions) packet.Packet) ens.Nameserver {
	t.Helper()

	ns, err := ens.NewWithContext(ctx, name, ip, nil)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	nsidValue := "ns1.example"
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.16", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) == "SOA" {
			return soaPacketWithEdns("example", 0, 0, []dns.EDNS0{
				&dns.NSID{Nsid: fmt.Sprintf("%x", nsidValue)},
			})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver16(ctx, &z)
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
	ctx := setupTest(t)

	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.16", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) == "SOA" {
			return soaPacket("example")
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver16(ctx, &z)
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

// cookieServer16 is a 16-byte (32-hex) server cookie used to build 24-byte
// (RFC 9018 v1) well-formed full cookies in the nameserver17 tests.
const cookieServer16 = "aabbccddeeff00112233445566778899"

func cookieFromOpts(opts *ens.QueryOptions) string {
	if opts == nil || opts.EDNSDetails == nil {
		return ""
	}
	for _, o := range opts.EDNSDetails.Data {
		if c, ok := o.(*dns.COOKIE); ok {
			return c.Cookie
		}
	}
	return ""
}

func clientPortion(cookieHex string) string {
	if len(cookieHex) >= 16 {
		return cookieHex[:16]
	}
	return cookieHex
}

func soaPacketWithCookieRcode(owner string, rcode int, cookieHex string) packet.Packet {
	msg := soaMsg(owner)
	msg.UDPSize = 1232
	msg.Rcode = uint16(rcode)
	if cookieHex != "" {
		msg.Pseudo = append(msg.Pseudo, &dns.COOKIE{Cookie: cookieHex})
	}
	return packet.Packet{Msg: msg}
}

func TestNameserver17Supported(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		c := cookieFromOpts(opts)
		if len(c) == 16 { // query 1: return a well-formed 24-byte full cookie
			return soaPacketWithCookieRcode("example", dns.RcodeSuccess, clientPortion(c)+cookieServer16)
		}
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, c) // query 2: accept it
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if !hasEntryTag(entries, "N17_COOKIE_SUPPORTED") {
		t.Fatalf("expected N17_COOKIE_SUPPORTED")
	}
	if !hasEntryTag(entries, "N17_COOKIE_ROUNDTRIP_OK") {
		t.Fatalf("expected N17_COOKIE_ROUNDTRIP_OK")
	}
	entry := firstEntryByTag(entries, "N17_COOKIE_SUPPORTED")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("expected typed server list for N17_COOKIE_SUPPORTED, got %#v", entry.Args["servers"])
	}
}

func TestNameserver17NoCookie(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	calls := 0
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		calls++
		return soaPacket("example") // NOERROR, no COOKIE option
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if !hasEntryTag(entries, "N17_NO_COOKIE") {
		t.Fatalf("expected N17_NO_COOKIE")
	}
	if hasEntryTag(entries, "N17_COOKIE_ROUNDTRIP_OK") {
		t.Fatalf("did not expect a round-trip query for a cookieless response")
	}
	if calls != 1 {
		t.Fatalf("expected exactly one query (no round-trip), got %d", calls)
	}
}

func TestNameserver17NonNoerrorQuery1(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeRefused
		return packet.Packet{Msg: msg}
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	for _, tag := range []string{"N17_NO_COOKIE", "N17_COOKIE_SUPPORTED", "N17_COOKIE_MALFORMED", "N17_COOKIE_CLIENT_ONLY", "N17_NO_RESPONSE"} {
		if hasEntryTag(entries, tag) {
			t.Fatalf("RCODE anomaly must not produce a cookie verdict, got %s", tag)
		}
	}
}

func TestNameserver17ClientOnly(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, clientPortion(cookieFromOpts(opts)))
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if !hasEntryTag(entries, "N17_COOKIE_CLIENT_ONLY") {
		t.Fatalf("expected N17_COOKIE_CLIENT_ONLY")
	}
}

func TestNameserver17Malformed(t *testing.T) {
	t.Run("invalid length", func(t *testing.T) {
		ctx := setupTest(t)
		origM4and5 := authoritativeNS
		t.Cleanup(func() { authoritativeNS = origM4and5 })

		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
			if strings.ToUpper(qtype) != "SOA" {
				return packet.Packet{}
			}
			// 12-byte cookie: client (8) + 4-byte server tail = invalid length.
			return soaPacketWithCookieRcode("example", dns.RcodeSuccess, clientPortion(cookieFromOpts(opts))+"aabbccdd")
		})
		authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
			return []ens.Nameserver{ns1}, nil
		}

		z := zone.Zone{Name: dnsname.New("example")}
		entries, err := Nameserver17(ctx, &z)
		if err != nil {
			t.Fatalf("nameserver17: %v", err)
		}
		entry := firstEntryByTag(entries, "N17_COOKIE_MALFORMED")
		if entry == nil {
			t.Fatalf("expected N17_COOKIE_MALFORMED")
		}
		if entry.Args["cookie_bytes"] != 12 {
			t.Fatalf("expected cookie_bytes=12, got %#v", entry.Args["cookie_bytes"])
		}
	})

	t.Run("wrong client echo", func(t *testing.T) {
		ctx := setupTest(t)
		origM4and5 := authoritativeNS
		t.Cleanup(func() { authoritativeNS = origM4and5 })

		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
			if strings.ToUpper(qtype) != "SOA" {
				return packet.Packet{}
			}
			// Valid length (24 B) but the client portion does not echo ours.
			return soaPacketWithCookieRcode("example", dns.RcodeSuccess, "ffffffffffffffff"+cookieServer16)
		})
		authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
			return []ens.Nameserver{ns1}, nil
		}

		z := zone.Zone{Name: dnsname.New("example")}
		entries, err := Nameserver17(ctx, &z)
		if err != nil {
			t.Fatalf("nameserver17: %v", err)
		}
		if !hasEntryTag(entries, "N17_COOKIE_MALFORMED") {
			t.Fatalf("expected N17_COOKIE_MALFORMED for a wrong client-cookie echo")
		}
	})
}

func TestNameserver17SelfRejectAfterRetry(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	calls := 0
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		c := cookieFromOpts(opts)
		client := clientPortion(c)
		if len(c) == 16 { // query 1: well-formed cookie
			return soaPacketWithCookieRcode("example", dns.RcodeSuccess, client+cookieServer16)
		}
		// query 2 and the corroborating retry both reject, each time issuing a
		// fresh server cookie so the retry is a real exchange (distinct cache key).
		calls++
		fresh := fmt.Sprintf("%016x%016x", uint64(calls), uint64(calls))
		return soaPacketWithCookieRcode("example", dns.RcodeBadCookie, client+fresh)
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if !hasEntryTag(entries, "N17_COOKIE_SELF_REJECT") {
		t.Fatalf("expected N17_COOKIE_SELF_REJECT")
	}
	if hasEntryTag(entries, "N17_COOKIE_ROUNDTRIP_OK") {
		t.Fatalf("did not expect N17_COOKIE_ROUNDTRIP_OK on a double BADCOOKIE")
	}
	if calls != 2 {
		t.Fatalf("expected query 2 plus one corroborating retry, got %d round-trip queries", calls)
	}
}

func TestNameserver17RotationNotFlagged(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	const freshA = "11111111111111112222222222222222"
	calls := 0
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		c := cookieFromOpts(opts)
		client := clientPortion(c)
		if len(c) == 16 { // query 1
			return soaPacketWithCookieRcode("example", dns.RcodeSuccess, client+cookieServer16)
		}
		calls++
		if calls == 1 { // query 2: stale secret, BADCOOKIE with a fresh cookie
			return soaPacketWithCookieRcode("example", dns.RcodeBadCookie, client+freshA)
		}
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, client+freshA) // retry accepted
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if !hasEntryTag(entries, "N17_COOKIE_ROUNDTRIP_OK") {
		t.Fatalf("expected N17_COOKIE_ROUNDTRIP_OK after a successful retry")
	}
	if hasEntryTag(entries, "N17_COOKIE_SELF_REJECT") {
		t.Fatalf("rotation must not be flagged as a self-reject")
	}
}

// TestNameserver17RequireServerCookie covers the strongest cookie posture: an
// enforcing server (e.g. BIND require-server-cookie) answers our client-only
// cookie with BADCOOKIE plus a fresh, well-formed Server Cookie, then accepts
// the full cookie on the round-trip. That must report N17_COOKIE_ENFORCED (not
// the plain N17_COOKIE_SUPPORTED) together with N17_COOKIE_ROUNDTRIP_OK.
func TestNameserver17RequireServerCookie(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		c := cookieFromOpts(opts)
		if len(c) == 16 { // query 1: client-only cookie rejected with a fresh Server Cookie
			return soaPacketWithCookieRcode("example", dns.RcodeBadCookie, clientPortion(c)+cookieServer16)
		}
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, c) // query 2: accepts the full cookie
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if !hasEntryTag(entries, "N17_COOKIE_ENFORCED") {
		t.Fatalf("expected N17_COOKIE_ENFORCED for a require-server-cookie server")
	}
	if !hasEntryTag(entries, "N17_COOKIE_ROUNDTRIP_OK") {
		t.Fatalf("expected N17_COOKIE_ROUNDTRIP_OK after the enforcing server accepted its own cookie")
	}
	if hasEntryTag(entries, "N17_COOKIE_SUPPORTED") {
		t.Fatalf("an enforcing server must report N17_COOKIE_ENFORCED, not N17_COOKIE_SUPPORTED")
	}
	entry := firstEntryByTag(entries, "N17_COOKIE_ENFORCED")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("expected typed server list for N17_COOKIE_ENFORCED, got %#v", entry.Args["servers"])
	}
}

// TestNameserver17BadCookieWithoutServerCookie pins the narrow carve-out: a
// BADCOOKIE reply that does NOT carry a well-formed Server Cookie (here only the
// Client Cookie is echoed) is not the enforcing signal and must stay a generic
// RCODE anomaly (graded by basic/N16) - no cookie verdict and no round-trip.
func TestNameserver17BadCookieWithoutServerCookie(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	calls := 0
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		calls++
		return soaPacketWithCookieRcode("example", dns.RcodeBadCookie, clientPortion(cookieFromOpts(opts)))
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	for _, tag := range []string{"N17_COOKIE_ENFORCED", "N17_COOKIE_SUPPORTED", "N17_COOKIE_CLIENT_ONLY", "N17_COOKIE_MALFORMED", "N17_NO_COOKIE", "N17_COOKIE_ROUNDTRIP_OK"} {
		if hasEntryTag(entries, tag) {
			t.Fatalf("a BADCOOKIE without a well-formed Server Cookie must not produce %s", tag)
		}
	}
	if calls != 1 {
		t.Fatalf("expected a single query 1 with no round-trip, got %d", calls)
	}
}

func TestNameserver17NoResponse(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, _ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return packet.Packet{} // no response
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if !hasEntryTag(entries, "N17_NO_RESPONSE") {
		t.Fatalf("expected N17_NO_RESPONSE")
	}
}

func TestNameserver17Truncated(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	calls := 0
	sawTCP := false
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		calls++
		if opts != nil && opts.UseVC != nil && *opts.UseVC {
			sawTCP = true
		}
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Truncated = true
		return packet.Packet{Msg: msg}
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if !hasEntryTag(entries, "N17_NO_RESPONSE") {
		t.Fatalf("expected N17_NO_RESPONSE for a truncated probe")
	}
	if calls != 1 {
		t.Fatalf("expected a single probe with no follow-up, got %d", calls)
	}
	if sawTCP {
		t.Fatalf("truncated cookie probe must not fall back to TCP")
	}
}

func TestNameserver17OversizedCookieSafe(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		// 41-byte cookie (client + 33-byte server tail): exceeds the 40-byte max.
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, clientPortion(cookieFromOpts(opts))+strings.Repeat("a", 66))
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	entry := firstEntryByTag(entries, "N17_COOKIE_MALFORMED")
	if entry == nil {
		t.Fatalf("expected N17_COOKIE_MALFORMED for an oversized cookie")
	}
	if entry.Args["cookie_bytes"] != 41 {
		t.Fatalf("expected cookie_bytes=41, got %#v", entry.Args["cookie_bytes"])
	}
}

func TestNameserver17UndersizedCookieSafe(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", func(_ string, qtype string, _ string, _ *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		// 4-byte cookie (8 hex): shorter than a Client Cookie; must not panic.
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, "aabbccdd")
	})
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	entry := firstEntryByTag(entries, "N17_COOKIE_MALFORMED")
	if entry == nil {
		t.Fatalf("expected N17_COOKIE_MALFORMED for an undersized cookie")
	}
	if entry.Args["cookie_bytes"] != 4 {
		t.Fatalf("expected cookie_bytes=4, got %#v", entry.Args["cookie_bytes"])
	}
}

func TestNameserver17ClientCookieStable(t *testing.T) {
	ctx := setupTest(t)
	origM4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origM4and5 })

	var mu sync.Mutex
	var seen []string
	record := func(opts *ens.QueryOptions) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, cookieFromOpts(opts))
	}
	handler := func(_ string, qtype string, _ string, opts *ens.QueryOptions) packet.Packet {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}
		}
		record(opts)
		return soaPacket("example") // cookieless: keeps each server to a single probe
	}
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.17", handler)
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.18", handler)
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	if _, err := Nameserver17(ctx, &z); err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("expected one probe per nameserver, got %d", len(seen))
	}
	for _, c := range seen {
		if len(c) != 16 {
			t.Fatalf("expected an 8-byte (16-hex) Client Cookie, got %q", c)
		}
	}
	if seen[0] != seen[1] {
		t.Fatalf("expected the same Client Cookie across probes, got %q and %q", seen[0], seen[1])
	}
}

func TestValidCookieLen(t *testing.T) {
	cases := []struct {
		n    int
		want bool
	}{
		{4, false}, {8, true}, {12, false}, {15, false}, {16, true}, {24, true}, {40, true}, {41, false},
	}
	for _, tc := range cases {
		if got := validCookieLen(tc.n); got != tc.want {
			t.Errorf("validCookieLen(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestClassifyCookie(t *testing.T) {
	const client = "0011223344556677" // 8-byte Client Cookie

	mk := func(cookieHex string) packet.Packet {
		msg := new(dns.Msg)
		if cookieHex != "" {
			msg.Pseudo = []dns.RR{&dns.COOKIE{Cookie: cookieHex}}
		}
		return packet.Packet{Msg: msg}
	}

	cases := []struct {
		name      string
		cookie    string
		wantTag   string
		wantBytes int
	}{
		{"no cookie", "", "N17_NO_COOKIE", 0},
		{"client only", client, "N17_COOKIE_CLIENT_ONLY", 8},
		{"undersized 4B", "aabbccdd", "N17_COOKIE_MALFORMED", 4},
		{"malformed 12B", client + "aabbccdd", "N17_COOKIE_MALFORMED", 12},
		{"supported 16B", client + "aabbccddeeff0011", "N17_COOKIE_SUPPORTED", 16},
		{"supported 24B", client + cookieServer16, "N17_COOKIE_SUPPORTED", 24},
		{"supported 40B", client + strings.Repeat("a", 64), "N17_COOKIE_SUPPORTED", 40},
		{"oversized 41B", client + strings.Repeat("a", 66), "N17_COOKIE_MALFORMED", 41},
		{"wrong echo", "ffffffffffffffff" + cookieServer16, "N17_COOKIE_MALFORMED", 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tag, full, n := classifyCookie(mk(tc.cookie), client)
			if tag != tc.wantTag {
				t.Fatalf("tag = %q, want %q", tag, tc.wantTag)
			}
			if n != tc.wantBytes {
				t.Fatalf("cookie bytes = %d, want %d", n, tc.wantBytes)
			}
			if tc.wantTag == "N17_COOKIE_SUPPORTED" && full != tc.cookie {
				t.Fatalf("full cookie = %q, want %q", full, tc.cookie)
			}
		})
	}
}
