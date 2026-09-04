package dnssec

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/badkeys"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestDNSSEC01DigestMatrix(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		keytag  uint16
		algo    uint8
		digest  uint8
		wantTag string
	}{
		{name: "AlgoOK", ip: "192.0.2.1", keytag: 12345, algo: 8, digest: 2, wantTag: "DS01_DS_ALGO_OK"},
		// digest 5 = GOST R 34.11-2012 (RFC 9558)
		{name: "DigestGOST12", ip: "192.0.2.31", keytag: 12345, algo: 8, digest: 5, wantTag: "DS01_DS_ALGO_OK"},
		// digest 6 = SM3 (RFC 9563)
		{name: "DigestSM3", ip: "192.0.2.32", keytag: 12345, algo: 8, digest: 6, wantTag: "DS01_DS_ALGO_OK"},
		{name: "Algo2Missing", ip: "192.0.2.10", keytag: 54321, algo: 8, digest: 1, wantTag: "DS01_DS_ALGO_2_MISSING"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)

			tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
				return nil, nil
			})
			tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
				return false
			})

			ns := tctest.NS(t, ctx, "ns1.example", tc.ip, func(q tctest.Query) packet.Packet {
				if q.Type != "DS" {
					return packet.Packet{}
				}
				return dsPacket(q.Name, tc.keytag, tc.algo, tc.digest)
			})

			tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
				return []nameserver.Nameserver{ns}, nil
			})

			z := zone.Zone{Name: dnsname.New("example")}
			entries, err := DNSSEC01(ctx, &z)
			if err != nil {
				t.Fatalf("dnssec01: %v", err)
			}
			tctest.RequireTags(t, entries, tc.wantTag)
		})
	}
}

func TestDNSSEC01UndelegatedDSOnlyUsesFakeDS(t *testing.T) {
	ctx := tctest.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".":       {"ns1.root": {"192.0.2.1"}},
		"example": {"ns-child.example": {"192.0.2.53"}},
	})

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
		servers := tctest.Servers(t, entry.Args["servers"])
		if len(servers) != 1 {
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
	// DS01 and DS05 classify the same algorithm numbers, so their tags must
	// never disagree. Unlike the boundary table above this walks the whole
	// uint8 domain, so a fork that drifts on an untabulated number is caught.
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
	servers := tctest.Servers(t, entry.Args["servers"])
	if len(servers) == 0 {
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

	r := tctest.Recursor(t, map[string]map[string][]string{
		".":       {"ns1.root": {"192.0.2.1"}},
		"example": {"ns-child.example": {"192.0.2.53"}},
	})

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
	servers := tctest.Servers(t, entry.Args["servers"])
	if len(servers) != 1 {
		t.Fatalf("expected one server for undelegated fake DS, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "-" {
		t.Fatalf("expected undelegated fake DS source (servers[0].ns='-'), got %#v", servers[0])
	}
}

func TestDNSSEC01ParallelParentQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
			return nil, nil
		})
		tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool {
			return false
		})

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if q.Type != "DS" {
					return packet.Packet{}
				}
				gate.Arrive(id)
				return dsPacket(q.Name, 12345, 8, 2)
			}
		}

		parent1 := tctest.NS(t, ctx, "ns-parent1.example", "192.0.2.80", hook("parent1"))
		parent2 := tctest.NS(t, ctx, "ns-parent2.example", "192.0.2.81", hook("parent2"))

		tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{parent1, parent2}, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC01(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "parent1", "parent2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec01: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DS01_DS_ALGO_OK")

		var gotServers []map[string]any
		for _, entry := range entries {
			if entry == nil || entry.Tag != "DS01_DS_ALGO_OK" {
				continue
			}
			gotServers = tctest.Servers(t, entry.Args["servers"])
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
	})
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
		key := tctest.DNSKEYRR(q.Name, 8, tctest.PublicKey("AwEAAc=="))
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

	key := tctest.DNSKEYRR("example", 8, tctest.Flags(dns.FlagSEP), tctest.PublicKey("AwEAAc=="))
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

	key, signer := tctest.SignedKey(t, owner, dns.ECDSAP256SHA256, tctest.SEP(), tctest.KeyTTL(3600))
	return key, tctest.Sign(t, key, signer, dns.TypeDNSKEY, []dns.RR{key})
}

func dnssec02Wire(t *testing.T, parentNS nameserver.Nameserver, childNS nameserver.Nameserver) {
	t.Helper()

	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parentNS}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{childNS}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})
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
	dnssec02Wire(t, parentNS, childNS)

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
	dnssec02Wire(t, parentNS, childNS)

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
	dnssec02Wire(t, parentNS, childNS)

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
	dnssec02Wire(t, parentNS, childNS)

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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		key := tctest.DNSKEYRR("example", 8, tctest.SEP(), tctest.PublicKey("AwEAAc=="))
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

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if q.Type != "DNSKEY" {
					return packet.Packet{}
				}
				gate.Arrive(id)
				return dnskeyPacket(q.Name, key)
			}
		}

		child1 := tctest.NS(t, ctx, "ns-child1.example", "192.0.2.101", hook("child1"))
		child2 := tctest.NS(t, ctx, "ns-child2.example", "192.0.2.102", hook("child2"))

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

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC02(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "child1", "child2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec02: %v", dsErr)
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
	})
}

func TestDNSSEC03NoNSEC3(t *testing.T) {
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.4", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			key := tctest.DNSKEYRR(q.Name, 8, tctest.PublicKey("AwEAAc=="))
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
	noNSEC3Servers := tctest.Servers(t, noNSEC3.Args["servers"])
	if len(noNSEC3Servers) != 1 {
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
			key := tctest.DNSKEYRR(q.Name, 8, tctest.PublicKey("AwEAAc=="))
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
	illegalServers := tctest.Servers(t, illegal.Args["servers"])
	if len(illegalServers) != 1 {
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

// The NSEC3 Salt field carries hex text, but RFC 5155 section 3.1.5 counts the salt
// in octets, which is what the message promises the reader ("{int} octets").
func TestDNSSEC03SaltLengthCountsOctets(t *testing.T) {
	// 8 hex characters, so 4 octets on the wire.
	const saltHex = "aabbccdd"
	const saltOctets = 4

	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.13", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			key := tctest.DNSKEYRR(q.Name, 8, tctest.PublicKey("AwEAAc=="))
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			nsec3 := &dns.NSEC3{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			nsec3.Hash = 1
			nsec3.Flags = 0
			nsec3.Iterations = 0
			nsec3.SaltLength = saltOctets
			nsec3.Salt = saltHex
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

	illegal := tctest.RequireTag(t, entries, "DS03_ILLEGAL_SALT_LENGTH")
	got, ok := illegal.Args["int"].(int)
	if !ok {
		t.Fatalf("expected an int salt length for DS03_ILLEGAL_SALT_LENGTH, got %#v", illegal.Args["int"])
	}
	if got != saltOctets {
		t.Fatalf("expected salt length %d octets, got %d (hex-string length would be %d)", saltOctets, got, len(saltHex))
	}

	servers := tctest.Servers(t, illegal.Args["servers"])
	if len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("unexpected typed server payload for DS03_ILLEGAL_SALT_LENGTH: %#v", illegal.Args["servers"])
	}
}

// An absent salt is the recommended practice and stays legal at length 0.
func TestDNSSEC03EmptySaltIsLegal(t *testing.T) {
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.13", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			key := tctest.DNSKEYRR(q.Name, 8, tctest.PublicKey("AwEAAc=="))
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			nsec3 := &dns.NSEC3{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 60}}
			nsec3.Hash = 1
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

	tctest.RequireTag(t, entries, "DS03_LEGAL_EMPTY_SALT")
	for _, entry := range entries {
		if entry != nil && entry.Tag == "DS03_ILLEGAL_SALT_LENGTH" {
			t.Fatalf("an absent salt must not be reported as illegal: %#v", entry.Args)
		}
	}
}

func TestDNSSEC03ParallelDNSKEYQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "DNSKEY":
					gate.Arrive(id)
					return dnskeyPacket(q.Name, key)
				case "NSEC":
					return nsecPacket(q.Name)
				default:
					return packet.Packet{}
				}
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.201", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.202", hook("ns2"))

		tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1, ns2}, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC03(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec03: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DS03_NO_NSEC3")

		var gotServers []map[string]any
		for _, entry := range entries {
			if entry == nil || entry.Tag != "DS03_NO_NSEC3" {
				continue
			}
			gotServers = tctest.Servers(t, entry.Args["servers"])
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
	})
}

func TestDNSSEC04ExpiredRRSIG(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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
	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		now := time.Unix(1700000000, 0).UTC()
		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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

		gate := tctest.NewGate()

		tctest.Stub(t, &zoneQueryOne, func(_ context.Context, _ *zone.Zone, _ string, rrtype string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch rrtype {
			case "DNSKEY":
				gate.Arrive(rrtype)
				return dnskeyResp, nil
			case "SOA":
				gate.Arrive(rrtype)
				return soaResp, nil
			default:
				return packet.Packet{}, nil
			}
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC04(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "DNSKEY", "SOA")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec04: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DURATION_OK")
	})
}

func TestDNSSEC05AlgorithmMatrix(t *testing.T) {
	cases := []struct {
		name     string
		ip       string
		algo     uint8
		wantArgs map[string]any
	}{
		{name: "AlgoOK", ip: "192.0.2.20", algo: 8},
		// 17 = SM2SM3 (RFC 9563)
		{name: "AlgoSM2SM3", ip: "192.0.2.33", algo: 17},
		// 18 = ML-DSA-44, IANA-assigned post-quantum signing algorithm. The tag
		// alone would still pass with a stale algorithm table, so the rendered
		// name is asserted too: before algorithm 18 was assigned it fell in the
		// unassigned range and reported the mnemonic UNASSIGNED.
		{name: "AlgoMLDSA44", ip: "192.0.2.33", algo: 18, wantArgs: map[string]any{
			"algo_num":   uint8(18),
			"algo_descr": "ML-DSA-44",
			"algo_mnemo": "MLDSA44",
		}},
		// 23 = ECC-GOST12 (RFC 9558)
		{name: "AlgoECCGOST12", ip: "192.0.2.34", algo: 23},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)

			tctest.NS(t, ctx, "ns1.example", tc.ip, func(q tctest.Query) packet.Packet {
				if q.Type != "DNSKEY" {
					return packet.Packet{}
				}
				key := tctest.DNSKEYRR(q.Name, tc.algo, tctest.PublicKey("AwEAAc=="))
				return dnskeyPacket(q.Name, key)
			})

			tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
				return tctest.NSItems("ns1.example/" + tc.ip), nil
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
			for arg, want := range tc.wantArgs {
				if entry.Args[arg] != want {
					t.Fatalf("expected %s %v, got %#v", arg, want, entry.Args[arg])
				}
			}
		})
	}
}

func TestDNSSEC05ParallelDNSKEYQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

		gate := tctest.NewGate()

		handler := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if q.Type != "DNSKEY" {
					return packet.Packet{}
				}
				gate.Arrive(id)
				return dnskeyPacket(q.Name, key)
			}
		}

		tctest.NS(t, ctx, "ns1.example", "192.0.2.220", handler("ns1"))
		tctest.NS(t, ctx, "ns2.example", "192.0.2.221", handler("ns2"))

		tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
			return tctest.NSItems("ns1.example/192.0.2.220", "ns2.example/192.0.2.221"), nil
		})
		tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
			return []nsdiscovery.NSItem{}, nil
		})

		z, err := zone.New("example")
		if err != nil {
			t.Fatalf("zone new: %v", err)
		}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC05(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec05: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DS05_ALGO_OK")

		var gotServers []map[string]any
		for _, entry := range entries {
			if entry == nil || entry.Tag != "DS05_ALGO_OK" {
				continue
			}
			gotServers = tctest.Servers(t, entry.Args["servers"])
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
	})
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
		return tctest.NSItems("ns2.example/192.0.2.21"), nil
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
		return tctest.NSItems("ns3.example/192.0.2.22"), nil
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

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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

	ds := tctest.DSRR("example", 11111, 8, 2, "DEADBEEF")
	dsSig := rrsigRecord("example", dns.TypeDS, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	tctest.NS(t, ctx, "ns-parent.example", "192.0.2.41", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return answerPacket(q.Name, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return tctest.NSItems("ns1.example/192.0.2.40"), nil
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
	signedServers := tctest.Servers(t, signedOnServer.Args["servers"])
	if len(signedServers) != 1 {
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
	parentServers := tctest.Servers(t, dsOnParent.Args["servers"])
	if len(parentServers) != 1 {
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
			return nil, nil
		})
		tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "SOA":
					gate.Arrive(id)
					return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
				case "DNSKEY":
					return dnskeyPacket(q.Name, key)
				default:
					return packet.Packet{}
				}
			}
		}

		tctest.NS(t, ctx, "ns1.example", "192.0.2.60", hook("ns1"))
		tctest.NS(t, ctx, "ns2.example", "192.0.2.61", hook("ns2"))

		tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
			return tctest.NSItems("ns1.example/192.0.2.60", "ns2.example/192.0.2.61"), nil
		})
		tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
			return []nsdiscovery.NSItem{}, nil
		})

		z, err := zone.New("example")
		if err != nil {
			t.Fatalf("zone new: %v", err)
		}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC07(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec07: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DS07_NOT_SIGNED_ON_SERVER", "DS07_NOT_SIGNED")

		var gotServers []map[string]any
		for _, entry := range entries {
			if entry == nil || entry.Tag != "DS07_NOT_SIGNED_ON_SERVER" {
				continue
			}
			gotServers = tctest.Servers(t, entry.Args["servers"])
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
	})
}

func TestDNSSEC07ParallelParentQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
			return nil, nil
		})

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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
			return tctest.NSItems("ns-child.example/192.0.2.62"), nil
		})
		tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
			return []nsdiscovery.NSItem{}, nil
		})

		ds := tctest.DSRR("example", 11111, 8, 2, "DEADBEEF")
		dsSig := rrsigRecord("example", dns.TypeDS, 11111, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if q.Type == "DS" {
					gate.Arrive(id)
					return answerPacket(q.Name, dns.TypeDS, ds, dsSig)
				}
				return packet.Packet{}
			}
		}

		parent1 := tctest.NS(t, ctx, "ns-parent1.example", "192.0.2.70", hook("parent1"))
		parent2 := tctest.NS(t, ctx, "ns-parent2.example", "192.0.2.71", hook("parent2"))

		tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{parent1, parent2}, nil
		})

		z, err := zone.New("example")
		if err != nil {
			t.Fatalf("zone new: %v", err)
		}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC07(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "parent1", "parent2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec07: %v", dsErr)
		}

		var gotServers []map[string]any
		for _, entry := range entries {
			if entry == nil || entry.Tag != "DS07_DS_ON_PARENT_SERVER" {
				continue
			}
			gotServers = tctest.Servers(t, entry.Args["servers"])
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
	})
}

func TestDNSSEC07NotSigned(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &zoneParent, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		return nil, nil
	})

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

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
		return tctest.NSItems("ns2.example/192.0.2.42"), nil
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
	notSignedServers := tctest.Servers(t, notSignedOnServer.Args["servers"])
	if len(notSignedServers) != 1 {
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

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

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
			return tctest.Response(tctest.Question(q.Name, dns.TypeDNSKEY),
				tctest.Rcode(dns.RcodeServerFailure), tctest.Secure())
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return tctest.NSItems("ns-noresp.example/192.0.2.170", "ns-noauth.example/192.0.2.171", "ns-rcode.example/192.0.2.172"), nil
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
		servers := tctest.Servers(t, entry.Args["servers"])
		if len(servers) != 1 {
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

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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

	ds := tctest.DSRR("example", 11111, 8, 2, "DEADBEEF")

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
		return tctest.NSItems("ns1.example/192.0.2.180"), nil
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
	servers := tctest.Servers(t, noDS.Args["servers"])
	if len(servers) != 1 {
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

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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

	ds := tctest.DSRR("example", 11111, 8, 2, "DEADBEEF")

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
		return tctest.NSItems("ns1.example/192.0.2.180"), nil
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
		return tctest.NSItems("ns1.example/192.0.2.160", "ns2.example/192.0.2.161"), nil
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
		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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

// stubAllDNSSECDiscovery points DNSSEC07's and DNSSEC11's discovery at a fixed
// parent/child topology. parentNameservers and zoneParent are stubbed to
// no-ops because DNSSEC07 calls them before deciding the zone is unsigned.
func stubAllDNSSECDiscovery(t *testing.T, parentNS, childNS nameserver.Nameserver) {
	t.Helper()

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return tctest.NSItems("ns1.example/192.0.2.160"), nil
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

	staleDS := tctest.DSRR("example", 54321, 8, 2, "DEADBEEF")

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

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

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
	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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
	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if q.Type != "DNSKEY" {
					return packet.Packet{}
				}
				gate.Arrive(id)
				return dnskeyPacket(q.Name, key)
			}
		}

		child1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.101", hook("child1"))
		child2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.102", hook("child2"))

		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{child1, child2}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC08(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "child1", "child2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec08: %v", dsErr)
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
	})
}

func TestDNSSEC09MissingRRSIG(t *testing.T) {
	ctx := tctest.Context(t)

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

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

// The .lv KSK: DS-linkable, exponent 2^32+1; nobody holds its private key.
func lvLargeExponentKSK(owner string) *dns.DNSKEY {
	return tctest.DNSKEYRR(owner, 8, tctest.SEP(), tctest.PublicKey(dnstest.LVKSK42018)) // RSASHA256
}

// The same key with a 65-bit exponent, which the local path declines too.
func lvUnverifiableExponentKSK(owner string) *dns.DNSKEY {
	return tctest.DNSKEYRR(owner, 8, tctest.SEP(), tctest.PublicKey(dnstest.LVKSK42018E65))
}

// largeExponentKeypair can sign: 2^32+1 like .lv, verified by the local path.
func largeExponentKeypair(t *testing.T) dnstest.Keypair {
	return dnstest.GenRSAKeyWithExponent(t, "example", dnstest.LVExponent, 1024, true)
}

// An exponent past even the local path gives the NOTICE, not the ERROR, and stays indeterminate.
func TestDNSSEC02RSAExponentUnsupported(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key := lvUnverifiableExponentKSK("example")
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
	key := tctest.DNSKEYRR("example", 8, tctest.SEP())
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
	key := lvUnverifiableExponentKSK("example")
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
	key := lvUnverifiableExponentKSK("example")
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

// dnssec02Entries runs DNSSEC02 with ds at the parent and key plus sig at the child.
func dnssec02Entries(t *testing.T, now time.Time, key *dns.DNSKEY, ds *dns.DS, sig *dns.RRSIG, parentIP, childIP string) []*logger.Entry {
	t.Helper()
	ctx := tctest.Context(t)
	parentNS := tctest.NS(t, ctx, "ns-parent.example", parentIP, func(q tctest.Query) packet.Packet {
		if q.Type != "DS" {
			return packet.Packet{}
		}
		return dsPacketFromDS(q.Name, ds)
	})
	childNS := tctest.NS(t, ctx, "ns-child.example", childIP, func(q tctest.Query) packet.Packet {
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
	return entries
}

// A DS-linked .lv-exponent key with a valid signature passes like any other key.
func TestDNSSEC02LargeExponentSignatureVerifies(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	kp := largeExponentKeypair(t)
	ds := kp.Key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}
	sig := dnstest.SignRRset(t, kp, []dns.RR{kp.Key}, "example", "example", now.Add(-time.Hour), now.Add(time.Hour))

	entries := dnssec02Entries(t, now, kp.Key, ds, sig, "198.51.100.1", "198.51.100.2")
	tctest.RequireTags(t, entries, "DS02_MATCH_DS_DNSKEY")
	tctest.RequireNoTag(t, entries,
		"DS02_RSA_EXPONENT_UNSUPPORTED",
		"DS02_RRSIG_NOT_VALID_BY_DNSKEY",
		"DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS",
		"DS02_NO_MATCHING_DNSKEY_RRSIG")
}

// A .lv-exponent key with a bad signature is a genuine failure, not the NOTICE.
func TestDNSSEC02LargeExponentBadSignature(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	key := lvLargeExponentKSK("example")
	ds := key.ToDS(2)
	if ds == nil {
		t.Fatal("expected DS from DNSKEY")
	}
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	entries := dnssec02Entries(t, now, key, ds, sig, "198.51.100.3", "198.51.100.4")
	tctest.RequireTags(t, entries, "DS02_RRSIG_NOT_VALID_BY_DNSKEY", "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS")
	tctest.RequireNoTag(t, entries, "DS02_RSA_EXPONENT_UNSUPPORTED")
}

// dnssec08Entries runs DNSSEC08 against one child serving key and sig.
func dnssec08Entries(t *testing.T, now time.Time, key *dns.DNSKEY, sig *dns.RRSIG, ip string) []*logger.Entry {
	t.Helper()
	ctx := tctest.Context(t)
	ns := tctest.NS(t, ctx, "ns1.example", ip, func(q tctest.Query) packet.Packet {
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
	return entries
}

func TestDNSSEC08LargeExponentSignatureVerifies(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	kp := largeExponentKeypair(t)
	sig := dnstest.SignRRset(t, kp, []dns.RR{kp.Key}, "example", "example", now.Add(-time.Hour), now.Add(time.Hour))

	entries := dnssec08Entries(t, now, kp.Key, sig, "198.51.100.5")
	tctest.RequireTags(t, entries, "DS08_DNSKEY_RRSIG_VALID")
	tctest.RequireNoTag(t, entries, "DS08_RSA_EXPONENT_UNSUPPORTED", "DS08_RRSIG_NOT_VALID_BY_DNSKEY")
}

func TestDNSSEC08LargeExponentBadSignature(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	key := lvLargeExponentKSK("example")
	sig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	entries := dnssec08Entries(t, now, key, sig, "198.51.100.6")
	tctest.RequireTags(t, entries, "DS08_RRSIG_NOT_VALID_BY_DNSKEY")
	tctest.RequireNoTag(t, entries, "DS08_RSA_EXPONENT_UNSUPPORTED", "DS08_DNSKEY_RRSIG_VALID")
}

// dnssec09Entries runs DNSSEC09 against one child serving key, soa and sig.
func dnssec09Entries(t *testing.T, now time.Time, key *dns.DNSKEY, soa *dns.SOA, sig *dns.RRSIG, ip string) []*logger.Entry {
	t.Helper()
	ctx := tctest.Context(t)
	ns := tctest.NS(t, ctx, "ns1.example", ip, func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			pkt := dnskeyPacket(q.Name, key)
			pkt.Timestamp = now
			return pkt
		case "SOA":
			pkt := answerPacket(q.Name, dns.TypeSOA, soa, sig)
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
	return entries
}

func TestDNSSEC09LargeExponentSignatureVerifies(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	kp := largeExponentKeypair(t)
	soa := soaRecord("example")
	sig := dnstest.SignRRset(t, kp, []dns.RR{soa}, "example", "example", now.Add(-time.Hour), now.Add(time.Hour))

	entries := dnssec09Entries(t, now, kp.Key, soa, sig, "198.51.100.7")
	tctest.RequireTags(t, entries, "DS09_SOA_RRSIG_VALID")
	tctest.RequireNoTag(t, entries, "DS09_RSA_EXPONENT_UNSUPPORTED", "DS09_RRSIG_NOT_VALID_BY_DNSKEY")
}

func TestDNSSEC09LargeExponentBadSignature(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	key := lvLargeExponentKSK("example")
	soa := soaRecord("example")
	sig := rrsigRecord("example", dns.TypeSOA, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

	entries := dnssec09Entries(t, now, key, soa, sig, "198.51.100.8")
	tctest.RequireTags(t, entries, "DS09_RRSIG_NOT_VALID_BY_DNSKEY")
	tctest.RequireNoTag(t, entries, "DS09_RSA_EXPONENT_UNSUPPORTED", "DS09_SOA_RRSIG_VALID")
}

func TestDNSSEC09ParallelQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "DNSKEY":
					gate.Arrive(id)
					return dnskeyPacket(q.Name, key)
				case "SOA":
					return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
				default:
					return packet.Packet{}
				}
			}
		}

		child1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.111", hook("child1"))
		child2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.112", hook("child2"))

		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{child1, child2}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC09(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "child1", "child2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec09: %v", dsErr)
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
	})
}

func TestDNSSEC10MissingSignature(t *testing.T) {
	ctx := tctest.Context(t)

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next.example")
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG}

	tctest.NS(t, ctx, "ns1.example", "192.0.2.70", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			return tctest.Response(tctest.Question(q.Name, dns.TypeNSEC), tctest.Secure())
		case "NSEC3PARAM":
			return tctest.Response(tctest.Question(q.Name, dns.TypeNSEC3PARAM), tctest.Secure(), tctest.Authority(nsec, soaRecord(q.Name)))
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return tctest.NSItems("ns1.example/192.0.2.70"), nil
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if q.Type != "DNSKEY" {
					return packet.Packet{}
				}
				gate.Arrive(id)
				return dnskeyPacket(q.Name, key)
			}
		}

		tctest.NS(t, ctx, "ns1.example", "192.0.2.201", hook("ns1"))
		tctest.NS(t, ctx, "ns2.example", "192.0.2.202", hook("ns2"))

		tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
			return tctest.NSItems("ns1.example/192.0.2.201", "ns2.example/192.0.2.202"), nil
		})
		tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
			return []nsdiscovery.NSItem{}, nil
		})

		z, err := zone.New("example")
		if err != nil {
			t.Fatalf("zone new: %v", err)
		}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC10(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec10: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DS10_NSEC_QUERY_RESPONSE_ERR")

		var gotServers []map[string]any
		for _, entry := range entries {
			if entry == nil || entry.Tag != "DS10_NSEC_QUERY_RESPONSE_ERR" {
				continue
			}
			gotServers = tctest.Servers(t, entry.Args["servers"])
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
	})
}

// Two NSEC3PARAM RRs at the apex (a legitimate parameter-rollover shape per
// RFC 5155, which imposes no cardinality on the apex NSEC3PARAM RRset) must
// not produce DS10_NSEC3PARAM_MISMATCHES_APEX, and the retired
// DS10_ERR_MULT_NSEC3PARAM tag must never appear.
func TestDNSSEC10MultipleNSEC3PARAMAllApex(t *testing.T) {
	ctx := tctest.Context(t)

	apex := dnsutil.Fqdn("example")

	key := tctest.DNSKEYRR(apex, 8, tctest.PublicKey("AwEAAc=="))

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
			return tctest.Response(tctest.Question(q.Name, dns.TypeNSEC), tctest.Secure())
		case "NSEC3PARAM":
			return tctest.Response(tctest.Question(q.Name, dns.TypeNSEC3PARAM), tctest.Secure(), tctest.Answers(param1, param2))
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return tctest.NSItems("ns1.example/192.0.2.80"), nil
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

	key := tctest.DNSKEYRR(apex, 8, tctest.PublicKey("AwEAAc=="))

	apexParam := &dns.NSEC3PARAM{Hdr: dns.Header{Name: apex, Class: dns.ClassINET, TTL: 60}}
	apexParam.Hash = 1

	offApexParam := &dns.NSEC3PARAM{Hdr: dns.Header{Name: offApex, Class: dns.ClassINET, TTL: 60}}
	offApexParam.Hash = 1

	tctest.NS(t, ctx, "ns1.example", "192.0.2.81", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			return dnskeyPacket(q.Name, key)
		case "NSEC":
			return tctest.Response(tctest.Question(q.Name, dns.TypeNSEC), tctest.Secure())
		case "NSEC3PARAM":
			return tctest.Response(tctest.Question(q.Name, dns.TypeNSEC3PARAM), tctest.Secure(), tctest.Answers(apexParam, offApexParam))
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return tctest.NSItems("ns1.example/192.0.2.81"), nil
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
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next." + apex)
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeRRSIG}
	return tctest.Response(tctest.Question(qname, dns.TypeNSEC), tctest.Secure(),
		tctest.Authority(soaRecord(apex), nsec))
}

// nsecInAnswerResponse builds a standard NSEC query response with the NSEC RR
// in the answer section (the conventional, non-RFC-4470 shape).
func nsecInAnswerResponse(qname string, apex string) packet.Packet {
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next." + apex)
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG}
	return tctest.Response(tctest.Question(qname, dns.TypeNSEC), tctest.Secure(),
		tctest.Answers(nsec))
}

// emptyNSEC3PARAMResponse builds a NODATA NSEC3PARAM response for an NSEC
// zone: NSEC in authority confirms NSEC3PARAM does not exist at the apex.
func emptyNSEC3PARAMResponse(qname string, apex string) packet.Packet {
	nsec := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(apex), Class: dns.ClassINET, TTL: 60}}
	nsec.NextDomain = dnsutil.Fqdn("next." + apex)
	nsec.TypeBitMap = []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG}
	return tctest.Response(tctest.Question(qname, dns.TypeNSEC3PARAM), tctest.Secure(),
		tctest.Authority(soaRecord(apex), nsec))
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
	key := tctest.DNSKEYRR(apex, 8, tctest.PublicKey("AwEAAc=="))

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
		return tctest.NSItems("ns1.example/192.0.2.90"), nil
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
	key := tctest.DNSKEYRR(apex, 8, tctest.PublicKey("AwEAAc=="))

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
		return tctest.NSItems("ns1.example/192.0.2.91"), nil
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
	key := tctest.DNSKEYRR(apex, 8, tctest.PublicKey("AwEAAc=="))

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
		return tctest.NSItems("ns1.example/192.0.2.92", "ns2.example/192.0.2.93"), nil
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2
		tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool { return false })

		ds := tctest.DSRR("example", 12345, 8, 2, "DEADBEEF")

		gate := tctest.NewGate()

		hook := func(id string, withDS bool) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if q.Type != "DS" {
					return packet.Packet{}
				}
				gate.Arrive(id)
				if withDS {
					return dsPacketFromDS(q.Name, ds)
				}
				return dsPacketFromDS(q.Name, nil)
			}
		}

		parent1 := tctest.NS(t, ctx, "ns-parent1.example", "192.0.2.80", hook("parent1", true))
		parent2 := tctest.NS(t, ctx, "ns-parent2.example", "192.0.2.81", hook("parent2", false))

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

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC11(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "parent1", "parent2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec11: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DS11_INCONSISTENT_DS", "DS11_PARENT_WITHOUT_DS", "DS11_PARENT_WITH_DS")
	})
}

func TestDNSSEC11ParallelChildQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2
		tctest.Stub(t, &hasFakeAddresses, func(_ *zone.Zone) bool { return false })

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))

		gate := tctest.NewGate()

		hook := func(id string, withDNSKEY bool) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "SOA":
					gate.Arrive(id)
					return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name))
				case "DNSKEY":
					if withDNSKEY {
						return dnskeyPacket(q.Name, key)
					}
					return dnskeyPacket(q.Name, nil)
				default:
					return packet.Packet{}
				}
			}
		}

		child1 := tctest.NS(t, ctx, "ns-child1.example", "192.0.2.90", hook("child1", true))
		child2 := tctest.NS(t, ctx, "ns-child2.example", "192.0.2.91", hook("child2", false))

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

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC11(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "child1", "child2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec11: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DS11_INCONSISTENT_SIGNED_ZONE", "DS11_NS_WITH_UNSIGNED_ZONE", "DS11_NS_WITH_SIGNED_ZONE")
	})
}

func TestDNSSEC11InconsistentDS(t *testing.T) {
	ctx := tctest.Context(t)

	ds := tctest.DSRR("example", 12345, 8, 1, "DEADBEEF")

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

	ds := tctest.DSRR("example", 54321, 8, 1, "FEEDBEEF")

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

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		now := time.Now().UTC()
		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
		keySig := rrsigRecord("example", dns.TypeDNSKEY, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())

		soaSig := rrsigRecord("example", dns.TypeSOA, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())
		soaSig.Algorithm = 13
		nsSig := rrsigRecord("example", dns.TypeNS, key.KeyTag(), now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix())
		nsSig.Algorithm = 13

		nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
		nsRR.Ns = dnsutil.Fqdn("ns.example")

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "DNSKEY":
					gate.Arrive(id)
					return answerPacket(q.Name, dns.TypeDNSKEY, key, keySig)
				case "SOA":
					return answerPacket(q.Name, dns.TypeSOA, soaRecord(q.Name), soaSig)
				case "NS":
					return answerPacket(q.Name, dns.TypeNS, nsRR, nsSig)
				default:
					return packet.Packet{}
				}
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.121", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.122", hook("ns2"))

		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1, ns2}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC13(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec13: %v", dsErr)
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
	})
}

func TestDNSSEC14KeySizeSmallerThanRec(t *testing.T) {
	ctx := tctest.Context(t)

	key := tctest.DNSKEYRR("example", 8)
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

// A public exponent above 2^31-1 is reported; the usual 65537 is not.
func TestDNSSEC14RSAExponentLarge(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  *dns.DNSKEY
		want bool
	}{
		{"lv KSK 2^32+1", lvLargeExponentKSK("example"), true},
		{"65537", tctest.DNSKEYRR("example", 8, tctest.PublicKey(dnstest.LBKSK3842)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			ns := tctest.NS(t, ctx, "ns1.example", "198.51.100.9", func(q tctest.Query) packet.Packet {
				if q.Type == "DNSKEY" {
					return dnskeyPacket(q.Name, tc.key)
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
			if !tc.want {
				tctest.RequireNoTag(t, entries, "DNSKEY_RSA_EXPONENT_LARGE")
				return
			}
			tctest.RequireTags(t, entries, "DNSKEY_RSA_EXPONENT_LARGE")
			if got := tctest.First(entries, "DNSKEY_RSA_EXPONENT_LARGE").Args["exponent_bits"]; got != 33 {
				t.Errorf("exponent_bits = %v, want 33", got)
			}
		})
	}
}

func TestDNSSEC14ParallelDNSKEYQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		key := tctest.DNSKEYRR("example", 8)
		if _, err := key.Generate(1024); err != nil {
			t.Fatalf("generate key: %v", err)
		}

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if q.Type != "DNSKEY" {
					return packet.Packet{}
				}
				gate.Arrive(id)
				return dnskeyPacket(q.Name, key)
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.131", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.132", hook("ns2"))

		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1, ns2}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC14(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec14: %v", dsErr)
		}

		tctest.RequireTags(t, entries, "DNSKEY_SMALLER_THAN_REC")
	})
}

func TestDNSSEC14ArgsSplit(t *testing.T) {
	cases := []struct {
		name         string
		ip           string
		handler      tctest.Handler
		ipv4Disabled bool
		wantTag      string
	}{
		{name: "NoResponse", ip: "192.0.2.141", wantTag: "NO_RESPONSE"},
		{
			name: "NoResponseDNSKEY",
			ip:   "192.0.2.142",
			handler: func(q tctest.Query) packet.Packet {
				if q.Type == "DNSKEY" {
					return answerPacket(q.Name, dns.TypeDNSKEY)
				}
				return packet.Packet{}
			},
			wantTag: "NO_RESPONSE_DNSKEY",
		},
		{name: "IPv4Disabled", ip: "192.0.2.143", ipv4Disabled: true, wantTag: "IPV4_DISABLED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)

			if tc.ipv4Disabled {
				profile.Effective().Net.IPv4 = false
			}

			ns := tctest.NS(t, ctx, "ns1.example", tc.ip, tc.handler)
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
			entry := tctest.RequireTag(t, entries, tc.wantTag)
			tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns1.example", Address: tc.ip})
		})
	}
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
		cds.KeyTag = 12345
		cds.Algorithm = 8
		cds.DigestType = 2
		cds.Digest = "DEADBEEF"

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "CDS":
					gate.Arrive(id)
					return answerPacket(q.Name, dns.TypeCDS, cds)
				case "CDNSKEY":
					return answerPacket(q.Name, dns.TypeCDNSKEY)
				default:
					return packet.Packet{}
				}
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.231", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.232", hook("ns2"))

		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1, ns2}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC15(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec15: %v", dsErr)
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
	})
}

// RFC 9975 digest-type filter: a CDS whose digest type is not MUST in IANA's
// "Implement for DNSSEC Delegation" column must not join the cross-server
// consistency check. NS1 publishes SHA-256 (MUST) and SHA-1 (MUST NOT), NS2
// only SHA-256, so the raw RRsets differ but the MUST-only views match and
// DS15_INCONSISTENT_CDS must not fire.
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		cds := &dns.CDS{DS: dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
		cds.KeyTag = 12345
		cds.Algorithm = 8
		cds.DigestType = 2
		cds.Digest = "DEADBEEF"

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "CDS":
					gate.Arrive(id)
					return answerPacket(q.Name, dns.TypeCDS, cds)
				case "DNSKEY":
					return answerPacket(q.Name, dns.TypeDNSKEY)
				default:
					return packet.Packet{}
				}
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.241", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.242", hook("ns2"))

		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1, ns2}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC16(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec16: %v", dsErr)
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
	})
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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		cdnskey := &dns.CDNSKEY{DNSKEY: dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}}
		cdnskey.Flags = dns.FlagZONE | dns.FlagSEP
		cdnskey.Protocol = 3
		cdnskey.Algorithm = 8
		cdnskey.PublicKey = "AwEAAc=="

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "CDNSKEY":
					gate.Arrive(id)
					return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey)
				case "DNSKEY":
					return answerPacket(q.Name, dns.TypeDNSKEY)
				default:
					return packet.Packet{}
				}
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.243", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.244", hook("ns2"))

		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1, ns2}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC17(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec17: %v", dsErr)
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
	})
}

func TestDNSSEC18NoMatchRRSIGDS(t *testing.T) {
	ctx := tctest.Context(t)

	key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
	keytag := key.KeyTag()

	ds := tctest.DSRR("example", keytag, 8, 1, "DEADBEEF")

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
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
		keytag := key.KeyTag()

		ds := tctest.DSRR("example", keytag, 8, 1, "DEADBEEF")

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

		gate := tctest.NewGate()

		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "CDS":
					gate.Arrive(id)
					return answerPacket(q.Name, dns.TypeCDS, cds, cdsSig)
				case "CDNSKEY":
					return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey, cdnskeySig)
				case "DNSKEY":
					return dnskeyPacket(q.Name, key)
				default:
					return packet.Packet{}
				}
			}
		}

		child1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.251", hook("ns1"))
		child2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.252", hook("ns2"))

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

		done := make(chan struct{})
		var entries []*logger.Entry
		var dsErr error
		go func() {
			entries, dsErr = DNSSEC18(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if dsErr != nil {
			t.Fatalf("dnssec18: %v", dsErr)
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
	})
}

func TestDNSSEC18ParallelOutputStable(t *testing.T) {

	runDNSSEC18 := func(parallel int) []*logger.Entry {
		profile.ResetEffective()
		if err := profile.Effective().Set("resolver.defaults.parallel", parallel); err != nil {
			t.Fatalf("set parallel: %v", err)
		}

		ctx := tctest.Context(t)

		key := tctest.DNSKEYRR("example", 8, tctest.PublicKey("AwEAAc=="))
		keytag := key.KeyTag()

		ds := tctest.DSRR("example", keytag, 8, 1, "DEADBEEF")

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
	k := tctest.DNSKEYRR(owner, 8, tctest.SEP(), tctest.PublicKey(pubKey))
	return k
}

func TestDNSSEC18CDSMatchesDS(t *testing.T) {
	ctx := tctest.Context(t)

	key := makeSEPKey("example", "AwEAAc==")
	keytag := key.KeyTag()

	// DS and CDS have identical (KeyTag,Algorithm,DigestType,Digest) tuples.
	ds := tctest.DSRR("example", keytag, 8, 2, "DEADBEEF")

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

	ds := tctest.DSRR("example", keytag, 8, 2, "AAAABBBB")

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

func TestDNSSEC18RolloverEvidence(t *testing.T) {
	// keyOld carries the DS at the parent; keyNew is the incoming SEP. Each case
	// publishes a different DNSKEY RRset and RRSIG set over the same pair.
	keyOld := makeSEPKey("example", "AwEAAc==")
	keyNew := makeSEPKey("example", "AwEAAb0=")
	keytagOld, keytagNew := keyOld.KeyTag(), keyNew.KeyTag()

	sigOld := rrsigRecord("example", dns.TypeDNSKEY, keytagOld, 1, 2)
	sigNew := rrsigRecord("example", dns.TypeDNSKEY, keytagNew, 1, 2)

	// want is one expected entry: the tag, its keytags count, and the exact
	// keytags where the case pins them.
	type want struct {
		tag     string
		count   int
		keytags []uint16
	}

	cases := []struct {
		name     string
		parentIP string
		childIP  string
		dnskeys  []dns.RR
		wants    []want
	}{
		{
			// Both keys published, only signed by the old one.
			name:     "MultiKSK",
			parentIP: "192.0.2.78",
			childIP:  "192.0.2.79",
			dnskeys:  []dns.RR{keyOld, keyNew, sigOld},
			wants: []want{
				{tag: "DS18_ROLLOVER_EVIDENCE_MULTI_KSK", count: 2},
				// keyNew has no DS, so DNSKEY_WITHOUT_DS also fires.
				{tag: "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS", count: 1, keytags: []uint16{keytagNew}},
			},
		},
		{
			// Both keys sign the DNSKEY RRset: classic double-signature phase.
			name:     "DoubleSig",
			parentIP: "192.0.2.80",
			childIP:  "192.0.2.81",
			dnskeys:  []dns.RR{keyOld, keyNew, sigOld, sigNew},
			wants:    []want{{tag: "DS18_ROLLOVER_EVIDENCE_DOUBLE_SIG", count: 2}},
		},
		{
			// Parent still has the DS but the old key left the DNSKEY RRset.
			name:     "DSWithoutDNSKEY",
			parentIP: "192.0.2.82",
			childIP:  "192.0.2.83",
			dnskeys:  []dns.RR{keyNew, sigNew},
			wants:    []want{{tag: "DS18_ROLLOVER_EVIDENCE_DS_WITHOUT_DNSKEY", count: 1, keytags: []uint16{keytagOld}}},
		},
		{
			// Both keys published, DS only for the old one.
			name:     "DNSKEYWithoutDS",
			parentIP: "192.0.2.84",
			childIP:  "192.0.2.85",
			dnskeys:  []dns.RR{keyOld, keyNew, sigOld},
			wants:    []want{{tag: "DS18_ROLLOVER_EVIDENCE_DNSKEY_WITHOUT_DS", count: 1, keytags: []uint16{keytagNew}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)

			ds := tctest.DSRR("example", keytagOld, 8, 2, "AABB")

			parentNS := tctest.NS(t, ctx, "pns1.example", tc.parentIP, func(q tctest.Query) packet.Packet {
				if q.Type == "DS" {
					return dsPacketFromDS("example", ds)
				}
				return packet.Packet{}
			})
			childNS := tctest.NS(t, ctx, "ns1.example", tc.childIP, func(q tctest.Query) packet.Packet {
				switch q.Type {
				case "CDS":
					return answerPacket(q.Name, dns.TypeCDS)
				case "CDNSKEY":
					return answerPacket(q.Name, dns.TypeCDNSKEY)
				case "DNSKEY":
					return answerPacket(q.Name, dns.TypeDNSKEY, tc.dnskeys...)
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
			for _, w := range tc.wants {
				e := tctest.RequireTag(t, entries, w.tag)
				kts, ok := e.Args["keytags"].([]uint16)
				if !ok || len(kts) != w.count {
					t.Errorf("expected %s keytags with %d entries, got %v", w.tag, w.count, e.Args["keytags"])
					continue
				}
				if w.keytags != nil && !slices.Equal(kts, w.keytags) {
					t.Errorf("expected %s keytags=%v, got %v", w.tag, w.keytags, kts)
				}
			}
		})
	}
}

func TestDNSSEC18NoCDSCDNSKEYButRolloverEvidence(t *testing.T) {
	ctx := tctest.Context(t)

	// No CDS/CDNSKEY published but multi-KSK is visible: on-demand publication model.
	key1 := makeSEPKey("example", "AwEAAc==")
	keytag1 := key1.KeyTag()
	key2 := makeSEPKey("example", "AwEAAb0=")

	ds := tctest.DSRR("example", keytag1, 8, 2, "AABB")

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

	ds := tctest.DSRR("example", keytag, 8, 2, "DEADBEEF")

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

	ds := tctest.DSRR("example", keytagOld, 8, 2, "AABB")

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

	ds1 := tctest.DSRR("example", keytag1, 8, 2, "AABB")

	ds2 := tctest.DSRR("example", keytag2, 8, 2, "CCDD")

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
	return dsPacketFromDS(owner, tctest.DSRR(owner, keytag, algo, digestType, "DEADBEEF"))
}

func dsPacketFromDS(owner string, ds *dns.DS) packet.Packet {
	opts := []tctest.MsgOpt{tctest.Question(owner, dns.TypeDS), tctest.Secure()}
	if ds != nil {
		opts = append(opts, tctest.Answers(ds))
	}
	return tctest.Response(opts...)
}

func dnskeyPacket(owner string, key *dns.DNSKEY) packet.Packet {
	opts := []tctest.MsgOpt{tctest.Question(owner, dns.TypeDNSKEY), tctest.Secure()}
	if key != nil {
		// Clone so each caller gets its own copy; DNSKEY.KeyTag() lazily
		// writes a cached field, which races with Len() when callers share
		// the same pointer across goroutines.
		keyCopy := *key
		opts = append(opts, tctest.Answers(&keyCopy))
	}
	return tctest.Response(opts...)
}

// nsecPacket is the one NSEC reply built without the DNSSEC response bits.
func nsecPacket(owner string) packet.Packet {
	return tctest.Response(tctest.Question(owner, dns.TypeNSEC))
}

func nsec3Packet(owner string, nsec3 *dns.NSEC3) packet.Packet {
	opts := []tctest.MsgOpt{tctest.Question(owner, dns.TypeNSEC), tctest.Secure()}
	if nsec3 != nil {
		opts = append(opts, tctest.Authority(nsec3))
	}
	return tctest.Response(opts...)
}

func rrsigRecord(owner string, typeCovered uint16, keytag uint16, inception int64, expiration int64) *dns.RRSIG {
	return tctest.RRSIGRR(owner, typeCovered, tctest.SigAlgo(dns.RSASHA256),
		tctest.KeyTag(keytag), tctest.Inception(time.Unix(inception, 0)),
		tctest.Expiration(time.Unix(expiration, 0)))
}

func answerPacket(owner string, qtype uint16, answers ...dns.RR) packet.Packet {
	return tctest.Response(tctest.Question(owner, qtype), tctest.Secure(),
		tctest.Answers(answers...))
}

func soaRecord(owner string) *dns.SOA {
	return tctest.SOARR(owner, tctest.MName("ns1.example"),
		tctest.RName("hostmaster.example"), tctest.SOATimers(60, 60, 60, 60))
}

func dnssec19P256Key(owner string) *dns.DNSKEY {
	key := tctest.DNSKEYRR(owner, dns.ECDSAP256SHA256, tctest.PublicKey("GojIhhXUN/u4v54ZQqGSnyhWJwaubCvTmeexv7bR6edbkrSqQpF64cYbcB7wNcP+e+MAnLr+Wi9xMWyQLc8NAA=="))
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

	key := tctest.DNSKEYRR(parentName, dns.RSASHA256, tctest.SEP(), tctest.KeyTTL(3600))
	priv, err := key.Generate(1024)
	if err != nil {
		t.Fatalf("generate parent key: %v", err)
	}

	childKey := tctest.DNSKEYRR(childName, dns.RSASHA256, tctest.SEP(), tctest.KeyTTL(3600), tctest.PublicKey("AwEAAc=="))
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
	return f.signedDSResponseForcedKeytag(t, sigKey, sigPriv, 0)
}

func (f dnssec21Fixture) unsignedDSResponse() packet.Packet {
	ds := *f.childDS
	return tctest.Response(tctest.Question(f.childName, dns.TypeDS), tctest.Secure(),
		tctest.Answers(&ds))
}

func (f dnssec21Fixture) parentDNSKEYResponse(keys ...*dns.DNSKEY) packet.Packet {
	answers := make([]dns.RR, 0, len(keys))
	for _, k := range keys {
		copyKey := *k
		answers = append(answers, &copyKey)
	}
	return tctest.Response(tctest.Question(f.parentName, dns.TypeDNSKEY), tctest.Secure(),
		tctest.Answers(answers...))
}

func (f dnssec21Fixture) emptyDNSKEYResponse() packet.Packet {
	return tctest.Response(tctest.Question(f.parentName, dns.TypeDNSKEY), tctest.Secure())
}

func (f dnssec21Fixture) emptyDSResponse() packet.Packet {
	return tctest.Response(tctest.Question(f.childName, dns.TypeDS), tctest.Secure())
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
	otherKey := tctest.DNSKEYRR(f.parentName, dns.RSASHA256, tctest.SEP(), tctest.KeyTTL(3600))
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

// A non-zero forcedKeytag replaces the key tag after signing, which models a DS
// RRSIG pointing at a parent key that did not sign it.
func (f dnssec21Fixture) signedDSResponseForcedKeytag(t *testing.T, sigKey *dns.DNSKEY, sigPriv crypto.PrivateKey, forcedKeytag uint16) packet.Packet {
	t.Helper()
	signer, ok := sigPriv.(crypto.Signer)
	if !ok {
		t.Fatalf("private key does not implement crypto.Signer")
	}
	ds := *f.childDS
	sig := tctest.Sign(t, sigKey, signer, dns.TypeDS, []dns.RR{&ds}, tctest.Signer(f.parentName))
	if forcedKeytag != 0 {
		sig.KeyTag = forcedKeytag
	}
	return tctest.Response(tctest.Question(f.childName, dns.TypeDS), tctest.Secure(),
		tctest.Answers(&ds, sig))
}

// The apex NSEC3 is owned by the hash of the apex name, not by the apex name
// itself, so the match is a hash comparison against the first owner label.
func TestNSEC3OwnerMatchesApex(t *testing.T) {
	const apex = "example.com"
	apexName := dnsname.New(apex)

	nsec3 := func(owner string, salt string, iterations uint16) *dns.NSEC3 {
		rr := &dns.NSEC3{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
		rr.Hash = 1
		rr.Iterations = iterations
		rr.Salt = salt
		return rr
	}

	hashed := dnsutil.NSEC3Name(apexName.FQDN(), "", 0)
	if hashed == "" {
		t.Fatal("could not hash the apex name")
	}

	if !nsec3OwnerMatchesApex(nsec3(hashed+"."+apex, "", 0), apexName) {
		t.Error("the apex hash owner did not match")
	}
	// The comparison is case-insensitive on the base32 label.
	if !nsec3OwnerMatchesApex(nsec3(strings.ToUpper(hashed)+"."+apex, "", 0), apexName) {
		t.Error("an upper-case owner label did not match")
	}
	if nsec3OwnerMatchesApex(nil, apexName) {
		t.Error("a nil record matched")
	}
	if nsec3OwnerMatchesApex(nsec3(apex, "", 0), apexName) {
		t.Error("an unhashed owner matched")
	}
	// Salt and iterations feed the hash, so a record using different parameters
	// owns a different name.
	if nsec3OwnerMatchesApex(nsec3(hashed+"."+apex, "AABB", 0), apexName) {
		t.Error("a record with a different salt matched")
	}
	if nsec3OwnerMatchesApex(nsec3(hashed+"."+apex, "", 5), apexName) {
		t.Error("a record with a different iteration count matched")
	}
}
