package dnssec

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
	methodsv2 "codeberg.org/pawal/gonemaster/engine/methodsv2"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestDNSSEC01AlgoOK(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		zoneParent = origZoneParent
		hasFakeAddresses = origHasFake
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	hasFakeAddresses = func(_ *zone.Zone) bool {
		return false
	}

	ns := newNameserver(t, "ns1.example", "192.0.2.1", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DS" {
			return packet.Packet{}
		}
		return dsPacket(qname, 12345, 8, 2)
	})

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	if !hasEntryTag(entries, "DS01_DS_ALGO_OK") {
		t.Fatalf("expected DS01_DS_ALGO_OK")
	}
}

func TestDNSSEC01DigestGOST12(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		zoneParent = origZoneParent
		hasFakeAddresses = origHasFake
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	hasFakeAddresses = func(_ *zone.Zone) bool {
		return false
	}

	ns := newNameserver(t, "ns1.example", "192.0.2.31", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DS" {
			return packet.Packet{}
		}
		return dsPacket(qname, 12345, 8, 5) // digest 5 = GOST R 34.11-2012 (RFC 9558)
	})

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	if !hasEntryTag(entries, "DS01_DS_ALGO_OK") {
		t.Fatalf("expected DS01_DS_ALGO_OK for digest algorithm 5 (GOST R 34.11-2012)")
	}
}

func TestDNSSEC01DigestSM3(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		zoneParent = origZoneParent
		hasFakeAddresses = origHasFake
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	hasFakeAddresses = func(_ *zone.Zone) bool {
		return false
	}

	ns := newNameserver(t, "ns1.example", "192.0.2.32", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DS" {
			return packet.Packet{}
		}
		return dsPacket(qname, 12345, 8, 6) // digest 6 = SM3 (RFC 9563)
	})

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	if !hasEntryTag(entries, "DS01_DS_ALGO_OK") {
		t.Fatalf("expected DS01_DS_ALGO_OK for digest algorithm 6 (SM3)")
	}
}

func TestDNSSEC01Algo2Missing(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		zoneParent = origZoneParent
		hasFakeAddresses = origHasFake
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	hasFakeAddresses = func(_ *zone.Zone) bool {
		return false
	}

	ns := newNameserver(t, "ns1.example", "192.0.2.10", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DS" {
			return packet.Packet{}
		}
		return dsPacket(qname, 54321, 8, 1)
	})

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	if !hasEntryTag(entries, "DS01_DS_ALGO_2_MISSING") {
		t.Fatalf("expected DS01_DS_ALGO_2_MISSING")
	}
}

func TestDNSSEC01UndelegatedDSOnlyUsesFakeDS(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		zoneParent = origZoneParent
		hasFakeAddresses = origHasFake
	})

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"ns1.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root addresses: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns-child.example": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake child addresses: %v", err)
	}

	parent, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new parent zone: %v", err)
	}
	child, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new child zone: %v", err)
	}

	parentNS, err := nameserver.NewWithContext(context.Background(), "ns1.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new parent nameserver: %v", err)
	}
	if err := parentNS.AddFakeDS("example", []nameserver.DSData{
		{
			KeyTag:     12345,
			Algorithm:  13,
			DigestType: 2,
			Digest:     "ABCD",
		},
	}); err != nil {
		t.Fatalf("add fake DS: %v", err)
	}

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return &parent, nil
	}
	hasFakeAddresses = func(_ *zone.Zone) bool {
		return true
	}

	entries, err := DNSSEC01(context.Background(), &child)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	if !hasEntryTag(entries, "DS01_DS_ALGO_OK") {
		t.Fatalf("expected DS01_DS_ALGO_OK from fake DS in undelegated mode")
	}
	if hasEntryTag(entries, "DS01_UNDEL_N_NO_UNDEL_DS") {
		t.Fatalf("did not expect DS01_UNDEL_N_NO_UNDEL_DS when fake DS is provided")
	}

	foundFakeSource := false
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS01_DS_ALGO_OK" {
			continue
		}
		servers, ok := entry.Args["servers"].([]map[string]any)
		if !ok || len(servers) != 1 {
			continue
		}
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
		if servers[0]["ns"] == "-" {
			foundFakeSource = true
			break
		}
	}
	if !foundFakeSource {
		t.Fatalf("expected DS01_DS_ALGO_OK to be sourced from undelegated fake DS (servers[0].ns='-')")
	}
}

func TestDNSSEC01ParallelParentQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		zoneParent = origZoneParent
		hasFakeAddresses = origHasFake
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	hasFakeAddresses = func(_ *zone.Zone) bool {
		return false
	}

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype != "DS" {
				return packet.Packet{}, nil
			}
			select {
			case started <- id:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return dsPacket(qname, 12345, 8, 2), nil
		}
	}

	parent1, err := nameserver.New("ns-parent1.example", "192.0.2.80", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent1.SetQueryHook(hook("parent1"))

	parent2, err := nameserver.New("ns-parent2.example", "192.0.2.81", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent2.SetQueryHook(hook("parent2"))

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent1, parent2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC01(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DS queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec01: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec01 did not finish")
	}

	if !hasEntryTag(entries, "DS01_DS_ALGO_OK") {
		t.Fatalf("expected DS01_DS_ALGO_OK")
	}

	var gotServers []map[string]any
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS01_DS_ALGO_OK" {
			continue
		}
		if servers, ok := entry.Args["servers"].([]map[string]any); ok {
			gotServers = servers
		}
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotServers) == 0 {
		t.Fatalf("expected typed servers for DS01_DS_ALGO_OK")
	}
	if len(gotServers) != 2 {
		t.Fatalf("expected two typed servers for DS01_DS_ALGO_OK, got %#v", gotServers)
	}
	if gotServers[0]["ns"] != "ns-parent1.example" || gotServers[1]["ns"] != "ns-parent2.example" {
		t.Fatalf("expected deterministic server order, got %#v", gotServers)
	}
}

func TestDNSSEC02NoDNSKEYForDS(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		method4 = origM4
		method5 = origM5
	})

	parentNS := newNameserver(t, "ns-parent.example", "192.0.2.2", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DS" {
			return packet.Packet{}
		}
		return dsPacket(qname, 9999, 8, 2)
	})

	childNS := newNameserver(t, "ns-child.example", "192.0.2.3", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 8
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(qname, key)
	})

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}
	if !hasEntryTag(entries, "DS02_NO_DNSKEY_FOR_DS") {
		t.Fatalf("expected DS02_NO_DNSKEY_FOR_DS")
	}
	if !hasEntryTag(entries, "DS02_NO_VALID_DNSKEY_FOR_ANY_DS") {
		t.Fatalf("expected DS02_NO_VALID_DNSKEY_FOR_ANY_DS")
	}
}

func TestDNSSEC02DNSKEYNotForZoneSigning(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		method4 = origM4
		method5 = origM5
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}

	parentNS := newNameserver(t, "ns-parent.example", "192.0.2.11", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DS" {
			return packet.Packet{}
		}
		return dsPacketFromDS(qname, ds)
	})

	childNS := newNameserver(t, "ns-child.example", "192.0.2.12", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(qname, key)
	})

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}
	if !hasEntryTag(entries, "DS02_DNSKEY_NOT_FOR_ZONE_SIGNING") {
		t.Fatalf("expected DS02_DNSKEY_NOT_FOR_ZONE_SIGNING")
	}
}

func TestDNSSEC02ParallelChildDNSKEYQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origGetParent := getParentNSNamesAndIPs
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		getParentNSNamesAndIPs = origGetParent
		method4 = origM4
		method5 = origM5
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}

	parentNS := newNameserver(t, "ns-parent.example", "192.0.2.100", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DS" {
			return packet.Packet{}
		}
		return dsPacketFromDS(qname, ds)
	})

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype != "DNSKEY" {
				return packet.Packet{}, nil
			}
			select {
			case started <- id:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return dnskeyPacket(qname, key), nil
		}
	}

	child1, err := nameserver.New("ns-child1.example", "192.0.2.101", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("child1"))

	child2, err := nameserver.New("ns-child2.example", "192.0.2.102", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("child2"))

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC02(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec02: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec02 did not finish")
	}

	if !hasEntryTag(entries, "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS") {
		t.Fatalf("expected DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS")
	}

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS" {
			continue
		}
		if list, ok := entry.Args["ns_ip_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_ip_list for DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS")
	}
	if nsList != "192.0.2.101;192.0.2.102" {
		t.Fatalf("expected deterministic ns_ip_list order, got %q", nsList)
	}
}

func TestDNSSEC03NoNSEC3(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM45 := method4and5
	t.Cleanup(func() {
		method4and5 = origM45
	})

	ns := newNameserver(t, "ns1.example", "192.0.2.4", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			key.Flags = dns.FlagZONE
			key.Protocol = 3
			key.Algorithm = 8
			key.PublicKey = "AwEAAc=="
			return dnskeyPacket(qname, key)
		case "NSEC":
			return nsecPacket(qname)
		default:
			return packet.Packet{}
		}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC03(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec03: %v", err)
	}
	if !hasEntryTag(entries, "DS03_NO_NSEC3") {
		t.Fatalf("expected DS03_NO_NSEC3")
	}
	noNSEC3 := firstEntryByTag(entries, "DS03_NO_NSEC3")
	if noNSEC3 == nil {
		t.Fatalf("missing DS03_NO_NSEC3 entry")
	}
	noNSEC3Servers, ok := noNSEC3.Args["servers"].([]map[string]any)
	if !ok || len(noNSEC3Servers) != 1 {
		t.Fatalf("expected one typed server for DS03_NO_NSEC3, got %#v", noNSEC3.Args["servers"])
	}
	if noNSEC3Servers[0]["ns"] != "ns1.example" {
		t.Fatalf("unexpected typed server payload for DS03_NO_NSEC3: %#v", noNSEC3Servers[0])
	}
	if _, ok := noNSEC3.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", noNSEC3.Args)
	}
}

func TestDNSSEC03IllegalHashAlgo(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM45 := method4and5
	t.Cleanup(func() {
		method4and5 = origM45
	})

	ns := newNameserver(t, "ns1.example", "192.0.2.13", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			key.Flags = dns.FlagZONE
			key.Protocol = 3
			key.Algorithm = 8
			key.PublicKey = "AwEAAc=="
			return dnskeyPacket(qname, key)
		case "NSEC":
			nsec3 := &dns.NSEC3{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			nsec3.Hash = 2
			nsec3.Flags = 0
			nsec3.Iterations = 0
			nsec3.SaltLength = 0
			nsec3.Salt = ""
			nsec3.HashLength = 0
			nsec3.NextDomain = ""
			return nsec3Packet(qname, nsec3)
		default:
			return packet.Packet{}
		}
	})

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC03(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec03: %v", err)
	}
	if !hasEntryTag(entries, "DS03_ILLEGAL_HASH_ALGO") {
		t.Fatalf("expected DS03_ILLEGAL_HASH_ALGO")
	}
	illegal := firstEntryByTag(entries, "DS03_ILLEGAL_HASH_ALGO")
	if illegal == nil {
		t.Fatalf("missing DS03_ILLEGAL_HASH_ALGO entry")
	}
	illegalServers, ok := illegal.Args["servers"].([]map[string]any)
	if !ok || len(illegalServers) != 1 {
		t.Fatalf("expected one typed server for DS03_ILLEGAL_HASH_ALGO, got %#v", illegal.Args["servers"])
	}
	if illegalServers[0]["ns"] != "ns1.example" {
		t.Fatalf("unexpected typed server payload for DS03_ILLEGAL_HASH_ALGO: %#v", illegalServers[0])
	}
	if _, ok := illegal.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", illegal.Args)
	}
	if algo, _ := illegal.Args["algo_num"].(uint8); algo != 2 {
		t.Fatalf("expected algo_num=2 for DS03_ILLEGAL_HASH_ALGO, got %#v", illegal.Args["algo_num"])
	}
}

func TestDNSSEC03ParallelDNSKEYQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM45 := method4and5
	t.Cleanup(func() {
		method4and5 = origM45
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "DNSKEY":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return dnskeyPacket(qname, key), nil
			case "NSEC":
				return nsecPacket(qname), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.201", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.202", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC03(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec03: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec03 did not finish")
	}

	if !hasEntryTag(entries, "DS03_NO_NSEC3") {
		t.Fatalf("expected DS03_NO_NSEC3")
	}

	var gotServers []map[string]any
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS03_NO_NSEC3" {
			continue
		}
		if servers, ok := entry.Args["servers"].([]map[string]any); ok {
			gotServers = servers
		}
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotServers) == 0 {
		t.Fatalf("expected typed servers for DS03_NO_NSEC3")
	}
	if len(gotServers) != 2 {
		t.Fatalf("expected two typed servers for DS03_NO_NSEC3, got %#v", gotServers)
	}
	if gotServers[0]["ns"] != "ns1.example" || gotServers[1]["ns"] != "ns2.example" {
		t.Fatalf("expected deterministic server order, got %#v", gotServers)
	}
}

func TestDNSSEC04ExpiredRRSIG(t *testing.T) {
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origQueryOne := zoneQueryOne
	t.Cleanup(func() {
		zoneQueryOne = origQueryOne
	})

	now := time.Unix(1700000000, 0).UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	soa := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	soa.Ns = "ns1.example."
	soa.Mbox = "hostmaster.example."
	soa.Serial = 1
	soa.Refresh = 60
	soa.Retry = 60
	soa.Expire = 60
	soa.Minttl = 60
	expiredSig := rrsigRecord("example", dns.TypeDNSKEY, 12345, now.Unix()-600, now.Unix()-1)

	dnskeyResp := answerPacket("example", dns.TypeDNSKEY, key, expiredSig)
	dnskeyResp.Timestamp = now
	soaResp := answerPacket("example", dns.TypeSOA, soa)
	soaResp.Timestamp = now

	zoneQueryOne = func(_ context.Context, _ *zone.Zone, _ string, rrtype string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch rrtype {
		case "DNSKEY":
			return dnskeyResp, nil
		case "SOA":
			return soaResp, nil
		default:
			return packet.Packet{}, nil
		}
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC04(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec04: %v", err)
	}
	if !hasEntryTag(entries, "RRSIG_EXPIRED") {
		t.Fatalf("expected RRSIG_EXPIRED")
	}
}

func TestDNSSEC04DurationOK(t *testing.T) {
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origQueryOne := zoneQueryOne
	t.Cleanup(func() {
		zoneQueryOne = origQueryOne
	})

	now := time.Unix(1700000000, 0).UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	soa := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	soa.Ns = "ns1.example."
	soa.Mbox = "hostmaster.example."
	soa.Serial = 1
	soa.Refresh = 60
	soa.Retry = 60
	soa.Expire = 60
	soa.Minttl = 60
	okSig := rrsigRecord("example", dns.TypeDNSKEY, 54321, now.Unix()-86400, now.Unix()+172800)

	dnskeyResp := answerPacket("example", dns.TypeDNSKEY, key, okSig)
	dnskeyResp.Timestamp = now
	soaResp := answerPacket("example", dns.TypeSOA, soa)
	soaResp.Timestamp = now

	zoneQueryOne = func(_ context.Context, _ *zone.Zone, _ string, rrtype string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch rrtype {
		case "DNSKEY":
			return dnskeyResp, nil
		case "SOA":
			return soaResp, nil
		default:
			return packet.Packet{}, nil
		}
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC04(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec04: %v", err)
	}
	if !hasEntryTag(entries, "DURATION_OK") {
		t.Fatalf("expected DURATION_OK")
	}
}

func TestDNSSEC04ParallelQueries(t *testing.T) {
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origQueryOne := zoneQueryOne
	t.Cleanup(func() {
		zoneQueryOne = origQueryOne
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	now := time.Unix(1700000000, 0).UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	soa := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	soa.Ns = "ns1.example."
	soa.Mbox = "hostmaster.example."
	soa.Serial = 1
	soa.Refresh = 60
	soa.Retry = 60
	soa.Expire = 60
	soa.Minttl = 60
	okSig := rrsigRecord("example", dns.TypeDNSKEY, 54321, now.Unix()-86400, now.Unix()+172800)

	dnskeyResp := answerPacket("example", dns.TypeDNSKEY, key, okSig)
	dnskeyResp.Timestamp = now
	soaResp := answerPacket("example", dns.TypeSOA, soa)
	soaResp.Timestamp = now

	started := make(chan string, 2)
	release := make(chan struct{})

	zoneQueryOne = func(ctx context.Context, _ *zone.Zone, _ string, rrtype string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch rrtype {
		case "DNSKEY":
			select {
			case started <- rrtype:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return dnskeyResp, nil
		case "SOA":
			select {
			case started <- rrtype:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return soaResp, nil
		default:
			return packet.Packet{}, nil
		}
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC04(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY/SOA queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec04: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec04 did not finish")
	}

	if !hasEntryTag(entries, "DURATION_OK") {
		t.Fatalf("expected DURATION_OK")
	}
}

func TestDNSSEC05AlgoOK(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	newNameserver(t, "ns1.example", "192.0.2.20", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 8
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(qname, key)
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.20"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	if !hasEntryTag(entries, "DS05_ALGO_OK") {
		t.Fatalf("expected DS05_ALGO_OK")
	}
}

func TestDNSSEC05AlgoSM2SM3(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	newNameserver(t, "ns1.example", "192.0.2.33", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 17 // SM2SM3 (RFC 9563)
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(qname, key)
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.33"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	if !hasEntryTag(entries, "DS05_ALGO_OK") {
		t.Fatalf("expected DS05_ALGO_OK for algorithm 17 (SM2SM3)")
	}
}

func TestDNSSEC05AlgoECCGOST12(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	newNameserver(t, "ns1.example", "192.0.2.34", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 23 // ECC-GOST12 (RFC 9558)
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(qname, key)
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.34"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	if !hasEntryTag(entries, "DS05_ALGO_OK") {
		t.Fatalf("expected DS05_ALGO_OK for algorithm 23 (ECC-GOST12)")
	}
}

func TestDNSSEC05ParallelDNSKEYQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	handler := func(id string) func(string, string, *nameserver.QueryOptions) packet.Packet {
		return func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if qtype != "DNSKEY" {
				return packet.Packet{}
			}
			select {
			case started <- id:
			default:
			}
			<-release
			return dnskeyPacket(qname, key)
		}
	}

	newNameserver(t, "ns1.example", "192.0.2.220", handler("ns1"))
	newNameserver(t, "ns2.example", "192.0.2.221", handler("ns2"))

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.220"),
				HasAddress: true,
			},
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.221"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC05(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec05: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec05 did not finish")
	}

	if !hasEntryTag(entries, "DS05_ALGO_OK") {
		t.Fatalf("expected DS05_ALGO_OK")
	}

	var gotServers []map[string]any
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS05_ALGO_OK" {
			continue
		}
		if servers, ok := entry.Args["servers"].([]map[string]any); ok {
			gotServers = servers
		}
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotServers) == 0 {
		t.Fatalf("expected typed servers for DS05_ALGO_OK")
	}
	if len(gotServers) != 2 {
		t.Fatalf("expected two typed servers for DS05_ALGO_OK, got %#v", gotServers)
	}
	if gotServers[0]["ns"] != "ns1.example" || gotServers[1]["ns"] != "ns2.example" {
		t.Fatalf("expected deterministic server order, got %#v", gotServers)
	}
}

func TestDNSSEC05ZoneNoDNSSEC(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	newNameserver(t, "ns2.example", "192.0.2.21", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(qname, nil)
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.21"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	if !hasEntryTag(entries, "DS05_ZONE_NO_DNSSEC") {
		t.Fatalf("expected DS05_ZONE_NO_DNSSEC")
	}
}

func TestDNSSEC05NoResponse(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	newNameserver(t, "ns3.example", "192.0.2.22", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return packet.Packet{}
		}
		return packet.Packet{}
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns3.example"),
				Address:    netip.MustParseAddr("192.0.2.22"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	if !hasEntryTag(entries, "DS05_NO_RESPONSE") {
		t.Fatalf("expected DS05_NO_RESPONSE")
	}
}

func TestDNSSEC06ExtraProcessingOK(t *testing.T) {
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origQueryAll := zoneQueryAll
	t.Cleanup(func() {
		zoneQueryAll = origQueryAll
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 12345, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
	resp := answerPacket("example", dns.TypeDNSKEY, key, sig)
	resp.AnswerFrom = "192.0.2.30"

	zoneQueryAll = func(_ context.Context, _ *zone.Zone, _ string, _ string, _ *nameserver.QueryOptions) ([]packet.Packet, error) {
		return []packet.Packet{resp}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC06(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec06: %v", err)
	}
	if !hasEntryTag(entries, "EXTRA_PROCESSING_OK") {
		t.Fatalf("expected EXTRA_PROCESSING_OK")
	}
}

func TestDNSSEC06ExtraProcessingBroken(t *testing.T) {
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origQueryAll := zoneQueryAll
	t.Cleanup(func() {
		zoneQueryAll = origQueryAll
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	resp := answerPacket("example", dns.TypeDNSKEY, key)
	resp.AnswerFrom = "192.0.2.31"

	zoneQueryAll = func(_ context.Context, _ *zone.Zone, _ string, _ string, _ *nameserver.QueryOptions) ([]packet.Packet, error) {
		return []packet.Packet{resp}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC06(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec06: %v", err)
	}
	if !hasEntryTag(entries, "EXTRA_PROCESSING_BROKEN") {
		t.Fatalf("expected EXTRA_PROCESSING_BROKEN")
	}
}

func TestDNSSEC07SignedZone(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	origParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
		getParentNSNamesAndIPs = origParent
		zoneParent = origZoneParent
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	newNameserver(t, "ns1.example", "192.0.2.40", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key, sig)
		default:
			return packet.Packet{}
		}
	})

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 11111
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "DEADBEEF"
	dsSig := rrsigRecord("example", dns.TypeDS, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	newNameserver(t, "ns-parent.example", "192.0.2.41", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return answerPacket(qname, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.40"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}
	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns, _ := nameserver.New("ns-parent.example", "192.0.2.41", nil)
		return []nameserver.Nameserver{ns}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	if !hasEntryTag(entries, "DS07_SIGNED_ON_SERVER") {
		t.Fatalf("expected DS07_SIGNED_ON_SERVER")
	}
	signedOnServer := firstEntryByTag(entries, "DS07_SIGNED_ON_SERVER")
	if signedOnServer == nil {
		t.Fatalf("missing DS07_SIGNED_ON_SERVER entry")
	}
	signedServers, ok := signedOnServer.Args["servers"].([]map[string]any)
	if !ok || len(signedServers) != 1 {
		t.Fatalf("expected typed servers for DS07_SIGNED_ON_SERVER, got %#v", signedOnServer.Args["servers"])
	}
	if signedServers[0]["ns"] != "ns1.example" {
		t.Fatalf("unexpected typed server payload for DS07_SIGNED_ON_SERVER: %#v", signedServers[0])
	}
	if _, ok := signedOnServer.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", signedOnServer.Args)
	}
	if !hasEntryTag(entries, "DS07_SIGNED") {
		t.Fatalf("expected DS07_SIGNED")
	}
	if !hasEntryTag(entries, "DS07_DS_ON_PARENT_SERVER") {
		t.Fatalf("expected DS07_DS_ON_PARENT_SERVER")
	}
	dsOnParent := firstEntryByTag(entries, "DS07_DS_ON_PARENT_SERVER")
	if dsOnParent == nil {
		t.Fatalf("missing DS07_DS_ON_PARENT_SERVER entry")
	}
	parentServers, ok := dsOnParent.Args["servers"].([]map[string]any)
	if !ok || len(parentServers) != 1 {
		t.Fatalf("expected typed servers for DS07_DS_ON_PARENT_SERVER, got %#v", dsOnParent.Args["servers"])
	}
	if parentServers[0]["ns"] != "ns-parent.example" {
		t.Fatalf("unexpected typed server payload for DS07_DS_ON_PARENT_SERVER: %#v", parentServers[0])
	}
	if _, ok := dsOnParent.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", dsOnParent.Args)
	}
	if !hasEntryTag(entries, "DS07_DS_FOR_SIGNED_ZONE") {
		t.Fatalf("expected DS07_DS_FOR_SIGNED_ZONE")
	}
}

func TestDNSSEC07ParallelChildQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	origParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
		getParentNSNamesAndIPs = origParent
		zoneParent = origZoneParent
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "SOA":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return answerPacket(qname, dns.TypeSOA, soaRecord(qname)), nil
			case "DNSKEY":
				return dnskeyPacket(qname, key), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.60", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.61", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.60"),
				HasAddress: true,
			},
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.61"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC07(ctx, &z)
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
		if dsErr != nil {
			t.Fatalf("dnssec07: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec07 did not finish")
	}

	if !hasEntryTag(entries, "DS07_NOT_SIGNED_ON_SERVER") {
		t.Fatalf("expected DS07_NOT_SIGNED_ON_SERVER")
	}
	if !hasEntryTag(entries, "DS07_NOT_SIGNED") {
		t.Fatalf("expected DS07_NOT_SIGNED")
	}

	var gotServers []map[string]any
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS07_NOT_SIGNED_ON_SERVER" {
			continue
		}
		if servers, ok := entry.Args["servers"].([]map[string]any); ok {
			gotServers = servers
		}
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotServers) == 0 {
		t.Fatalf("expected typed servers for DS07_NOT_SIGNED_ON_SERVER")
	}
	if len(gotServers) != 2 {
		t.Fatalf("expected two typed servers for DS07_NOT_SIGNED_ON_SERVER, got %#v", gotServers)
	}
	if gotServers[0]["ns"] != "ns1.example" || gotServers[1]["ns"] != "ns2.example" {
		t.Fatalf("expected deterministic server order, got %#v", gotServers)
	}
}

func TestDNSSEC07ParallelParentQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	origParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
		getParentNSNamesAndIPs = origParent
		zoneParent = origZoneParent
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	newNameserver(t, "ns-child.example", "192.0.2.62", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key, sig)
		default:
			return packet.Packet{}
		}
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns-child.example"),
				Address:    netip.MustParseAddr("192.0.2.62"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 11111
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "DEADBEEF"
	dsSig := rrsigRecord("example", dns.TypeDS, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype == "DS" {
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return answerPacket(qname, dns.TypeDS, ds, dsSig), nil
			}
			return packet.Packet{}, nil
		}
	}

	parent1, err := nameserver.New("ns-parent1.example", "192.0.2.70", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent1.SetQueryHook(hook("parent1"))

	parent2, err := nameserver.New("ns-parent2.example", "192.0.2.71", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent2.SetQueryHook(hook("parent2"))

	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent1, parent2}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC07(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DS queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec07: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec07 did not finish")
	}

	var gotServers []map[string]any
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS07_DS_ON_PARENT_SERVER" {
			continue
		}
		if servers, ok := entry.Args["servers"].([]map[string]any); ok {
			gotServers = servers
		}
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotServers) == 0 {
		t.Fatalf("expected typed servers for DS07_DS_ON_PARENT_SERVER")
	}
	if len(gotServers) != 2 {
		t.Fatalf("expected two typed servers for DS07_DS_ON_PARENT_SERVER, got %#v", gotServers)
	}
	if gotServers[0]["ns"] != "ns-parent1.example" || gotServers[1]["ns"] != "ns-parent2.example" {
		t.Fatalf("expected deterministic server order, got %#v", gotServers)
	}
}

func TestDNSSEC07NotSigned(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	origParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
		getParentNSNamesAndIPs = origParent
		zoneParent = origZoneParent
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	newNameserver(t, "ns2.example", "192.0.2.42", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		default:
			return packet.Packet{}
		}
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.42"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}
	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	if !hasEntryTag(entries, "DS07_NOT_SIGNED_ON_SERVER") {
		t.Fatalf("expected DS07_NOT_SIGNED_ON_SERVER")
	}
	notSignedOnServer := firstEntryByTag(entries, "DS07_NOT_SIGNED_ON_SERVER")
	if notSignedOnServer == nil {
		t.Fatalf("missing DS07_NOT_SIGNED_ON_SERVER entry")
	}
	notSignedServers, ok := notSignedOnServer.Args["servers"].([]map[string]any)
	if !ok || len(notSignedServers) != 1 {
		t.Fatalf("expected typed servers for DS07_NOT_SIGNED_ON_SERVER, got %#v", notSignedOnServer.Args["servers"])
	}
	if notSignedServers[0]["ns"] != "ns2.example" {
		t.Fatalf("unexpected typed server payload for DS07_NOT_SIGNED_ON_SERVER: %#v", notSignedServers[0])
	}
	if _, ok := notSignedOnServer.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", notSignedOnServer.Args)
	}
	if !hasEntryTag(entries, "DS07_NOT_SIGNED") {
		t.Fatalf("expected DS07_NOT_SIGNED")
	}
}

func TestDNSSEC07ChildOutcomeTagsTypedServers(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	origParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
		getParentNSNamesAndIPs = origParent
		zoneParent = origZoneParent
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	newNameserver(t, "ns-noresp.example", "192.0.2.170", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		case "DNSKEY":
			return packet.Packet{}
		default:
			return packet.Packet{}
		}
	})

	newNameserver(t, "ns-noauth.example", "192.0.2.171", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		case "DNSKEY":
			p := dnskeyPacket(qname, key)
			p.Msg.Authoritative = false
			return p
		default:
			return packet.Packet{}
		}
	})

	newNameserver(t, "ns-rcode.example", "192.0.2.172", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		case "DNSKEY":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.TypeDNSKEY)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeServerFailure
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		default:
			return packet.Packet{}
		}
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns-noresp.example"),
				Address:    netip.MustParseAddr("192.0.2.170"),
				HasAddress: true,
			},
			{
				Name:       dnsname.New("ns-noauth.example"),
				Address:    netip.MustParseAddr("192.0.2.171"),
				HasAddress: true,
			},
			{
				Name:       dnsname.New("ns-rcode.example"),
				Address:    netip.MustParseAddr("192.0.2.172"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	if !hasEntryTag(entries, "DS07_NO_RESPONSE_DNSKEY") {
		t.Fatalf("expected DS07_NO_RESPONSE_DNSKEY")
	}
	if !hasEntryTag(entries, "DS07_NON_AUTH_RESPONSE_DNSKEY") {
		t.Fatalf("expected DS07_NON_AUTH_RESPONSE_DNSKEY")
	}
	if !hasEntryTag(entries, "DS07_UNEXP_RCODE_RESP_DNSKEY") {
		t.Fatalf("expected DS07_UNEXP_RCODE_RESP_DNSKEY")
	}

	noResp := firstEntryByTag(entries, "DS07_NO_RESPONSE_DNSKEY")
	noAuth := firstEntryByTag(entries, "DS07_NON_AUTH_RESPONSE_DNSKEY")
	unexp := firstEntryByTag(entries, "DS07_UNEXP_RCODE_RESP_DNSKEY")
	if noResp == nil || noAuth == nil || unexp == nil {
		t.Fatalf("expected child outcome entries to be present")
	}
	for _, entry := range []*logger.Entry{noResp, noAuth, unexp} {
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
	}
	if rcode, _ := unexp.Args["rcode"].(string); rcode != "SERVFAIL" {
		t.Fatalf("expected rcode SERVFAIL, got %#v", unexp.Args["rcode"])
	}
	expectOneServer := func(entry *logger.Entry, ns string) {
		t.Helper()
		servers, ok := entry.Args["servers"].([]map[string]any)
		if !ok || len(servers) != 1 {
			t.Fatalf("expected one typed server for %s, got %#v", entry.Tag, entry.Args["servers"])
		}
		if servers[0]["ns"] != ns {
			t.Fatalf("unexpected typed server payload for %s: %#v", entry.Tag, servers[0])
		}
	}
	expectOneServer(noResp, "ns-noresp.example")
	expectOneServer(noAuth, "ns-noauth.example")
	expectOneServer(unexp, "ns-rcode.example")
}

func TestDNSSEC07NoDSOnParentServerTypedServers(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	origParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
		getParentNSNamesAndIPs = origParent
		zoneParent = origZoneParent
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	newNameserver(t, "ns1.example", "192.0.2.180", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key, sig)
		default:
			return packet.Packet{}
		}
	})

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 11111
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "DEADBEEF"

	newNameserver(t, "ns-parent.example", "192.0.2.181", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return answerPacket(qname, dns.TypeDS, ds)
		}
		return packet.Packet{}
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.180"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}
	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns, _ := nameserver.New("ns-parent.example", "192.0.2.181", nil)
		return []nameserver.Nameserver{ns}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	if !hasEntryTag(entries, "DS07_NO_DS_ON_PARENT_SERVER") {
		t.Fatalf("expected DS07_NO_DS_ON_PARENT_SERVER")
	}
	noDS := firstEntryByTag(entries, "DS07_NO_DS_ON_PARENT_SERVER")
	if noDS == nil {
		t.Fatalf("missing DS07_NO_DS_ON_PARENT_SERVER entry")
	}
	servers, ok := noDS.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected typed servers for DS07_NO_DS_ON_PARENT_SERVER, got %#v", noDS.Args["servers"])
	}
	if servers[0]["ns"] != "ns-parent.example" {
		t.Fatalf("unexpected typed server payload for DS07_NO_DS_ON_PARENT_SERVER: %#v", servers[0])
	}
	if _, ok := noDS.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", noDS.Args)
	}
}

func TestDNSSECAllParallelOutputStable(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	origParent := getParentNSNamesAndIPs
	origZoneParent := zoneParent
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
		getParentNSNamesAndIPs = origParent
		zoneParent = origZoneParent
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	getParentNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.160"),
				HasAddress: true,
			},
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.161"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	runAll := func(parallel int) []*logger.Entry {
		profile.ResetEffective()
		if err := profile.Effective().Set("resolver.defaults.parallel", parallel); err != nil {
			t.Fatalf("set parallel: %v", err)
		}
		if err := profile.Effective().Set("test_cases", []any{"dnssec07"}); err != nil {
			t.Fatalf("set test_cases: %v", err)
		}

		nameserver.EmptyCache()
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 8
		key.PublicKey = "AwEAAc=="
		handler := func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			switch qtype {
			case "SOA":
				return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
			case "DNSKEY":
				return dnskeyPacket(qname, key)
			default:
				return packet.Packet{}
			}
		}
		newNameserver(t, "ns1.example", "192.0.2.160", handler)
		newNameserver(t, "ns2.example", "192.0.2.161", handler)

		z, err := zone.New("example")
		if err != nil {
			t.Fatalf("zone new: %v", err)
		}
		entries, err := All(context.Background(), &z)
		if err != nil {
			t.Fatalf("dnssec all: %v", err)
		}
		return entries
	}

	sequentialEntries := runAll(1)
	parallelEntries := runAll(2)

	if !hasEntryTag(sequentialEntries, "DS07_NOT_SIGNED_ON_SERVER") || !hasEntryTag(sequentialEntries, "DS07_NOT_SIGNED") {
		t.Fatalf("expected unsigned DNSSEC07 tags in sequential run")
	}
	if !hasEntryTag(parallelEntries, "DS07_NOT_SIGNED_ON_SERVER") || !hasEntryTag(parallelEntries, "DS07_NOT_SIGNED") {
		t.Fatalf("expected unsigned DNSSEC07 tags in parallel run")
	}

	sequentialNormalized := normalizeEntriesForComparison(sequentialEntries)
	parallelNormalized := normalizeEntriesForComparison(parallelEntries)
	if len(sequentialNormalized) != len(parallelNormalized) {
		t.Fatalf("entry count changed with parallelism: sequential=%v parallel=%v", sequentialNormalized, parallelNormalized)
	}
	for i := range sequentialNormalized {
		if sequentialNormalized[i] != parallelNormalized[i] {
			t.Fatalf("entry[%d] changed with parallelism: %q != %q", i, sequentialNormalized[i], parallelNormalized[i])
		}
	}
}

func TestDNSSEC08MissingRRSIG(t *testing.T) {
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

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	ns := newNameserver(t, "ns1.example", "192.0.2.50", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(qname, key)
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC08(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec08: %v", err)
	}
	if !hasEntryTag(entries, "DS08_MISSING_RRSIG_IN_RESPONSE") {
		t.Fatalf("expected DS08_MISSING_RRSIG_IN_RESPONSE")
	}
}

func TestDNSSEC08RRSIGNotYetValid(t *testing.T) {
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

	now := time.Unix(1700000000, 0).UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(time.Hour).Unix(), now.Add(2*time.Hour).Unix())

	ns := newNameserver(t, "ns2.example", "192.0.2.51", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(qname, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC08(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec08: %v", err)
	}
	if !hasEntryTag(entries, "DS08_DNSKEY_RRSIG_NOT_YET_VALID") {
		t.Fatalf("expected DS08_DNSKEY_RRSIG_NOT_YET_VALID")
	}
}

func TestDNSSEC08RRSIGNotValidByDNSKEY(t *testing.T) {
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

	now := time.Unix(1700000000, 0).UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	ns := newNameserver(t, "ns3.example", "192.0.2.52", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(qname, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC08(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec08: %v", err)
	}
	if !hasEntryTag(entries, "DS08_RRSIG_NOT_VALID_BY_DNSKEY") {
		t.Fatalf("expected DS08_RRSIG_NOT_VALID_BY_DNSKEY")
	}
}

func TestDNSSEC08ParallelDNSKEYQueries(t *testing.T) {
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

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype != "DNSKEY" {
				return packet.Packet{}, nil
			}
			select {
			case started <- id:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return dnskeyPacket(qname, key), nil
		}
	}

	child1, err := nameserver.New("ns1.example", "192.0.2.101", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("child1"))

	child2, err := nameserver.New("ns2.example", "192.0.2.102", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("child2"))

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC08(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec08: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec08 did not finish")
	}

	if !hasEntryTag(entries, "DS08_MISSING_RRSIG_IN_RESPONSE") {
		t.Fatalf("expected DS08_MISSING_RRSIG_IN_RESPONSE")
	}

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS08_MISSING_RRSIG_IN_RESPONSE" {
			continue
		}
		if list, ok := entry.Args["ns_ip_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_ip_list for DS08_MISSING_RRSIG_IN_RESPONSE")
	}
	if nsList != "192.0.2.101;192.0.2.102" {
		t.Fatalf("expected deterministic ns_ip_list order, got %q", nsList)
	}
}

func TestDNSSEC09MissingRRSIG(t *testing.T) {
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

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	ns := newNameserver(t, "ns1.example", "192.0.2.60", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		default:
			return packet.Packet{}
		}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC09(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec09: %v", err)
	}
	if !hasEntryTag(entries, "DS09_MISSING_RRSIG_IN_RESPONSE") {
		t.Fatalf("expected DS09_MISSING_RRSIG_IN_RESPONSE")
	}
}

func TestDNSSEC09ParallelQueries(t *testing.T) {
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

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "DNSKEY":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return dnskeyPacket(qname, key), nil
			case "SOA":
				return answerPacket(qname, dns.TypeSOA, soaRecord(qname)), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	child1, err := nameserver.New("ns1.example", "192.0.2.111", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("child1"))

	child2, err := nameserver.New("ns2.example", "192.0.2.112", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("child2"))

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC09(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec09: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec09 did not finish")
	}

	if !hasEntryTag(entries, "DS09_MISSING_RRSIG_IN_RESPONSE") {
		t.Fatalf("expected DS09_MISSING_RRSIG_IN_RESPONSE")
	}

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS09_MISSING_RRSIG_IN_RESPONSE" {
			continue
		}
		if list, ok := entry.Args["ns_ip_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_ip_list for DS09_MISSING_RRSIG_IN_RESPONSE")
	}
	if nsList != "192.0.2.111;192.0.2.112" {
		t.Fatalf("expected deterministic ns_ip_list order, got %q", nsList)
	}
}

func TestDNSSEC10MissingSignature(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next.example")
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG}

	newNameserver(t, "ns1.example", "192.0.2.70", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		case "NSEC":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.TypeNSEC)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		case "NSEC3PARAM":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.TypeNSEC3PARAM)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			msg.Ns = append(msg.Ns, nsec, soaRecord(qname))
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		default:
			return packet.Packet{}
		}
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.70"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}
	if !hasEntryTag(entries, "DS10_NSEC_MISSING_SIGNATURE") {
		t.Fatalf("expected DS10_NSEC_MISSING_SIGNATURE")
	}
}

func TestDNSSEC10ParallelQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype != "DNSKEY" {
				return packet.Packet{}, nil
			}
			select {
			case started <- id:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return dnskeyPacket(qname, key), nil
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.201", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.202", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.201"),
				HasAddress: true,
			},
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.202"),
				HasAddress: true,
			},
		}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC10(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec10: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec10 did not finish")
	}

	if !hasEntryTag(entries, "DS10_NSEC_QUERY_RESPONSE_ERR") {
		t.Fatalf("expected DS10_NSEC_QUERY_RESPONSE_ERR")
	}

	var gotServers []map[string]any
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS10_NSEC_QUERY_RESPONSE_ERR" {
			continue
		}
		if servers, ok := entry.Args["servers"].([]map[string]any); ok {
			gotServers = servers
		}
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotServers) == 0 {
		t.Fatalf("expected typed servers for DS10_NSEC_QUERY_RESPONSE_ERR")
	}
	if len(gotServers) != 2 {
		t.Fatalf("expected two typed servers for DS10_NSEC_QUERY_RESPONSE_ERR, got %#v", gotServers)
	}
	if gotServers[0]["ns"] != "ns1.example" || gotServers[1]["ns"] != "ns2.example" {
		t.Fatalf("expected deterministic server order, got %#v", gotServers)
	}
}

func TestDNSSEC11ParallelParentQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParent := parentNameservers
	origM4 := method4
	origM5 := method5
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentNameservers = origParent
		method4 = origM4
		method5 = origM5
		hasFakeAddresses = origHasFake
	})

	profile.Effective().Resolver.Defaults.Parallel = 2
	hasFakeAddresses = func(_ *zone.Zone) bool { return false }

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 12345
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "DEADBEEF"

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string, withDS bool) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype != "DS" {
				return packet.Packet{}, nil
			}
			select {
			case started <- id:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			if withDS {
				return dsPacketFromDS(qname, ds), nil
			}
			return dsPacketFromDS(qname, nil), nil
		}
	}

	parent1, err := nameserver.New("ns-parent1.example", "192.0.2.80", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent1.SetQueryHook(hook("parent1", true))

	parent2, err := nameserver.New("ns-parent2.example", "192.0.2.81", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent2.SetQueryHook(hook("parent2", false))

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent1, parent2}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC11(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DS queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec11: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec11 did not finish")
	}

	if !hasEntryTag(entries, "DS11_INCONSISTENT_DS") {
		t.Fatalf("expected DS11_INCONSISTENT_DS")
	}
	if !hasEntryTag(entries, "DS11_PARENT_WITHOUT_DS") {
		t.Fatalf("expected DS11_PARENT_WITHOUT_DS")
	}
	if !hasEntryTag(entries, "DS11_PARENT_WITH_DS") {
		t.Fatalf("expected DS11_PARENT_WITH_DS")
	}
}

func TestDNSSEC11ParallelChildQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParent := parentNameservers
	origM4 := method4
	origM5 := method5
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentNameservers = origParent
		method4 = origM4
		method5 = origM5
		hasFakeAddresses = origHasFake
	})

	profile.Effective().Resolver.Defaults.Parallel = 2
	hasFakeAddresses = func(_ *zone.Zone) bool { return false }

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string, withDNSKEY bool) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "SOA":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return answerPacket(qname, dns.TypeSOA, soaRecord(qname)), nil
			case "DNSKEY":
				if withDNSKEY {
					return dnskeyPacket(qname, key), nil
				}
				return dnskeyPacket(qname, nil), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	child1, err := nameserver.New("ns-child1.example", "192.0.2.90", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("child1", true))

	child2, err := nameserver.New("ns-child2.example", "192.0.2.91", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("child2", false))

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC11(ctx, &z)
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
		if dsErr != nil {
			t.Fatalf("dnssec11: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec11 did not finish")
	}

	if !hasEntryTag(entries, "DS11_INCONSISTENT_SIGNED_ZONE") {
		t.Fatalf("expected DS11_INCONSISTENT_SIGNED_ZONE")
	}
	if !hasEntryTag(entries, "DS11_NS_WITH_UNSIGNED_ZONE") {
		t.Fatalf("expected DS11_NS_WITH_UNSIGNED_ZONE")
	}
	if !hasEntryTag(entries, "DS11_NS_WITH_SIGNED_ZONE") {
		t.Fatalf("expected DS11_NS_WITH_SIGNED_ZONE")
	}
}

func TestDNSSEC11InconsistentDS(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParentNS := parentNameservers
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		parentNameservers = origParentNS
		method4 = origM4
		method5 = origM5
	})

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 12345
	ds.Algorithm = 8
	ds.DigestType = 1
	ds.Digest = "DEADBEEF"

	nsWithDS := newNameserver(t, "ns1.example", "192.0.2.80", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS(qname, ds)
		}
		return packet.Packet{}
	})
	nsWithoutDS := newNameserver(t, "ns2.example", "192.0.2.81", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS(qname, nil)
		}
		return packet.Packet{}
	})

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{nsWithDS, nsWithoutDS}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC11(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec11: %v", err)
	}
	if !hasEntryTag(entries, "DS11_INCONSISTENT_DS") {
		t.Fatalf("expected DS11_INCONSISTENT_DS")
	}
	if !hasEntryTag(entries, "DS11_PARENT_WITHOUT_DS") {
		t.Fatalf("expected DS11_PARENT_WITHOUT_DS")
	}
	if !hasEntryTag(entries, "DS11_PARENT_WITH_DS") {
		t.Fatalf("expected DS11_PARENT_WITH_DS")
	}
}

func TestDNSSEC11DSButUnsignedZone(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParentNS := parentNameservers
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		parentNameservers = origParentNS
		method4 = origM4
		method5 = origM5
	})

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 54321
	ds.Algorithm = 8
	ds.DigestType = 1
	ds.Digest = "FEEDBEEF"

	parentNS := newNameserver(t, "ns1.example", "192.0.2.82", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS(qname, ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "nschild.example", "192.0.2.83", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname))
		case "DNSKEY":
			return dnskeyPacket(qname, nil)
		default:
			return packet.Packet{}
		}
	})

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC11(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec11: %v", err)
	}
	if !hasEntryTag(entries, "DS11_DS_BUT_UNSIGNED_ZONE") {
		t.Fatalf("expected DS11_DS_BUT_UNSIGNED_ZONE")
	}
}

func TestDNSSEC13AlgoNotSigned(t *testing.T) {
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

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	nsRR.Ns = "ns1.example."

	makeRRSIG := func(owner string, typeCovered uint16) *dns.RRSIG {
		rr := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
		rr.TypeCovered = typeCovered
		rr.Algorithm = 13
		rr.Inception = 1
		rr.Expiration = 2
		rr.KeyTag = 12345
		rr.SignerName = dnsutil.Fqdn(owner)
		return rr
	}

	ns := newNameserver(t, "ns1.example", "192.0.2.90", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key, makeRRSIG(qname, dns.TypeDNSKEY))
		case "SOA":
			return answerPacket(qname, dns.TypeSOA, soaRecord(qname), makeRRSIG(qname, dns.TypeSOA))
		case "NS":
			return answerPacket(qname, dns.TypeNS, nsRR, makeRRSIG(qname, dns.TypeNS))
		default:
			return packet.Packet{}
		}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC13(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec13: %v", err)
	}
	if !hasEntryTag(entries, "DS13_ALGO_NOT_SIGNED_DNSKEY") {
		t.Fatalf("expected DS13_ALGO_NOT_SIGNED_DNSKEY")
	}
	if !hasEntryTag(entries, "DS13_ALGO_NOT_SIGNED_SOA") {
		t.Fatalf("expected DS13_ALGO_NOT_SIGNED_SOA")
	}
	if !hasEntryTag(entries, "DS13_ALGO_NOT_SIGNED_NS") {
		t.Fatalf("expected DS13_ALGO_NOT_SIGNED_NS")
	}
}

func TestDNSSEC13ParallelQueries(t *testing.T) {
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

	now := time.Now().UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	keySig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	soaSig := rrsigRecord("example", dns.TypeSOA, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())
	soaSig.Algorithm = 13
	nsSig := rrsigRecord("example", dns.TypeNS, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())
	nsSig.Algorithm = 13

	nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	nsRR.Ns = dnsutil.Fqdn("ns.example")

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "DNSKEY":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return answerPacket(qname, dns.TypeDNSKEY, key, keySig), nil
			case "SOA":
				return answerPacket(qname, dns.TypeSOA, soaRecord(qname), soaSig), nil
			case "NS":
				return answerPacket(qname, dns.TypeNS, nsRR, nsSig), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.121", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.122", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC13(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec13: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec13 did not finish")
	}

	if !hasEntryTag(entries, "DS13_ALGO_NOT_SIGNED_SOA") {
		t.Fatalf("expected DS13_ALGO_NOT_SIGNED_SOA")
	}
	if !hasEntryTag(entries, "DS13_ALGO_NOT_SIGNED_NS") {
		t.Fatalf("expected DS13_ALGO_NOT_SIGNED_NS")
	}

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS13_ALGO_NOT_SIGNED_SOA" {
			continue
		}
		if list, ok := entry.Args["ns_ip_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_ip_list for DS13_ALGO_NOT_SIGNED_SOA")
	}
	if nsList != "192.0.2.121;192.0.2.122" {
		t.Fatalf("expected deterministic ns_ip_list order, got %q", nsList)
	}
}

func TestDNSSEC14KeySizeSmallerThanRec(t *testing.T) {
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

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	if _, err := key.Generate(1024); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	ns := newNameserver(t, "ns1.example", "192.0.2.91", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnskeyPacket(qname, key)
		}
		return packet.Packet{}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC14(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec14: %v", err)
	}
	if !hasEntryTag(entries, "DNSKEY_SMALLER_THAN_REC") {
		t.Fatalf("expected DNSKEY_SMALLER_THAN_REC")
	}
}

func TestDNSSEC14ParallelDNSKEYQueries(t *testing.T) {
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

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	if _, err := key.Generate(1024); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype != "DNSKEY" {
				return packet.Packet{}, nil
			}
			select {
			case started <- id:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return dnskeyPacket(qname, key), nil
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.131", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.132", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC14(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec14: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec14 did not finish")
	}

	if !hasEntryTag(entries, "DNSKEY_SMALLER_THAN_REC") {
		t.Fatalf("expected DNSKEY_SMALLER_THAN_REC")
	}
}

func TestDNSSEC14NoResponseArgsSplit(t *testing.T) {
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

	ns := newNameserver(t, "ns1.example", "192.0.2.141", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return packet.Packet{}
		}
		return packet.Packet{}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC14(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec14: %v", err)
	}
	entry := firstEntryByTag(entries, "NO_RESPONSE")
	if entry == nil {
		t.Fatalf("expected NO_RESPONSE")
	}
	if _, ok := entry.Args["arg_schema"]; ok {
		t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
	}
	if nsArg, ok := entry.Args["ns"].(string); !ok || nsArg != "ns1.example" {
		t.Fatalf("expected ns=ns1.example, got %#v", entry.Args["ns"])
	}
	if nsArg, _ := entry.Args["ns"].(string); strings.Contains(nsArg, "/") {
		t.Fatalf("expected nameserver-only ns argument, got %q", nsArg)
	}
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.141" {
		t.Fatalf("expected address=192.0.2.141, got %#v", entry.Args["address"])
	}
}

func TestDNSSEC14NoResponseDNSKEYArgsSplit(t *testing.T) {
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

	ns := newNameserver(t, "ns1.example", "192.0.2.142", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return answerPacket(qname, dns.TypeDNSKEY)
		}
		return packet.Packet{}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC14(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec14: %v", err)
	}
	entry := firstEntryByTag(entries, "NO_RESPONSE_DNSKEY")
	if entry == nil {
		t.Fatalf("expected NO_RESPONSE_DNSKEY")
	}
	if _, ok := entry.Args["arg_schema"]; ok {
		t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
	}
	if nsArg, ok := entry.Args["ns"].(string); !ok || nsArg != "ns1.example" {
		t.Fatalf("expected ns=ns1.example, got %#v", entry.Args["ns"])
	}
	if nsArg, _ := entry.Args["ns"].(string); strings.Contains(nsArg, "/") {
		t.Fatalf("expected nameserver-only ns argument, got %q", nsArg)
	}
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.142" {
		t.Fatalf("expected address=192.0.2.142, got %#v", entry.Args["address"])
	}
}

func TestDNSSEC14IPv4DisabledArgsSplit(t *testing.T) {
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

	profile.Effective().Net.IPv4 = false

	ns := newNameserver(t, "ns1.example", "192.0.2.143", nil)
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC14(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec14: %v", err)
	}
	entry := firstEntryByTag(entries, "IPV4_DISABLED")
	if entry == nil {
		t.Fatalf("expected IPV4_DISABLED")
	}
	if _, ok := entry.Args["arg_schema"]; ok {
		t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
	}
	if nsArg, ok := entry.Args["ns"].(string); !ok || nsArg != "ns1.example" {
		t.Fatalf("expected ns=ns1.example, got %#v", entry.Args["ns"])
	}
	if nsArg, _ := entry.Args["ns"].(string); strings.Contains(nsArg, "/") {
		t.Fatalf("expected nameserver-only ns argument, got %q", nsArg)
	}
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.143" {
		t.Fatalf("expected address=192.0.2.143, got %#v", entry.Args["address"])
	}
}

func TestDNSSEC15NoCDSCDNSKEY(t *testing.T) {
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

	ns := newNameserver(t, "ns1.example", "192.0.2.92", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		default:
			return packet.Packet{}
		}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC15(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec15: %v", err)
	}
	if !hasEntryTag(entries, "DS15_NO_CDS_CDNSKEY") {
		t.Fatalf("expected DS15_NO_CDS_CDNSKEY")
	}
}

func TestDNSSEC15ParallelQueries(t *testing.T) {
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

	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = 12345
	cds.Algorithm = 8
	cds.DigestType = 2
	cds.Digest = "DEADBEEF"

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "CDS":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return answerPacket(qname, dns.TypeCDS, cds), nil
			case "CDNSKEY":
				return answerPacket(qname, dns.TypeCDNSKEY), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.231", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.232", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC15(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel CDS queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec15: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec15 did not finish")
	}

	if !hasEntryTag(entries, "DS15_HAS_CDS_NO_CDNSKEY") {
		t.Fatalf("expected DS15_HAS_CDS_NO_CDNSKEY")
	}

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS15_HAS_CDS_NO_CDNSKEY" {
			continue
		}
		if list, ok := entry.Args["ns_ip_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_ip_list for DS15_HAS_CDS_NO_CDNSKEY")
	}
	if nsList != "192.0.2.231;192.0.2.232" {
		t.Fatalf("expected deterministic ns_ip_list order, got %q", nsList)
	}
}

func TestDNSSEC16CDSWithoutDNSKEY(t *testing.T) {
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

	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = 12345
	cds.Algorithm = 8
	cds.DigestType = 1
	cds.Digest = "DEADBEEF"

	ns := newNameserver(t, "ns1.example", "192.0.2.93", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cds)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY)
		default:
			return packet.Packet{}
		}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC16(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec16: %v", err)
	}
	if !hasEntryTag(entries, "DS16_CDS_WITHOUT_DNSKEY") {
		t.Fatalf("expected DS16_CDS_WITHOUT_DNSKEY")
	}
}

func TestDNSSEC16ParallelQueries(t *testing.T) {
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

	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = 12345
	cds.Algorithm = 8
	cds.DigestType = 2
	cds.Digest = "DEADBEEF"

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "CDS":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return answerPacket(qname, dns.TypeCDS, cds), nil
			case "DNSKEY":
				return answerPacket(qname, dns.TypeDNSKEY), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.241", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.242", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC16(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel CDS queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec16: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec16 did not finish")
	}

	if !hasEntryTag(entries, "DS16_CDS_WITHOUT_DNSKEY") {
		t.Fatalf("expected DS16_CDS_WITHOUT_DNSKEY")
	}

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS16_CDS_WITHOUT_DNSKEY" {
			continue
		}
		if list, ok := entry.Args["ns_ip_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_ip_list for DS16_CDS_WITHOUT_DNSKEY")
	}
	if nsList != "192.0.2.241;192.0.2.242" {
		t.Fatalf("expected deterministic ns_ip_list order, got %q", nsList)
	}
}

func TestDNSSEC17CDNSKEYWithoutDNSKEY(t *testing.T) {
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

	cdnskey := &dns.CDNSKEY{DNSKEY: dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdnskey.Flags = dns.FlagZONE
	cdnskey.Protocol = 3
	cdnskey.Algorithm = 8
	cdnskey.PublicKey = "AwEAAc=="

	ns := newNameserver(t, "ns1.example", "192.0.2.94", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY, cdnskey)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY)
		default:
			return packet.Packet{}
		}
	})

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC17(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec17: %v", err)
	}
	if !hasEntryTag(entries, "DS17_CDNSKEY_WITHOUT_DNSKEY") {
		t.Fatalf("expected DS17_CDNSKEY_WITHOUT_DNSKEY")
	}
}

func TestDNSSEC17ParallelQueries(t *testing.T) {
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

	cdnskey := &dns.CDNSKEY{DNSKEY: dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdnskey.Flags = dns.FlagZONE | dns.FlagSEP
	cdnskey.Protocol = 3
	cdnskey.Algorithm = 8
	cdnskey.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "CDNSKEY":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return answerPacket(qname, dns.TypeCDNSKEY, cdnskey), nil
			case "DNSKEY":
				return answerPacket(qname, dns.TypeDNSKEY), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.243", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.New("ns2.example", "192.0.2.244", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC17(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel CDNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec17: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec17 did not finish")
	}

	if !hasEntryTag(entries, "DS17_CDNSKEY_WITHOUT_DNSKEY") {
		t.Fatalf("expected DS17_CDNSKEY_WITHOUT_DNSKEY")
	}

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS17_CDNSKEY_WITHOUT_DNSKEY" {
			continue
		}
		if list, ok := entry.Args["ns_ip_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_ip_list for DS17_CDNSKEY_WITHOUT_DNSKEY")
	}
	if nsList != "192.0.2.243;192.0.2.244" {
		t.Fatalf("expected deterministic ns_ip_list order, got %q", nsList)
	}
}

func TestDNSSEC18NoMatchRRSIGDS(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParentNS := parentNameservers
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		parentNameservers = origParentNS
		method4 = origM4
		method5 = origM5
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	keytag := key.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytag
	ds.Algorithm = 8
	ds.DigestType = 1
	ds.Digest = "DEADBEEF"

	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = keytag
	cds.Algorithm = 8
	cds.DigestType = 1
	cds.Digest = "DEADBEEF"

	cdnskey := &dns.CDNSKEY{DNSKEY: dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdnskey.Flags = dns.FlagZONE
	cdnskey.Protocol = 3
	cdnskey.Algorithm = 8
	cdnskey.PublicKey = "AwEAAc=="

	badKeytag := keytag + 1
	cdsSig := rrsigRecord("example", dns.TypeCDS, badKeytag, 1, 2)
	cdnskeySig := rrsigRecord("example", dns.TypeCDNSKEY, badKeytag, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.95", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS(qname, ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.96", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cds, cdsSig)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY, cdnskey, cdnskeySig)
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		default:
			return packet.Packet{}
		}
	})

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_NO_MATCH_CDS_RRSIG_DS") {
		t.Fatalf("expected DS18_NO_MATCH_CDS_RRSIG_DS")
	}
	if !hasEntryTag(entries, "DS18_NO_MATCH_CDNSKEY_RRSIG_DS") {
		t.Fatalf("expected DS18_NO_MATCH_CDNSKEY_RRSIG_DS")
	}
}

func TestDNSSEC18ParallelQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParentNS := parentNameservers
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		parentNameservers = origParentNS
		method4 = origM4
		method5 = origM5
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	keytag := key.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytag
	ds.Algorithm = 8
	ds.DigestType = 1
	ds.Digest = "DEADBEEF"

	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = keytag
	cds.Algorithm = 8
	cds.DigestType = 1
	cds.Digest = "DEADBEEF"

	cdnskey := &dns.CDNSKEY{DNSKEY: dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdnskey.Flags = dns.FlagZONE
	cdnskey.Protocol = 3
	cdnskey.Algorithm = 8
	cdnskey.PublicKey = "AwEAAc=="

	badKeytag := keytag + 1
	cdsSig := rrsigRecord("example", dns.TypeCDS, badKeytag, 1, 2)
	cdnskeySig := rrsigRecord("example", dns.TypeCDNSKEY, badKeytag, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.250", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS(qname, ds)
		}
		return packet.Packet{}
	})

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "CDS":
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return answerPacket(qname, dns.TypeCDS, cds, cdsSig), nil
			case "CDNSKEY":
				return answerPacket(qname, dns.TypeCDNSKEY, cdnskey, cdnskeySig), nil
			case "DNSKEY":
				return dnskeyPacket(qname, key), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	child1, err := nameserver.New("ns1.example", "192.0.2.251", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("ns1"))

	child2, err := nameserver.New("ns2.example", "192.0.2.252", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("ns2"))

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var dsErr error
	go func() {
		entries, dsErr = DNSSEC18(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel CDS queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if dsErr != nil {
			t.Fatalf("dnssec18: %v", dsErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("dnssec18 did not finish")
	}

	if !hasEntryTag(entries, "DS18_NO_MATCH_CDS_RRSIG_DS") {
		t.Fatalf("expected DS18_NO_MATCH_CDS_RRSIG_DS")
	}
	if !hasEntryTag(entries, "DS18_NO_MATCH_CDNSKEY_RRSIG_DS") {
		t.Fatalf("expected DS18_NO_MATCH_CDNSKEY_RRSIG_DS")
	}

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS18_NO_MATCH_CDS_RRSIG_DS" {
			continue
		}
		if list, ok := entry.Args["ns_ip_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_ip_list for DS18_NO_MATCH_CDS_RRSIG_DS")
	}
	if nsList != "192.0.2.251;192.0.2.252" {
		t.Fatalf("expected deterministic ns_ip_list order, got %q", nsList)
	}
}

func TestDNSSEC18ParallelOutputStable(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParentNS := parentNameservers
	origM4 := method4
	origM5 := method5
	t.Cleanup(func() {
		parentNameservers = origParentNS
		method4 = origM4
		method5 = origM5
	})

	runDNSSEC18 := func(parallel int) []*logger.Entry {
		profile.ResetEffective()
		if err := profile.Effective().Set("resolver.defaults.parallel", parallel); err != nil {
			t.Fatalf("set parallel: %v", err)
		}

		nameserver.EmptyCache()

		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 8
		key.PublicKey = "AwEAAc=="
		keytag := key.KeyTag()

		ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
		ds.KeyTag = keytag
		ds.Algorithm = 8
		ds.DigestType = 1
		ds.Digest = "DEADBEEF"

		cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
		cds.KeyTag = keytag
		cds.Algorithm = 8
		cds.DigestType = 1
		cds.Digest = "DEADBEEF"

		cdnskey := &dns.CDNSKEY{DNSKEY: dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
		cdnskey.Flags = dns.FlagZONE
		cdnskey.Protocol = 3
		cdnskey.Algorithm = 8
		cdnskey.PublicKey = "AwEAAc=="

		badKeytag := keytag + 1
		cdsSig := rrsigRecord("example", dns.TypeCDS, badKeytag, 1, 2)
		cdnskeySig := rrsigRecord("example", dns.TypeCDNSKEY, badKeytag, 1, 2)

		parentNS := newNameserver(t, "pns1.example", "192.0.2.253", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if qtype == "DS" {
				return dsPacketFromDS(qname, ds)
			}
			return packet.Packet{}
		})

		childHook := func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			switch qtype {
			case "CDS":
				return answerPacket(qname, dns.TypeCDS, cds, cdsSig)
			case "CDNSKEY":
				return answerPacket(qname, dns.TypeCDNSKEY, cdnskey, cdnskeySig)
			case "DNSKEY":
				return dnskeyPacket(qname, key)
			default:
				return packet.Packet{}
			}
		}

		child1 := newNameserver(t, "ns1.example", "192.0.2.254", childHook)
		child2 := newNameserver(t, "ns2.example", "192.0.2.255", childHook)

		parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{parentNS}, nil
		}
		method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{child1, child2}, nil
		}
		method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		}

		z := zone.Zone{Name: dnsname.New("example")}
		entries, err := DNSSEC18(context.Background(), &z)
		if err != nil {
			t.Fatalf("dnssec18: %v", err)
		}
		if !hasEntryTag(entries, "DS18_NO_MATCH_CDS_RRSIG_DS") || !hasEntryTag(entries, "DS18_NO_MATCH_CDNSKEY_RRSIG_DS") {
			t.Fatalf("expected no-match tags in dnssec18 output")
		}
		return entries
	}

	sequentialEntries := runDNSSEC18(1)
	parallelEntries := runDNSSEC18(2)

	sequentialNormalized := normalizeEntriesForComparison(sequentialEntries)
	parallelNormalized := normalizeEntriesForComparison(parallelEntries)
	if len(sequentialNormalized) != len(parallelNormalized) {
		t.Fatalf("entry count changed with parallelism: sequential=%v parallel=%v", sequentialNormalized, parallelNormalized)
	}
	for i := range sequentialNormalized {
		if sequentialNormalized[i] != parallelNormalized[i] {
			t.Fatalf("entry[%d] changed with parallelism: %q != %q", i, sequentialNormalized[i], parallelNormalized[i])
		}
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

func normalizeEntriesForComparison(entries []*logger.Entry) []string {
	normalized := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if strings.EqualFold(entry.Module, "System") &&
			strings.EqualFold(entry.Testcase, "Unspecified") &&
			strings.HasPrefix(strings.ToUpper(entry.Level()), "DEBUG") {
			// System debug entries are intentionally verbose and their emission
			// order can vary under parallel execution.
			continue
		}
		item := entry.Module + ":" + entry.Testcase + ":" + entry.Tag
		if args := entry.ArgString(); args != "" {
			item += " " + args
		}
		normalized = append(normalized, item)
	}
	return normalized
}

func dsPacket(owner string, keytag uint16, algo uint8, digestType uint8) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.TypeDS)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	dsRR := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	dsRR.KeyTag = keytag
	dsRR.Algorithm = algo
	dsRR.DigestType = digestType
	dsRR.Digest = "DEADBEEF"
	msg.Answer = []dns.RR{dsRR}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func dsPacketFromDS(owner string, ds *dns.DS) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.TypeDS)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	if ds != nil {
		msg.Answer = append(msg.Answer, ds)
	}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func dnskeyPacket(owner string, key *dns.DNSKEY) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.TypeDNSKEY)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	if key != nil {
		msg.Answer = append(msg.Answer, key)
	}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func nsecPacket(owner string) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.TypeNSEC)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	return packet.Packet{Msg: msg}
}

func nsec3Packet(owner string, nsec3 *dns.NSEC3) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.TypeNSEC)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	if nsec3 != nil {
		msg.Ns = append(msg.Ns, nsec3)
	}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func rrsigRecord(owner string, typeCovered uint16, keytag uint16, inception int64, expiration int64) *dns.RRSIG {
	rr := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	rr.TypeCovered = typeCovered
	rr.Algorithm = 8
	rr.Inception = uint32(inception)
	rr.Expiration = uint32(expiration)
	rr.KeyTag = keytag
	rr.SignerName = dnsutil.Fqdn(owner)
	return rr
}

func answerPacket(owner string, qtype uint16, answers ...dns.RR) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), qtype)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = append(msg.Answer, answers...)
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func soaRecord(owner string) *dns.SOA {
	rr := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	rr.Ns = "ns1.example."
	rr.Mbox = "hostmaster.example."
	rr.Serial = 1
	rr.Refresh = 60
	rr.Retry = 60
	rr.Expire = 60
	rr.Minttl = 60
	return rr
}
