package delegation

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestDelegation01Counts(t *testing.T) {
	ctx := tctest.Context(t)
	stubNoDelegationGlueGap(t)

	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	})
	tctest.Stub(t, &apexNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
		ns2 := tctest.NS(t, ctx, "ns2.example", "2001:db8::1", nil)
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.2", nil)
		return []nameserver.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation01(ctx, &z)
	if err != nil {
		t.Fatalf("delegation01: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "ENOUGH_NS_DEL")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected typed server list for ENOUGH_NS_DEL, got %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
	entry = tctest.RequireTag(t, entries, "NOT_ENOUGH_NS_CHILD")
	servers, ok = entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("expected typed server list for NOT_ENOUGH_NS_CHILD, got %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
	entry = tctest.RequireTag(t, entries, "NOT_ENOUGH_IPV4_NS_DEL")
	servers, ok = entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server for NOT_ENOUGH_IPV4_NS_DEL, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "ns1.example" || servers[0]["address"] != "192.0.2.1" {
		t.Fatalf("unexpected typed server payload: %#v", servers[0])
	}
	addresses, ok := entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 1 || addresses[0] != "192.0.2.1" {
		t.Fatalf("unexpected typed addresses payload: %#v", entry.Args["addresses"])
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
	entry = tctest.RequireTag(t, entries, "NOT_ENOUGH_IPV6_NS_DEL")
	servers, ok = entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server for NOT_ENOUGH_IPV6_NS_DEL, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "ns2.example" || servers[0]["address"] != "2001:db8::1" {
		t.Fatalf("unexpected typed server payload: %#v", servers[0])
	}
	addresses, ok = entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 1 || addresses[0] != "2001:db8::1" {
		t.Fatalf("unexpected typed addresses payload: %#v", entry.Args["addresses"])
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
	entry = tctest.RequireTag(t, entries, "NOT_ENOUGH_IPV4_NS_CHILD")
	servers, ok = entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server for NOT_ENOUGH_IPV4_NS_CHILD, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "ns1.example" || servers[0]["address"] != "192.0.2.2" {
		t.Fatalf("unexpected typed server payload: %#v", servers[0])
	}
	addresses, ok = entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 1 || addresses[0] != "192.0.2.2" {
		t.Fatalf("unexpected typed addresses payload: %#v", entry.Args["addresses"])
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
	tctest.RequireTags(t, entries, "NO_IPV6_NS_CHILD")
}

func TestDelegation01EnoughIPv4ChildTypedArgsOrder(t *testing.T) {
	ctx := tctest.Context(t)
	stubNoDelegationGlueGap(t)

	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{
			dnsname.New("ns1.example"),
			dnsname.New("ns2.example"),
		}, nil
	})
	tctest.Stub(t, &apexNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{
			dnsname.New("ns2.example"),
			dnsname.New("ns1.example"),
		}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{
			tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil),
			tctest.NS(t, ctx, "ns2.example", "192.0.2.2", nil),
		}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{
			tctest.NS(t, ctx, "ns2.example", "192.0.2.22", nil),
			tctest.NS(t, ctx, "ns1.example", "192.0.2.11", nil),
		}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation01(ctx, &z)
	if err != nil {
		t.Fatalf("delegation01: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "ENOUGH_IPV4_NS_CHILD")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected two typed servers for ENOUGH_IPV4_NS_CHILD, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "ns1.example" || servers[0]["address"] != "192.0.2.11" {
		t.Fatalf("unexpected first typed server payload: %#v", servers[0])
	}
	if servers[1]["ns"] != "ns2.example" || servers[1]["address"] != "192.0.2.22" {
		t.Fatalf("unexpected second typed server payload: %#v", servers[1])
	}
	addresses, ok := entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 2 {
		t.Fatalf("expected two typed addresses for ENOUGH_IPV4_NS_CHILD, got %#v", entry.Args["addresses"])
	}
	if addresses[0] != "192.0.2.11" || addresses[1] != "192.0.2.22" {
		t.Fatalf("expected deterministic address order, got %v", addresses)
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
}

func TestDelegation01NoIPv4ChildNoLegacyKeys(t *testing.T) {
	ctx := tctest.Context(t)
	stubNoDelegationGlueGap(t)

	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	})
	tctest.Stub(t, &apexNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{
			tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil),
		}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{
			tctest.NS(t, ctx, "ns1.example", "2001:db8::53", nil),
		}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation01(ctx, &z)
	if err != nil {
		t.Fatalf("delegation01: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "NO_IPV4_NS_CHILD")
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
	if _, ok := entry.Args["servers"]; ok {
		t.Fatalf("did not expect servers for NO_IPV4_NS_CHILD: %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["addresses"]; ok {
		t.Fatalf("did not expect addresses for NO_IPV4_NS_CHILD: %#v", entry.Args["addresses"])
	}
}

// stubDelegation01Counts neutralises Delegation01's count-section method calls
// so a test can focus on the in-bailiwick glue check. The four count sources
// return empty, which only affects the count tags, not the glue check.
func stubDelegation01Counts(t *testing.T) {
	emptyNames := func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) { return nil, nil }
	emptyNS := func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) { return nil, nil }
	tctest.Stub(t, &glueNames, emptyNames)
	tctest.Stub(t, &apexNSNames, emptyNames)
	tctest.Stub(t, &glueNameservers, emptyNS)
	tctest.Stub(t, &apexNameservers, emptyNS)
}

// TestDelegation01InBailiwickGlueMissing verifies that an in-bailiwick
// delegation NS name shipped by the parent without A/AAAA glue is flagged,
// while an in-bailiwick name that does carry glue and an out-of-bailiwick name
// (resolved elsewhere) are not.
func TestDelegation01InBailiwickGlueMissing(t *testing.T) {
	ctx := tctest.Context(t)

	stubDelegation01Counts(t)

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return tctest.NSItems("ns1.example", "ns2.example/192.0.2.1", "ns.example.net"), nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation01(ctx, &z)
	if err != nil {
		t.Fatalf("delegation01: %v", err)
	}

	flagged := tctest.ArgValues(entries, "IN_DOMAIN_GLUE_MISSING", "ns")
	if len(flagged) != 1 || flagged[0] != "ns1.example" {
		t.Fatalf("expected only ns1.example flagged for missing glue, got %v", flagged)
	}
}

// TestDelegation01InBailiwickGluePresent verifies that no glue-missing tag is
// emitted when every in-bailiwick delegation NS name carries glue.
func TestDelegation01InBailiwickGluePresent(t *testing.T) {
	ctx := tctest.Context(t)

	stubDelegation01Counts(t)

	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return tctest.NSItems("ns1.example/192.0.2.1", "ns2.example/2001:db8::1"), nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation01(ctx, &z)
	if err != nil {
		t.Fatalf("delegation01: %v", err)
	}

	tctest.RequireNoTag(t, entries, "IN_DOMAIN_GLUE_MISSING")
}

func TestDelegation02DuplicateIPs(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1, ns2}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns3 := tctest.NS(t, ctx, "ns3.example", "192.0.2.2", nil)
		return []nameserver.Nameserver{ns3}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation02(ctx, &z)
	if err != nil {
		t.Fatalf("delegation02: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "DEL_NS_SAME_IP")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected typed server list for DEL_NS_SAME_IP, got %#v", entry.Args["servers"])
	}
	if address, _ := entry.Args["address"].(string); address != "192.0.2.1" {
		t.Fatalf("expected address=192.0.2.1 for DEL_NS_SAME_IP, got %#v", entry.Args["address"])
	}
	if _, ok := entry.Args["ns_ip"]; ok {
		t.Fatalf("legacy key ns_ip should not be present: %#v", entry.Args)
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
	tctest.RequireTags(t, entries, "CHILD_DISTINCT_NS_IP", "SAME_IP_ADDRESS")
	entry = tctest.First(entries, "SAME_IP_ADDRESS")
	servers, ok = entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected typed server list for SAME_IP_ADDRESS, got %#v", entry.Args["servers"])
	}
	if address, _ := entry.Args["address"].(string); address != "192.0.2.1" {
		t.Fatalf("expected address=192.0.2.1 for SAME_IP_ADDRESS, got %#v", entry.Args["address"])
	}
	if _, ok := entry.Args["ns_ip"]; ok {
		t.Fatalf("legacy key ns_ip should not be present: %#v", entry.Args)
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
}

func TestDelegation03ReferralSizeOK(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &parentZone, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		parent := zone.Zone{Name: dnsname.New(".")}
		return &parent, nil
	})
	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation03(ctx, &z)
	if err != nil {
		t.Fatalf("delegation03: %v", err)
	}
	tctest.RequireTags(t, entries, "REFERRAL_SIZE_OK")
}

// referralNSNames builds n distinct long delegation NS names so the
// synthesized referral packet grows past the size thresholds.
func referralNSNames(n int) []dnsname.Name {
	names := make([]dnsname.Name, n)
	for i := range names {
		label := strings.Repeat("a", 60) + string(rune('a'+i%26)) + string(rune('a'+i/26))
		names[i] = dnsname.New(label + ".example")
	}
	return names
}

func TestDelegation03ReferralSizeLarge(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &parentZone, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		parent := zone.Zone{Name: dnsname.New(".")}
		return &parent, nil
	})
	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return referralNSNames(5), nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation03(ctx, &z)
	if err != nil {
		t.Fatalf("delegation03: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "REFERRAL_SIZE_LARGE")
	size, ok := entry.Args["size"].(int)
	if !ok {
		t.Fatalf("expected int size arg, got %#v", entry.Args["size"])
	}
	if size <= 512 || size > 1232 {
		t.Fatalf("expected size in (512, 1232], got %d", size)
	}
}

func TestDelegation03ReferralSizeTooLarge(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &parentZone, func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		parent := zone.Zone{Name: dnsname.New(".")}
		return &parent, nil
	})
	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return referralNSNames(16), nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation03(ctx, &z)
	if err != nil {
		t.Fatalf("delegation03: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "REFERRAL_SIZE_TOO_LARGE")
	size, ok := entry.Args["size"].(int)
	if !ok {
		t.Fatalf("expected int size arg, got %#v", entry.Args["size"])
	}
	if size <= 1232 {
		t.Fatalf("expected size > 1232, got %d", size)
	}
}

func TestDelegation04Authoritative(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
			if strings.EqualFold(q.Type, "SOA") {
				return soaPacket("example", true)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation04(ctx, &z)
	if err != nil {
		t.Fatalf("delegation04: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "ARE_AUTHORITATIVE")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("expected typed server list for ARE_AUTHORITATIVE, got %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
}

func TestDelegation04NotAuthoritative(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
			if strings.EqualFold(q.Type, "SOA") {
				return soaPacket("example", false)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation04(ctx, &z)
	if err != nil {
		t.Fatalf("delegation04: %v", err)
	}
	tctest.RequireTags(t, entries, "IS_NOT_AUTHORITATIVE")
}

func TestDelegation04ParallelQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		gate := tctest.NewGate()
		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if strings.EqualFold(q.Type, "SOA") && q.Opts != nil && q.Opts.UseVC != nil && !*q.Opts.UseVC {
					gate.Arrive(id)
				}
				return soaPacket("example", false)
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.2", hook("ns2"))

		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns2}, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var delErr error
		go func() {
			entries, delErr = Delegation04(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if delErr != nil {
			t.Fatalf("delegation04: %v", delErr)
		}

		var order []string
		for _, entry := range entries {
			if entry == nil || entry.Tag != "IS_NOT_AUTHORITATIVE" {
				continue
			}
			if _, ok := entry.Args["arg_schema"]; ok {
				t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
			}
			ns, _ := entry.Args["ns"].(string)
			if strings.Contains(ns, "/") {
				t.Fatalf("expected nameserver-only ns argument, got %q", ns)
			}
			address, _ := entry.Args["address"].(string)
			proto, _ := entry.Args["proto"].(string)
			order = append(order, ns+"|"+address+"|"+proto)
		}
		if len(order) != 4 {
			t.Fatalf("expected 4 not-authoritative entries, got %v", order)
		}
		if order[0] != "ns1.example|192.0.2.1|UDP" || order[1] != "ns1.example|192.0.2.1|TCP" ||
			order[2] != "ns2.example|192.0.2.2|UDP" || order[3] != "ns2.example|192.0.2.2|TCP" {
			t.Fatalf("expected deterministic log order, got %v", order)
		}
	})
}

func TestDelegation05InBailiwickCNAME(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &allNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
			if strings.EqualFold(q.Type, "A") && strings.EqualFold(q.Name, "ns1.example") {
				return cnamePacket(q.Name, "alias.example")
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation05(ctx, &z)
	if err != nil {
		t.Fatalf("delegation05: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "NS_IS_CNAME")
	if ns, _ := entry.Args["ns"].(string); ns != "ns1.example" {
		t.Fatalf("expected ns=ns1.example, got %#v", entry.Args["ns"])
	}
	if _, ok := entry.Args["nsname"]; ok {
		t.Fatalf("legacy key nsname should not be present: %#v", entry.Args)
	}
	tctest.RequireNoTag(t, entries, "NO_NS_CNAME")
}

func TestDelegation05ParallelQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		gate := tctest.NewGate()
		hook := func(id string) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				if strings.EqualFold(q.Type, "A") && strings.EqualFold(q.Name, "ns1.example") {
					gate.Arrive(id)
				}
				return packet.Packet{}
			}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", hook("ns1"))
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.2", hook("ns2"))

		tctest.Stub(t, &allNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
			return []dnsname.Name{dnsname.New("ns1.example")}, nil
		})
		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1}, nil
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns2}, nil
		})

		z := zone.Zone{Name: dnsname.New("example")}

		done := make(chan struct{})
		var entries []*logger.Entry
		var delErr error
		go func() {
			entries, delErr = Delegation05(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1", "ns2")
		gate.Release()

		<-done
		if delErr != nil {
			t.Fatalf("delegation05: %v", delErr)
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
			t.Fatalf("expected deterministic nameserver order, got %v", order)
		}
		if len(addresses) != 2 {
			t.Fatalf("expected 2 no-response addresses, got %v", addresses)
		}
		if addresses[0] != "192.0.2.1" || addresses[1] != "192.0.2.2" {
			t.Fatalf("expected deterministic address order, got %v", addresses)
		}
	})
}

func TestDelegation05OutOfBailiwickNoCNAME(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &allNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.other")}, nil
	})
	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	})
	tctest.Stub(t, &recurse, func(_ context.Context, _ *zone.Zone, _ string, _ string) (packet.Packet, error) {
		return packet.Packet{Msg: new(dns.Msg)}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation05(ctx, &z)
	if err != nil {
		t.Fatalf("delegation05: %v", err)
	}
	tctest.RequireTags(t, entries, "NO_NS_CNAME")
}

func TestDelegation06SOANotExists(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
			if strings.EqualFold(q.Type, "SOA") {
				return noAnswerPacket()
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation06(ctx, &z)
	if err != nil {
		t.Fatalf("delegation06: %v", err)
	}
	tctest.RequireTags(t, entries, "SOA_NOT_EXISTS")
}

func TestDelegation06SOAExists(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
			if strings.EqualFold(q.Type, "SOA") {
				return soaPacket("example", true)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation06(ctx, &z)
	if err != nil {
		t.Fatalf("delegation06: %v", err)
	}
	tctest.RequireTags(t, entries, "SOA_EXISTS")
}

func TestDelegation07NameMismatch(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	})
	tctest.Stub(t, &apexNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns2.example"), dnsname.New("ns3.example")}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation07(ctx, &z)
	if err != nil {
		t.Fatalf("delegation07: %v", err)
	}
	tctest.RequireTags(t, entries, "EXTRA_NAME_PARENT", "EXTRA_NAME_CHILD")
}

func TestDelegation07NamesMatch(t *testing.T) {
	ctx := tctest.Context(t)

	tctest.Stub(t, &glueNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	})
	tctest.Stub(t, &apexNSNames, func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation07(ctx, &z)
	if err != nil {
		t.Fatalf("delegation07: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "NAMES_MATCH")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["ns"] != "ns1.example" {
		t.Fatalf("expected typed server list for NAMES_MATCH, got %#v", entry.Args["servers"])
	}
	if _, ok := entry.Args["names"]; ok {
		t.Fatalf("legacy key names should not be present: %#v", entry.Args)
	}
}

func TestDelegation07UndelegatedReportsExtraNameChild(t *testing.T) {
	tctest.Setup(t)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := tctest.Recursor(t, map[string]map[string][]string{
		"example.com": {
			"ns1.example.com": {"192.0.2.11"},
			"ns2.example.com": {"192.0.2.12"},
			"ns3.example.com": {"192.0.2.13"},
		},
	})

	makeNSAnswer := func(nsNames ...string) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		for _, nsName := range nsNames {
			nsRR := &dns.NS{}
			nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn("example.com"), Class: dns.ClassINET, TTL: 60}
			nsRR.Ns = dnsutil.Fqdn(nsName)
			msg.Answer = append(msg.Answer, nsRR)
		}
		return packet.Packet{Msg: msg}
	}

	setHook := func(name, ip string, nsNames ...string) {
		t.Helper()
		tctest.NSOn(t, ctx, r, name, ip, func(q tctest.Query) packet.Packet {
			if !strings.EqualFold(q.Name, "example.com") || !strings.EqualFold(q.Type, "NS") {
				return packet.Packet{}
			}
			return makeNSAnswer(nsNames...)
		})
	}

	setHook("ns1.example.com", "192.0.2.11", "ns1.example.com", "ns4.example.com")
	setHook("ns2.example.com", "192.0.2.12", "ns1.example.com", "ns5.example.com")
	setHook("ns3.example.com", "192.0.2.13", "ns4.example.com")

	z := tctest.Zone(t, "example.com", r)

	entries, err := Delegation07(ctx, z)
	if err != nil {
		t.Fatalf("delegation07: %v", err)
	}
	tctest.RequireTags(t, entries, "EXTRA_NAME_PARENT", "EXTRA_NAME_CHILD")
	tctest.RequireNoTag(t, entries, "NAMES_MATCH")
}

// stubNoDelegationGlueGap neutralises Delegation01's in-bailiwick glue check
// by returning no delegation items, so the count-focused tests are unaffected
// by it. The dedicated glue tests stub delegationNameservers themselves.
func stubNoDelegationGlueGap(t *testing.T) {
	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})
}

func soaPacket(owner string, authoritative bool) packet.Packet {
	opts := []tctest.MsgOpt{tctest.Answers(tctest.SOARR(owner,
		tctest.MName("ns1.example"), tctest.RName("hostmaster.example")))}
	if !authoritative {
		opts = append(opts, tctest.NotAuthoritative())
	}
	return tctest.Response(opts...)
}

func cnamePacket(owner string, target string) packet.Packet {
	return tctest.Response(tctest.Answers(tctest.CNAMERR(owner, target)))
}

func noAnswerPacket() packet.Packet {
	return tctest.Response()
}
