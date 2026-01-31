package dnssec

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	methodsv2 "codeberg.org/pawal/gonemaster/engine/methodsv2"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
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

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS01_DS_ALGO_OK" {
			continue
		}
		if list, ok := entry.Args["ns_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_list for DS01_DS_ALGO_OK")
	}
	if nsList != "ns-parent1.example/192.0.2.80;ns-parent2.example/192.0.2.81" {
		t.Fatalf("expected deterministic ns_list order, got %q", nsList)
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
		key := &dns.DNSKEY{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(qname),
				Rrtype: dns.TypeDNSKEY,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Flags:     dns.ZONE,
			Protocol:  3,
			Algorithm: 8,
			PublicKey: "AwEAAc==",
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.SEP,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE | dns.SEP,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
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
			key := &dns.DNSKEY{
				Hdr: dns.RR_Header{
					Name:   dns.Fqdn(qname),
					Rrtype: dns.TypeDNSKEY,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				Flags:     dns.ZONE,
				Protocol:  3,
				Algorithm: 8,
				PublicKey: "AwEAAc==",
			}
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
			key := &dns.DNSKEY{
				Hdr: dns.RR_Header{
					Name:   dns.Fqdn(qname),
					Rrtype: dns.TypeDNSKEY,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				Flags:     dns.ZONE,
				Protocol:  3,
				Algorithm: 8,
				PublicKey: "AwEAAc==",
			}
			return dnskeyPacket(qname, key)
		case "NSEC":
			nsec3 := &dns.NSEC3{
				Hdr: dns.RR_Header{
					Name:   dns.Fqdn(qname),
					Rrtype: dns.TypeNSEC3,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				Hash:       2,
				Flags:      0,
				Iterations: 0,
				SaltLength: 0,
				Salt:       "",
				HashLength: 0,
				NextDomain: "",
			}
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}

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

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS03_NO_NSEC3" {
			continue
		}
		if list, ok := entry.Args["ns_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_list for DS03_NO_NSEC3")
	}
	if nsList != "ns1.example/192.0.2.201;ns2.example/192.0.2.202" {
		t.Fatalf("expected deterministic ns_list order, got %q", nsList)
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
	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
	soa := &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Ns:      "ns1.example.",
		Mbox:    "hostmaster.example.",
		Serial:  1,
		Refresh: 60,
		Retry:   60,
		Expire:  60,
		Minttl:  60,
	}
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
	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
	soa := &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Ns:      "ns1.example.",
		Mbox:    "hostmaster.example.",
		Serial:  1,
		Refresh: 60,
		Retry:   60,
		Expire:  60,
		Minttl:  60,
	}
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
		key := &dns.DNSKEY{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(qname),
				Rrtype: dns.TypeDNSKEY,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Flags:     dns.ZONE,
			Protocol:  3,
			Algorithm: 8,
			PublicKey: "AwEAAc==",
		}
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
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

	ds := &dns.DS{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDS,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		KeyTag:     11111,
		Algorithm:  8,
		DigestType: 2,
		Digest:     "DEADBEEF",
	}
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
	if !hasEntryTag(entries, "DS07_SIGNED") {
		t.Fatalf("expected DS07_SIGNED")
	}
	if !hasEntryTag(entries, "DS07_DS_ON_PARENT_SERVER") {
		t.Fatalf("expected DS07_DS_ON_PARENT_SERVER")
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}

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

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS07_NOT_SIGNED_ON_SERVER" {
			continue
		}
		if list, ok := entry.Args["ns_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_list for DS07_NOT_SIGNED_ON_SERVER")
	}
	if nsList != "ns1.example/192.0.2.60;ns2.example/192.0.2.61" {
		t.Fatalf("expected deterministic ns_list order, got %q", nsList)
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
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

	ds := &dns.DS{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDS,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		KeyTag:     11111,
		Algorithm:  8,
		DigestType: 2,
		Digest:     "DEADBEEF",
	}
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

	var nsList string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "DS07_DS_ON_PARENT_SERVER" {
			continue
		}
		if list, ok := entry.Args["ns_list"].(string); ok {
			nsList = list
			break
		}
	}
	if nsList == "" {
		t.Fatalf("expected ns_list for DS07_DS_ON_PARENT_SERVER")
	}
	if nsList != "ns-parent1.example/192.0.2.70;ns-parent2.example/192.0.2.71" {
		t.Fatalf("expected deterministic ns_list order, got %q", nsList)
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}

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
	if !hasEntryTag(entries, "DS07_NOT_SIGNED") {
		t.Fatalf("expected DS07_NOT_SIGNED")
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}

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
	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
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
	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}

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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
	nsec := &dns.NSEC{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeNSEC,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		NextDomain: dns.Fqdn("next.example"),
		TypeBitMap: []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG},
	}

	newNameserver(t, "ns1.example", "192.0.2.70", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnskeyPacket(qname, key)
		case "NSEC":
			msg := new(dns.Msg)
			msg.SetQuestion(dns.Fqdn(qname), dns.TypeNSEC)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			msg.SetEdns0(1232, true)
			return packet.Packet{Msg: msg}
		case "NSEC3PARAM":
			msg := new(dns.Msg)
			msg.SetQuestion(dns.Fqdn(qname), dns.TypeNSEC3PARAM)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			msg.Ns = append(msg.Ns, nsec, soaRecord(qname))
			msg.SetEdns0(1232, true)
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

	ds := &dns.DS{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDS,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		KeyTag:     12345,
		Algorithm:  8,
		DigestType: 1,
		Digest:     "DEADBEEF",
	}

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

	ds := &dns.DS{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDS,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		KeyTag:     54321,
		Algorithm:  8,
		DigestType: 1,
		Digest:     "FEEDBEEF",
	}

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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
	nsRR := &dns.NS{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeNS,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Ns: "ns1.example.",
	}

	makeRRSIG := func(owner string, typeCovered uint16) *dns.RRSIG {
		return &dns.RRSIG{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeRRSIG,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			TypeCovered: typeCovered,
			Algorithm:   13,
			Inception:   1,
			Expiration:  2,
			KeyTag:      12345,
			SignerName:  dns.Fqdn(owner),
		}
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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
	}
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

	cds := &dns.CDS{
		DS: dns.DS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn("example"),
				Rrtype: dns.TypeCDS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			KeyTag:     12345,
			Algorithm:  8,
			DigestType: 1,
			Digest:     "DEADBEEF",
		},
	}

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

	cdnskey := &dns.CDNSKEY{
		DNSKEY: dns.DNSKEY{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn("example"),
				Rrtype: dns.TypeCDNSKEY,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Flags:     dns.ZONE,
			Protocol:  3,
			Algorithm: 8,
			PublicKey: "AwEAAc==",
		},
	}

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

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Flags:     dns.ZONE,
		Protocol:  3,
		Algorithm: 8,
		PublicKey: "AwEAAc==",
	}
	keytag := key.KeyTag()

	ds := &dns.DS{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn("example"),
			Rrtype: dns.TypeDS,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		KeyTag:     keytag,
		Algorithm:  8,
		DigestType: 1,
		Digest:     "DEADBEEF",
	}

	cds := &dns.CDS{
		DS: dns.DS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn("example"),
				Rrtype: dns.TypeCDS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			KeyTag:     keytag,
			Algorithm:  8,
			DigestType: 1,
			Digest:     "DEADBEEF",
		},
	}

	cdnskey := &dns.CDNSKEY{
		DNSKEY: dns.DNSKEY{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn("example"),
				Rrtype: dns.TypeCDNSKEY,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Flags:     dns.ZONE,
			Protocol:  3,
			Algorithm: 8,
			PublicKey: "AwEAAc==",
		},
	}

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

func dsPacket(owner string, keytag uint16, algo uint8, digestType uint8) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(owner), dns.TypeDS)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.DS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeDS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			KeyTag:     keytag,
			Algorithm:  algo,
			DigestType: digestType,
			Digest:     "DEADBEEF",
		},
	}
	msg.SetEdns0(1232, true)
	return packet.Packet{Msg: msg}
}

func dsPacketFromDS(owner string, ds *dns.DS) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(owner), dns.TypeDS)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	if ds != nil {
		msg.Answer = append(msg.Answer, ds)
	}
	msg.SetEdns0(1232, true)
	return packet.Packet{Msg: msg}
}

func dnskeyPacket(owner string, key *dns.DNSKEY) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(owner), dns.TypeDNSKEY)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	if key != nil {
		msg.Answer = append(msg.Answer, key)
	}
	msg.SetEdns0(1232, true)
	return packet.Packet{Msg: msg}
}

func nsecPacket(owner string) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(owner), dns.TypeNSEC)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	return packet.Packet{Msg: msg}
}

func nsec3Packet(owner string, nsec3 *dns.NSEC3) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(owner), dns.TypeNSEC)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	if nsec3 != nil {
		msg.Ns = append(msg.Ns, nsec3)
	}
	msg.SetEdns0(1232, true)
	return packet.Packet{Msg: msg}
}

func rrsigRecord(owner string, typeCovered uint16, keytag uint16, inception int64, expiration int64) *dns.RRSIG {
	return &dns.RRSIG{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(owner),
			Rrtype: dns.TypeRRSIG,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		TypeCovered: typeCovered,
		Algorithm:   8,
		Inception:   uint32(inception),
		Expiration:  uint32(expiration),
		KeyTag:      keytag,
		SignerName:  dns.Fqdn(owner),
	}
}

func answerPacket(owner string, qtype uint16, answers ...dns.RR) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(owner), qtype)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = append(msg.Answer, answers...)
	msg.SetEdns0(1232, true)
	return packet.Packet{Msg: msg}
}

func soaRecord(owner string) *dns.SOA {
	return &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(owner),
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Ns:      "ns1.example.",
		Mbox:    "hostmaster.example.",
		Serial:  1,
		Refresh: 60,
		Retry:   60,
		Expire:  60,
		Minttl:  60,
	}
}
