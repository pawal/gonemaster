package dnssec

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/badkeys"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
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

	origGetParent := parentNameservers
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentNameservers = origGetParent
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origGetParent := parentNameservers
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentNameservers = origGetParent
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origGetParent := parentNameservers
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentNameservers = origGetParent
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origGetParent := parentNameservers
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentNameservers = origGetParent
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origGetParent := parentNameservers
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentNameservers = origGetParent
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origGetParent := parentNameservers
	origZoneParent := zoneParent
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentNameservers = origGetParent
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origGetParent := parentNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentNameservers = origGetParent
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origGetParent := parentNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentNameservers = origGetParent
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origGetParent := parentNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentNameservers = origGetParent
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	var gotAddresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS" {
			continue
		}
		if addresses, ok := entry.Args["addresses"].([]string); ok {
			gotAddresses = addresses
		}
		if _, ok := entry.Args["ns_ip_list"]; ok {
			t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotAddresses) == 0 {
		t.Fatalf("expected addresses for DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS")
	}
	if strings.Join(gotAddresses, ";") != "192.0.2.101;192.0.2.102" {
		t.Fatalf("expected deterministic addresses order, got %#v", gotAddresses)
	}
}

func TestDNSSEC03NoNSEC3(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM45 := authoritativeNS
	t.Cleanup(func() {
		authoritativeNS = origM45
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

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM45 := authoritativeNS
	t.Cleanup(func() {
		authoritativeNS = origM45
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

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM45 := authoritativeNS
	t.Cleanup(func() {
		authoritativeNS = origM45
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

	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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
	expirationEntry := firstEntryByTag(entries, "RRSIG_EXPIRATION")
	if expirationEntry == nil {
		t.Fatalf("expected RRSIG_EXPIRATION")
	}
	dateRaw, ok := expirationEntry.Args["date"].(string)
	if !ok || dateRaw == "" {
		t.Fatalf("expected non-empty RFC3339 date string, got %#v", expirationEntry.Args["date"])
	}
	parsed, err := time.Parse(time.RFC3339, dateRaw)
	if err != nil {
		t.Fatalf("expected RFC3339 date, got %q (%v)", dateRaw, err)
	}
	wantExpiration := time.Unix(now.Unix()-1, 0).UTC()
	if !parsed.Equal(wantExpiration) {
		t.Fatalf("expected expiration %s, got %s", wantExpiration.Format(time.RFC3339), parsed.Format(time.RFC3339))
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.20"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.33"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.34"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
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
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	newNameserver(t, "ns2.example", "192.0.2.21", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(qname, nil)
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.21"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	newNameserver(t, "ns3.example", "192.0.2.22", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return packet.Packet{}
		}
		return packet.Packet{}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns3.example"),
				Address:    netip.MustParseAddr("192.0.2.22"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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
	entry := firstEntryByTag(entries, "EXTRA_PROCESSING_OK")
	if entry == nil {
		t.Fatalf("missing EXTRA_PROCESSING_OK entry")
	}
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.30" {
		t.Fatalf("expected address=192.0.2.30, got %#v", entry.Args["address"])
	}
	if _, ok := entry.Args["server"]; ok {
		t.Fatalf("legacy key server should not be present: %#v", entry.Args)
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
	entry := firstEntryByTag(entries, "EXTRA_PROCESSING_BROKEN")
	if entry == nil {
		t.Fatalf("missing EXTRA_PROCESSING_BROKEN entry")
	}
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.31" {
		t.Fatalf("expected address=192.0.2.31, got %#v", entry.Args["address"])
	}
	if _, ok := entry.Args["server"]; ok {
		t.Fatalf("legacy key server should not be present: %#v", entry.Args)
	}
}

func TestDNSSEC07SignedZone(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	origParent := parentNameservers
	origZoneParent := zoneParent
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		parentNameservers = origParent
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.40"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}
	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	origParent := parentNameservers
	origZoneParent := zoneParent
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		parentNameservers = origParent
		zoneParent = origZoneParent
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
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
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	origParent := parentNameservers
	origZoneParent := zoneParent
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		parentNameservers = origParent
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns-child.example"),
				Address:    netip.MustParseAddr("192.0.2.62"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	origParent := parentNameservers
	origZoneParent := zoneParent
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		parentNameservers = origParent
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.42"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}
	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	origParent := parentNameservers
	origZoneParent := zoneParent
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		parentNameservers = origParent
		zoneParent = origZoneParent
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
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
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	origParent := parentNameservers
	origZoneParent := zoneParent
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		parentNameservers = origParent
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

	dsSig := rrsigRecord("example", dns.TypeDS, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	newNameserver(t, "ns-parent-no-ds.example", "192.0.2.181", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return answerPacket(qname, dns.TypeDS, ds)
		}
		return packet.Packet{}
	})

	newNameserver(t, "ns-parent-with-ds.example", "192.0.2.182", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return answerPacket(qname, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.180"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}
	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		nsNoDS, _ := nameserver.New("ns-parent-no-ds.example", "192.0.2.181", nil)
		nsWithDS, _ := nameserver.New("ns-parent-with-ds.example", "192.0.2.182", nil)
		return []nameserver.Nameserver{nsNoDS, nsWithDS}, nil
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
	if servers[0]["ns"] != "ns-parent-no-ds.example" {
		t.Fatalf("unexpected typed server payload for DS07_NO_DS_ON_PARENT_SERVER: %#v", servers[0])
	}
	if _, ok := noDS.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", noDS.Args)
	}
	if hasEntryTag(entries, "DS07_NO_DS_FOR_SIGNED_ZONE") {
		t.Fatalf("DS07_NO_DS_FOR_SIGNED_ZONE should not fire when at least one parent serves DS")
	}
}

func TestDNSSEC07NoDSOnAllParentServersSuppressesPerServerTag(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	origParent := parentNameservers
	origZoneParent := zoneParent
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		parentNameservers = origParent
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

	newNameserver(t, "ns-parent-a.example", "192.0.2.181", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return answerPacket(qname, dns.TypeDS, ds)
		}
		return packet.Packet{}
	})
	newNameserver(t, "ns-parent-b.example", "192.0.2.182", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return answerPacket(qname, dns.TypeDS, ds)
		}
		return packet.Packet{}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.180"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}
	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		nsA, _ := nameserver.New("ns-parent-a.example", "192.0.2.181", nil)
		nsB, _ := nameserver.New("ns-parent-b.example", "192.0.2.182", nil)
		return []nameserver.Nameserver{nsA, nsB}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	if !hasEntryTag(entries, "DS07_NO_DS_FOR_SIGNED_ZONE") {
		t.Fatalf("expected DS07_NO_DS_FOR_SIGNED_ZONE when no parent serves DS")
	}
	if hasEntryTag(entries, "DS07_NO_DS_ON_PARENT_SERVER") {
		t.Fatalf("DS07_NO_DS_ON_PARENT_SERVER should be suppressed when every parent fails to return DS")
	}
}

func TestDNSSECAllParallelOutputStable(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	origParent := parentNameservers
	origZoneParent := zoneParent
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		parentNameservers = origParent
		zoneParent = origZoneParent
	})

	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}
	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
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
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	var gotAddresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS08_MISSING_RRSIG_IN_RESPONSE" {
			continue
		}
		if addresses, ok := entry.Args["addresses"].([]string); ok {
			gotAddresses = addresses
		}
		if _, ok := entry.Args["ns_ip_list"]; ok {
			t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotAddresses) == 0 {
		t.Fatalf("expected addresses for DS08_MISSING_RRSIG_IN_RESPONSE")
	}
	if strings.Join(gotAddresses, ";") != "192.0.2.101;192.0.2.102" {
		t.Fatalf("expected deterministic addresses order, got %#v", gotAddresses)
	}
}

func TestDNSSEC09MissingRRSIG(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	var gotAddresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS09_MISSING_RRSIG_IN_RESPONSE" {
			continue
		}
		if addresses, ok := entry.Args["addresses"].([]string); ok {
			gotAddresses = addresses
		}
		if _, ok := entry.Args["ns_ip_list"]; ok {
			t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotAddresses) == 0 {
		t.Fatalf("expected addresses for DS09_MISSING_RRSIG_IN_RESPONSE")
	}
	if strings.Join(gotAddresses, ";") != "192.0.2.111;192.0.2.112" {
		t.Fatalf("expected deterministic addresses order, got %#v", gotAddresses)
	}
}

func TestDNSSEC10MissingSignature(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.70"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
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

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
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
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
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

// Two NSEC3PARAM RRs at the apex (a legitimate parameter-rollover shape per
// RFC 5155, which imposes no cardinality on the apex NSEC3PARAM RRset) must
// not produce DS10_NSEC3PARAM_MISMATCHES_APEX, and the retired
// DS10_ERR_MULT_NSEC3PARAM tag must never appear.
func TestDNSSEC10MultipleNSEC3PARAMAllApex(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	apex := dnsutil.Fqdn("example")

	key := &dns.DNSKEY{Hdr: dns.Header{Name: apex, Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	param1 := &dns.NSEC3PARAM{Hdr: dns.Header{Name: apex, Class: dns.ClassINET, TTL: 60}}
	param1.Hash = 1
	param1.Iterations = 0
	param1.SaltLength = 0
	param1.Salt = ""

	param2 := &dns.NSEC3PARAM{Hdr: dns.Header{Name: apex, Class: dns.ClassINET, TTL: 60}}
	param2.Hash = 1
	param2.Iterations = 5
	param2.SaltLength = 2
	param2.Salt = "abcd"

	newNameserver(t, "ns1.example", "192.0.2.80", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
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
			msg.Answer = append(msg.Answer, param1, param2)
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		default:
			return packet.Packet{}
		}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.80"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}
	if hasEntryTag(entries, "DS10_NSEC3PARAM_MISMATCHES_APEX") {
		t.Fatalf("unexpected DS10_NSEC3PARAM_MISMATCHES_APEX when all NSEC3PARAM RRs are at apex")
	}
	if hasEntryTag(entries, "DS10_ERR_MULT_NSEC3PARAM") {
		t.Fatalf("retired tag DS10_ERR_MULT_NSEC3PARAM must not be emitted")
	}
}

// Two NSEC3PARAM RRs where one has the wrong owner must trigger
// DS10_NSEC3PARAM_MISMATCHES_APEX (proving the apex-owner check loops over
// every RR in the RRset, not just the first one).
func TestDNSSEC10MultipleNSEC3PARAMOneOffApex(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	apex := dnsutil.Fqdn("example")
	offApex := dnsutil.Fqdn("sub.example")

	key := &dns.DNSKEY{Hdr: dns.Header{Name: apex, Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	apexParam := &dns.NSEC3PARAM{Hdr: dns.Header{Name: apex, Class: dns.ClassINET, TTL: 60}}
	apexParam.Hash = 1

	offApexParam := &dns.NSEC3PARAM{Hdr: dns.Header{Name: offApex, Class: dns.ClassINET, TTL: 60}}
	offApexParam.Hash = 1

	newNameserver(t, "ns1.example", "192.0.2.81", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
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
			msg.Answer = append(msg.Answer, apexParam, offApexParam)
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		default:
			return packet.Packet{}
		}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.81"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}
	if !hasEntryTag(entries, "DS10_NSEC3PARAM_MISMATCHES_APEX") {
		t.Fatalf("expected DS10_NSEC3PARAM_MISMATCHES_APEX when one of multiple NSEC3PARAM RRs is off-apex")
	}
	if hasEntryTag(entries, "DS10_ERR_MULT_NSEC3PARAM") {
		t.Fatalf("retired tag DS10_ERR_MULT_NSEC3PARAM must not be emitted")
	}
}

// serverRows normalises the `servers` log argument (which `setTypedServersFromNames`
// can construct as either []any{map[string]any{...}} or
// []map[string]any{...}, depending on the call site) to a single shape so
// tests can read it. Each row is the {ns, address} object.
func serverRows(t *testing.T, v any) []map[string]any {
	t.Helper()
	switch s := v.(type) {
	case []map[string]any:
		return s
	case []any:
		out := make([]map[string]any, 0, len(s))
		for _, item := range s {
			row, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("server entry has unexpected type %T (%#v)", item, item)
			}
			out = append(out, row)
		}
		return out
	default:
		t.Fatalf("'servers' arg has unexpected type %T (%#v)", v, v)
		return nil
	}
}

// nsecAuthorityNSECResponse builds a NODATA response to an NSEC query that
// carries the NSEC RR in the authority section (RFC 4470 white-lies / RFC 9824
// compact denial of existence). The SOA RR is included so that NODATA-shape
// checks pass; no RRSIG is included (the test deliberately ignores signature
// coverage).
func nsecAuthorityNSECResponse(qname string, apex string) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.TypeNSEC)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next." + apex)
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeRRSIG}
	msg.Ns = append(msg.Ns, soaRecord(apex), nsec)
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

// nsecInAnswerResponse builds a standard NSEC query response with the NSEC RR
// in the answer section (the conventional, non-RFC-4470 shape).
func nsecInAnswerResponse(qname string, apex string) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.TypeNSEC)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next." + apex)
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG}
	msg.Answer = []dns.RR{nsec}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

// emptyNSEC3PARAMResponse builds a NODATA NSEC3PARAM response for an NSEC
// zone: NSEC in authority confirms NSEC3PARAM does not exist at the apex.
func emptyNSEC3PARAMResponse(qname string, apex string) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.TypeNSEC3PARAM)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next." + apex)
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG}
	msg.Ns = append(msg.Ns, soaRecord(apex), nsec)
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

// TestDNSSEC10NonstandardNSECResponseEmitted exercises the single-nameserver
// case where the NSEC query response carries NSEC in the authority section.
// The tag DS10_NONSTANDARD_NSEC_RESPONSE must fire, the false-positive
// DS10_INCONSISTENT_NSEC must not, and the zone must still register as having
// NSEC evidence (DS10_HAS_NSEC).
func TestDNSSEC10NonstandardNSECResponseEmitted(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	log := logger.New()
	log.SetProfile(profile.Effective())
	util.SetLogger(log)
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	apex := "example"
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	newNameserver(t, "ns1.example", "192.0.2.90", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		case "NSEC":
			return nsecAuthorityNSECResponse(qname, apex)
		case "NSEC3PARAM":
			return emptyNSEC3PARAMResponse(qname, apex)
		default:
			return packet.Packet{}
		}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.90"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}

	z, err := zone.New(apex)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}

	if !hasEntryTag(entries, "DS10_NONSTANDARD_NSEC_RESPONSE") {
		t.Fatalf("expected DS10_NONSTANDARD_NSEC_RESPONSE when NSEC arrives in authority section")
	}
	if hasEntryTag(entries, "DS10_INCONSISTENT_NSEC") {
		t.Fatalf("DS10_INCONSISTENT_NSEC must not fire for a single NSEC-in-authority responder")
	}
	if !hasEntryTag(entries, "DS10_HAS_NSEC") {
		t.Fatalf("authority-section NSEC must still count as NSEC evidence (DS10_HAS_NSEC)")
	}

	entry := firstEntryByTag(entries, "DS10_NONSTANDARD_NSEC_RESPONSE")
	if entry == nil {
		t.Fatalf("entry lookup returned nil after positive hasEntryTag")
	}
	if !strings.EqualFold(entry.Level(), "NOTICE") {
		t.Fatalf("DS10_NONSTANDARD_NSEC_RESPONSE level = %q, want NOTICE", entry.Level())
	}
	servers, ok := entry.Args["servers"]
	if !ok {
		t.Fatalf("DS10_NONSTANDARD_NSEC_RESPONSE missing 'servers' arg, got args=%v", entry.Args)
	}
	rows := serverRows(t, servers)
	if len(rows) != 1 {
		t.Fatalf("DS10_NONSTANDARD_NSEC_RESPONSE 'servers' length = %d, want 1 (got %#v)", len(rows), servers)
	}
}

// TestDNSSEC10NonstandardNSECResponseNotEmittedForStandard verifies that the
// new tag stays silent when every nameserver returns NSEC in the answer
// section (the conventional shape).
func TestDNSSEC10NonstandardNSECResponseNotEmittedForStandard(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	log := logger.New()
	log.SetProfile(profile.Effective())
	util.SetLogger(log)
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	apex := "example"
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	newNameserver(t, "ns1.example", "192.0.2.91", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		case "NSEC":
			return nsecInAnswerResponse(qname, apex)
		case "NSEC3PARAM":
			return emptyNSEC3PARAMResponse(qname, apex)
		default:
			return packet.Packet{}
		}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.91"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}

	z, err := zone.New(apex)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}

	if hasEntryTag(entries, "DS10_NONSTANDARD_NSEC_RESPONSE") {
		t.Fatalf("DS10_NONSTANDARD_NSEC_RESPONSE must not fire for conventional NSEC-in-answer responders")
	}
	if !hasEntryTag(entries, "DS10_HAS_NSEC") {
		t.Fatalf("expected DS10_HAS_NSEC for a normal NSEC zone")
	}
}

// TestDNSSEC10NonstandardNSECResponseMixedServers checks the mixed case: one
// nameserver returns NSEC-in-answer, another NSEC-in-authority. Both shapes
// represent valid NSEC evidence, so the consistency check must not fire
// (DS10_INCONSISTENT_NSEC absent), but the non-standard tag must call out
// only the nameserver that used the authority-section shape.
func TestDNSSEC10NonstandardNSECResponseMixedServers(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	log := logger.New()
	log.SetProfile(profile.Effective())
	util.SetLogger(log)
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	apex := "example"
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	newNameserver(t, "ns1.example", "192.0.2.92", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		case "NSEC":
			return nsecInAnswerResponse(qname, apex)
		case "NSEC3PARAM":
			return emptyNSEC3PARAMResponse(qname, apex)
		default:
			return packet.Packet{}
		}
	})
	newNameserver(t, "ns2.example", "192.0.2.93", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		case "NSEC":
			return nsecAuthorityNSECResponse(qname, apex)
		case "NSEC3PARAM":
			return emptyNSEC3PARAMResponse(qname, apex)
		default:
			return packet.Packet{}
		}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.92"),
				HasAddress: true,
			},
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.93"),
				HasAddress: true,
			},
		}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}

	z, err := zone.New(apex)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}

	if !hasEntryTag(entries, "DS10_NONSTANDARD_NSEC_RESPONSE") {
		t.Fatalf("expected DS10_NONSTANDARD_NSEC_RESPONSE for the authority-section responder")
	}
	if hasEntryTag(entries, "DS10_INCONSISTENT_NSEC") {
		t.Fatalf("DS10_INCONSISTENT_NSEC must not fire when authority-section NSEC is treated as equivalent evidence")
	}
	if !hasEntryTag(entries, "DS10_HAS_NSEC") {
		t.Fatalf("expected DS10_HAS_NSEC when both nameservers present NSEC evidence")
	}

	entry := firstEntryByTag(entries, "DS10_NONSTANDARD_NSEC_RESPONSE")
	rows := serverRows(t, entry.Args["servers"])
	if len(rows) != 1 {
		t.Fatalf("'servers' length = %d, want 1 (only the non-standard responder)", len(rows))
	}
	if got, _ := rows[0]["ns"].(string); !strings.HasPrefix(got, "ns2.example") {
		t.Fatalf("servers[0].ns = %q, want ns2.example...", got)
	}
}

func TestDNSSEC11ParallelParentQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParent := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentApexNameservers = origParent
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent1, parent2}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origParent := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	origHasFake := hasFakeAddresses
	t.Cleanup(func() {
		parentApexNameservers = origParent
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origParentNS := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentApexNameservers = origParentNS
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{nsWithDS, nsWithoutDS}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origParentNS := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentApexNameservers = origParentNS
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	var gotAddresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS13_ALGO_NOT_SIGNED_SOA" {
			continue
		}
		if addresses, ok := entry.Args["addresses"].([]string); ok {
			gotAddresses = addresses
		}
		if _, ok := entry.Args["ns_ip_list"]; ok {
			t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotAddresses) == 0 {
		t.Fatalf("expected addresses for DS13_ALGO_NOT_SIGNED_SOA")
	}
	if strings.Join(gotAddresses, ";") != "192.0.2.121;192.0.2.122" {
		t.Fatalf("expected deterministic addresses order, got %#v", gotAddresses)
	}
}

func TestDNSSEC14KeySizeSmallerThanRec(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	ns := newNameserver(t, "ns1.example", "192.0.2.141", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return packet.Packet{}
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	ns := newNameserver(t, "ns1.example", "192.0.2.142", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return answerPacket(qname, dns.TypeDNSKEY)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	profile.Effective().Net.IPv4 = false

	ns := newNameserver(t, "ns1.example", "192.0.2.143", nil)
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	var gotAddresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS15_HAS_CDS_NO_CDNSKEY" {
			continue
		}
		if addresses, ok := entry.Args["addresses"].([]string); ok {
			gotAddresses = addresses
		}
		if _, ok := entry.Args["ns_ip_list"]; ok {
			t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotAddresses) == 0 {
		t.Fatalf("expected addresses for DS15_HAS_CDS_NO_CDNSKEY")
	}
	if strings.Join(gotAddresses, ";") != "192.0.2.231;192.0.2.232" {
		t.Fatalf("expected deterministic addresses order, got %#v", gotAddresses)
	}
}

// TestDNSSEC15IgnoresNonMUSTCDSDigest verifies the RFC 9975 digest-type
// filter: CDS records whose digest type is not designated MUST in IANA's
// "Implement for DNSSEC Delegation" column must not participate in the
// cross-server consistency check.
//
// NS1 publishes both a SHA-256 (digest type 2, MUST) and a SHA-1
// (digest type 1, MUST NOT) CDS for the same key. NS2 publishes only the
// SHA-256 CDS. The raw RRsets differ; the MUST-only views are identical.
// Before the filter was added, this scenario produced
// DS15_INCONSISTENT_CDS. After the filter it must not.
func TestDNSSEC15IgnoresNonMUSTCDSDigest(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	cdsSHA256 := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdsSHA256.KeyTag = 12345
	cdsSHA256.Algorithm = 8
	cdsSHA256.DigestType = 2
	cdsSHA256.Digest = "DEADBEEF"

	cdsSHA1 := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdsSHA1.KeyTag = 12345
	cdsSHA1.Algorithm = 8
	cdsSHA1.DigestType = 1
	cdsSHA1.Digest = "ABCD"

	ns1 := newNameserver(t, "ns1.example", "192.0.2.241", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cdsSHA256, cdsSHA1)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		}
		return packet.Packet{}
	})

	ns2 := newNameserver(t, "ns2.example", "192.0.2.242", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cdsSHA256)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC15(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec15: %v", err)
	}
	if hasEntryTag(entries, "DS15_INCONSISTENT_CDS") {
		t.Fatalf("DS15_INCONSISTENT_CDS must not fire when only the SHA-1 CDS diverges across servers (RFC 9975 digest filter)")
	}
}

// TestDNSSEC15InconsistencyOnMUSTCDSDigest is the counterpart of
// TestDNSSEC15IgnoresNonMUSTCDSDigest. When two servers diverge on a CDS
// record that uses a MUST digest type, the filter must not mask the
// divergence: DS15_INCONSISTENT_CDS is still required.
func TestDNSSEC15InconsistencyOnMUSTCDSDigest(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	cdsA := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdsA.KeyTag = 12345
	cdsA.Algorithm = 8
	cdsA.DigestType = 2
	cdsA.Digest = "DEADBEEF"

	cdsB := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdsB.KeyTag = 12345
	cdsB.Algorithm = 8
	cdsB.DigestType = 2
	cdsB.Digest = "CAFEBABE"

	ns1 := newNameserver(t, "ns1.example", "192.0.2.243", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cdsA)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		}
		return packet.Packet{}
	})

	ns2 := newNameserver(t, "ns2.example", "192.0.2.244", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cdsB)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC15(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec15: %v", err)
	}
	if !hasEntryTag(entries, "DS15_INCONSISTENT_CDS") {
		t.Fatalf("expected DS15_INCONSISTENT_CDS when MUST-digest CDS records diverge across servers")
	}
}

// TestDNSSEC15MismatchOnAlgorithmDifference verifies that the CDS/CDNSKEY
// cross-RRset match requires algorithm equality as well as key-tag
// equality. RFC 9975 talks in terms of the underlying key; two records
// that share a key tag but use different DNSSEC algorithms are not the
// same key. Before this fix the keytag-only match silently treated them
// as paired and did not emit DS15_MISMATCH_CDS_CDNSKEY.
func TestDNSSEC15MismatchOnAlgorithmDifference(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	cdnskey := &dns.CDNSKEY{DNSKEY: dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdnskey.Flags = dns.FlagZONE
	cdnskey.Protocol = 3
	cdnskey.Algorithm = 13
	cdnskey.PublicKey = "AwEAAc=="

	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = cdnskey.KeyTag()
	cds.Algorithm = 8
	cds.DigestType = 2
	cds.Digest = "DEADBEEF"

	if cds.Algorithm == cdnskey.Algorithm {
		t.Fatalf("test setup invalid: CDS and CDNSKEY must have differing algorithms")
	}

	ns := newNameserver(t, "ns1.example", "192.0.2.245", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cds)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY, cdnskey)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC15(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec15: %v", err)
	}
	if !hasEntryTag(entries, "DS15_MISMATCH_CDS_CDNSKEY") {
		t.Fatalf("expected DS15_MISMATCH_CDS_CDNSKEY when CDS and CDNSKEY share a key tag but use different DNSSEC algorithms")
	}
}

func TestDNSSEC16CDSWithoutDNSKEY(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	var gotAddresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS16_CDS_WITHOUT_DNSKEY" {
			continue
		}
		if addresses, ok := entry.Args["addresses"].([]string); ok {
			gotAddresses = addresses
		}
		if _, ok := entry.Args["ns_ip_list"]; ok {
			t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotAddresses) == 0 {
		t.Fatalf("expected addresses for DS16_CDS_WITHOUT_DNSKEY")
	}
	if strings.Join(gotAddresses, ";") != "192.0.2.241;192.0.2.242" {
		t.Fatalf("expected deterministic addresses order, got %#v", gotAddresses)
	}
}

func TestDNSSEC17CDNSKEYWithoutDNSKEY(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
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

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	var gotAddresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS17_CDNSKEY_WITHOUT_DNSKEY" {
			continue
		}
		if addresses, ok := entry.Args["addresses"].([]string); ok {
			gotAddresses = addresses
		}
		if _, ok := entry.Args["ns_ip_list"]; ok {
			t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotAddresses) == 0 {
		t.Fatalf("expected addresses for DS17_CDNSKEY_WITHOUT_DNSKEY")
	}
	if strings.Join(gotAddresses, ";") != "192.0.2.243;192.0.2.244" {
		t.Fatalf("expected deterministic addresses order, got %#v", gotAddresses)
	}
}

func TestDNSSEC18NoMatchRRSIGDS(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParentNS := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentApexNameservers = origParentNS
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	origParentNS := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentApexNameservers = origParentNS
		glueNameservers = origM4
		apexNameservers = origM5
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

	parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

	var gotAddresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS18_NO_MATCH_CDS_RRSIG_DS" {
			continue
		}
		if addresses, ok := entry.Args["addresses"].([]string); ok {
			gotAddresses = addresses
		}
		if _, ok := entry.Args["ns_ip_list"]; ok {
			t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
		}
		break
	}
	if len(gotAddresses) == 0 {
		t.Fatalf("expected addresses for DS18_NO_MATCH_CDS_RRSIG_DS")
	}
	if strings.Join(gotAddresses, ";") != "192.0.2.251;192.0.2.252" {
		t.Fatalf("expected deterministic addresses order, got %#v", gotAddresses)
	}
}

func TestDNSSEC18ParallelOutputStable(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origParentNS := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentApexNameservers = origParentNS
		glueNameservers = origM4
		apexNameservers = origM5
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

		parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{parentNS}, nil
		}
		glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{child1, child2}, nil
		}
		apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
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

// ---- DNSSEC18 rollover-detection tests ----------------------------------------

// setDNSSEC18Mocks wires the three injectable function variables and returns a
// cleanup function.  Both glueNameservers and apexNameservers serve childNSs; parentNSs is
// served by parentApexNameservers.
func setDNSSEC18Mocks(
	t *testing.T,
	parentNSs []nameserver.Nameserver,
	childNSs []nameserver.Nameserver,
) {
	t.Helper()
	origPNS := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		parentApexNameservers = origPNS
		glueNameservers = origM4
		apexNameservers = origM5
	})
	parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return parentNSs, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return childNSs, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
}

// makeSEPKey returns a DNSKEY with the zone and SEP flags set.
func makeSEPKey(owner string, pubKey string) *dns.DNSKEY {
	k := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	k.Flags = dns.FlagZONE | dns.FlagSEP
	k.Protocol = 3
	k.Algorithm = 8
	k.PublicKey = pubKey
	return k
}

// dnssec18Setup initialises common test state and returns an env ready for DNSSEC18.
func dnssec18Setup(t *testing.T) {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })
}

func TestDNSSEC18CDSMatchesDS(t *testing.T) {
	dnssec18Setup(t)

	key := makeSEPKey("example", "AwEAAc==")
	keytag := key.KeyTag()

	// DS and CDS have identical (KeyTag,Algorithm,DigestType,Digest) tuples.
	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytag
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "DEADBEEF"

	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = keytag
	cds.Algorithm = 8
	cds.DigestType = 2
	cds.Digest = "DEADBEEF"

	cdsSig := rrsigRecord("example", dns.TypeCDS, keytag, 1, 2)
	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.70", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.71", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cds, cdsSig)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY) // no CDNSKEY records
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_CDS_MATCHES_DS") {
		t.Fatal("expected DS18_CDS_MATCHES_DS")
	}
	if hasEntryTag(entries, "DS18_CDS_ROLLOVER_SIGNALED") {
		t.Fatal("unexpected DS18_CDS_ROLLOVER_SIGNALED when CDS matches DS")
	}
	e := firstEntryByTag(entries, "DS18_CDS_MATCHES_DS")
	cdsKTs, ok := e.Args["cds_keytags"].([]uint16)
	if !ok || len(cdsKTs) != 1 || cdsKTs[0] != keytag {
		t.Errorf("expected cds_keytags=[%d], got %v", keytag, e.Args["cds_keytags"])
	}
	dsKTs, ok := e.Args["ds_keytags"].([]uint16)
	if !ok || len(dsKTs) != 1 || dsKTs[0] != keytag {
		t.Errorf("expected ds_keytags=[%d], got %v", keytag, e.Args["ds_keytags"])
	}
}

func TestDNSSEC18CDSRolloverSignaled(t *testing.T) {
	dnssec18Setup(t)

	key := makeSEPKey("example", "AwEAAc==")
	keytag := key.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytag
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "AAAABBBB"

	// CDS has a different keytag/digest than the parent DS → rollover signaled.
	newKeytag := keytag + 1
	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = newKeytag
	cds.Algorithm = 8
	cds.DigestType = 2
	cds.Digest = "CCCCDDDD"

	// RRSIG still signed by the current DS keytag (chain of trust intact).
	cdsSig := rrsigRecord("example", dns.TypeCDS, keytag, 1, 2)
	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.72", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.73", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cds, cdsSig)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_CDS_ROLLOVER_SIGNALED") {
		t.Fatal("expected DS18_CDS_ROLLOVER_SIGNALED")
	}
	if hasEntryTag(entries, "DS18_CDS_MATCHES_DS") {
		t.Fatal("unexpected DS18_CDS_MATCHES_DS when CDS differs from DS")
	}
	// Verify keytag args are present and correct.
	e := firstEntryByTag(entries, "DS18_CDS_ROLLOVER_SIGNALED")
	if e == nil {
		t.Fatal("no entry for DS18_CDS_ROLLOVER_SIGNALED")
	}
	cdsKTs, ok := e.Args["cds_keytags"].([]uint16)
	if !ok || len(cdsKTs) == 0 {
		t.Errorf("expected cds_keytags []uint16, got %T %v", e.Args["cds_keytags"], e.Args["cds_keytags"])
	}
	dsKTs, ok := e.Args["ds_keytags"].([]uint16)
	if !ok || len(dsKTs) == 0 {
		t.Errorf("expected ds_keytags []uint16, got %T %v", e.Args["ds_keytags"], e.Args["ds_keytags"])
	}
}

func TestDNSSEC18CDNSKEYMatchesDS(t *testing.T) {
	dnssec18Setup(t)

	// Use ToCDNSKEY so the digest computation is guaranteed to match ToDS.
	key := makeSEPKey("example", "AwEAAc==")
	keytag := key.KeyTag()
	parentDS := key.ToDS(dns.SHA256) // actual cryptographic hash

	cdnskey := key.ToCDNSKEY()

	cdnskeySig := rrsigRecord("example", dns.TypeCDNSKEY, keytag, 1, 2)
	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.74", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", parentDS)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.75", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS) // no CDS
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY, cdnskey, cdnskeySig)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_CDNSKEY_MATCHES_DS") {
		t.Fatal("expected DS18_CDNSKEY_MATCHES_DS")
	}
	if hasEntryTag(entries, "DS18_CDNSKEY_ROLLOVER_SIGNALED") {
		t.Fatal("unexpected DS18_CDNSKEY_ROLLOVER_SIGNALED when CDNSKEY matches DS")
	}
	e := firstEntryByTag(entries, "DS18_CDNSKEY_MATCHES_DS")
	cdnsKTs, ok := e.Args["cdnskey_keytags"].([]uint16)
	if !ok || len(cdnsKTs) != 1 || cdnsKTs[0] != keytag {
		t.Errorf("expected cdnskey_keytags=[%d], got %v", keytag, e.Args["cdnskey_keytags"])
	}
	dsKTs, ok := e.Args["ds_keytags"].([]uint16)
	if !ok || len(dsKTs) != 1 || dsKTs[0] != keytag {
		t.Errorf("expected ds_keytags=[%d], got %v", keytag, e.Args["ds_keytags"])
	}
}

func TestDNSSEC18CDNSKEYRolloverSignaled(t *testing.T) {
	dnssec18Setup(t)

	// key1 is the current KSK (DS at parent).
	key1 := makeSEPKey("example", "AwEAAc==")
	keytag1 := key1.KeyTag()
	parentDS := key1.ToDS(dns.SHA256)

	// key2 is the incoming KSK; CDNSKEY signals "install this instead".
	key2 := makeSEPKey("example", "AwEAAb0=")
	cdnskey2 := key2.ToCDNSKEY()

	// RRSIG still from key1 (current chain of trust).
	cdnskeySig := rrsigRecord("example", dns.TypeCDNSKEY, keytag1, 1, 2)
	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag1, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.76", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", parentDS)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.77", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY, cdnskey2, cdnskeySig)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key1, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_CDNSKEY_ROLLOVER_SIGNALED") {
		t.Fatal("expected DS18_CDNSKEY_ROLLOVER_SIGNALED")
	}
	if hasEntryTag(entries, "DS18_CDNSKEY_MATCHES_DS") {
		t.Fatal("unexpected DS18_CDNSKEY_MATCHES_DS when CDNSKEY differs from DS")
	}
}

func TestDNSSEC18RolloverEvidenceMultiKSK(t *testing.T) {
	dnssec18Setup(t)

	// key1 has DS at parent; key2 is a new SEP awaiting DS publication.
	key1 := makeSEPKey("example", "AwEAAc==")
	keytag1 := key1.KeyTag()
	key2 := makeSEPKey("example", "AwEAAb0=")
	keytag2 := key2.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytag1
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "AABB"

	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag1, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.78", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.79", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			// Both keys published, only signed by key1.
			return answerPacket(qname, dns.TypeDNSKEY, key1, key2, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_MULTI_KSK") {
		t.Fatal("expected DS18_ROLLOVER_EVIDENCE_MULTI_KSK")
	}
	e := firstEntryByTag(entries, "DS18_ROLLOVER_EVIDENCE_MULTI_KSK")
	kts, ok := e.Args["keytags"].([]uint16)
	if !ok || len(kts) != 2 {
		t.Errorf("expected keytags with 2 entries, got %v", e.Args["keytags"])
	}
	// key2 has no DS → DNSKEY_WITHOUT_DS also fires.
	if !hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS") {
		t.Fatal("expected DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS for key2")
	}
	e2 := firstEntryByTag(entries, "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS")
	kts2, ok := e2.Args["keytags"].([]uint16)
	if !ok || len(kts2) != 1 || kts2[0] != keytag2 {
		t.Errorf("expected keytags=[%d], got %v", keytag2, e2.Args["keytags"])
	}
}

func TestDNSSEC18RolloverEvidenceDoubleSig(t *testing.T) {
	dnssec18Setup(t)

	// Both keys sign the DNSKEY RRset: classic double-signature phase.
	key1 := makeSEPKey("example", "AwEAAc==")
	keytag1 := key1.KeyTag()
	key2 := makeSEPKey("example", "AwEAAb0=")
	keytag2 := key2.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytag1
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "AABB"

	// Two RRSIGs from two different KSKs.
	dnskeyRRSIG1 := rrsigRecord("example", dns.TypeDNSKEY, keytag1, 1, 2)
	dnskeyRRSIG2 := rrsigRecord("example", dns.TypeDNSKEY, keytag2, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.80", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.81", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key1, key2, dnskeyRRSIG1, dnskeyRRSIG2)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG") {
		t.Fatal("expected DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG")
	}
	e := firstEntryByTag(entries, "DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG")
	kts, ok := e.Args["keytags"].([]uint16)
	if !ok || len(kts) != 2 {
		t.Errorf("expected 2 signer keytags, got %v", e.Args["keytags"])
	}
}

func TestDNSSEC18RolloverEvidenceDSWithoutDNSKEY(t *testing.T) {
	dnssec18Setup(t)

	// Parent still has DS for old key but child DNSKEY RRset no longer contains it.
	keyOld := makeSEPKey("example", "AwEAAc==")
	keytagOld := keyOld.KeyTag()
	keyNew := makeSEPKey("example", "AwEAAb0=")
	keytagNew := keyNew.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytagOld
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "AABB"

	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytagNew, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.82", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.83", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			// Only new key; old key already removed from DNSKEY RRset.
			return answerPacket(qname, dns.TypeDNSKEY, keyNew, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY") {
		t.Fatal("expected DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY")
	}
	e := firstEntryByTag(entries, "DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY")
	kts, ok := e.Args["keytags"].([]uint16)
	if !ok || len(kts) != 1 || kts[0] != keytagOld {
		t.Errorf("expected orphaned keytag [%d], got %v", keytagOld, e.Args["keytags"])
	}
}

func TestDNSSEC18RolloverEvidenceDNSKEYWithoutDS(t *testing.T) {
	dnssec18Setup(t)

	// Old key still in DS; new key published in DNSKEY but DS not yet updated.
	keyOld := makeSEPKey("example", "AwEAAc==")
	keytagOld := keyOld.KeyTag()
	keyNew := makeSEPKey("example", "AwEAAb0=")
	keytagNew := keyNew.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytagOld
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "AABB"

	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytagOld, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.84", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.85", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			// Both old and new key; DS only for old.
			return answerPacket(qname, dns.TypeDNSKEY, keyOld, keyNew, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS") {
		t.Fatal("expected DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS")
	}
	e := firstEntryByTag(entries, "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS")
	kts, ok := e.Args["keytags"].([]uint16)
	if !ok || len(kts) != 1 || kts[0] != keytagNew {
		t.Errorf("expected orphaned keytag [%d], got %v", keytagNew, e.Args["keytags"])
	}
}

func TestDNSSEC18NoCDSCDNSKEYButRolloverEvidence(t *testing.T) {
	dnssec18Setup(t)

	// No CDS/CDNSKEY published but multi-KSK is visible: on-demand publication model.
	key1 := makeSEPKey("example", "AwEAAc==")
	keytag1 := key1.KeyTag()
	key2 := makeSEPKey("example", "AwEAAb0=")

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytag1
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "AABB"

	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag1, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.86", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.87", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key1, key2, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE") {
		t.Fatal("expected DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE")
	}
	// Underlying evidence that triggered the tag must also be present.
	if !hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_MULTI_KSK") {
		t.Fatal("expected DS18_ROLLOVER_EVIDENCE_MULTI_KSK as supporting evidence")
	}
}

// TestDNSSEC18CDSDeleteOnlySkipsContentComparison verifies that a CDS RRset
// containing only DELETE sentinels (Algorithm == 0) skips MATCHES_DS and
// ROLLOVER_SIGNALED in favour of DNSSEC16/17.
func TestDNSSEC18CDSDeleteOnlySkipsContentComparison(t *testing.T) {
	dnssec18Setup(t)

	key := makeSEPKey("example", "AwEAAc==")
	keytag := key.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytag
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "DEADBEEF"

	// DELETE sentinel: Algorithm == 0 per RFC 8078.
	cdsDelete := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdsDelete.KeyTag = 0
	cdsDelete.Algorithm = 0
	cdsDelete.DigestType = 0
	cdsDelete.Digest = "00"

	cdsSig := rrsigRecord("example", dns.TypeCDS, keytag, 1, 2)
	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.88", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.89", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cdsDelete, cdsSig)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if hasEntryTag(entries, "DS18_CDS_MATCHES_DS") {
		t.Fatal("unexpected DS18_CDS_MATCHES_DS for DELETE-only CDS RRset")
	}
	if hasEntryTag(entries, "DS18_CDS_ROLLOVER_SIGNALED") {
		t.Fatal("unexpected DS18_CDS_ROLLOVER_SIGNALED for DELETE-only CDS RRset")
	}
}

// TestDNSSEC18CDSBothOldAndNewKeyMidRollover verifies that a CDS RRset listing
// both the current DS keytag and an incoming keytag emits ROLLOVER_SIGNALED
// (the canonical bootstrap state).
func TestDNSSEC18CDSBothOldAndNewKeyMidRollover(t *testing.T) {
	dnssec18Setup(t)

	keyOld := makeSEPKey("example", "AwEAAc==")
	keytagOld := keyOld.KeyTag()
	keyNew := makeSEPKey("example", "AwEAAb0=")
	keytagNew := keyNew.KeyTag()

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = keytagOld
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "AABB"

	cdsOld := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdsOld.KeyTag = keytagOld
	cdsOld.Algorithm = 8
	cdsOld.DigestType = 2
	cdsOld.Digest = "AABB"

	cdsNew := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdsNew.KeyTag = keytagNew
	cdsNew.Algorithm = 8
	cdsNew.DigestType = 2
	cdsNew.Digest = "CCDD"

	cdsSig := rrsigRecord("example", dns.TypeCDS, keytagOld, 1, 2)
	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytagOld, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.90", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.91", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS, cdsOld, cdsNew, cdsSig)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, keyOld, keyNew, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_CDS_ROLLOVER_SIGNALED") {
		t.Fatal("expected DS18_CDS_ROLLOVER_SIGNALED when CDS lists both old and new keys")
	}
	if hasEntryTag(entries, "DS18_CDS_MATCHES_DS") {
		t.Fatal("unexpected DS18_CDS_MATCHES_DS when CDS is a strict superset of DS")
	}
	e := firstEntryByTag(entries, "DS18_CDS_ROLLOVER_SIGNALED")
	cdsKTs, _ := e.Args["cds_keytags"].([]uint16)
	if len(cdsKTs) != 2 {
		t.Errorf("expected 2 CDS keytags (old+new), got %v", cdsKTs)
	}
}

// TestDNSSEC18NoCDSCDNSKEYButOnlyDoubleSig verifies the umbrella tag fires when
// the only step-15 evidence is DOUBLE_SIG.
func TestDNSSEC18NoCDSCDNSKEYButOnlyDoubleSig(t *testing.T) {
	dnssec18Setup(t)

	// Both keys are SEP and both sign DNSKEY: DOUBLE_SIG fires.
	// Both keys are also covered by parent DS, so DNSKEY_WITHOUT_DS and
	// DS_WITHOUT_DNSKEY do NOT fire. MULTI_KSK fires too (two SEP keys).
	// To isolate DOUBLE_SIG as a contributor we still rely on rolloverEvidence
	// being any of {MULTI_KSK, DOUBLE_SIG, DS_WITHOUT_DNSKEY, DNSKEY_WITHOUT_DS}.
	// This test asserts the umbrella tag is present and DOUBLE_SIG is emitted.
	key1 := makeSEPKey("example", "AwEAAc==")
	keytag1 := key1.KeyTag()
	key2 := makeSEPKey("example", "AwEAAb0=")
	keytag2 := key2.KeyTag()

	ds1 := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds1.KeyTag = keytag1
	ds1.Algorithm = 8
	ds1.DigestType = 2
	ds1.Digest = "AABB"

	ds2 := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds2.KeyTag = keytag2
	ds2.Algorithm = 8
	ds2.DigestType = 2
	ds2.Digest = "CCDD"

	dnskeyRRSIG1 := rrsigRecord("example", dns.TypeDNSKEY, keytag1, 1, 2)
	dnskeyRRSIG2 := rrsigRecord("example", dns.TypeDNSKEY, keytag2, 1, 2)

	parentNS := newNameserver(t, "pns1.example", "192.0.2.92", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return answerPacket(qname, dns.TypeDS, ds1, ds2)
		}
		return packet.Packet{}
	})
	childNS := newNameserver(t, "ns1.example", "192.0.2.93", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "CDS":
			return answerPacket(qname, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(qname, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(qname, dns.TypeDNSKEY, key1, key2, dnskeyRRSIG1, dnskeyRRSIG2)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	if !hasEntryTag(entries, "DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE") {
		t.Fatal("expected DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE")
	}
	if !hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG") {
		t.Fatal("expected DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG")
	}
	// Both keys are in DS, so these orphan tags must NOT fire.
	if hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS") {
		t.Fatal("unexpected DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS when DS covers all KSKs")
	}
	if hasEntryTag(entries, "DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY") {
		t.Fatal("unexpected DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY when DNSKEY covers all DS")
	}
}

func TestDNSSEC19CleanZone(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	if err := profile.Effective().Set("badkeys.path", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	newNameserver(t, "ns1.example", "192.0.2.201", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(qname, dnssec19P256Key(qname))
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.201"),
			HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	if !hasEntryTag(entries, "DS19_KEY_OK") {
		t.Fatalf("expected DS19_KEY_OK")
	}
	if !hasEntryTag(entries, "DS19_BLOCKLIST_NOT_FOUND") {
		t.Fatalf("expected DS19_BLOCKLIST_NOT_FOUND")
	}
}

func TestDNSSEC19BlocklistedKey(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	key := dnssec19P256Key("example")
	dir := t.TempDir()
	writeDNSSEC19BlocklistFixture(t, dir, key.Algorithm, key.PublicKey, 7, "unit-blocklist")
	if err := profile.Effective().Set("badkeys.path", dir); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	newNameserver(t, "ns1.example", "192.0.2.202", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(qname, dnssec19P256Key(qname))
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.202"),
			HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	if !hasEntryTag(entries, "DS19_BADKEY_BLOCKLIST") {
		t.Fatalf("expected DS19_BADKEY_BLOCKLIST")
	}
	if hasEntryTag(entries, "DS19_KEY_OK") {
		t.Fatalf("did not expect DS19_KEY_OK for blocklisted key")
	}
	if hasEntryTag(entries, "DS19_BLOCKLIST_NOT_FOUND") {
		t.Fatalf("did not expect DS19_BLOCKLIST_NOT_FOUND when fixture blocklist exists")
	}

	blocklisted := firstEntryByTag(entries, "DS19_BADKEY_BLOCKLIST")
	if blocklisted == nil {
		t.Fatalf("missing DS19_BADKEY_BLOCKLIST entry")
	}
	if got, _ := blocklisted.Args["blocklist_name"].(string); got != "unit-blocklist" {
		t.Fatalf("unexpected blocklist_name: got %q want %q", got, "unit-blocklist")
	}
}

func TestDNSSEC19NoDNSKEY(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	if err := profile.Effective().Set("badkeys.path", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	newNameserver(t, "ns1.example", "192.0.2.203", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(qname, nil)
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.203"),
			HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	if !hasEntryTag(entries, "DS19_NO_DNSKEY") {
		t.Fatalf("expected DS19_NO_DNSKEY")
	}
	if hasEntryTag(entries, "DS19_NO_RESPONSE") {
		t.Fatalf("did not expect DS19_NO_RESPONSE when nameserver answered without DNSKEY")
	}
}

func TestDNSSEC19NoResponse(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	if err := profile.Effective().Set("badkeys.path", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	newNameserver(t, "ns1.example", "192.0.2.204", func(_ string, _ string, _ *nameserver.QueryOptions) packet.Packet {
		return packet.Packet{}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.204"),
			HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	if !hasEntryTag(entries, "DS19_NO_RESPONSE") {
		t.Fatalf("expected DS19_NO_RESPONSE")
	}
	if hasEntryTag(entries, "DS19_NO_DNSKEY") {
		t.Fatalf("did not expect DS19_NO_DNSKEY when nameserver did not respond")
	}
}

func TestDNSSEC19TransportDisabled(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	if err := profile.Effective().Set("net.ipv4", false); err != nil {
		t.Fatalf("set net.ipv4: %v", err)
	}
	if err := profile.Effective().Set("badkeys.path", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	newNameserver(t, "ns1.example", "192.0.2.205", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(qname, dnssec19P256Key(qname))
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.205"),
			HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	if !hasEntryTag(entries, "IPV4_DISABLED") {
		t.Fatalf("expected IPV4_DISABLED")
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
		// Clone so each caller gets its own copy; DNSKEY.KeyTag() lazily
		// writes a cached field, which races with Len() when callers share
		// the same pointer across goroutines.
		keyCopy := *key
		msg.Answer = append(msg.Answer, &keyCopy)
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

func dnssec19P256Key(owner string) *dns.DNSKEY {
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = dns.ECDSAP256SHA256
	key.PublicKey = "GojIhhXUN/u4v54ZQqGSnyhWJwaubCvTmeexv7bR6edbkrSqQpF64cYbcB7wNcP+e+MAnLr+Wi9xMWyQLc8NAA=="
	return key
}

// --- DNSSEC20 tests ---

func TestDNSSEC20BitmapOK(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	newNameserver(t, "ns1.example", "192.0.2.201", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, dnssec19P256Key(qname))
		case "NSEC":
			nsecRR := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			nsecRR.NextDomain = "\\000." + dnsutil.Fqdn(qname)
			nsecRR.TypeBitMap = []uint16{dns.TypeA, dns.TypeNS, dns.TypeSOA, dns.TypeAAAA, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeDNSKEY}
			return answerPacket(qname, dns.TypeNSEC, nsecRR)
		case "A":
			aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			aRR.Addr = netip.MustParseAddr("192.0.2.1")
			return answerPacket(qname, dns.TypeA, aRR)
		case "AAAA":
			aaaaRR := &dns.AAAA{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			aaaaRR.Addr = netip.MustParseAddr("2001:db8::1")
			return answerPacket(qname, dns.TypeAAAA, aaaaRR)
		default:
			return packet.Packet{}
		}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.201"), HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC20(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec20: %v", err)
	}

	if !hasEntryTag(entries, "DS20_BITMAP_OK") {
		t.Fatalf("expected DS20_BITMAP_OK, got tags: %v", entryTags(entries))
	}
	if hasEntryTag(entries, "DS20_NSEC_BITMAP_MISMATCHES_RRTYPE") {
		t.Fatalf("unexpected DS20_NSEC_BITMAP_MISMATCHES_RRTYPE")
	}
}

func TestDNSSEC20NSECSubsetBitmap(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	newNameserver(t, "ns1.example", "192.0.2.201", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, dnssec19P256Key(qname))
		case "NSEC":
			nsecRR := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			nsecRR.NextDomain = "\\000." + dnsutil.Fqdn(qname)
			nsecRR.TypeBitMap = []uint16{dns.TypeNS, dns.TypeSOA, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeDNSKEY}
			return answerPacket(qname, dns.TypeNSEC, nsecRR)
		case "A":
			aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			aRR.Addr = netip.MustParseAddr("3.13.31.214")
			return answerPacket(qname, dns.TypeA, aRR)
		default:
			return packet.Packet{}
		}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.201"), HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC20(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec20: %v", err)
	}

	if !hasEntryTag(entries, "DS20_NSEC_BITMAP_MISMATCHES_RRTYPE") {
		t.Fatalf("expected DS20_NSEC_BITMAP_MISMATCHES_RRTYPE, got tags: %v", entryTags(entries))
	}
	entry := firstEntryByTag(entries, "DS20_NSEC_BITMAP_MISMATCHES_RRTYPE")
	if entry == nil {
		t.Fatal("missing DS20_NSEC_BITMAP_MISMATCHES_RRTYPE entry")
	}
	if rrtype, _ := entry.Args["query_type"].(string); rrtype != "A" {
		t.Fatalf("expected rrtype=A, got %q", rrtype)
	}
	if hasEntryTag(entries, "DS20_BITMAP_OK") {
		t.Fatalf("unexpected DS20_BITMAP_OK when bitmap has mismatches")
	}
}

func TestDNSSEC20NSEC3SubsetBitmap(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	// Build an NSEC3 record whose owner hash matches the apex.
	apexHash := dnsutil.NSEC3Name("example.", "", 0)
	nsec3Owner := apexHash + ".example."

	newNameserver(t, "ns1.example", "192.0.2.201", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, dnssec19P256Key(qname))
		case "NSEC":
			// NSEC3 zone: return NSEC3 in authority section (NODATA).
			nsec3RR := &dns.NSEC3{Hdr: dns.Header{Name: nsec3Owner, Class: dns.ClassINET, TTL: 60}}
			nsec3RR.Hash = dns.SHA1
			nsec3RR.Iterations = 0
			nsec3RR.Salt = ""
			nsec3RR.TypeBitMap = []uint16{dns.TypeNS, dns.TypeSOA, dns.TypeRRSIG, dns.TypeDNSKEY, dns.TypeNSEC3PARAM}
			return nsec3Packet(qname, nsec3RR)
		case "A":
			aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			aRR.Addr = netip.MustParseAddr("3.13.31.214")
			return answerPacket(qname, dns.TypeA, aRR)
		case "AAAA":
			aaaaRR := &dns.AAAA{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
			aaaaRR.Addr = netip.MustParseAddr("2001:db8::1")
			return answerPacket(qname, dns.TypeAAAA, aaaaRR)
		default:
			return packet.Packet{}
		}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.201"), HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC20(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec20: %v", err)
	}

	if !hasEntryTag(entries, "DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE") {
		t.Fatalf("expected DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE, got tags: %v", entryTags(entries))
	}
	// Both A and AAAA should be missing from the bitmap.
	count := 0
	for _, e := range entries {
		if e != nil && e.Tag == "DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 NSEC3 mismatch tags (A and AAAA), got %d", count)
	}
}

func TestDNSSEC20NoDNSSEC(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	newNameserver(t, "ns1.example", "192.0.2.201", func(_ string, _ string, _ *nameserver.QueryOptions) packet.Packet {
		return packet.Packet{}
	})

	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.201"), HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC20(context.Background(), &z)
	if err != nil {
		t.Fatalf("dnssec20: %v", err)
	}

	if !hasEntryTag(entries, "DS20_NO_DNSSEC") {
		t.Fatalf("expected DS20_NO_DNSSEC, got tags: %v", entryTags(entries))
	}
}

func entryTags(entries []*logger.Entry) []string {
	var tags []string
	for _, e := range entries {
		if e != nil {
			tags = append(tags, e.Tag)
		}
	}
	return tags
}

func writeDNSSEC19BlocklistFixture(t *testing.T, dir string, algo uint8, publicKey string, sourceID byte, sourceName string) {
	t.Helper()

	keyData, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKey))
	if err != nil {
		t.Fatalf("decode DNSKEY public key: %v", err)
	}
	parsed, err := badkeys.ParseDNSKEY(algo, keyData)
	if err != nil {
		t.Fatalf("parse DNSKEY for blocklist fixture: %v", err)
	}

	hash := badkeys.BKHASH120(parsed.Val)
	entry := append(append([]byte{}, hash[:]...), sourceID)

	if err := os.WriteFile(filepath.Join(dir, "blocklist.dat"), entry, 0o644); err != nil {
		t.Fatalf("write blocklist.dat: %v", err)
	}

	meta := map[string]any{
		"blocklists": []map[string]any{
			{
				"id":   int(sourceID),
				"name": sourceName,
			},
		},
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal badkeysdata.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "badkeysdata.json"), raw, 0o644); err != nil {
		t.Fatalf("write badkeysdata.json: %v", err)
	}
}

// --- DNSSEC21 tests ---

type dnssec21Fixture struct {
	parentName string
	childName  string
	parentKey  *dns.DNSKEY
	parentPriv crypto.PrivateKey
	childDS    *dns.DS
}

func newDNSSEC21Fixture(t *testing.T) dnssec21Fixture {
	t.Helper()
	parentName := "parent"
	childName := "child.parent"

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(parentName), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	priv, err := key.Generate(1024)
	if err != nil {
		t.Fatalf("generate parent key: %v", err)
	}

	childKey := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(childName), Class: dns.ClassINET, TTL: 3600}}
	childKey.Flags = dns.FlagZONE | dns.FlagSEP
	childKey.Protocol = 3
	childKey.Algorithm = dns.RSASHA256
	childKey.PublicKey = "AwEAAc=="
	ds := childKey.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatalf("child DS is nil")
	}

	return dnssec21Fixture{
		parentName: parentName,
		childName:  childName,
		parentKey:  key,
		parentPriv: priv,
		childDS:    ds,
	}
}

func (f dnssec21Fixture) signedDSResponse(t *testing.T, sigKey *dns.DNSKEY, sigPriv crypto.PrivateKey) packet.Packet {
	t.Helper()
	ds := *f.childDS
	dsRRset := []dns.RR{&ds}
	now := time.Now().UTC()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(f.childName), Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = sigKey.Algorithm
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(24 * time.Hour).Unix())
	sig.KeyTag = sigKey.KeyTag()
	sig.SignerName = dnsutil.Fqdn(f.parentName)
	signer, ok := sigPriv.(crypto.Signer)
	if !ok {
		t.Fatalf("private key does not implement crypto.Signer")
	}
	if err := sig.Sign(signer, dsRRset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign DS RRset: %v", err)
	}
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(f.childName), dns.TypeDS)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{&ds, sig}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func (f dnssec21Fixture) unsignedDSResponse() packet.Packet {
	ds := *f.childDS
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(f.childName), dns.TypeDS)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{&ds}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func (f dnssec21Fixture) parentDNSKEYResponse(keys ...*dns.DNSKEY) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(f.parentName), dns.TypeDNSKEY)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	for _, k := range keys {
		copyKey := *k
		msg.Answer = append(msg.Answer, &copyKey)
	}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func (f dnssec21Fixture) emptyDNSKEYResponse() packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(f.parentName), dns.TypeDNSKEY)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func (f dnssec21Fixture) emptyDSResponse() packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(f.childName), dns.TypeDS)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}

func (f dnssec21Fixture) installMocks(t *testing.T, parentNS nameserver.Nameserver) {
	t.Helper()
	parentZone, err := zone.New(f.parentName)
	if err != nil {
		t.Fatalf("parent zone new: %v", err)
	}
	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return &parentZone, nil
	}
	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
}

func resetDNSSEC21Mocks(t *testing.T) {
	t.Helper()
	origParent := zoneParent
	origGetParent := parentNameservers
	t.Cleanup(func() {
		zoneParent = origParent
		parentNameservers = origGetParent
	})
}

func dnssec21Setup(t *testing.T) {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })
	resetDNSSEC21Mocks(t)
}

func TestDNSSEC21Verified(t *testing.T) {
	dnssec21Setup(t)

	f := newDNSSEC21Fixture(t)
	dsResp := f.signedDSResponse(t, f.parentKey, f.parentPriv)
	dnskeyResp := f.parentDNSKEYResponse(f.parentKey)

	parentNS := newNameserver(t, "ns1.parent", "192.0.2.221", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DS":
			return dsResp
		case "DNSKEY":
			return dnskeyResp
		}
		return packet.Packet{}
	})
	f.installMocks(t, parentNS)

	z, err := zone.New(f.childName)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC21(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	if !hasEntryTag(entries, "DS21_DS_RRSIG_VERIFIED") {
		t.Fatalf("expected DS21_DS_RRSIG_VERIFIED, got tags: %v", entryTagsDS21(entries))
	}
	for _, badTag := range []string{"DS21_DS_RRSIG_NOT_VERIFIABLE", "DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY", "DS21_NO_DS_RRSIG", "DS21_PARENT_DNSKEY_MISSING"} {
		if hasEntryTag(entries, badTag) {
			t.Fatalf("did not expect %s, got tags: %v", badTag, entryTagsDS21(entries))
		}
	}
}

func TestDNSSEC21RRSIGNotVerifiable(t *testing.T) {
	dnssec21Setup(t)

	f := newDNSSEC21Fixture(t)
	// Sign the DS RRset with a key that is NOT published at the parent.
	otherKey := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(f.parentName), Class: dns.ClassINET, TTL: 3600}}
	otherKey.Flags = dns.FlagZONE | dns.FlagSEP
	otherKey.Protocol = 3
	otherKey.Algorithm = dns.RSASHA256
	otherPriv, err := otherKey.Generate(1024)
	if err != nil {
		t.Fatalf("generate impostor key: %v", err)
	}

	// Force the impostor RRSIG to claim a keytag that DOES exist at the parent,
	// so the testcase reaches signature verification (instead of NO_DNSKEY_FOR_DS_RRSIG).
	dsResp := f.signedDSResponseForcedKeytag(t, otherKey, otherPriv, f.parentKey.KeyTag())
	dnskeyResp := f.parentDNSKEYResponse(f.parentKey)

	parentNS := newNameserver(t, "ns1.parent", "192.0.2.222", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DS":
			return dsResp
		case "DNSKEY":
			return dnskeyResp
		}
		return packet.Packet{}
	})
	f.installMocks(t, parentNS)

	z, err := zone.New(f.childName)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC21(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	if !hasEntryTag(entries, "DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY") {
		t.Fatalf("expected DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY, got tags: %v", entryTagsDS21(entries))
	}
	if !hasEntryTag(entries, "DS21_DS_RRSIG_NOT_VERIFIABLE") {
		t.Fatalf("expected DS21_DS_RRSIG_NOT_VERIFIABLE, got tags: %v", entryTagsDS21(entries))
	}
	if hasEntryTag(entries, "DS21_DS_RRSIG_VERIFIED") {
		t.Fatalf("did not expect DS21_DS_RRSIG_VERIFIED, got tags: %v", entryTagsDS21(entries))
	}
}

func TestDNSSEC21NoParentDNSKEY(t *testing.T) {
	dnssec21Setup(t)

	f := newDNSSEC21Fixture(t)
	dsResp := f.signedDSResponse(t, f.parentKey, f.parentPriv)
	emptyDNSKEY := f.emptyDNSKEYResponse()

	parentNS := newNameserver(t, "ns1.parent", "192.0.2.223", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DS":
			return dsResp
		case "DNSKEY":
			return emptyDNSKEY
		}
		return packet.Packet{}
	})
	f.installMocks(t, parentNS)

	z, err := zone.New(f.childName)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC21(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	if !hasEntryTag(entries, "DS21_PARENT_DNSKEY_MISSING") {
		t.Fatalf("expected DS21_PARENT_DNSKEY_MISSING, got tags: %v", entryTagsDS21(entries))
	}
	if hasEntryTag(entries, "DS21_DS_RRSIG_VERIFIED") {
		t.Fatalf("did not expect DS21_DS_RRSIG_VERIFIED")
	}
}

func TestDNSSEC21NoDSRRSIG(t *testing.T) {
	dnssec21Setup(t)

	f := newDNSSEC21Fixture(t)
	parentNS := newNameserver(t, "ns1.parent", "192.0.2.224", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DS":
			return f.unsignedDSResponse()
		case "DNSKEY":
			return f.parentDNSKEYResponse(f.parentKey)
		}
		return packet.Packet{}
	})
	f.installMocks(t, parentNS)

	z, err := zone.New(f.childName)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC21(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	if !hasEntryTag(entries, "DS21_NO_DS_RRSIG") {
		t.Fatalf("expected DS21_NO_DS_RRSIG, got tags: %v", entryTagsDS21(entries))
	}
}

func TestDNSSEC21RootZone(t *testing.T) {
	dnssec21Setup(t)

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}
	zoneParent = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	}

	z, err := zone.New(".")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC21(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	if !hasEntryTag(entries, "DS21_NO_PARENT_ZONE") {
		t.Fatalf("expected DS21_NO_PARENT_ZONE, got tags: %v", entryTagsDS21(entries))
	}
}

func TestDNSSEC21UnsignedDelegation(t *testing.T) {
	dnssec21Setup(t)

	f := newDNSSEC21Fixture(t)
	emptyDS := f.emptyDSResponse()
	parentNS := newNameserver(t, "ns1.parent", "192.0.2.225", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DS":
			return emptyDS
		case "DNSKEY":
			return f.parentDNSKEYResponse(f.parentKey)
		}
		return packet.Packet{}
	})
	f.installMocks(t, parentNS)

	z, err := zone.New(f.childName)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC21(context.Background(), &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	for _, badTag := range []string{
		"DS21_DS_RRSIG_VERIFIED",
		"DS21_DS_RRSIG_NOT_VERIFIABLE",
		"DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY",
		"DS21_NO_DS_RRSIG",
		"DS21_PARENT_DNSKEY_MISSING",
	} {
		if hasEntryTag(entries, badTag) {
			t.Fatalf("unsigned delegation should emit no DS21 findings, got %s", badTag)
		}
	}
}

func entryTagsDS21(entries []*logger.Entry) []string {
	tags := []string{}
	for _, e := range entries {
		if e == nil {
			continue
		}
		if strings.HasPrefix(e.Tag, "DS21_") {
			tags = append(tags, e.Tag)
		}
	}
	return tags
}

func (f dnssec21Fixture) signedDSResponseForcedKeytag(t *testing.T, sigKey *dns.DNSKEY, sigPriv crypto.PrivateKey, forcedKeytag uint16) packet.Packet {
	t.Helper()
	ds := *f.childDS
	dsRRset := []dns.RR{&ds}
	now := time.Now().UTC()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(f.childName), Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = sigKey.Algorithm
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(24 * time.Hour).Unix())
	sig.KeyTag = sigKey.KeyTag()
	sig.SignerName = dnsutil.Fqdn(f.parentName)
	signer, ok := sigPriv.(crypto.Signer)
	if !ok {
		t.Fatalf("private key does not implement crypto.Signer")
	}
	if err := sig.Sign(signer, dsRRset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign DS RRset: %v", err)
	}
	sig.KeyTag = forcedKeytag
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(f.childName), dns.TypeDS)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{&ds, sig}
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
}
