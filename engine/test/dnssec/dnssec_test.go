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
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestDNSSEC01AlgoOK(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return false
	})

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacket(q.Name, 12345, 8, 2)
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	tctest.RequireTags(t, entries, "DS01_DS_ALGO_OK")
}

func TestDNSSEC01DigestGOST12(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return false
	})

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.31", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacket(q.Name, 12345, 8, 5) // digest 5 = GOST R 34.11-2012 (RFC 9558)
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	tctest.RequireTags(t, entries, "DS01_DS_ALGO_OK")
}

func TestDNSSEC01DigestSM3(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return false
	})

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.32", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacket(q.Name, 12345, 8, 6) // digest 6 = SM3 (RFC 9563)
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	tctest.RequireTags(t, entries, "DS01_DS_ALGO_OK")
}

func TestDNSSEC01Algo2Missing(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return false
	})

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.10", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacket(q.Name, 54321, 8, 1)
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	tctest.RequireTags(t, entries, "DS01_DS_ALGO_2_MISSING")
}

func TestDNSSEC01UndelegatedDSOnlyUsesFakeDS(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS, err := nameserver.NewWithContext(ctx, "ns1.root", "192.0.2.1", r.Client())
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

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return &parent, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return true
	})

	entries, err := DNSSEC01(ctx, &child)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	tctest.RequireTags(t, entries, "DS01_DS_ALGO_OK")
	tctest.RequireNoTag(t, entries, "DS01_UNDEL_N_NO_UNDEL_DS")

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

func TestDNSSEC01TagForKeyAlgorithmTable(t *testing.T) {
	// Covers every range boundary of the classification. Agreement with
	// dnssec05TagForAlgorithm is a separate invariant, checked over the whole
	// algorithm domain by TestDNSSEC01TagMirrorsDNSSEC05.
	cases := []struct {
		algo uint8
		want string
	}{
		{0, "DS01_KEY_ALGO_NOT_ZONE_SIGN"},
		{1, "DS01_KEY_ALGO_DEPRECATED"},
		{2, "DS01_KEY_ALGO_NOT_ZONE_SIGN"},
		{3, "DS01_KEY_ALGO_DEPRECATED"},
		{4, "DS01_KEY_ALGO_RESERVED"},
		{5, "DS01_KEY_ALGO_DEPRECATED"},
		{8, "DS01_KEY_ALGO_OK"},
		{10, "DS01_KEY_ALGO_NOT_RECOMMENDED"},
		{13, "DS01_KEY_ALGO_OK"},
		{16, "DS01_KEY_ALGO_OK"},
		{17, "DS01_KEY_ALGO_OK"},
		{18, "DS01_KEY_ALGO_OK"},
		{19, "DS01_KEY_ALGO_UNASSIGNED"},
		{22, "DS01_KEY_ALGO_UNASSIGNED"},
		{23, "DS01_KEY_ALGO_OK"},
		{24, "DS01_KEY_ALGO_UNASSIGNED"},
		{122, "DS01_KEY_ALGO_UNASSIGNED"},
		{123, "DS01_KEY_ALGO_RESERVED"},
		{251, "DS01_KEY_ALGO_RESERVED"},
		{252, "DS01_KEY_ALGO_NOT_ZONE_SIGN"},
		{253, "DS01_KEY_ALGO_PRIVATE"},
		{254, "DS01_KEY_ALGO_PRIVATE"},
		{255, "DS01_KEY_ALGO_RESERVED"},
	}
	for _, tc := range cases {
		if got := dnssec01TagForKeyAlgorithm(tc.algo); got != tc.want {
			t.Errorf("algo %d: got %s, want %s", tc.algo, got, tc.want)
		}
	}
}

func TestDNSSEC01TagMirrorsDNSSEC05(t *testing.T) {
	// DS01 classifies the DNSKEY algorithm a DS record points at and DS05
	// classifies the DNSKEY algorithm itself, so the two must never disagree
	// about what a given number means. dnssec01TagForKeyAlgorithm currently
	// guarantees that by deriving its tag from dnssec05TagForAlgorithm, which
	// makes this hold by construction. It earns its keep the day that
	// delegation is replaced by a second switch: unlike the boundary table
	// above, it walks the whole uint8 domain, so a fork that drifts on an
	// untabulated algorithm number cannot slip through.
	for i := 0; i < 256; i++ {
		algo := uint8(i)
		ds05 := dnssec05TagForAlgorithm(algo)
		// TrimPrefix silently returns the string unchanged when the prefix is
		// absent, which would hand DS01 a malformed tag rather than fail.
		if !strings.HasPrefix(ds05, "DS05_ALGO_") {
			t.Errorf("algo %d: DS05 tag %s lacks the DS05_ALGO_ prefix", algo, ds05)
			continue
		}
		want := "DS01_KEY_ALGO_" + strings.TrimPrefix(ds05, "DS05_ALGO_")
		if got := dnssec01TagForKeyAlgorithm(algo); got != want {
			t.Errorf("algo %d: DS01 class %s diverges from DS05 class %s", algo, got, ds05)
		}
	}
}

func TestDNSSEC01KeyAlgoPrivate(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return false
	})

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.33", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		// DS algorithm field 253 (private use), digest algorithm 2 (acceptable).
		return dsPacket(q.Name, 12345, 253, 2)
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "DS01_KEY_ALGO_PRIVATE")
	if entry.Args["ds_key_algo_num"] != uint8(253) {
		t.Fatalf("expected ds_key_algo_num 253, got %#v", entry.Args["ds_key_algo_num"])
	}
	if entry.Args["keytag"] != uint16(12345) {
		t.Fatalf("expected keytag 12345, got %#v", entry.Args["keytag"])
	}
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) == 0 {
		t.Fatalf("expected emitting servers list, got %#v", entry.Args["servers"])
	}
}

func TestDNSSEC01KeyAlgoOK(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return false
	})

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.34", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacket(q.Name, 12345, 13, 2)
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC01(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "DS01_KEY_ALGO_OK")
	if entry.Args["ds_key_algo_num"] != uint8(13) {
		t.Fatalf("expected ds_key_algo_num 13, got %#v", entry.Args["ds_key_algo_num"])
	}
	// The digest classification must be unaffected by the new tags.
	tctest.RequireTags(t, entries, "DS01_DS_ALGO_OK")
}

func TestDNSSEC01KeyAlgoUndelegated(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS, err := nameserver.NewWithContext(ctx, "ns1.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new parent nameserver: %v", err)
	}
	if err := parentNS.AddFakeDS("example", []nameserver.DSData{
		{
			KeyTag:     12345,
			Algorithm:  253,
			DigestType: 2,
			Digest:     "ABCD",
		},
	}); err != nil {
		t.Fatalf("add fake DS: %v", err)
	}

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return &parent, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return true
	})

	entries, err := DNSSEC01(ctx, &child)
	if err != nil {
		t.Fatalf("dnssec01: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "DS01_KEY_ALGO_PRIVATE")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one server for undelegated fake DS, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "-" {
		t.Fatalf("expected undelegated fake DS source (servers[0].ns='-'), got %#v", servers[0])
	}
}

func TestDNSSEC01ParallelParentQueries(t *testing.T) {
	ctx := tctest.Context(t)

	profile.Effective().Resolver.Defaults.Parallel = 2

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
		return false
	})

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

	parent1, err := nameserver.NewWithContext(ctx, "ns-parent1.example", "192.0.2.80", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent1.SetQueryHook(hook("parent1"))

	parent2, err := nameserver.NewWithContext(ctx, "ns-parent2.example", "192.0.2.81", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent2.SetQueryHook(hook("parent2"))

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent1, parent2}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS01_DS_ALGO_OK")

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
	ctx := tctest.Context(t)

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.2", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacket(q.Name, 9999, 8, 2)
	})

	childNS := tctest.NS(t, ctx, "ns-child.example", "192.0.2.3", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 8
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(q.Name, key)
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}
	tctest.RequireTags(t, entries, "DS02_NO_DNSKEY_FOR_DS", "DS02_NO_VALID_DNSKEY_FOR_ANY_DS")
}

func TestDNSSEC02DNSKEYNotForZoneSigning(t *testing.T) {
	ctx := tctest.Context(t)

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.11", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacketFromDS(q.Name, ds)
	})

	childNS := tctest.NS(t, ctx, "ns-child.example", "192.0.2.12", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(q.Name, key)
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}
	tctest.RequireTags(t, entries, "DS02_DNSKEY_NOT_FOR_ZONE_SIGNING")
}

// signedDNSKEYPair generates a real ECDSAP256SHA256 zone key and a valid
// RRSIG over its single-record DNSKEY RRset, so DNSSEC02's signature
// verification genuinely succeeds unless the DS is rejected earlier.
func signedDNSKEYPair(t *testing.T, owner string) (*dns.DNSKEY, *dns.RRSIG) {
	t.Helper()

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = dns.ECDSAP256SHA256
	priv, err := key.Generate(256)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	now := time.Now().UTC()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = key.Algorithm
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(24 * time.Hour).Unix())
	sig.KeyTag = key.KeyTag()
	sig.SignerName = dnsutil.Fqdn(owner)
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("private key does not implement crypto.Signer")
	}
	if err := sig.Sign(signer, []dns.RR{key}, &dns.SignOption{}); err != nil {
		t.Fatalf("sign DNSKEY rrset: %v", err)
	}
	return key, sig
}

func dnssec02Wire(t *testing.T, parentNS nameserver.Nameserver, childNS nameserver.Nameserver) func() {
	t.Helper()

	origGetParent := parentNameservers
	origGlue := glueNameservers
	origApex := apexNameservers

	parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	return func() {
		parentNameservers = origGetParent
		glueNameservers = origGlue
		apexNameservers = origApex
	}
}

func TestDNSSEC02DSAlgorithmMismatch(t *testing.T) {
	ctx := tctest.Context(t)

	key, sig := signedDNSKEYPair(t, "example")

	// The published DS carries algorithm 253 although the DNSKEY uses 13;
	// keytag and digest still match (the dragangaming.com/alvinsong.top shape).
	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}
	mismatchDS := *ds
	mismatchDS.Algorithm = 253

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.40", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		dsCopy := mismatchDS
		return dsPacketFromDS(q.Name, &dsCopy)
	})
	childNS := tctest.NS(t, ctx, "ns-child.example", "192.0.2.41", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		keyCopy := *key
		sigCopy := *sig
		return answerPacket(q.Name, dns.TypeDNSKEY, &keyCopy, &sigCopy)
	})
	t.Cleanup(dnssec02Wire(t, parentNS, childNS))

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}

	entry := tctest.RequireTag(t, entries, "DS02_DS_ALGO_DNSKEY_MISMATCH")
	if entry.Args["ds_key_algo_num"] != uint8(253) {
		t.Fatalf("expected ds_key_algo_num 253, got %#v", entry.Args["ds_key_algo_num"])
	}
	if entry.Args["ds_key_algo_mnemo"] != "PRIVATEDNS" {
		t.Fatalf("expected ds_key_algo_mnemo PRIVATEDNS, got %#v", entry.Args["ds_key_algo_mnemo"])
	}
	if entry.Args["algo_num"] != uint8(13) {
		t.Fatalf("expected algo_num 13, got %#v", entry.Args["algo_num"])
	}
	if entry.Args["algo_mnemo"] != "ECDSAP256SHA256" {
		t.Fatalf("expected algo_mnemo ECDSAP256SHA256, got %#v", entry.Args["algo_mnemo"])
	}
	if entry.Args["keytag"] != key.KeyTag() {
		t.Fatalf("expected keytag %d, got %#v", key.KeyTag(), entry.Args["keytag"])
	}

	// The mismatched DS must not count as a match even though keytag and
	// digest agree and the DNSKEY RRSIG is valid.
	tctest.RequireNoTag(t, entries, "DS02_MATCH_DS_DNSKEY")
	// The specific mismatch tag replaces the generic digest-mismatch tag.
	tctest.RequireNoTag(t, entries, "DS02_NO_MATCH_DS_DNSKEY")
	tctest.RequireTags(t, entries, "DS02_NO_VALID_DNSKEY_FOR_ANY_DS")
}

func TestDNSSEC02DSAlgorithmMismatchAlongsideValidDS(t *testing.T) {
	ctx := tctest.Context(t)

	key, sig := signedDNSKEYPair(t, "example")

	goodDS := key.ToDS(2)
	if goodDS == nil {
		t.Fatal("expected DS from DNSKEY")
	}
	mismatchDS := *goodDS
	mismatchDS.Algorithm = 253

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.42", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		goodCopy := *goodDS
		badCopy := mismatchDS
		return answerPacket(q.Name, dns.TypeDS, &goodCopy, &badCopy)
	})
	childNS := tctest.NS(t, ctx, "ns-child.example", "192.0.2.43", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		keyCopy := *key
		sigCopy := *sig
		return answerPacket(q.Name, dns.TypeDNSKEY, &keyCopy, &sigCopy)
	})
	t.Cleanup(dnssec02Wire(t, parentNS, childNS))

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}

	tctest.RequireTags(t, entries, "DS02_MATCH_DS_DNSKEY", "DS02_DS_ALGO_DNSKEY_MISMATCH")
	tctest.RequireNoTag(t, entries, "DS02_NO_VALID_DNSKEY_FOR_ANY_DS")
}

func TestDNSSEC02DSAlgorithmMismatchUnsupportedDigest(t *testing.T) {
	ctx := tctest.Context(t)

	key, sig := signedDNSKEYPair(t, "example")

	// Unsupported digest type: this branch used to accept any keytag
	// candidate without an algorithm check.
	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}
	mismatchDS := *ds
	mismatchDS.Algorithm = 253
	mismatchDS.DigestType = 6

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.44", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		dsCopy := mismatchDS
		return dsPacketFromDS(q.Name, &dsCopy)
	})
	childNS := tctest.NS(t, ctx, "ns-child.example", "192.0.2.45", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		keyCopy := *key
		sigCopy := *sig
		return answerPacket(q.Name, dns.TypeDNSKEY, &keyCopy, &sigCopy)
	})
	t.Cleanup(dnssec02Wire(t, parentNS, childNS))

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}

	tctest.RequireTags(t, entries, "DS02_DS_ALGO_DNSKEY_MISMATCH")
	tctest.RequireNoTag(t, entries, "DS02_MATCH_DS_DNSKEY")
	tctest.RequireTags(t, entries, "DS02_NO_VALID_DNSKEY_FOR_ANY_DS")
}

func TestDNSSEC02MatchWithoutAlgorithmMismatch(t *testing.T) {
	ctx := tctest.Context(t)

	key, sig := signedDNSKEYPair(t, "example")

	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.46", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		dsCopy := *ds
		return dsPacketFromDS(q.Name, &dsCopy)
	})
	childNS := tctest.NS(t, ctx, "ns-child.example", "192.0.2.47", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		keyCopy := *key
		sigCopy := *sig
		return answerPacket(q.Name, dns.TypeDNSKEY, &keyCopy, &sigCopy)
	})
	t.Cleanup(dnssec02Wire(t, parentNS, childNS))

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}

	// Happy path: a DS whose algorithm field equals the DNSKEY algorithm
	// still matches, and the mismatch tag never appears.
	tctest.RequireTags(t, entries, "DS02_MATCH_DS_DNSKEY")
	tctest.RequireNoTag(t, entries, "DS02_DS_ALGO_DNSKEY_MISMATCH", "DS02_NO_VALID_DNSKEY_FOR_ANY_DS")
}

func TestDNSSEC02ParallelChildDNSKEYQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.100", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacketFromDS(q.Name, ds)
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

	child1, err := nameserver.NewWithContext(ctx, "ns-child1.example", "192.0.2.101", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("child1"))

	child2, err := nameserver.NewWithContext(ctx, "ns-child2.example", "192.0.2.102", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("child2"))

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS")

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
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.4", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			key.Flags = dns.FlagZONE
			key.Protocol = 3
			key.Algorithm = 8
			key.PublicKey = "AwEAAc=="
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			return nsecPacket(q.Name)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC03(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec03: %v", err)
	}
	noNSEC3 := tctest.RequireTag(t, entries, "DS03_NO_NSEC3")
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
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.13", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			key.Flags = dns.FlagZONE
			key.Protocol = 3
			key.Algorithm = 8
			key.PublicKey = "AwEAAc=="
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			nsec3 := &dns.NSEC3{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			nsec3.Hash = 2
			nsec3.Flags = 0
			nsec3.Iterations = 0
			nsec3.SaltLength = 0
			nsec3.Salt = ""
			nsec3.HashLength = 0
			nsec3.NextDomain = ""
			return nsec3Packet(q.Name, nsec3)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC03(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec03: %v", err)
	}
	illegal := tctest.RequireTag(t, entries, "DS03_ILLEGAL_HASH_ALGO")
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
	ctx := tctest.Context(t)

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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.201", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.202", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS03_NO_NSEC3")

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
	ctx := tctest.Context(t)

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

	tctest.Stub(t, &zoneQueryOne, func(_ context.Context, _ *zone.Zone, _ string, rrtype string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch rrtype {
		case "DNSKEY":
			return dnskeyResp, nil
		case "SOA":
			return soaResp, nil
		default:
			return packet.Packet{}, nil
		}
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC04(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec04: %v", err)
	}
	expirationEntry := tctest.RequireTag(t, entries, "RRSIG_EXPIRATION")
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
	tctest.RequireTags(t, entries, "RRSIG_EXPIRED")
}

func TestDNSSEC04DurationOK(t *testing.T) {
	ctx := tctest.Context(t)

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

	tctest.Stub(t, &zoneQueryOne, func(_ context.Context, _ *zone.Zone, _ string, rrtype string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch rrtype {
		case "DNSKEY":
			return dnskeyResp, nil
		case "SOA":
			return soaResp, nil
		default:
			return packet.Packet{}, nil
		}
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC04(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec04: %v", err)
	}
	tctest.RequireTags(t, entries, "DURATION_OK")
}

func TestDNSSEC04ParallelQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	tctest.Stub(t, &zoneQueryOne, func(ctx context.Context, _ *zone.Zone, _ string, rrtype string, _ *nameserver.QueryOptions) (packet.Packet, error) {
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
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DURATION_OK")
}

func TestDNSSEC05AlgoOK(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns1.example", "192.0.2.20", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 8
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(q.Name, key)
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.20"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	tctest.RequireTags(t, entries, "DS05_ALGO_OK")
}

func TestDNSSEC05AlgoSM2SM3(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns1.example", "192.0.2.33", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 17 // SM2SM3 (RFC 9563)
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(q.Name, key)
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.33"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	tctest.RequireTags(t, entries, "DS05_ALGO_OK")
}

func TestDNSSEC05AlgoMLDSA44(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns1.example", "192.0.2.33", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 18 // ML-DSA-44, IANA-assigned post-quantum signing algorithm
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(q.Name, key)
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.33"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "DS05_ALGO_OK")
	// The tag alone would still pass with a stale algorithm table, so assert the
	// rendered name too: before algorithm 18 was assigned it fell in the
	// unassigned range and reported the mnemonic UNASSIGNED.
	if entry.Args["algo_num"] != uint8(18) {
		t.Fatalf("expected algo_num 18, got %#v", entry.Args["algo_num"])
	}
	if entry.Args["algo_descr"] != "ML-DSA-44" {
		t.Fatalf("expected algo_descr ML-DSA-44, got %#v", entry.Args["algo_descr"])
	}
	if entry.Args["algo_mnemo"] != "MLDSA44" {
		t.Fatalf("expected algo_mnemo MLDSA44, got %#v", entry.Args["algo_mnemo"])
	}
}

func TestDNSSEC05AlgoECCGOST12(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns1.example", "192.0.2.34", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 23 // ECC-GOST12 (RFC 9558)
		key.PublicKey = "AwEAAc=="
		return dnskeyPacket(q.Name, key)
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.34"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	tctest.RequireTags(t, entries, "DS05_ALGO_OK")
}

func TestDNSSEC05ParallelDNSKEYQueries(t *testing.T) {
	ctx := tctest.Context(t)

	profile.Effective().Resolver.Defaults.Parallel = 2

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	started := make(chan string, 2)
	release := make(chan struct{})

	handler := func(id string) tctest.Handler {
		return func(q tctest.Query) packet.Packet {
			if q.Type != "DNSKEY" {
				return packet.Packet{}
			}
			select {
			case started <- id:
			default:
			}
			<-release
			return dnskeyPacket(q.Name, key)
		}
	}

	tctest.NS(t, ctx, "ns1.example", "192.0.2.220", handler("ns1"))
	tctest.NS(t, ctx, "ns2.example", "192.0.2.221", handler("ns2"))

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
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
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS05_ALGO_OK")

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
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns2.example", "192.0.2.21", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(q.Name, nil)
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.21"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	tctest.RequireTags(t, entries, "DS05_ZONE_NO_DNSSEC")
}

func TestDNSSEC05NoResponse(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns3.example", "192.0.2.22", func(q tctest.Query) packet.Packet {
		if q.Type == "DNSKEY" {
			return packet.Packet{}
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns3.example"),
				Address:    netip.MustParseAddr("192.0.2.22"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC05(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec05: %v", err)
	}
	tctest.RequireTags(t, entries, "DS05_NO_RESPONSE")
}

func TestDNSSEC06ExtraProcessingOK(t *testing.T) {
	ctx := tctest.Context(t)

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 12345, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
	resp := answerPacket("example", dns.TypeDNSKEY, key, sig)
	resp.AnswerFrom = "192.0.2.30"

	tctest.Stub(t, &zoneQueryAll, func(_ context.Context, _ *zone.Zone, _ string, _ string, _ *nameserver.QueryOptions) ([]packet.Packet, error) {
		return []packet.Packet{resp}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC06(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec06: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "EXTRA_PROCESSING_OK")
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.30" {
		t.Fatalf("expected address=192.0.2.30, got %#v", entry.Args["address"])
	}
	if _, ok := entry.Args["server"]; ok {
		t.Fatalf("legacy key server should not be present: %#v", entry.Args)
	}
}

func TestDNSSEC06ExtraProcessingBroken(t *testing.T) {
	ctx := tctest.Context(t)

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	resp := answerPacket("example", dns.TypeDNSKEY, key)
	resp.AnswerFrom = "192.0.2.31"

	tctest.Stub(t, &zoneQueryAll, func(_ context.Context, _ *zone.Zone, _ string, _ string, _ *nameserver.QueryOptions) ([]packet.Packet, error) {
		return []packet.Packet{resp}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC06(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec06: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "EXTRA_PROCESSING_BROKEN")
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.31" {
		t.Fatalf("expected address=192.0.2.31, got %#v", entry.Args["address"])
	}
	if _, ok := entry.Args["server"]; ok {
		t.Fatalf("legacy key server should not be present: %#v", entry.Args)
	}
}

func TestDNSSEC07SignedZone(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	tctest.NS(t, ctx, "ns1.example", "192.0.2.40", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
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

	tctest.NS(t, ctx, "ns-parent.example", "192.0.2.41", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return answerPacket(q.Name, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.40"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns, _ := nameserver.NewWithContext(ctx, "ns-parent.example", "192.0.2.41", nil)
		return []nameserver.Nameserver{ns}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	signedOnServer := tctest.RequireTag(t, entries, "DS07_SIGNED_ON_SERVER")
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
	tctest.RequireTags(t, entries, "DS07_SIGNED", "DS07_DS_ON_PARENT_SERVER")
	dsOnParent := tctest.RequireTag(t, entries, "DS07_DS_ON_PARENT_SERVER")
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
	tctest.RequireTags(t, entries, "DS07_DS_FOR_SIGNED_ZONE")
}

func TestDNSSEC07ParallelChildQueries(t *testing.T) {
	ctx := tctest.Context(t)

	profile.Effective().Resolver.Defaults.Parallel = 2

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.60", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.61", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
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
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS07_NOT_SIGNED_ON_SERVER", "DS07_NOT_SIGNED")

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
	ctx := tctest.Context(t)

	profile.Effective().Resolver.Defaults.Parallel = 2

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	tctest.NS(t, ctx, "ns-child.example", "192.0.2.62", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns-child.example"),
				Address:    netip.MustParseAddr("192.0.2.62"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

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

	parent1, err := nameserver.NewWithContext(ctx, "ns-parent1.example", "192.0.2.70", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent1.SetQueryHook(hook("parent1"))

	parent2, err := nameserver.NewWithContext(ctx, "ns-parent2.example", "192.0.2.71", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent2.SetQueryHook(hook("parent2"))

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent1, parent2}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	tctest.NS(t, ctx, "ns2.example", "192.0.2.42", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns2.example"),
				Address:    netip.MustParseAddr("192.0.2.42"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	notSignedOnServer := tctest.RequireTag(t, entries, "DS07_NOT_SIGNED_ON_SERVER")
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
	tctest.RequireTags(t, entries, "DS07_NOT_SIGNED")
}

func TestDNSSEC07ChildOutcomeTagsTypedServers(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	tctest.NS(t, ctx, "ns-noresp.example", "192.0.2.170", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			return packet.Packet{}
		default:
			return packet.Packet{}
		}
	})

	tctest.NS(t, ctx, "ns-noauth.example", "192.0.2.171", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			p := dnskeyPacket(q.Name, key)
			p.Msg.Authoritative = false
			return p
		default:
			return packet.Packet{}
		}
	})

	tctest.NS(t, ctx, "ns-rcode.example", "192.0.2.172", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(q.Name), dns.TypeDNSKEY)
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

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
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
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	tctest.RequireTags(t, entries, "DS07_NO_RESPONSE_DNSKEY", "DS07_NON_AUTH_RESPONSE_DNSKEY", "DS07_UNEXP_RCODE_RESP_DNSKEY")

	noResp := tctest.First(entries, "DS07_NO_RESPONSE_DNSKEY")
	noAuth := tctest.First(entries, "DS07_NON_AUTH_RESPONSE_DNSKEY")
	unexp := tctest.First(entries, "DS07_UNEXP_RCODE_RESP_DNSKEY")
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
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	tctest.NS(t, ctx, "ns1.example", "192.0.2.180", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
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

	tctest.NS(t, ctx, "ns-parent-no-ds.example", "192.0.2.181", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return answerPacket(q.Name, dns.TypeDS, ds)
		}
		return packet.Packet{}
	})

	tctest.NS(t, ctx, "ns-parent-with-ds.example", "192.0.2.182", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return answerPacket(q.Name, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.180"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		nsNoDS, _ := nameserver.NewWithContext(ctx, "ns-parent-no-ds.example", "192.0.2.181", nil)
		nsWithDS, _ := nameserver.NewWithContext(ctx, "ns-parent-with-ds.example", "192.0.2.182", nil)
		return []nameserver.Nameserver{nsNoDS, nsWithDS}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	noDS := tctest.RequireTag(t, entries, "DS07_NO_DS_ON_PARENT_SERVER")
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
	tctest.RequireNoTag(t, entries, "DS07_NO_DS_FOR_SIGNED_ZONE")
}

func TestDNSSEC07NoDSOnAllParentServersSuppressesPerServerTag(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	tctest.NS(t, ctx, "ns1.example", "192.0.2.180", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
		default:
			return packet.Packet{}
		}
	})

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 11111
	ds.Algorithm = 8
	ds.DigestType = 2
	ds.Digest = "DEADBEEF"

	tctest.NS(t, ctx, "ns-parent-a.example", "192.0.2.181", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return answerPacket(q.Name, dns.TypeDS, ds)
		}
		return packet.Packet{}
	})
	tctest.NS(t, ctx, "ns-parent-b.example", "192.0.2.182", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return answerPacket(q.Name, dns.TypeDS, ds)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.180"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		nsA, _ := nameserver.NewWithContext(ctx, "ns-parent-a.example", "192.0.2.181", nil)
		nsB, _ := nameserver.NewWithContext(ctx, "ns-parent-b.example", "192.0.2.182", nil)
		return []nameserver.Nameserver{nsA, nsB}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC07(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec07: %v", err)
	}
	tctest.RequireTags(t, entries, "DS07_NO_DS_FOR_SIGNED_ZONE")
	tctest.RequireNoTag(t, entries, "DS07_NO_DS_ON_PARENT_SERVER")
}

func TestDNSSECAllParallelOutputStable(t *testing.T) {

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
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
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	runAll := func(parallel int) []*logger.Entry {
		profile.ResetEffective()
		if err := profile.Effective().Set("resolver.defaults.parallel", parallel); err != nil {
			t.Fatalf("set parallel: %v", err)
		}
		if err := profile.Effective().Set("test_cases", []any{"dnssec07"}); err != nil {
			t.Fatalf("set test_cases: %v", err)
		}

		ctx := tctest.Context(t)
		key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
		key.Flags = dns.FlagZONE
		key.Protocol = 3
		key.Algorithm = 8
		key.PublicKey = "AwEAAc=="
		handler := func(q tctest.Query) packet.Packet {
			switch q.Type {
			case "SOA":
				return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
			case "DNSKEY":
				return dnskeyPacket(q.Name, key)
			default:
				return packet.Packet{}
			}
		}
		tctest.NS(t, ctx, "ns1.example", "192.0.2.160", handler)
		tctest.NS(t, ctx, "ns2.example", "192.0.2.161", handler)

		z, err := zone.New("example")
		if err != nil {
			t.Fatalf("zone new: %v", err)
		}
		entries, err := All(ctx, &z)
		if err != nil {
			t.Fatalf("dnssec all: %v", err)
		}
		return entries
	}

	sequentialEntries := runAll(1)
	parallelEntries := runAll(2)

	if !tctest.Has(sequentialEntries, "DS07_NOT_SIGNED_ON_SERVER") || !tctest.Has(sequentialEntries, "DS07_NOT_SIGNED") {
		t.Fatalf("expected unsigned DNSSEC07 tags in sequential run")
	}
	if !tctest.Has(parallelEntries, "DS07_NOT_SIGNED_ON_SERVER") || !tctest.Has(parallelEntries, "DS07_NOT_SIGNED") {
		t.Fatalf("expected unsigned DNSSEC07 tags in parallel run")
	}

	sequentialNormalized := tctest.NormalizeStable(sequentialEntries)
	parallelNormalized := tctest.NormalizeStable(parallelEntries)
	if len(sequentialNormalized) != len(parallelNormalized) {
		t.Fatalf("entry count changed with parallelism: sequential=%v parallel=%v", sequentialNormalized, parallelNormalized)
	}
	for i := range sequentialNormalized {
		if sequentialNormalized[i] != parallelNormalized[i] {
			t.Fatalf("entry[%d] changed with parallelism: %q != %q", i, sequentialNormalized[i], parallelNormalized[i])
		}
	}
}

// stubAllDNSSECDiscovery points both DNSSEC07's and DNSSEC11's nameserver
// discovery at a fixed parent/child topology so All() can be exercised on an
// unsigned zone without touching the network. DNSSEC07 finds the child via
// delegationNameservers/zoneNameservers and never queries the parent for an
// unsigned zone; DNSSEC11 finds the parent via parentApexNameservers and the
// child via glueNameservers. parentNameservers and zoneParent are stubbed to
// no-ops because DNSSEC07 still calls them before deciding the zone is
// unsigned.
func stubAllDNSSECDiscovery(t *testing.T, parentNS, childNS nameserver.Nameserver) {
	t.Helper()

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.160"), HasAddress: true},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})
	tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
}

// unsignedChildNameserver serves an authoritative SOA and an authoritative but
// empty DNSKEY response, i.e. a zone that is not signed.
func unsignedChildNameserver(t *testing.T, ctx context.Context) nameserver.Nameserver {
	t.Helper()
	return tctest.NS(t, ctx, "ns1.example", "192.0.2.160", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			return dnskeyPacket(q.Name, nil)
		default:
			return packet.Packet{}
		}
	})
}

// TestDNSSECAllUnsignedStaleParentDS covers the case this whole change is
// about: an unsigned child whose parent still publishes a DS. Before the
// DNSSEC11-before-short-circuit ordering, All() emitted DS07_NOT_SIGNED and
// returned, so the stale DS was never reported. Now DNSSEC11 runs first and
// emits DS11_DS_BUT_UNSIGNED_ZONE, while the short-circuit must still block
// every later testcase (dnssec01 is enabled here but must not run).
func TestDNSSECAllUnsignedStaleParentDS(t *testing.T) {

	ctx := tctest.Context(t)

	staleDS := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	staleDS.KeyTag = 54321
	staleDS.Algorithm = 8
	staleDS.DigestType = 2
	staleDS.Digest = "DEADBEEF"

	parentNS := tctest.NS(t, ctx, "ns.parent", "192.0.2.200", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS(q.Name, staleDS)
		}
		return packet.Packet{}
	})
	childNS := unsignedChildNameserver(t, ctx)
	stubAllDNSSECDiscovery(t, parentNS, childNS)

	profile.ResetEffective()
	if err := profile.Effective().Set("test_cases", []any{"dnssec07", "dnssec11", "dnssec01"}); err != nil {
		t.Fatalf("set test_cases: %v", err)
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := All(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec all: %v", err)
	}

	tctest.RequireTags(t, entries, "DS07_NOT_SIGNED", "DS11_DS_BUT_UNSIGNED_ZONE")
	// The short-circuit must still fire after DNSSEC11: only dnssec07 and
	// dnssec11 may run, so exactly two TEST_CASE_START entries, and no dnssec01
	// output despite it being enabled.
	if got := tctest.Count(entries, "TEST_CASE_START"); got != 2 {
		t.Fatalf("expected exactly 2 testcases to run (dnssec07, dnssec11), got %d TEST_CASE_START", got)
	}
	if tctest.Has(entries, "DS01_DS_ALGO_OK") || tctest.Has(entries, "DS01_PARENT_ZONE_NO_DS") {
		t.Fatalf("dnssec01 ran despite the DS07_NOT_SIGNED short-circuit")
	}
}

// TestDNSSECAllUnsignedNoParentDS is the ordinary unsigned zone: no DS at the
// parent. DNSSEC11 now runs and emits DS11_NO_PARENT_DS (INFO, no score
// penalty), the short-circuit still blocks the rest, and no DS-but-unsigned
// error is raised.
func TestDNSSECAllUnsignedNoParentDS(t *testing.T) {

	ctx := tctest.Context(t)

	parentNS := tctest.NS(t, ctx, "ns.parent", "192.0.2.200", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS(q.Name, nil)
		}
		return packet.Packet{}
	})
	childNS := unsignedChildNameserver(t, ctx)
	stubAllDNSSECDiscovery(t, parentNS, childNS)

	profile.ResetEffective()
	if err := profile.Effective().Set("test_cases", []any{"dnssec07", "dnssec11", "dnssec01"}); err != nil {
		t.Fatalf("set test_cases: %v", err)
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := All(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec all: %v", err)
	}

	tctest.RequireTags(t, entries, "DS07_NOT_SIGNED", "DS11_NO_PARENT_DS")
	tctest.RequireNoTag(t, entries, "DS11_DS_BUT_UNSIGNED_ZONE")
	if got := tctest.Count(entries, "TEST_CASE_START"); got != 2 {
		t.Fatalf("expected exactly 2 testcases to run (dnssec07, dnssec11), got %d TEST_CASE_START", got)
	}
}

func TestDNSSEC08MissingRRSIG(t *testing.T) {
	ctx := tctest.Context(t)

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.50", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(q.Name, key)
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC08(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec08: %v", err)
	}
	tctest.RequireTags(t, entries, "DS08_MISSING_RRSIG_IN_RESPONSE")
}

func TestDNSSEC08RRSIGNotYetValid(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(time.Hour).Unix(), now.Add(2*time.Hour).Unix())

	ns := tctest.NS(t, ctx, "ns2.example", "192.0.2.51", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC08(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec08: %v", err)
	}
	tctest.RequireTags(t, entries, "DS08_DNSKEY_RRSIG_NOT_YET_VALID")
}

func TestDNSSEC08RRSIGNotValidByDNSKEY(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	ns := tctest.NS(t, ctx, "ns3.example", "192.0.2.52", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC08(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec08: %v", err)
	}
	tctest.RequireTags(t, entries, "DS08_RRSIG_NOT_VALID_BY_DNSKEY")
}

func TestDNSSEC08ParallelDNSKEYQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	child1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.101", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("child1"))

	child2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.102", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("child2"))

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS08_MISSING_RRSIG_IN_RESPONSE")

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
	ctx := tctest.Context(t)

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.60", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC09(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec09: %v", err)
	}
	tctest.RequireTags(t, entries, "DS09_MISSING_RRSIG_IN_RESPONSE")
}

// lvKSK42018Pub is the real .lv KSK public key (RSASHA256, 2048-bit, public
// exponent 2^32+1). Both miekg/dns and crypto/rsa reject an exponent this
// large, so verifyRRSIG can never succeed for it. The key is otherwise
// well-formed: it matches a DS built from it, which is what makes it a DS-linked
// key in DNSSEC02.
const lvKSK42018Pub = "BQEAAAAByLU9dUcHHcl1eLgjLidTJKlwxsU9a580xierZ+WyfRBI47L3LLXAZZ0ub6Sea3qKP2mhP5ZBG/reXvyh3OSlHa39WoMiUUZFcuouCajBg7XeLGVPL4U1Ja1UW9wq/Oc8WU1dq4e+2Q8Dt8tipFvbL0AD0BhJAsfQuT3wperedwQAUKId0/JQOFNTWhEJaYN2P5IIhyRKWQp8OhtKmdNYQ5jfqqpXVO4zyqV+4ZxWurXJS8c7bKrE3OAewWEGAtTjeElfQ2CFAKWVjMOLeZ86+mgw7p3UHhGB+KuRaKg6fAtTcQYBF78Xe40wuj9EgGL19mp9v6tDwFe+Epow4SFSPQ=="

func lvLargeExponentKSK(owner string) *dns.DNSKEY {
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = 8 // RSASHA256
	key.PublicKey = lvKSK42018Pub
	return key
}

// A DS-linked DNSKEY whose only problem is an RSA exponent gonemaster cannot
// verify locally must be reclassified from the ERROR DS02_RRSIG_NOT_VALID_BY_DNSKEY
// to the NOTICE DS02_RSA_EXPONENT_UNSUPPORTED, and the DNSKEY-signed-by-DS
// aggregate must treat it as indeterminate rather than a hard failure. This is
// the .lv scenario, which validates on public resolvers but not here.
func TestDNSSEC02RSAExponentUnsupported(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key := lvLargeExponentKSK("example")
	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.90", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacketFromDS(q.Name, ds)
	})
	childNS := tctest.NS(t, ctx, "ns-child.example", "192.0.2.91", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}
	tctest.RequireTags(t, entries, "DS02_RSA_EXPONENT_UNSUPPORTED")
	// None of the false-failure tags may fire for the indeterminate key.
	for _, tag := range []string{
		"DS02_RRSIG_NOT_VALID_BY_DNSKEY",
		"DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS",
		"DS02_NO_MATCHING_DNSKEY_RRSIG",
		"DS02_NO_VALID_DNSKEY_FOR_ANY_DS",
	} {
		if tctest.Has(entries, tag) {
			t.Errorf("did not expect %s for a large-exponent DS-linked key", tag)
		}
	}
}

// A DS-matching DNSKEY whose exponent is normal (65537) must still fail as
// before: the RSA-exponent reclassification must not swallow a genuine bad
// signature. This is the negative control for DNSSEC02.
func TestDNSSEC02RRSIGNotValidByDNSKEYNormalExponent(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = 8
	if _, err := key.Generate(1024); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}
	// A fabricated RRSIG that will not verify against the generated key.
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	parentNS := tctest.NS(t, ctx, "ns-parent.example", "192.0.2.92", func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacketFromDS(q.Name, ds)
	})
	childNS := tctest.NS(t, ctx, "ns-child.example", "192.0.2.93", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC02(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec02: %v", err)
	}
	tctest.RequireNoTag(t, entries, "DS02_RSA_EXPONENT_UNSUPPORTED")
	tctest.RequireTags(t, entries, "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS")
}

// DNSSEC08 must reclassify the same way: a DNSKEY RRSIG that cannot be checked
// only because of a large RSA exponent becomes the NOTICE, not the ERROR, and
// the nameserver is not counted as a DS08_DNSKEY_RRSIG_VALID pass.
func TestDNSSEC08RSAExponentUnsupported(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key := lvLargeExponentKSK("example")
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.94", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC08(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec08: %v", err)
	}
	tctest.RequireTags(t, entries, "DS08_RSA_EXPONENT_UNSUPPORTED")
	for _, tag := range []string{"DS08_RRSIG_NOT_VALID_BY_DNSKEY", "DS08_DNSKEY_RRSIG_VALID"} {
		if tctest.Has(entries, tag) {
			t.Errorf("did not expect %s for a large-exponent key", tag)
		}
	}
}

// DNSSEC09 covers the SOA RRSIG path with the same reclassification.
func TestDNSSEC09RSAExponentUnsupported(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key := lvLargeExponentKSK("example")
	sig := rrsigRecord("example", dns.TypeSOA, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.95", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			pkt := dnskeyPacket(q.Name, key)
			pkt.Timestamp = now
			return pkt
		case "SOA":
			pkt := answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name), sig)
			pkt.Timestamp = now
			return pkt
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC09(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec09: %v", err)
	}
	tctest.RequireTags(t, entries, "DS09_RSA_EXPONENT_UNSUPPORTED")
	for _, tag := range []string{"DS09_RRSIG_NOT_VALID_BY_DNSKEY", "DS09_SOA_RRSIG_VALID"} {
		if tctest.Has(entries, tag) {
			t.Errorf("did not expect %s for a large-exponent key", tag)
		}
	}
}

func TestDNSSEC09ParallelQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	child1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.111", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("child1"))

	child2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.112", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("child2"))

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS09_MISSING_RRSIG_IN_RESPONSE")

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
	ctx := tctest.Context(t)

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next.example")
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG}

	tctest.NS(t, ctx, "ns1.example", "192.0.2.70", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(q.Name), dns.TypeNSEC)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		case "NSEC3PARAM":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(q.Name), dns.TypeNSEC3PARAM)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			msg.Ns = append(msg.Ns, nsec, soaRecord(q.Name))
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.70"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}
	tctest.RequireTags(t, entries, "DS10_NSEC_MISSING_SIGNATURE")
}

func TestDNSSEC10ParallelQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.201", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.202", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
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
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS10_NSEC_QUERY_RESPONSE_ERR")

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
	ctx := tctest.Context(t)

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

	tctest.NS(t, ctx, "ns1.example", "192.0.2.80", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(q.Name), dns.TypeNSEC)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		case "NSEC3PARAM":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(q.Name), dns.TypeNSEC3PARAM)
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

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.80"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}
	tctest.RequireNoTag(t, entries, "DS10_NSEC3PARAM_MISMATCHES_APEX", "DS10_ERR_MULT_NSEC3PARAM")
}

// Two NSEC3PARAM RRs where one has the wrong owner must trigger
// DS10_NSEC3PARAM_MISMATCHES_APEX (proving the apex-owner check loops over
// every RR in the RRset, not just the first one).
func TestDNSSEC10MultipleNSEC3PARAMOneOffApex(t *testing.T) {
	ctx := tctest.Context(t)

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

	tctest.NS(t, ctx, "ns1.example", "192.0.2.81", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(q.Name), dns.TypeNSEC)
			msg.Response = true
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			msg.UDPSize = 1232
			msg.Security = true
			return packet.Packet{Msg: msg}
		case "NSEC3PARAM":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn(q.Name), dns.TypeNSEC3PARAM)
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

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.81"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}
	tctest.RequireTags(t, entries, "DS10_NSEC3PARAM_MISMATCHES_APEX")
	tctest.RequireNoTag(t, entries, "DS10_ERR_MULT_NSEC3PARAM")
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
	ctx := tctest.Context(t)

	log := logger.New()
	log.SetProfile(profile.Effective())
	util.SetLogger(log)

	apex := "example"
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	tctest.NS(t, ctx, "ns1.example", "192.0.2.90", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			return nsecAuthorityNSECResponse(q.Name, apex)
		case "NSEC3PARAM":
			return emptyNSEC3PARAMResponse(q.Name, apex)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.90"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New(apex)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}

	tctest.RequireTags(t, entries, "DS10_NONSTANDARD_NSEC_RESPONSE")
	tctest.RequireNoTag(t, entries, "DS10_INCONSISTENT_NSEC")
	tctest.RequireTags(t, entries, "DS10_HAS_NSEC")

	entry := tctest.RequireTag(t, entries, "DS10_NONSTANDARD_NSEC_RESPONSE")
	if !strings.EqualFold(entry.Level(), "NOTICE") {
		t.Fatalf("DS10_NONSTANDARD_NSEC_RESPONSE level = %q, want NOTICE", entry.Level())
	}
	servers, ok := entry.Args["servers"]
	if !ok {
		t.Fatalf("DS10_NONSTANDARD_NSEC_RESPONSE missing 'servers' arg, got args=%v", entry.Args)
	}
	rows := tctest.Servers(t, servers)
	if len(rows) != 1 {
		t.Fatalf("DS10_NONSTANDARD_NSEC_RESPONSE 'servers' length = %d, want 1 (got %#v)", len(rows), servers)
	}
}

// TestDNSSEC10NonstandardNSECResponseNotEmittedForStandard verifies that the
// new tag stays silent when every nameserver returns NSEC in the answer
// section (the conventional shape).
func TestDNSSEC10NonstandardNSECResponseNotEmittedForStandard(t *testing.T) {
	ctx := tctest.Context(t)

	log := logger.New()
	log.SetProfile(profile.Effective())
	util.SetLogger(log)

	apex := "example"
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	tctest.NS(t, ctx, "ns1.example", "192.0.2.91", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			return nsecInAnswerResponse(q.Name, apex)
		case "NSEC3PARAM":
			return emptyNSEC3PARAMResponse(q.Name, apex)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{
				Name:       dnsname.New("ns1.example"),
				Address:    netip.MustParseAddr("192.0.2.91"),
				HasAddress: true,
			},
		}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New(apex)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}

	tctest.RequireNoTag(t, entries, "DS10_NONSTANDARD_NSEC_RESPONSE")
	tctest.RequireTags(t, entries, "DS10_HAS_NSEC")
}

// TestDNSSEC10NonstandardNSECResponseMixedServers checks the mixed case: one
// nameserver returns NSEC-in-answer, another NSEC-in-authority. Both shapes
// represent valid NSEC evidence, so the consistency check must not fire
// (DS10_INCONSISTENT_NSEC absent), but the non-standard tag must call out
// only the nameserver that used the authority-section shape.
func TestDNSSEC10NonstandardNSECResponseMixedServers(t *testing.T) {
	ctx := tctest.Context(t)

	log := logger.New()
	log.SetProfile(profile.Effective())
	util.SetLogger(log)

	apex := "example"
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	key.PublicKey = "AwEAAc=="

	tctest.NS(t, ctx, "ns1.example", "192.0.2.92", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			return nsecInAnswerResponse(q.Name, apex)
		case "NSEC3PARAM":
			return emptyNSEC3PARAMResponse(q.Name, apex)
		default:
			return packet.Packet{}
		}
	})
	tctest.NS(t, ctx, "ns2.example", "192.0.2.93", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			return nsecAuthorityNSECResponse(q.Name, apex)
		case "NSEC3PARAM":
			return emptyNSEC3PARAMResponse(q.Name, apex)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
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
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	z, err := zone.New(apex)
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC10(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec10: %v", err)
	}

	tctest.RequireTags(t, entries, "DS10_NONSTANDARD_NSEC_RESPONSE")
	tctest.RequireNoTag(t, entries, "DS10_INCONSISTENT_NSEC")
	tctest.RequireTags(t, entries, "DS10_HAS_NSEC")

	entry := tctest.First(entries, "DS10_NONSTANDARD_NSEC_RESPONSE")
	rows := tctest.Servers(t, entry.Args["servers"])
	if len(rows) != 1 {
		t.Fatalf("'servers' length = %d, want 1 (only the non-standard responder)", len(rows))
	}
	if got, _ := rows[0]["ns"].(string); !strings.HasPrefix(got, "ns2.example") {
		t.Fatalf("servers[0].ns = %q, want ns2.example...", got)
	}
}

func TestDNSSEC11ParallelParentQueries(t *testing.T) {
	ctx := tctest.Context(t)

	profile.Effective().Resolver.Defaults.Parallel = 2
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool { return false })

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

	parent1, err := nameserver.NewWithContext(ctx, "ns-parent1.example", "192.0.2.80", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent1.SetQueryHook(hook("parent1", true))

	parent2, err := nameserver.NewWithContext(ctx, "ns-parent2.example", "192.0.2.81", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	parent2.SetQueryHook(hook("parent2", false))

	tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent1, parent2}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS11_INCONSISTENT_DS", "DS11_PARENT_WITHOUT_DS", "DS11_PARENT_WITH_DS")
}

func TestDNSSEC11ParallelChildQueries(t *testing.T) {
	ctx := tctest.Context(t)

	profile.Effective().Resolver.Defaults.Parallel = 2
	tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool { return false })

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

	child1, err := nameserver.NewWithContext(ctx, "ns-child1.example", "192.0.2.90", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("child1", true))

	child2, err := nameserver.NewWithContext(ctx, "ns-child2.example", "192.0.2.91", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("child2", false))

	tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS11_INCONSISTENT_SIGNED_ZONE", "DS11_NS_WITH_UNSIGNED_ZONE", "DS11_NS_WITH_SIGNED_ZONE")
}

func TestDNSSEC11InconsistentDS(t *testing.T) {
	ctx := tctest.Context(t)

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 12345
	ds.Algorithm = 8
	ds.DigestType = 1
	ds.Digest = "DEADBEEF"

	nsWithDS := tctest.NS(t, ctx, "ns1.example", "192.0.2.80", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS(q.Name, ds)
		}
		return packet.Packet{}
	})
	nsWithoutDS := tctest.NS(t, ctx, "ns2.example", "192.0.2.81", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS(q.Name, nil)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{nsWithDS, nsWithoutDS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC11(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec11: %v", err)
	}
	tctest.RequireTags(t, entries, "DS11_INCONSISTENT_DS", "DS11_PARENT_WITHOUT_DS", "DS11_PARENT_WITH_DS")
}

func TestDNSSEC11DSButUnsignedZone(t *testing.T) {
	ctx := tctest.Context(t)

	ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	ds.KeyTag = 54321
	ds.Algorithm = 8
	ds.DigestType = 1
	ds.Digest = "FEEDBEEF"

	parentNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.82", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS(q.Name, ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "nschild.example", "192.0.2.83", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
		case "DNSKEY":
			return dnskeyPacket(q.Name, nil)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC11(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec11: %v", err)
	}
	tctest.RequireTags(t, entries, "DS11_DS_BUT_UNSIGNED_ZONE")
}

func TestDNSSEC13AlgoNotSigned(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.90", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, makeRRSIG(q.Name, dns.TypeDNSKEY))
		case "SOA":
			return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name), makeRRSIG(q.Name, dns.TypeSOA))
		case "NS":
			return answerPacket(q.Name, dns.TypeNS, nsRR, makeRRSIG(q.Name, dns.TypeNS))
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC13(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec13: %v", err)
	}
	tctest.RequireTags(t, entries, "DS13_ALGO_NOT_SIGNED_DNSKEY", "DS13_ALGO_NOT_SIGNED_SOA", "DS13_ALGO_NOT_SIGNED_NS")
}

func TestDNSSEC13ParallelQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.121", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.122", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS13_ALGO_NOT_SIGNED_SOA", "DS13_ALGO_NOT_SIGNED_NS")

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
	ctx := tctest.Context(t)

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = 8
	if _, err := key.Generate(1024); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.91", func(q tctest.Query) packet.Packet {
		if q.Type == "DNSKEY" {
			return dnskeyPacket(q.Name, key)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC14(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec14: %v", err)
	}
	tctest.RequireTags(t, entries, "DNSKEY_SMALLER_THAN_REC")
}

func TestDNSSEC14ParallelDNSKEYQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.131", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.132", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DNSKEY_SMALLER_THAN_REC")
}

func TestDNSSEC14NoResponseArgsSplit(t *testing.T) {
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.141", func(q tctest.Query) packet.Packet {
		if q.Type == "DNSKEY" {
			return packet.Packet{}
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC14(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec14: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "NO_RESPONSE")
	tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns1.example", Address: "192.0.2.141"})
}

func TestDNSSEC14NoResponseDNSKEYArgsSplit(t *testing.T) {
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.142", func(q tctest.Query) packet.Packet {
		if q.Type == "DNSKEY" {
			return answerPacket(q.Name, dns.TypeDNSKEY)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC14(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec14: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "NO_RESPONSE_DNSKEY")
	tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns1.example", Address: "192.0.2.142"})
}

func TestDNSSEC14IPv4DisabledArgsSplit(t *testing.T) {
	ctx := tctest.Context(t)

	profile.Effective().Net.IPv4 = false

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.143", nil)
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC14(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec14: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "IPV4_DISABLED")
	tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns1.example", Address: "192.0.2.143"})
}

func TestDNSSEC15NoCDSCDNSKEY(t *testing.T) {
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.92", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC15(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec15: %v", err)
	}
	tctest.RequireTags(t, entries, "DS15_NO_CDS_CDNSKEY")
}

func TestDNSSEC15ParallelQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.231", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.232", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS15_HAS_CDS_NO_CDNSKEY")

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
	ctx := tctest.Context(t)

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

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.241", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cdsSHA256, cdsSHA1)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		}
		return packet.Packet{}
	})

	ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.242", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cdsSHA256)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC15(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec15: %v", err)
	}
	tctest.RequireNoTag(t, entries, "DS15_INCONSISTENT_CDS")
	// The filter only silences the false-positive ERROR. The operator still
	// needs to know they published an inert record, so the NOTICE must fire,
	// and it must name the IP of the server that returned the SHA-1 CDS.
	notice := tctest.RequireTag(t, entries, "DS15_CDS_NON_MUST_DIGEST")
	gotAddresses, ok := notice.Args["addresses"].([]string)
	if !ok {
		t.Fatalf("DS15_CDS_NON_MUST_DIGEST addresses arg has unexpected type: %#v", notice.Args["addresses"])
	}
	if want := []string{"192.0.2.241"}; len(gotAddresses) != 1 || gotAddresses[0] != want[0] {
		t.Fatalf("DS15_CDS_NON_MUST_DIGEST addresses=%v, want %v (only NS1 published SHA-1)", gotAddresses, want)
	}
}

// TestDNSSEC15InconsistencyOnMUSTCDSDigest is the counterpart of
// TestDNSSEC15IgnoresNonMUSTCDSDigest. When two servers diverge on a CDS
// record that uses a MUST digest type, the filter must not mask the
// divergence: DS15_INCONSISTENT_CDS is still required.
func TestDNSSEC15InconsistencyOnMUSTCDSDigest(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.243", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cdsA)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		}
		return packet.Packet{}
	})

	ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.244", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cdsB)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC15(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec15: %v", err)
	}
	tctest.RequireTags(t, entries, "DS15_INCONSISTENT_CDS")
}

// TestDNSSEC15MismatchOnAlgorithmDifference verifies that the CDS/CDNSKEY
// cross-RRset match requires algorithm equality as well as key-tag
// equality. RFC 9975 talks in terms of the underlying key; two records
// that share a key tag but use different DNSSEC algorithms are not the
// same key. Before this fix the keytag-only match silently treated them
// as paired and did not emit DS15_MISMATCH_CDS_CDNSKEY.
func TestDNSSEC15MismatchOnAlgorithmDifference(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.245", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cds)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC15(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec15: %v", err)
	}
	tctest.RequireTags(t, entries, "DS15_MISMATCH_CDS_CDNSKEY")
}

func TestDNSSEC16CDSWithoutDNSKEY(t *testing.T) {
	ctx := tctest.Context(t)

	cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cds.KeyTag = 12345
	cds.Algorithm = 8
	cds.DigestType = 1
	cds.Digest = "DEADBEEF"

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.93", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cds)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC16(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec16: %v", err)
	}
	tctest.RequireTags(t, entries, "DS16_CDS_WITHOUT_DNSKEY")
}

func TestDNSSEC16ParallelQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.241", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.242", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS16_CDS_WITHOUT_DNSKEY")

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
	ctx := tctest.Context(t)

	cdnskey := &dns.CDNSKEY{DNSKEY: dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
	cdnskey.Flags = dns.FlagZONE
	cdnskey.Protocol = 3
	cdnskey.Algorithm = 8
	cdnskey.PublicKey = "AwEAAc=="

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.94", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC17(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec17: %v", err)
	}
	tctest.RequireTags(t, entries, "DS17_CDNSKEY_WITHOUT_DNSKEY")
}

func TestDNSSEC17ParallelQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.243", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.244", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS17_CDNSKEY_WITHOUT_DNSKEY")

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
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.95", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS(q.Name, ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.96", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cds, cdsSig)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey, cdnskeySig)
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec18: %v", err)
	}
	tctest.RequireTags(t, entries, "DS18_NO_MATCH_CDS_RRSIG_DS", "DS18_NO_MATCH_CDNSKEY_RRSIG_DS")
}

func TestDNSSEC18ParallelQueries(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.250", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS(q.Name, ds)
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

	child1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.251", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child1.SetQueryHook(hook("ns1"))

	child2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.252", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	child2.SetQueryHook(hook("ns2"))

	tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{child1, child2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	tctest.RequireTags(t, entries, "DS18_NO_MATCH_CDS_RRSIG_DS", "DS18_NO_MATCH_CDNSKEY_RRSIG_DS")

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

	runDNSSEC18 := func(parallel int) []*logger.Entry {
		profile.ResetEffective()
		if err := profile.Effective().Set("resolver.defaults.parallel", parallel); err != nil {
			t.Fatalf("set parallel: %v", err)
		}

		ctx := tctest.Context(t)

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

		parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.253", func(q tctest.Query) packet.Packet {
			if q.Type == "DS" {
				return dsPacketFromDS(q.Name, ds)
			}
			return packet.Packet{}
		})

		childHook := func(q tctest.Query) packet.Packet {
			switch q.Type {
			case "CDS":
				return answerPacket(q.Name, dns.TypeCDS, cds, cdsSig)
			case "CDNSKEY":
				return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey, cdnskeySig)
			case "DNSKEY":
				return dnskeyPacket(q.Name, key)
			default:
				return packet.Packet{}
			}
		}

		child1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.254", childHook)
		child2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.255", childHook)

		tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{parentNS}, nil
		})
		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{child1, child2}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}
		entries, err := DNSSEC18(ctx, &z)
		if err != nil {
			t.Fatalf("dnssec18: %v", err)
		}
		if !tctest.Has(entries, "DS18_NO_MATCH_CDS_RRSIG_DS") || !tctest.Has(entries, "DS18_NO_MATCH_CDNSKEY_RRSIG_DS") {
			t.Fatalf("expected no-match tags in dnssec18 output")
		}
		return entries
	}

	sequentialEntries := runDNSSEC18(1)
	parallelEntries := runDNSSEC18(2)

	sequentialNormalized := tctest.NormalizeStable(sequentialEntries)
	parallelNormalized := tctest.NormalizeStable(parallelEntries)
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
	tctest.Stub(t, &parentApexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return parentNSs, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return childNSs, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
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

func TestDNSSEC18CDSMatchesDS(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.70", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.71", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cds, cdsSig)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY) // no CDNSKEY records
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireTags(t, entries, "DS18_CDS_MATCHES_DS")
	tctest.RequireNoTag(t, entries, "DS18_CDS_ROLLOVER_SIGNALED")
	e := tctest.First(entries, "DS18_CDS_MATCHES_DS")
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
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.72", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.73", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cds, cdsSig)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireTags(t, entries, "DS18_CDS_ROLLOVER_SIGNALED")
	tctest.RequireNoTag(t, entries, "DS18_CDS_MATCHES_DS")
	// Verify keytag args are present and correct.
	e := tctest.First(entries, "DS18_CDS_ROLLOVER_SIGNALED")
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
	ctx := tctest.Context(t)

	// Use ToCDNSKEY so the digest computation is guaranteed to match ToDS.
	key := makeSEPKey("example", "AwEAAc==")
	keytag := key.KeyTag()
	parentDS := key.ToDS(dns.SHA256) // actual cryptographic hash

	cdnskey := key.ToCDNSKEY()

	cdnskeySig := rrsigRecord("example", dns.TypeCDNSKEY, keytag, 1, 2)
	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag, 1, 2)

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.74", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", parentDS)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.75", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS) // no CDS
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey, cdnskeySig)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireTags(t, entries, "DS18_CDNSKEY_MATCHES_DS")
	tctest.RequireNoTag(t, entries, "DS18_CDNSKEY_ROLLOVER_SIGNALED")
	e := tctest.First(entries, "DS18_CDNSKEY_MATCHES_DS")
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
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.76", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", parentDS)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.77", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey2, cdnskeySig)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key1, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireTags(t, entries, "DS18_CDNSKEY_ROLLOVER_SIGNALED")
	tctest.RequireNoTag(t, entries, "DS18_CDNSKEY_MATCHES_DS")
}

func TestDNSSEC18RolloverEvidenceMultiKSK(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.78", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.79", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			// Both keys published, only signed by key1.
			return answerPacket(q.Name, dns.TypeDNSKEY, key1, key2, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	e := tctest.RequireTag(t, entries, "DS18_ROLLOVER_EVIDENCE_MULTI_KSK")
	kts, ok := e.Args["keytags"].([]uint16)
	if !ok || len(kts) != 2 {
		t.Errorf("expected keytags with 2 entries, got %v", e.Args["keytags"])
	}
	// key2 has no DS → DNSKEY_WITHOUT_DS also fires.
	e2 := tctest.RequireTag(t, entries, "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS")
	kts2, ok := e2.Args["keytags"].([]uint16)
	if !ok || len(kts2) != 1 || kts2[0] != keytag2 {
		t.Errorf("expected keytags=[%d], got %v", keytag2, e2.Args["keytags"])
	}
}

func TestDNSSEC18RolloverEvidenceDoubleSig(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.80", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.81", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key1, key2, dnskeyRRSIG1, dnskeyRRSIG2)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	e := tctest.RequireTag(t, entries, "DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG")
	kts, ok := e.Args["keytags"].([]uint16)
	if !ok || len(kts) != 2 {
		t.Errorf("expected 2 signer keytags, got %v", e.Args["keytags"])
	}
}

func TestDNSSEC18RolloverEvidenceDSWithoutDNSKEY(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.82", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.83", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			// Only new key; old key already removed from DNSKEY RRset.
			return answerPacket(q.Name, dns.TypeDNSKEY, keyNew, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	e := tctest.RequireTag(t, entries, "DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY")
	kts, ok := e.Args["keytags"].([]uint16)
	if !ok || len(kts) != 1 || kts[0] != keytagOld {
		t.Errorf("expected orphaned keytag [%d], got %v", keytagOld, e.Args["keytags"])
	}
}

func TestDNSSEC18RolloverEvidenceDNSKEYWithoutDS(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.84", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.85", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			// Both old and new key; DS only for old.
			return answerPacket(q.Name, dns.TypeDNSKEY, keyOld, keyNew, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	e := tctest.RequireTag(t, entries, "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS")
	kts, ok := e.Args["keytags"].([]uint16)
	if !ok || len(kts) != 1 || kts[0] != keytagNew {
		t.Errorf("expected orphaned keytag [%d], got %v", keytagNew, e.Args["keytags"])
	}
}

func TestDNSSEC18NoCDSCDNSKEYButRolloverEvidence(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.86", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.87", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key1, key2, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireTags(t, entries, "DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE")
	// Underlying evidence that triggered the tag must also be present.
	tctest.RequireTags(t, entries, "DS18_ROLLOVER_EVIDENCE_MULTI_KSK")
}

// TestDNSSEC18CDSDeleteOnlySkipsContentComparison verifies that a CDS RRset
// containing only DELETE sentinels (Algorithm == 0) skips MATCHES_DS and
// ROLLOVER_SIGNALED in favour of DNSSEC16/17.
func TestDNSSEC18CDSDeleteOnlySkipsContentComparison(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.88", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.89", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cdsDelete, cdsSig)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireNoTag(t, entries, "DS18_CDS_MATCHES_DS", "DS18_CDS_ROLLOVER_SIGNALED")
}

// TestDNSSEC18CDSBothOldAndNewKeyMidRollover verifies that a CDS RRset listing
// both the current DS keytag and an incoming keytag emits ROLLOVER_SIGNALED
// (the canonical bootstrap state).
func TestDNSSEC18CDSBothOldAndNewKeyMidRollover(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.90", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", ds)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.91", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS, cdsOld, cdsNew, cdsSig)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, keyOld, keyNew, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireTags(t, entries, "DS18_CDS_ROLLOVER_SIGNALED")
	tctest.RequireNoTag(t, entries, "DS18_CDS_MATCHES_DS")
	e := tctest.First(entries, "DS18_CDS_ROLLOVER_SIGNALED")
	cdsKTs, _ := e.Args["cds_keytags"].([]uint16)
	if len(cdsKTs) != 2 {
		t.Errorf("expected 2 CDS keytags (old+new), got %v", cdsKTs)
	}
}

// TestDNSSEC18NoCDSCDNSKEYButOnlyDoubleSig verifies the umbrella tag fires when
// the only step-15 evidence is DOUBLE_SIG.
func TestDNSSEC18NoCDSCDNSKEYButOnlyDoubleSig(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.92", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return answerPacket(q.Name, dns.TypeDS, ds1, ds2)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.93", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key1, key2, dnskeyRRSIG1, dnskeyRRSIG2)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireTags(t, entries, "DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE", "DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG")
	// Both keys are in DS, so these orphan tags must NOT fire.
	tctest.RequireNoTag(t, entries, "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS", "DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY")
}

func TestDNSSEC19CleanZone(t *testing.T) {
	ctx := tctest.Context(t)

	if err := profile.Effective().Set("badkeys.path", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	tctest.NS(t, ctx, "ns1.example", "192.0.2.201", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(q.Name, dnssec19P256Key(q.Name))
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.201"),
			HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	tctest.RequireTags(t, entries, "DS19_KEY_OK", "DS19_BLOCKLIST_NOT_FOUND")
}

func TestDNSSEC19BlocklistedKey(t *testing.T) {
	ctx := tctest.Context(t)

	key := dnssec19P256Key("example")
	dir := t.TempDir()
	writeDNSSEC19BlocklistFixture(t, dir, key.Algorithm, key.PublicKey, 7, "unit-blocklist")
	if err := profile.Effective().Set("badkeys.path", dir); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	tctest.NS(t, ctx, "ns1.example", "192.0.2.202", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(q.Name, dnssec19P256Key(q.Name))
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.202"),
			HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	tctest.RequireTags(t, entries, "DS19_BADKEY_BLOCKLIST")
	tctest.RequireNoTag(t, entries, "DS19_KEY_OK", "DS19_BLOCKLIST_NOT_FOUND")

	blocklisted := tctest.RequireTag(t, entries, "DS19_BADKEY_BLOCKLIST")
	if got, _ := blocklisted.Args["blocklist_name"].(string); got != "unit-blocklist" {
		t.Fatalf("unexpected blocklist_name: got %q want %q", got, "unit-blocklist")
	}
}

func TestDNSSEC19NoDNSKEY(t *testing.T) {
	ctx := tctest.Context(t)

	if err := profile.Effective().Set("badkeys.path", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	tctest.NS(t, ctx, "ns1.example", "192.0.2.203", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(q.Name, nil)
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.203"),
			HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	tctest.RequireTags(t, entries, "DS19_NO_DNSKEY")
	tctest.RequireNoTag(t, entries, "DS19_NO_RESPONSE")
}

func TestDNSSEC19NoResponse(t *testing.T) {
	ctx := tctest.Context(t)

	if err := profile.Effective().Set("badkeys.path", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	tctest.NS(t, ctx, "ns1.example", "192.0.2.204", func(q tctest.Query) packet.Packet {
		return packet.Packet{}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.204"),
			HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	tctest.RequireTags(t, entries, "DS19_NO_RESPONSE")
	tctest.RequireNoTag(t, entries, "DS19_NO_DNSKEY")
}

func TestDNSSEC19TransportDisabled(t *testing.T) {
	ctx := tctest.Context(t)

	if err := profile.Effective().Set("net.ipv4", false); err != nil {
		t.Fatalf("set net.ipv4: %v", err)
	}
	if err := profile.Effective().Set("badkeys.path", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("set badkeys.path: %v", err)
	}

	tctest.NS(t, ctx, "ns1.example", "192.0.2.205", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		return dnskeyPacket(q.Name, dnssec19P256Key(q.Name))
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.205"),
			HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC19(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec19: %v", err)
	}

	tctest.RequireTags(t, entries, "IPV4_DISABLED")
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
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns1.example", "192.0.2.201", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, dnssec19P256Key(q.Name))
		case "NSEC":
			nsecRR := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			nsecRR.NextDomain = "\\000." + dnsutil.Fqdn(q.Name)
			nsecRR.TypeBitMap = []uint16{dns.TypeA, dns.TypeNS, dns.TypeSOA, dns.TypeAAAA, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeDNSKEY}
			return answerPacket(q.Name, dns.TypeNSEC, nsecRR)
		case "A":
			aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			aRR.Addr = netip.MustParseAddr("192.0.2.1")
			return answerPacket(q.Name, dns.TypeA, aRR)
		case "AAAA":
			aaaaRR := &dns.AAAA{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			aaaaRR.Addr = netip.MustParseAddr("2001:db8::1")
			return answerPacket(q.Name, dns.TypeAAAA, aaaaRR)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.201"), HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC20(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec20: %v", err)
	}

	tctest.RequireTags(t, entries, "DS20_BITMAP_OK")
	tctest.RequireNoTag(t, entries, "DS20_NSEC_BITMAP_MISMATCHES_RRTYPE")
}

func TestDNSSEC20NSECSubsetBitmap(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns1.example", "192.0.2.201", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, dnssec19P256Key(q.Name))
		case "NSEC":
			nsecRR := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			nsecRR.NextDomain = "\\000." + dnsutil.Fqdn(q.Name)
			nsecRR.TypeBitMap = []uint16{dns.TypeNS, dns.TypeSOA, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeDNSKEY}
			return answerPacket(q.Name, dns.TypeNSEC, nsecRR)
		case "A":
			aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			aRR.Addr = netip.MustParseAddr("3.13.31.214")
			return answerPacket(q.Name, dns.TypeA, aRR)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.201"), HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC20(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec20: %v", err)
	}

	entry := tctest.RequireTag(t, entries, "DS20_NSEC_BITMAP_MISMATCHES_RRTYPE")
	if entry == nil {
		t.Fatal("missing DS20_NSEC_BITMAP_MISMATCHES_RRTYPE entry")
	}
	if rrtype, _ := entry.Args["query_type"].(string); rrtype != "A" {
		t.Fatalf("expected rrtype=A, got %q", rrtype)
	}
	tctest.RequireNoTag(t, entries, "DS20_BITMAP_OK")
}

func TestDNSSEC20NSEC3SubsetBitmap(t *testing.T) {
	ctx := tctest.Context(t)

	// Build an NSEC3 record whose owner hash matches the apex.
	apexHash := dnsutil.NSEC3Name("example.", "", 0)
	nsec3Owner := apexHash + ".example."

	tctest.NS(t, ctx, "ns1.example", "192.0.2.201", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, dnssec19P256Key(q.Name))
		case "NSEC":
			// NSEC3 zone: return NSEC3 in authority section (NODATA).
			nsec3RR := &dns.NSEC3{Hdr: dns.Header{Name: nsec3Owner, Class: dns.ClassINET, TTL: 60}}
			nsec3RR.Hash = dns.SHA1
			nsec3RR.Iterations = 0
			nsec3RR.Salt = ""
			nsec3RR.TypeBitMap = []uint16{dns.TypeNS, dns.TypeSOA, dns.TypeRRSIG, dns.TypeDNSKEY, dns.TypeNSEC3PARAM}
			return nsec3Packet(q.Name, nsec3RR)
		case "A":
			aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			aRR.Addr = netip.MustParseAddr("3.13.31.214")
			return answerPacket(q.Name, dns.TypeA, aRR)
		case "AAAA":
			aaaaRR := &dns.AAAA{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			aaaaRR.Addr = netip.MustParseAddr("2001:db8::1")
			return answerPacket(q.Name, dns.TypeAAAA, aaaaRR)
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.201"), HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC20(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec20: %v", err)
	}

	tctest.RequireTags(t, entries, "DS20_NSEC3_BITMAP_MISMATCHES_RRTYPE")
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
	ctx := tctest.Context(t)

	tctest.NS(t, ctx, "ns1.example", "192.0.2.201", func(q tctest.Query) packet.Packet {
		return packet.Packet{}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.201"), HasAddress: true,
		}}, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC20(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec20: %v", err)
	}

	tctest.RequireTags(t, entries, "DS20_NO_DNSSEC")
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
	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return &parentZone, nil
	})
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
}

func TestDNSSEC21Verified(t *testing.T) {
	ctx := tctest.Context(t)

	f := newDNSSEC21Fixture(t)
	dsResp := f.signedDSResponse(t, f.parentKey, f.parentPriv)
	dnskeyResp := f.parentDNSKEYResponse(f.parentKey)

	parentNS := tctest.NS(t, ctx, "ns1.parent", "192.0.2.221", func(q tctest.Query) packet.Packet {
		switch q.Type {
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
	entries, err := DNSSEC21(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	tctest.RequireTags(t, entries, "DS21_DS_RRSIG_VERIFIED")
	for _, badTag := range []string{"DS21_DS_RRSIG_NOT_VERIFIABLE", "DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY", "DS21_NO_DS_RRSIG", "DS21_PARENT_DNSKEY_MISSING"} {
		tctest.RequireNoTag(t, entries, badTag)
	}
}

func TestDNSSEC21RRSIGNotVerifiable(t *testing.T) {
	ctx := tctest.Context(t)

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

	parentNS := tctest.NS(t, ctx, "ns1.parent", "192.0.2.222", func(q tctest.Query) packet.Packet {
		switch q.Type {
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
	entries, err := DNSSEC21(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	tctest.RequireTags(t, entries, "DS21_DS_RRSIG_NOT_VALID_BY_DNSKEY", "DS21_DS_RRSIG_NOT_VERIFIABLE")
	tctest.RequireNoTag(t, entries, "DS21_DS_RRSIG_VERIFIED")
}

func TestDNSSEC21NoParentDNSKEY(t *testing.T) {
	ctx := tctest.Context(t)

	f := newDNSSEC21Fixture(t)
	dsResp := f.signedDSResponse(t, f.parentKey, f.parentPriv)
	emptyDNSKEY := f.emptyDNSKEYResponse()

	parentNS := tctest.NS(t, ctx, "ns1.parent", "192.0.2.223", func(q tctest.Query) packet.Packet {
		switch q.Type {
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
	entries, err := DNSSEC21(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	tctest.RequireTags(t, entries, "DS21_PARENT_DNSKEY_MISSING")
	tctest.RequireNoTag(t, entries, "DS21_DS_RRSIG_VERIFIED")
}

func TestDNSSEC21NoDSRRSIG(t *testing.T) {
	ctx := tctest.Context(t)

	f := newDNSSEC21Fixture(t)
	parentNS := tctest.NS(t, ctx, "ns1.parent", "192.0.2.224", func(q tctest.Query) packet.Packet {
		switch q.Type {
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
	entries, err := DNSSEC21(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	tctest.RequireTags(t, entries, "DS21_NO_DS_RRSIG")
}

func TestDNSSEC21RootZone(t *testing.T) {
	ctx := tctest.Context(t)

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
	entries, err := DNSSEC21(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC21: %v", err)
	}
	tctest.RequireTags(t, entries, "DS21_NO_PARENT_ZONE")
}

func TestDNSSEC21UnsignedDelegation(t *testing.T) {
	ctx := tctest.Context(t)

	f := newDNSSEC21Fixture(t)
	emptyDS := f.emptyDSResponse()
	parentNS := tctest.NS(t, ctx, "ns1.parent", "192.0.2.225", func(q tctest.Query) packet.Packet {
		switch q.Type {
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
	entries, err := DNSSEC21(ctx, &z)
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
		tctest.RequireNoTag(t, entries, badTag)
	}
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
