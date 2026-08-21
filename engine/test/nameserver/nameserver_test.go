package nameserver

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"unicode/utf8"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	ens "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestNameserver01RecursorAndNoRecursor(t *testing.T) {
	ctx := tctest.Context(t)

	// A genuine open recursor advertises recursion available (RA=1); when
	// asked for a name outside any zone it serves it recurses and returns a
	// non-authoritative NXDOMAIN. The RA bit is what separates it from an
	// authoritative-only server that also answers NXDOMAIN (see
	// TestNameserver01NxdomainWithoutRANotRecursor).
	nsRecursor := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		msg.RecursionAvailable = true
		return packet.Packet{Msg: msg}
	})
	nsNoRecursor := tctest.NS(t, ctx, "ns2.example", "192.0.2.2", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsRecursor, nsNoRecursor}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	tctest.RequireTags(t, entries, "IS_A_RECURSOR", "NO_RECURSOR")
}

func TestNameserver01NxdomainWithAANotRecursor(t *testing.T) {
	ctx := tctest.Context(t)

	// Server returns NXDOMAIN with AA=1 on all probes (fake root authority).
	// This should NOT be classified as a recursor.
	nsFakeRoot := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		msg.Authoritative = true
		return packet.Packet{Msg: msg}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsFakeRoot}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	tctest.RequireNoTag(t, entries, "IS_A_RECURSOR")
	tctest.RequireTags(t, entries, "NO_RECURSOR")
}

func TestNameserver01NxdomainWithoutRANotRecursor(t *testing.T) {
	ctx := tctest.Context(t)

	// Regression for the last.org false positive: a non-authoritative NXDOMAIN
	// with RA=0 proves the server is not recursing, so it must be NO_RECURSOR.
	nsCloudflare := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		return packet.Packet{Msg: msg}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsCloudflare}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	tctest.RequireNoTag(t, entries, "IS_A_RECURSOR")
	tctest.RequireTags(t, entries, "NO_RECURSOR")
}

func TestNameserver01RAWithAnswerIsRecursor(t *testing.T) {
	ctx := tctest.Context(t)

	// Server returns NOERROR with RA=1 and a real ANSWER record for the
	// out-of-bailiwick probe. That is recursion: the server resolved a name
	// it has no authority over and returned data.
	nsRAAnswer := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.RecursionAvailable = true
		a := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Type), Class: dns.ClassINET, TTL: 60}}
		a.Addr = netip.MustParseAddr("203.0.113.1")
		msg.Answer = []dns.RR{a}
		return packet.Packet{Msg: msg}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsRAAnswer}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	tctest.RequireTags(t, entries, "IS_A_RECURSOR")
}

func TestNameserver01RAReferralIsNotRecursor(t *testing.T) {
	ctx := tctest.Context(t)

	// Authoritative-only server that returns NOERROR + RA=1 + empty
	// ANSWER + non-empty AUTHORITY (a referral). This is the leaked-RA
	// pattern observed against ns1.kptc.kp in the cohort: a referral,
	// not recursion. The new rule must NOT flag this as a recursor.
	nsRAReferral := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.RecursionAvailable = true
		_ = q.Type
		nsRR := &dns.NS{Hdr: dns.Header{Name: ".", Class: dns.ClassINET, TTL: 3600}}
		nsRR.Ns = "a.root-servers.net."
		msg.Ns = []dns.RR{nsRR}
		return packet.Packet{Msg: msg}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsRAReferral}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	tctest.RequireNoTag(t, entries, "IS_A_RECURSOR")
	tctest.RequireTags(t, entries, "NO_RECURSOR")
}

func TestNameserver01RAOnSomeAnswerIsRecursor(t *testing.T) {
	ctx := tctest.Context(t)

	// One probe gets a recursive answer (RA=1 + real ANSWER record); the
	// other two get authoritative-style NXDOMAIN responses. A single
	// recursive response is enough to classify the server as a recursor.
	queries := 0
	nsMixed := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		queries++
		msg := new(dns.Msg)
		if queries == 1 {
			msg.Rcode = dns.RcodeSuccess
			msg.RecursionAvailable = true
			a := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Type), Class: dns.ClassINET, TTL: 60}}
			a.Addr = netip.MustParseAddr("203.0.113.1")
			msg.Answer = []dns.RR{a}
			return packet.Packet{Msg: msg}
		}
		msg.Rcode = dns.RcodeNameError
		msg.Authoritative = true
		return packet.Packet{Msg: msg}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsMixed}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	tctest.RequireTags(t, entries, "IS_A_RECURSOR")
}

func TestNameserver01NxdomainMixedAAIsRecursor(t *testing.T) {
	ctx := tctest.Context(t)

	// A recursor (RA=1) returns NXDOMAIN on all probes, but only some have
	// AA=1. Since not ALL NXDOMAIN responses are authoritative and recursion
	// is available, classify as recursor.
	queryCount := 0
	nsMixed := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeNameError
		msg.RecursionAvailable = true
		queryCount++
		if queryCount <= 2 {
			msg.Authoritative = true
		}
		return packet.Packet{Msg: msg}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{nsMixed}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver01(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver01: %v", err)
	}
	tctest.RequireTags(t, entries, "IS_A_RECURSOR")
}

func TestNameserver01ParallelQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		gate := tctest.NewGate()
		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if strings.EqualFold(q.Type, "A") && strings.EqualFold(q.Name, nonExistentNames[0]) {
					gate.Arrive(id)
				}
				msg := new(dns.Msg)
				msg.Rcode = dns.RcodeNameError
				// RA=1 so both servers classify as recursors; this test
				// exercises parallel fan-out and the consolidated entry.
				msg.RecursionAvailable = true
				return packet.Packet{Msg: msg}
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.2", hook("ns2"))

		tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
			return []ens.Nameserver{ns1, ns2}, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var nsErr error
		go func() {
			entries, nsErr = Nameserver01(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if nsErr != nil {
			t.Fatalf("nameserver01: %v", nsErr)
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
	})
}

func TestNameserver02EDNS0Support(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.3", func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Type, "SOA") {
			return soaPacketWithEdns("example", 0, 0, nil)
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver02(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver02: %v", err)
	}
	tctest.RequireTags(t, entries, "EDNS0_SUPPORT")
}

func TestNameserver03AXFRAvailable(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.4", nil)
	soaRR := soaRecord("example")
	ns1.SetAXFRHook(func(_ context.Context, _ string, callback func(dns.RR) bool, _ string) error {
		if callback != nil {
			callback(soaRR)
		}
		return nil
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver03(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver03: %v", err)
	}
	tctest.RequireTags(t, entries, "AXFR_AVAILABLE")
}

// axfrRestoreContext builds a context with a controllable AXFR cache and a
// no-network profile so a restored transfer must be replayed from cache.
func axfrRestoreContext(t *testing.T, store *ens.CacheStore) context.Context {
	t.Helper()
	tctest.Setup(t)
	prof := dnstest.DefaultProfile(t)
	prof.NoNetwork = true
	ctx := ens.WithCache(context.Background(), store)
	return profile.WithContext(ctx, prof)
}

func packSOAWire(t *testing.T, owner string) []byte {
	t.Helper()
	m := new(dns.Msg)
	m.Answer = []dns.RR{soaRecord(owner)}
	if err := m.Pack(); err != nil {
		t.Fatalf("pack soa: %v", err)
	}
	return append([]byte(nil), m.Data...)
}

func TestNameserver03AXFRRestoredAvailable(t *testing.T) {
	store := ens.NewCacheStore()
	if err := store.ImportAXFREntries([]ens.AXFREntry{
		{Address: "192.0.2.4", Name: "example", QClass: "IN", RRs: [][]byte{packSOAWire(t, "example")}},
	}); err != nil {
		t.Fatalf("import axfr: %v", err)
	}
	ctx := axfrRestoreContext(t, store)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.4", nil) // no AXFR hook
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver03(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver03: %v", err)
	}
	tctest.RequireTags(t, entries, "AXFR_AVAILABLE")
	tctest.RequireNoTag(t, entries, "AXFR_FAILURE")
}

func TestNameserver03AXFRRestoredFailure(t *testing.T) {
	store := ens.NewCacheStore()
	if err := store.ImportAXFREntries([]ens.AXFREntry{
		{Address: "192.0.2.4", Name: "example", QClass: "IN", NoTransfer: true},
	}); err != nil {
		t.Fatalf("import axfr: %v", err)
	}
	ctx := axfrRestoreContext(t, store)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.4", nil)
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver03(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver03: %v", err)
	}
	tctest.RequireTags(t, entries, "AXFR_FAILURE")
	tctest.RequireNoTag(t, entries, "AXFR_AVAILABLE")
}

func TestNameserver04DifferentSourceIP(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.5", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg, AnswerFrom: "192.0.2.99:53"}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver04(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver04: %v", err)
	}
	tctest.RequireTags(t, entries, "DIFFERENT_SOURCE_IP")
}

func TestNameserver05AAAAWellProcessed(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.6", func(q tctest.Query) packet.Packet {
		switch strings.ToUpper(q.Type) {
		case "A":
			return aPacket(q.Name, "192.0.2.10")
		case "AAAA":
			return aaaaPacket(q.Name, "2001:db8::1")
		}
		return packet.Packet{}
	})

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver05(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver05: %v", err)
	}
	tctest.RequireTags(t, entries, "AAAA_WELL_PROCESSED")
}

func TestNameserver05ParallelQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		gate := tctest.NewGate()
		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if strings.EqualFold(q.Type, "A") && strings.EqualFold(q.Name, "example") {
					gate.Arrive(id)
				}
				return packet.Packet{}
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.2", hook("ns2"))

		tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
			return []ens.Nameserver{ns1, ns2}, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var nsErr error
		go func() {
			entries, nsErr = Nameserver05(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if nsErr != nil {
			t.Fatalf("nameserver05: %v", nsErr)
		}

		var order []string
		var addresses []string
		for _, entry := range tctest.All(entries, "NO_RESPONSE") {
			tctest.RequireArgShape(t, entry, tctest.ArgShape{})
			if ns, ok := entry.Args["ns"].(string); ok {
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
	})
}

func TestNameserver06NotResolved(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	})
	tctest.Stub(t, &apexNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns2.example")}, nil
	})
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.7", nil)
	tctest.Stub(t, &allNameservers, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver06(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver06: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "CAN_NOT_BE_RESOLVED")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns2.example" {
		t.Fatalf("expected typed unresolved nameserver list, got %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
}

func TestNameserver07NoUpwardReferral(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.8", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver07(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver07: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "NO_UPWARD_REFERRAL")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("expected typed nameserver list for NO_UPWARD_REFERRAL, got %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
}

func TestNameserver08QNameCaseInsensitive(t *testing.T) {
	ctx := tctest.Context(t)

	fixedRand := rand.New(rand.NewSource(42))
	tctest.Stub(t, &scrambleCaseFunc, func(s string) string { return util.ScrambleCaseWith(s, fixedRand) })

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.9", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(strings.ToLower(q.Name)), dns.TypeSOA)
		return packet.Packet{Msg: msg}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver08(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver08: %v", err)
	}
	tctest.RequireTags(t, entries, "QNAME_CASE_INSENSITIVE")
}

func TestNameserver08DoesNotReuseDifferentCaseCachedPacket(t *testing.T) {
	ctx := tctest.Context(t)

	var calls int
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.98", func(q tctest.Query) packet.Packet {
		calls++
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(q.Name), dns.StringToType[q.Type])
		msg.Authoritative = true
		msg.Rcode = dns.RcodeSuccess
		cname := &dns.CNAME{Hdr: dns.Header{Name: dnsutil.Fqdn(q.Name), Class: dns.ClassINET, TTL: 3600}}
		cname.Target = "alias.example."
		msg.Answer = []dns.RR{cname}
		return packet.Packet{Msg: msg}
	})

	if _, err := ns1.QueryWithOptions(ctx, "www.example", "SOA", nil); err != nil {
		t.Fatalf("prime lower-case cache entry: %v", err)
	}

	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})
	tctest.Stub(t, &scrambleCaseFunc, func(string) string { return "wWw.eXaMpLe" })

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver08(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver08: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected mixed-case query to bypass differently cased cached packet, got %d network calls", calls)
	}
	tctest.RequireTags(t, entries, "QNAME_CASE_SENSITIVE")
	tctest.RequireNoTag(t, entries, "QNAME_CASE_INSENSITIVE")
}

func TestNameserver09CaseQueriesSameAnswer(t *testing.T) {
	ctx := tctest.Context(t)

	fixedRand := rand.New(rand.NewSource(42))
	tctest.Stub(t, &scrambleCaseFunc, func(s string) string { return util.ScrambleCaseWith(s, fixedRand) })

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.10", func(q tctest.Query) packet.Packet {
		return soaPacket("example")
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver09(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver09: %v", err)
	}
	tctest.RequireTags(t, entries, "CASE_QUERY_SAME_ANSWER", "CASE_QUERIES_RESULTS_OK")
}

func TestNameserver10NoResponseEDNS1(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.11", func(q tctest.Query) packet.Packet {
		if q.Opts != nil && q.Opts.EDNSDetails != nil && q.Opts.EDNSDetails.Version != nil {
			if *q.Opts.EDNSDetails.Version == 0 {
				return soaPacket("example")
			}
			if *q.Opts.EDNSDetails.Version == 1 {
				return packet.Packet{}
			}
		}
		return packet.Packet{}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver10(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver10: %v", err)
	}
	tctest.RequireTags(t, entries, "N10_NO_RESPONSE_EDNS1_QUERY")

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
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.12", func(q tctest.Query) packet.Packet {
		if q.Opts != nil && q.Opts.EDNSDetails != nil && len(q.Opts.EDNSDetails.Data) > 0 {
			return soaPacketWithEdns("example", 0, 0, []dns.EDNS0{&dns.ERFC3597{EDNS0Code: 137}})
		}
		return soaPacketWithEdns("example", 0, 0, nil)
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver11(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver11: %v", err)
	}
	tctest.RequireTags(t, entries, "N11_RETURNS_UNKNOWN_OPTION_CODE")

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
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.13", func(q tctest.Query) packet.Packet {
		return soaPacketWithEdns("example", 0, 3, nil)
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver12(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver12: %v", err)
	}
	tctest.RequireTags(t, entries, "Z_FLAGS_NOTCLEAR")
}

func TestNameserver13MissingOptInTruncated(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.14", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Truncated = true
		return packet.Packet{Msg: msg}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver13(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver13: %v", err)
	}
	tctest.RequireTags(t, entries, "MISSING_OPT_IN_TRUNCATED")
}

func TestNameserver13NoEdnsSupport(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.14", func(q tctest.Query) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeFormatError
		// No EDNS OPT record in response
		return packet.Packet{Msg: msg}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver13(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver13: %v", err)
	}
	tctest.RequireTags(t, entries, "NO_EDNS_SUPPORT")
}

func TestNameserver15SoftwareVersionAndWrongClass(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.15", func(q tctest.Query) packet.Packet {
		switch strings.ToUpper(q.Type) {
		case "SOA":
			return soaPacket("example")
		case "TXT":
			if strings.EqualFold(q.Name, "version.bind") {
				return txtPacket("version.bind", "bind 9", dns.ClassINET)
			}
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver15(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver15: %v", err)
	}
	tctest.RequireTags(t, entries, "N15_SOFTWARE_VERSION", "N15_WRONG_CLASS")

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

func soaRecord(owner string) dns.RR {
	return tctest.SOARR(owner, tctest.MName("ns1.example"), tctest.RName("hostmaster.example"))
}

func soaMsg(owner string) *dns.Msg {
	return tctest.Response(tctest.Answers(soaRecord(owner))).Msg
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

// The address and TXT answers model recursor replies, so AA stays clear.
func aPacket(name string, address string) packet.Packet {
	return tctest.Response(tctest.NotAuthoritative(), tctest.Answers(tctest.ARR(name, address)))
}

func aaaaPacket(name string, address string) packet.Packet {
	return tctest.Response(tctest.NotAuthoritative(), tctest.Answers(tctest.AAAARR(name, address)))
}

func txtPacket(name string, value string, class uint16) packet.Packet {
	return tctest.Response(tctest.NotAuthoritative(),
		tctest.Answers(tctest.Class(class, tctest.TXTRR(name, value))...))
}

func TestNameserver16HasNSID(t *testing.T) {
	ctx := tctest.Context(t)

	nsidValue := "ns1.example"
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.16", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) == "SOA" {
			return soaPacketWithEdns("example", 0, 0, []dns.EDNS0{
				&dns.NSID{Nsid: fmt.Sprintf("%x", nsidValue)},
			})
		}
		return packet.Packet{}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver16(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver16: %v", err)
	}
	tctest.RequireTags(t, entries, "N16_HAS_NSID")
	tctest.RequireNoTag(t, entries, "N16_NO_NSID_REVEALED")

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
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.16", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) == "SOA" {
			return soaPacket("example")
		}
		return packet.Packet{}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver16(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver16: %v", err)
	}
	tctest.RequireNoTag(t, entries, "N16_HAS_NSID")
	tctest.RequireTags(t, entries, "N16_NO_NSID_REVEALED")

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

// binaryNSID is the raw 16-byte NSID from a real dig capture of a server that
// returns opaque binary (e.g. Google Cloud DNS). Most bytes are not printable
// ASCII, so the old string(decoded) path lost them to U+FFFD on JSON marshal.
var binaryNSID = []byte{
	0xa7, 0x62, 0xa8, 0xce, 0x20, 0x22, 0xe8, 0xc1,
	0x01, 0xb7, 0xed, 0x73, 0xc2, 0x63, 0x0c, 0x51,
}

// binaryNSIDDigForm is dig's presentation of binaryNSID: space-separated hex
// bytes followed by the printable-ASCII rendering ('.' for non-printable).
const binaryNSIDDigForm = `a7 62 a8 ce 20 22 e8 c1 01 b7 ed 73 c2 63 0c 51 (".b.. ".....s.c.Q")`

func TestNSIDValue(t *testing.T) {
	cases := []struct {
		name   string
		raw    []byte
		want   string
		wantOK bool
	}{
		{"printable ascii label", []byte("gpdns-fra"), "gpdns-fra", true},
		{"printable utf8 label", []byte("café-1"), "café-1", true},
		{"trailing whitespace trimmed", []byte("ns1.example  \n"), "ns1.example", true},
		{"binary opaque", binaryNSID, binaryNSIDDigForm, true},
		{"single invalid utf8 byte", []byte{0xff}, `ff (".")`, true},
		{"embedded control char", []byte("ns1\x01"), `6e 73 31 01 ("ns1.")`, true},
		{"empty", []byte{}, "", false},
		{"whitespace only", []byte("   "), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := nsidValue(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("value = %q, want %q", got, tc.want)
			}
			// The whole point of the fix: output is always valid UTF-8, so no
			// byte is silently replaced by U+FFFD when the run is marshaled.
			if !utf8.ValidString(got) {
				t.Fatalf("value %q is not valid UTF-8", got)
			}
		})
	}
}

// TestNameserver16BinaryNSID confirms an opaque binary NSID reaches the
// N16_HAS_NSID arg in dig's lossless hex+ASCII form rather than as mojibake.
func TestNameserver16BinaryNSID(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.16", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) == "SOA" {
			return soaPacketWithEdns("example", 0, 0, []dns.EDNS0{
				&dns.NSID{Nsid: hex.EncodeToString(binaryNSID)},
			})
		}
		return packet.Packet{}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver16(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver16: %v", err)
	}

	var hasNSID *logger.Entry
	for _, item := range entries {
		if item != nil && item.Tag == "N16_HAS_NSID" {
			hasNSID = item
			break
		}
	}
	if hasNSID == nil {
		t.Fatalf("expected N16_HAS_NSID entry")
	}
	nsid, ok := hasNSID.Args["nsid"].(string)
	if !ok {
		t.Fatalf("nsid arg is not a string: %#v", hasNSID.Args["nsid"])
	}
	if nsid != binaryNSIDDigForm {
		t.Fatalf("nsid arg = %q, want %q", nsid, binaryNSIDDigForm)
	}
	if !utf8.ValidString(nsid) {
		t.Fatalf("nsid arg %q is not valid UTF-8 (would lose bytes on marshal)", nsid)
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
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		c := cookieFromOpts(q.Opts)
		if len(c) == 16 { // query 1: return a well-formed 24-byte full cookie
			return soaPacketWithCookieRcode("example", dns.RcodeSuccess, clientPortion(c)+cookieServer16)
		}
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, c) // query 2: accept it
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	tctest.RequireTags(t, entries, "N17_COOKIE_SUPPORTED", "N17_COOKIE_ROUNDTRIP_OK")
	entry := tctest.RequireTag(t, entries, "N17_COOKIE_SUPPORTED")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("expected typed server list for N17_COOKIE_SUPPORTED, got %#v", entry.Args["servers"])
	}
}

func TestNameserver17NoCookie(t *testing.T) {
	ctx := tctest.Context(t)

	calls := 0
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		calls++
		return soaPacket("example") // NOERROR, no COOKIE option
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	tctest.RequireTags(t, entries, "N17_NO_COOKIE")
	tctest.RequireNoTag(t, entries, "N17_COOKIE_ROUNDTRIP_OK")
	if calls != 1 {
		t.Fatalf("expected exactly one query (no round-trip), got %d", calls)
	}
}

func TestNameserver17NonNoerrorQuery1(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeRefused
		return packet.Packet{Msg: msg}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	for _, tag := range []string{"N17_NO_COOKIE", "N17_COOKIE_SUPPORTED", "N17_COOKIE_MALFORMED", "N17_COOKIE_CLIENT_ONLY", "N17_NO_RESPONSE"} {
		tctest.RequireNoTag(t, entries, tag)
	}
}

func TestNameserver17ClientOnly(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, clientPortion(cookieFromOpts(q.Opts)))
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	tctest.RequireTags(t, entries, "N17_COOKIE_CLIENT_ONLY")
}

func TestNameserver17Malformed(t *testing.T) {
	t.Run("invalid length", func(t *testing.T) {
		ctx := tctest.Context(t)

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
			if strings.ToUpper(q.Type) != "SOA" {
				return packet.Packet{}
			}
			// 12-byte cookie: client (8) + 4-byte server tail = invalid length.
			return soaPacketWithCookieRcode("example", dns.RcodeSuccess, clientPortion(cookieFromOpts(q.Opts))+"aabbccdd")
		})
		tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
			return []ens.Nameserver{ns1}, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}
		entries, err := Nameserver17(ctx, &z)
		if err != nil {
			t.Fatalf("nameserver17: %v", err)
		}
		entry := tctest.RequireTag(t, entries, "N17_COOKIE_MALFORMED")
		if entry.Args["cookie_bytes"] != 12 {
			t.Fatalf("expected cookie_bytes=12, got %#v", entry.Args["cookie_bytes"])
		}
	})

	t.Run("wrong client echo", func(t *testing.T) {
		ctx := tctest.Context(t)

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
			if strings.ToUpper(q.Type) != "SOA" {
				return packet.Packet{}
			}
			// Valid length (24 B) but the client portion does not echo ours.
			return soaPacketWithCookieRcode("example", dns.RcodeSuccess, "ffffffffffffffff"+cookieServer16)
		})
		tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
			return []ens.Nameserver{ns1}, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}
		entries, err := Nameserver17(ctx, &z)
		if err != nil {
			t.Fatalf("nameserver17: %v", err)
		}
		tctest.RequireTags(t, entries, "N17_COOKIE_MALFORMED")
	})
}

func TestNameserver17SelfRejectAfterRetry(t *testing.T) {
	ctx := tctest.Context(t)

	calls := 0
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		c := cookieFromOpts(q.Opts)
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
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	tctest.RequireTags(t, entries, "N17_COOKIE_SELF_REJECT")
	tctest.RequireNoTag(t, entries, "N17_COOKIE_ROUNDTRIP_OK")
	if calls != 2 {
		t.Fatalf("expected query 2 plus one corroborating retry, got %d round-trip queries", calls)
	}
}

func TestNameserver17RotationNotFlagged(t *testing.T) {
	ctx := tctest.Context(t)

	const freshA = "11111111111111112222222222222222"
	calls := 0
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		c := cookieFromOpts(q.Opts)
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
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	tctest.RequireTags(t, entries, "N17_COOKIE_ROUNDTRIP_OK")
	tctest.RequireNoTag(t, entries, "N17_COOKIE_SELF_REJECT")
}

// TestNameserver17RequireServerCookie covers the strongest cookie posture: an
// enforcing server (e.g. BIND require-server-cookie) answers our client-only
// cookie with BADCOOKIE plus a fresh, well-formed Server Cookie, then accepts
// the full cookie on the round-trip. That must report N17_COOKIE_ENFORCED (not
// the plain N17_COOKIE_SUPPORTED) together with N17_COOKIE_ROUNDTRIP_OK.
func TestNameserver17RequireServerCookie(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		c := cookieFromOpts(q.Opts)
		if len(c) == 16 { // query 1: client-only cookie rejected with a fresh Server Cookie
			return soaPacketWithCookieRcode("example", dns.RcodeBadCookie, clientPortion(c)+cookieServer16)
		}
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, c) // query 2: accepts the full cookie
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	tctest.RequireTags(t, entries, "N17_COOKIE_ENFORCED", "N17_COOKIE_ROUNDTRIP_OK")
	tctest.RequireNoTag(t, entries, "N17_COOKIE_SUPPORTED")
	entry := tctest.RequireTag(t, entries, "N17_COOKIE_ENFORCED")
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
	ctx := tctest.Context(t)

	calls := 0
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		calls++
		return soaPacketWithCookieRcode("example", dns.RcodeBadCookie, clientPortion(cookieFromOpts(q.Opts)))
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	for _, tag := range []string{"N17_COOKIE_ENFORCED", "N17_COOKIE_SUPPORTED", "N17_COOKIE_CLIENT_ONLY", "N17_COOKIE_MALFORMED", "N17_NO_COOKIE", "N17_COOKIE_ROUNDTRIP_OK"} {
		tctest.RequireNoTag(t, entries, tag)
	}
	if calls != 1 {
		t.Fatalf("expected a single query 1 with no round-trip, got %d", calls)
	}
}

func TestNameserver17NoResponse(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		return packet.Packet{} // no response
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	tctest.RequireTags(t, entries, "N17_NO_RESPONSE")
}

func TestNameserver17Truncated(t *testing.T) {
	ctx := tctest.Context(t)

	calls := 0
	sawTCP := false
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		calls++
		if q.Opts != nil && q.Opts.UseVC != nil && *q.Opts.UseVC {
			sawTCP = true
		}
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Truncated = true
		return packet.Packet{Msg: msg}
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	tctest.RequireTags(t, entries, "N17_NO_RESPONSE")
	if calls != 1 {
		t.Fatalf("expected a single probe with no follow-up, got %d", calls)
	}
	if sawTCP {
		t.Fatalf("truncated cookie probe must not fall back to TCP")
	}
}

func TestNameserver17OversizedCookieSafe(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		// 41-byte cookie (client + 33-byte server tail): exceeds the 40-byte max.
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, clientPortion(cookieFromOpts(q.Opts))+strings.Repeat("a", 66))
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "N17_COOKIE_MALFORMED")
	if entry.Args["cookie_bytes"] != 41 {
		t.Fatalf("expected cookie_bytes=41, got %#v", entry.Args["cookie_bytes"])
	}
}

func TestNameserver17UndersizedCookieSafe(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		// 4-byte cookie (8 hex): shorter than a Client Cookie; must not panic.
		return soaPacketWithCookieRcode("example", dns.RcodeSuccess, "aabbccdd")
	})
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver17(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver17: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "N17_COOKIE_MALFORMED")
	if entry.Args["cookie_bytes"] != 4 {
		t.Fatalf("expected cookie_bytes=4, got %#v", entry.Args["cookie_bytes"])
	}
}

func TestNameserver17ClientCookieStable(t *testing.T) {
	ctx := tctest.Context(t)

	var mu sync.Mutex
	var seen []string
	record := func(opts *ens.QueryOptions) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, cookieFromOpts(opts))
	}
	handler := func(q tctest.Query) packet.Packet {
		if strings.ToUpper(q.Type) != "SOA" {
			return packet.Packet{}
		}
		record(q.Opts)
		return soaPacket("example") // cookieless: keeps each server to a single probe
	}
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.17", handler)
	ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.18", handler)
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	})

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

// ns18Server builds a fake nameserver whose SOA reply carries the given rcode and EDE options.
func ns18Server(t *testing.T, ctx context.Context, name, ip string, rcode uint16, edes ...*dns.EDE) ens.Nameserver {
	t.Helper()
	return tctest.NS(t, ctx, name, ip, func(q tctest.Query) packet.Packet {
		if !strings.EqualFold(q.Type, "SOA") {
			return packet.Packet{}
		}
		msg := new(dns.Msg)
		msg.Rcode = rcode
		if rcode == dns.RcodeSuccess {
			msg.Authoritative = true
			msg.Answer = []dns.RR{soaRecord(name)}
		}
		for _, ede := range edes {
			msg.Pseudo = append(msg.Pseudo, ede)
		}
		return packet.Packet{Msg: msg}
	})
}

// runNameserver18 stubs the authoritative set and runs the testcase against zone "example".
func runNameserver18(t *testing.T, ctx context.Context, servers ...ens.Nameserver) []*logger.Entry {
	t.Helper()
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]ens.Nameserver, error) {
		return servers, nil
	})
	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Nameserver18(ctx, &z)
	if err != nil {
		t.Fatalf("nameserver18: %v", err)
	}
	return entries
}

func TestNameserver18NoEDE(t *testing.T) {
	ctx := tctest.Context(t)
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess))
	tctest.RequireTags(t, entries, "N18_NO_EXTENDED_ERROR")
}

func TestNameserver18ServerError(t *testing.T) {
	ctx := tctest.Context(t)
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess, &dns.EDE{InfoCode: 20, ExtraText: "lame"}))
	e := tctest.RequireTag(t, entries, "N18_SERVER_ERROR_REPORTED")
	if e.Args["info_code"] != 20 {
		t.Fatalf("info_code = %v, want 20", e.Args["info_code"])
	}
	if e.Args["info_name"] != "Not Authoritative" {
		t.Fatalf("info_name = %v, want \"Not Authoritative\"", e.Args["info_name"])
	}
	if e.Args["extra_text"] != "lame" {
		t.Fatalf("extra_text = %v, want \"lame\"", e.Args["extra_text"])
	}
	servers, ok := e.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" || servers[0]["address"] != "192.0.2.1" {
		t.Fatalf("servers = %#v", e.Args["servers"])
	}
}

func TestNameserver18ResolverRoleConfusion(t *testing.T) {
	ctx := tctest.Context(t)
	// Stale Answer (3) is a resolver/cache code, observable at DO=0.
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess, &dns.EDE{InfoCode: 3}))
	tctest.RequireTags(t, entries, "N18_RESOLVER_BEHAVIOR_REPORTED")
}

func TestNameserver18Filtered(t *testing.T) {
	ctx := tctest.Context(t)
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess, &dns.EDE{InfoCode: 15}))
	tctest.RequireTags(t, entries, "N18_FILTERED_RESPONSE")
}

func TestNameserver18BenignAnnotation(t *testing.T) {
	ctx := tctest.Context(t)
	// Not Ready (14) is a named, benign/transient annotation.
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess, &dns.EDE{InfoCode: 14}))
	e := tctest.RequireTag(t, entries, "N18_EXTENDED_ERROR_REPORTED")
	if e.Args["info_name"] != "Not Ready" {
		t.Fatalf("info_name = %v, want \"Not Ready\"", e.Args["info_name"])
	}
}

func TestNameserver18UnnamedCode(t *testing.T) {
	ctx := tctest.Context(t)
	// 49152 is private-use: permanently unnamed in any IANA-tracking library, so this
	// proves the "code N" fallback regardless of the dns library version.
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess, &dns.EDE{InfoCode: 49152}))
	e := tctest.RequireTag(t, entries, "N18_EXTENDED_ERROR_REPORTED")
	if e.Args["info_name"] != "code 49152" {
		t.Fatalf("info_name = %v, want \"code 49152\"", e.Args["info_name"])
	}
}

func TestNameserver18MultipleEDE(t *testing.T) {
	ctx := tctest.Context(t)
	// One response carrying two EDE options of different classes -> two findings.
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess,
		&dns.EDE{InfoCode: 20}, &dns.EDE{InfoCode: 15}))
	if !tctest.Has(entries, "N18_SERVER_ERROR_REPORTED") || !tctest.Has(entries, "N18_FILTERED_RESPONSE") {
		t.Fatalf("expected both server-error and filtered tags, got %v", tctest.Tags(entries))
	}
}

func TestNameserver18MultipleServersSameCode(t *testing.T) {
	ctx := tctest.Context(t)
	s1 := ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess, &dns.EDE{InfoCode: 20, ExtraText: "x"})
	s2 := ns18Server(t, ctx, "ns2.example", "192.0.2.2", dns.RcodeSuccess, &dns.EDE{InfoCode: 20, ExtraText: "x"})
	entries := runNameserver18(t, ctx, s1, s2)
	tctest.RequireCount(t, entries, "N18_SERVER_ERROR_REPORTED", 1)
	e := tctest.RequireTag(t, entries, "N18_SERVER_ERROR_REPORTED")
	servers, ok := e.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected both servers listed, got %#v", e.Args["servers"])
	}
}

func TestNameserver18NonNoerrorWithEDE(t *testing.T) {
	ctx := tctest.Context(t)
	// EDE rides on REFUSED; it is captured, and the clean tag must NOT appear.
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeRefused, &dns.EDE{InfoCode: 18}))
	tctest.RequireTags(t, entries, "N18_SERVER_ERROR_REPORTED")
	tctest.RequireNoTag(t, entries, "N18_NO_EXTENDED_ERROR")
}

func TestNameserver18NonNoerrorNoEDE(t *testing.T) {
	ctx := tctest.Context(t)
	// REFUSED without EDE is left to other testcases: no clean tag, no observed-EDE tag.
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeRefused))
	for _, tag := range []string{"N18_NO_EXTENDED_ERROR", "N18_SERVER_ERROR_REPORTED", "N18_EXTENDED_ERROR_REPORTED", "N18_FILTERED_RESPONSE", "N18_RESOLVER_BEHAVIOR_REPORTED"} {
		tctest.RequireNoTag(t, entries, tag)
	}
}

func TestNameserver18NoResponse(t *testing.T) {
	ctx := tctest.Context(t)
	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		return packet.Packet{}
	})
	entries := runNameserver18(t, ctx, ns1)
	tctest.RequireTags(t, entries, "N18_NO_RESPONSE")
}

func TestNameserver18ExtraTextSanitized(t *testing.T) {
	ctx := tctest.Context(t)
	// Invalid UTF-8 bytes, an over-length payload, and a trailing NUL.
	raw := "start" + string([]byte{0xff, 0xfe}) + strings.Repeat("a", 300) + "\x00"
	entries := runNameserver18(t, ctx, ns18Server(t, ctx, "ns1.example", "192.0.2.1", dns.RcodeSuccess, &dns.EDE{InfoCode: 20, ExtraText: raw}))
	e := tctest.RequireTag(t, entries, "N18_SERVER_ERROR_REPORTED")
	text, ok := e.Args["extra_text"].(string)
	if !ok {
		t.Fatalf("extra_text not a string: %#v", e.Args["extra_text"])
	}
	if !utf8.ValidString(text) {
		t.Fatalf("extra_text is not valid UTF-8: %q", text)
	}
	if strings.ContainsRune(text, 0) {
		t.Fatalf("extra_text still contains NUL: %q", text)
	}
	if !strings.HasSuffix(text, "...") {
		t.Fatalf("over-length extra_text should be marked truncated, got %q", text)
	}
	if len(text) > 259 { // 256-byte cap + "..."
		t.Fatalf("extra_text not capped: len=%d", len(text))
	}
}

func TestEdeTagForCode(t *testing.T) {
	cases := []struct {
		code uint16
		want string
	}{
		{18, "N18_SERVER_ERROR_REPORTED"},
		{20, "N18_SERVER_ERROR_REPORTED"},
		{21, "N18_SERVER_ERROR_REPORTED"},
		{4, "N18_FILTERED_RESPONSE"},
		{15, "N18_FILTERED_RESPONSE"},
		{16, "N18_FILTERED_RESPONSE"},
		{17, "N18_FILTERED_RESPONSE"},
		{3, "N18_RESOLVER_BEHAVIOR_REPORTED"},
		{6, "N18_RESOLVER_BEHAVIOR_REPORTED"},
		{33, "N18_RESOLVER_BEHAVIOR_REPORTED"}, // Negative Trust Anchor (RFC 7646)
		{0, "N18_EXTENDED_ERROR_REPORTED"},
		{14, "N18_EXTENDED_ERROR_REPORTED"},
		{30, "N18_EXTENDED_ERROR_REPORTED"},
		{31, "N18_EXTENDED_ERROR_REPORTED"},    // Rate Limited (benign)
		{32, "N18_EXTENDED_ERROR_REPORTED"},    // Over Quota (benign)
		{34, "N18_EXTENDED_ERROR_REPORTED"},    // unassigned
		{49152, "N18_EXTENDED_ERROR_REPORTED"}, // private-use
	}
	for _, tc := range cases {
		if got := edeTagForCode(tc.code); got != tc.want {
			t.Errorf("edeTagForCode(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestEscapeUnprintable(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"BIND 9.18.1", "BIND 9.18.1"}, // plain ASCII unchanged
		{"a b", "a b"},                 // space preserved
		{`a\b`, `a\\b`},                // backslash doubled
		{"a\tb", `a\009b`},             // TAB escaped
		{"a\nb", `a\010b`},             // LF escaped
		{"\x7f", `\127`},               // DEL escaped
		{"\xff", `\255`},               // high byte escaped
		{"\xc3\x28", `\195(`},          // invalid UTF-8 escaped per byte
	}
	for _, tc := range cases {
		if got := escapeUnprintable(tc.in); got != tc.want {
			t.Errorf("escapeUnprintable(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
