package delegation

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
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

func TestDelegation01Counts(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)
	stubNoDelegationGlueGap(t)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM2 := glueNames
	origM3 := apexNSNames
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNames = origM2
		apexNSNames = origM3
		glueNameservers = origM4
		apexNameservers = origM5
	})

	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	}
	apexNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", nil)
		ns2 := newNameserver(t, ctx, "ns2.example", "2001:db8::1", nil)
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.2", nil)
		return []nameserver.Nameserver{ns1}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)
	stubNoDelegationGlueGap(t)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM2 := glueNames
	origM3 := apexNSNames
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNames = origM2
		apexNSNames = origM3
		glueNameservers = origM4
		apexNameservers = origM5
	})

	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{
			dnsname.New("ns1.example"),
			dnsname.New("ns2.example"),
		}, nil
	}
	apexNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{
			dnsname.New("ns2.example"),
			dnsname.New("ns1.example"),
		}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{
			newNameserver(t, ctx, "ns1.example", "192.0.2.1", nil),
			newNameserver(t, ctx, "ns2.example", "192.0.2.2", nil),
		}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{
			newNameserver(t, ctx, "ns2.example", "192.0.2.22", nil),
			newNameserver(t, ctx, "ns1.example", "192.0.2.11", nil),
		}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)
	stubNoDelegationGlueGap(t)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM2 := glueNames
	origM3 := apexNSNames
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNames = origM2
		apexNSNames = origM3
		glueNameservers = origM4
		apexNameservers = origM5
	})

	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	apexNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{
			newNameserver(t, ctx, "ns1.example", "192.0.2.1", nil),
		}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{
			newNameserver(t, ctx, "ns1.example", "2001:db8::53", nil),
		}, nil
	}

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
	origM2 := glueNames
	origM3 := apexNSNames
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNames = origM2
		apexNSNames = origM3
		glueNameservers = origM4
		apexNameservers = origM5
	})
	emptyNames := func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) { return nil, nil }
	emptyNS := func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) { return nil, nil }
	glueNames = emptyNames
	apexNSNames = emptyNames
	glueNameservers = emptyNS
	apexNameservers = emptyNS
}

// TestDelegation01InBailiwickGlueMissing verifies that an in-bailiwick
// delegation NS name shipped by the parent without A/AAAA glue is flagged,
// while an in-bailiwick name that does carry glue and an out-of-bailiwick name
// (resolved elsewhere) are not.
func TestDelegation01InBailiwickGlueMissing(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubDelegation01Counts(t)

	origDel := delegationNameservers
	t.Cleanup(func() { delegationNameservers = origDel })
	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{Name: dnsname.New("ns1.example")},
			{Name: dnsname.New("ns2.example"), Address: netip.MustParseAddr("192.0.2.1"), HasAddress: true},
			{Name: dnsname.New("ns.example.net")},
		}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubDelegation01Counts(t)

	origDel := delegationNameservers
	t.Cleanup(func() { delegationNameservers = origDel })
	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.1"), HasAddress: true},
			{Name: dnsname.New("ns2.example"), Address: netip.MustParseAddr("2001:db8::1"), HasAddress: true},
		}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation01(ctx, &z)
	if err != nil {
		t.Fatalf("delegation01: %v", err)
	}

	tctest.RequireNoTag(t, entries, "IN_DOMAIN_GLUE_MISSING")
}

func TestDelegation02DuplicateIPs(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", nil)
		ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1, ns2}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns3 := newNameserver(t, ctx, "ns3.example", "192.0.2.2", nil)
		return []nameserver.Nameserver{ns3}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM1 := parentZone
	origM2 := glueNames
	origM4 := glueNameservers
	t.Cleanup(func() {
		parentZone = origM1
		glueNames = origM2
		glueNameservers = origM4
	})

	parentZone = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		parent := zone.Zone{Name: dnsname.New(".")}
		return &parent, nil
	}
	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM1 := parentZone
	origM2 := glueNames
	origM4 := glueNameservers
	t.Cleanup(func() {
		parentZone = origM1
		glueNames = origM2
		glueNameservers = origM4
	})

	parentZone = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		parent := zone.Zone{Name: dnsname.New(".")}
		return &parent, nil
	}
	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return referralNSNames(5), nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM1 := parentZone
	origM2 := glueNames
	origM4 := glueNameservers
	t.Cleanup(func() {
		parentZone = origM1
		glueNames = origM2
		glueNameservers = origM4
	})

	parentZone = func(_ context.Context, _ *zone.Zone) (*zone.Zone, error) {
		parent := zone.Zone{Name: dnsname.New(".")}
		return &parent, nil
	}
	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return referralNSNames(16), nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", nil)
		return []nameserver.Nameserver{ns1}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "SOA") {
				return soaPacket("example", true)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "SOA") {
				return soaPacket("example", false)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation04(ctx, &z)
	if err != nil {
		t.Fatalf("delegation04: %v", err)
	}
	tctest.RequireTags(t, entries, "IS_NOT_AUTHORITATIVE")
}

func TestDelegation04ParallelQueries(t *testing.T) {
	ctx := testCtx()
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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
}

func TestDelegation05InBailiwickCNAME(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := allNSNames
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		allNSNames = origM23
		glueNameservers = origM4
		apexNameservers = origM5
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "A") && strings.EqualFold(qname, "ns1.example") {
				return cnamePacket(qname, "alias.example")
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := allNSNames
	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		allNSNames = origM23
		glueNameservers = origM4
		apexNameservers = origM5
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

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
}

func TestDelegation05OutOfBailiwickNoCNAME(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := allNSNames
	origM4 := glueNameservers
	origM5 := apexNameservers
	origRecurse := recurse
	t.Cleanup(func() {
		allNSNames = origM23
		glueNameservers = origM4
		apexNameservers = origM5
		recurse = origRecurse
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.other")}, nil
	}
	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}
	recurse = func(_ context.Context, _ *zone.Zone, _ string, _ string) (packet.Packet, error) {
		return packet.Packet{Msg: new(dns.Msg)}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation05(ctx, &z)
	if err != nil {
		t.Fatalf("delegation05: %v", err)
	}
	tctest.RequireTags(t, entries, "NO_NS_CNAME")
}

func TestDelegation06SOANotExists(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "SOA") {
				return noAnswerPacket()
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation06(ctx, &z)
	if err != nil {
		t.Fatalf("delegation06: %v", err)
	}
	tctest.RequireTags(t, entries, "SOA_NOT_EXISTS")
}

func TestDelegation06SOAExists(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			if strings.EqualFold(qtype, "SOA") {
				return soaPacket("example", true)
			}
			return packet.Packet{}
		})
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation06(ctx, &z)
	if err != nil {
		t.Fatalf("delegation06: %v", err)
	}
	tctest.RequireTags(t, entries, "SOA_EXISTS")
}

func TestDelegation07NameMismatch(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM2 := glueNames
	origM3 := apexNSNames
	t.Cleanup(func() {
		glueNames = origM2
		apexNSNames = origM3
	})

	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	}
	apexNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns2.example"), dnsname.New("ns3.example")}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Delegation07(ctx, &z)
	if err != nil {
		t.Fatalf("delegation07: %v", err)
	}
	tctest.RequireTags(t, entries, "EXTRA_NAME_PARENT", "EXTRA_NAME_CHILD")
}

func TestDelegation07NamesMatch(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM2 := glueNames
	origM3 := apexNSNames
	t.Cleanup(func() {
		glueNames = origM2
		apexNSNames = origM3
	})

	glueNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}
	apexNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}

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
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	origM2 := glueNames
	origM3 := apexNSNames
	t.Cleanup(func() {
		glueNames = origM2
		apexNSNames = origM3
	})

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
			nsRR := &dns.NS{}
			nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn("example.com"), Class: dns.ClassINET, TTL: 60}
			nsRR.Ns = dnsutil.Fqdn(nsName)
			msg.Answer = append(msg.Answer, nsRR)
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
	tctest.RequireTags(t, entries, "EXTRA_NAME_PARENT", "EXTRA_NAME_CHILD")
	tctest.RequireNoTag(t, entries, "NAMES_MATCH")
}

func testCtx() context.Context {
	return nameserver.WithCache(context.Background(), nameserver.NewCacheStore())
}

// stubNoDelegationGlueGap neutralises Delegation01's in-bailiwick glue check
// by returning no delegation items, so the count-focused tests are unaffected
// by it. The dedicated glue tests stub delegationNameservers themselves.
func stubNoDelegationGlueGap(t *testing.T) {
	orig := delegationNameservers
	t.Cleanup(func() { delegationNameservers = orig })
	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}
}

func newNameserver(t *testing.T, ctx context.Context, name string, ip string, handler func(qname string, qtype string, opts *nameserver.QueryOptions) packet.Packet) nameserver.Nameserver {
	t.Helper()

	ns, err := nameserver.NewWithContext(ctx, name, ip, nil)
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

func soaPacket(owner string, authoritative bool) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = authoritative
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = "ns1.example."
	soaRR.Mbox = "hostmaster.example."
	soaRR.Serial = 1
	soaRR.Refresh = 3600
	soaRR.Retry = 600
	soaRR.Expire = 86400
	soaRR.Minttl = 60
	msg.Answer = []dns.RR{soaRR}
	return packet.Packet{Msg: msg}
}

func cnamePacket(owner string, target string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	cnameRR := &dns.CNAME{}
	cnameRR.Hdr = dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}
	cnameRR.Target = dnsutil.Fqdn(target)
	msg.Answer = []dns.RR{cnameRR}
	return packet.Packet{Msg: msg}
}

func noAnswerPacket() packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	return packet.Packet{Msg: msg}
}
