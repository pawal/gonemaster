package consistency

import (
	"context"
	"net/netip"
	"sort"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestConsistency01MultipleSerials(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 2, "ns2.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency01(ctx, &z)
	if err != nil {
		t.Fatalf("consistency01: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_SOA_SERIALS") {
		t.Fatalf("expected MULTIPLE_SOA_SERIALS")
	}
	if !hasEntryTag(entries, "SOA_SERIAL_VARIATION") {
		t.Fatalf("expected SOA_SERIAL_VARIATION")
	}
	if !hasEntryTag(entries, "SOA_SERIAL") {
		t.Fatalf("expected SOA_SERIAL")
	}
	entry := firstEntryByTag(entries, "SOA_SERIAL")
	if entry == nil {
		t.Fatalf("missing SOA_SERIAL entry")
	}
	if _, ok := entry.Args["servers"]; !ok {
		t.Fatalf("expected typed servers in SOA_SERIAL args: %#v", entry.Args)
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["address"] != "192.0.2.1" {
		t.Fatalf("expected endpoint-preserving typed server for SOA_SERIAL, got %#v", entry.Args["servers"])
	}
}

// Serials 9 and 100 must be ordered numerically: a lexicographic string sort ranks
// "100" before "9" and computes a negative delta, which previously hid the variation
// entirely. The oldest must be 9, the newest 100, and ns1 (serving 9) is the laggard.
func TestConsistency01LexicalTrapSerialVariation(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 9, "ns1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 100, "ns2.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency01(ctx, &z)
	if err != nil {
		t.Fatalf("consistency01: %v", err)
	}

	entry := firstEntryByTag(entries, "SOA_SERIAL_VARIATION")
	if entry == nil {
		t.Fatalf("expected SOA_SERIAL_VARIATION for serials 9 and 100 (numeric ordering)")
	}
	if got := entry.Args["serial_min"]; got != "9" {
		t.Fatalf("expected serial_min=9 (oldest), got %#v", got)
	}
	if got := entry.Args["serial_max"]; got != "100" {
		t.Fatalf("expected serial_max=100 (newest), got %#v", got)
	}
	behind := serverEndpointsAtKey(entry.Args, "servers_behind")
	if len(behind) != 1 || behind[0] != "ns1.example/192.0.2.1" {
		t.Fatalf("expected servers_behind to name ns1.example/192.0.2.1, got %v", behind)
	}
}

// Near the 32-bit wrap boundary serial 1 is newer than 4294967294 under RFC 1982,
// so newest/oldest selection and the wrap-safe distance must treat 1 as the newest.
func TestConsistency01SerialWraparound(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 4294967294, "ns1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns2.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency01(ctx, &z)
	if err != nil {
		t.Fatalf("consistency01: %v", err)
	}

	entry := firstEntryByTag(entries, "SOA_SERIAL_VARIATION")
	if entry == nil {
		t.Fatalf("expected SOA_SERIAL_VARIATION across the wrap boundary")
	}
	if got := entry.Args["serial_min"]; got != "4294967294" {
		t.Fatalf("expected serial_min=4294967294 (oldest under RFC 1982), got %#v", got)
	}
	if got := entry.Args["serial_max"]; got != "1" {
		t.Fatalf("expected serial_max=1 (newest under RFC 1982), got %#v", got)
	}
	behind := serverEndpointsAtKey(entry.Args, "servers_behind")
	if len(behind) != 1 || behind[0] != "ns1.example/192.0.2.1" {
		t.Fatalf("expected servers_behind to name ns1.example/192.0.2.1 (serving the older serial), got %v", behind)
	}
}

func TestConsistency02MultipleRnames(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns2.example", "admin.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency02(ctx, &z)
	if err != nil {
		t.Fatalf("consistency02: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_SOA_RNAMES") {
		t.Fatalf("expected MULTIPLE_SOA_RNAMES")
	}
	if !hasEntryTag(entries, "SOA_RNAME") {
		t.Fatalf("expected SOA_RNAME")
	}
	entry := firstEntryByTag(entries, "SOA_RNAME")
	if entry == nil {
		t.Fatalf("missing SOA_RNAME entry")
	}
	if _, ok := entry.Args["servers"]; !ok {
		t.Fatalf("expected typed servers in SOA_RNAME args: %#v", entry.Args)
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
}

func TestConsistency03MultipleTimeSets(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "ns2.example", "hostmaster.example", 7200, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency03(ctx, &z)
	if err != nil {
		t.Fatalf("consistency03: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_SOA_TIME_PARAMETER_SET") {
		t.Fatalf("expected MULTIPLE_SOA_TIME_PARAMETER_SET")
	}
	if !hasEntryTag(entries, "SOA_TIME_PARAMETER_SET") {
		t.Fatalf("expected SOA_TIME_PARAMETER_SET")
	}
	entry := firstEntryByTag(entries, "SOA_TIME_PARAMETER_SET")
	if entry == nil {
		t.Fatalf("missing SOA_TIME_PARAMETER_SET entry")
	}
	if _, ok := entry.Args["servers"]; !ok {
		t.Fatalf("expected typed servers in SOA_TIME_PARAMETER_SET args: %#v", entry.Args)
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
}

func TestConsistency04MultipleNSSets(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "NS") {
			return nsPacket("example", []string{"ns1.example"})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "NS") {
			return nsPacket("example", []string{"ns1.example", "ns2.example"})
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency04(ctx, &z)
	if err != nil {
		t.Fatalf("consistency04: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_NS_SET") {
		t.Fatalf("expected MULTIPLE_NS_SET")
	}
	if !hasEntryTag(entries, "NS_SET") {
		t.Fatalf("expected NS_SET")
	}
	entry := firstEntryByTag(entries, "NS_SET")
	if entry == nil {
		t.Fatalf("expected NS_SET entry")
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
	nsSet, ok := entry.Args["ns_set_servers"].([]map[string]any)
	if !ok || len(nsSet) == 0 {
		t.Fatalf("expected typed ns_set_servers for NS_SET, got %#v", entry.Args["ns_set_servers"])
	}
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["address"] != "192.0.2.1" {
		t.Fatalf("expected endpoint-preserving typed servers for NS_SET, got %#v", entry.Args["servers"])
	}
}

func TestConsistency04OneNSSetTypedServers(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "NS") {
			return nsPacket("example", []string{"ns1.example", "ns2.example"})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "NS") {
			return nsPacket("example", []string{"ns1.example", "ns2.example"})
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency04(ctx, &z)
	if err != nil {
		t.Fatalf("consistency04: %v", err)
	}
	if !hasEntryTag(entries, "ONE_NS_SET") {
		t.Fatalf("expected ONE_NS_SET")
	}
	entry := firstEntryByTag(entries, "ONE_NS_SET")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected typed server list for ONE_NS_SET, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "ns1.example" || servers[1]["ns"] != "ns2.example" {
		t.Fatalf("expected sorted typed servers for ONE_NS_SET, got %#v", servers)
	}
	if _, ok := entry.Args["nsname_list"]; ok {
		t.Fatalf("legacy key nsname_list should not be present: %#v", entry.Args)
	}
	if hasEntryTag(entries, "INCONSISTENT_NS_TTL") {
		t.Fatalf("did not expect INCONSISTENT_NS_TTL when all servers share one TTL")
	}
}

func TestConsistency04ParallelNSQueries(t *testing.T) {
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

	hook := func(id string, nsNames []string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if strings.EqualFold(qtype, "NS") {
				select {
				case started <- id:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return packet.Packet{}, ctx.Err()
				}
				return nsPacket(qname, nsNames), nil
			}
			return packet.Packet{}, nil
		}
	}

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1", []string{"ns1.example"}))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2", []string{"ns1.example", "ns2.example"}))

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
	var consErr error
	go func() {
		entries, consErr = Consistency04(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel NS queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if consErr != nil {
			t.Fatalf("consistency04: %v", consErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("consistency04 did not finish")
	}

	var order []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "NS_SET" {
			continue
		}
		if name := firstServerName(entry.Args); name != "" {
			order = append(order, name)
		}
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 NS_SET entries, got %v", order)
	}
	if order[0] != "ns1.example" || order[1] != "ns2.example" {
		t.Fatalf("expected deterministic log order, got %v", order)
	}
}

// Two servers serving the same NS names but with different apex NS RRset TTLs must
// raise INCONSISTENT_NS_TTL with the observed min/max bounds, while the name-set
// comparison still reports a single consistent NS set.
func TestConsistency04InconsistentNSTTL(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "NS") {
			return nsPacketTTL("example", []string{"ns1.example", "ns2.example"}, 3600)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "NS") {
			return nsPacketTTL("example", []string{"ns1.example", "ns2.example"}, 86400)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency04(ctx, &z)
	if err != nil {
		t.Fatalf("consistency04: %v", err)
	}

	if !hasEntryTag(entries, "ONE_NS_SET") {
		t.Fatalf("expected ONE_NS_SET for matching NS names")
	}
	entry := firstEntryByTag(entries, "INCONSISTENT_NS_TTL")
	if entry == nil {
		t.Fatalf("expected INCONSISTENT_NS_TTL for differing apex NS RRset TTLs")
	}
	if got := entry.Args["count"]; got != 2 {
		t.Fatalf("expected count=2, got %#v", got)
	}
	if got := entry.Args["ttl_min"]; got != 3600 {
		t.Fatalf("expected ttl_min=3600, got %#v", got)
	}
	if got := entry.Args["ttl_max"]; got != 86400 {
		t.Fatalf("expected ttl_max=86400, got %#v", got)
	}
}

func TestConsistency05AddressesMatch(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := allNSNames
	origM45 := allNameservers
	origParent := queryParentAll
	t.Cleanup(func() {
		allNSNames = origM23
		allNameservers = origM45
		queryParentAll = origParent
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	}

	authNS := newNameserver(t, ctx, "auth.example", "192.0.2.53", func(qname string, qtype string) packet.Packet {
		switch strings.ToUpper(qtype) {
		case "A":
			switch strings.ToLower(qname) {
			case "ns1.example":
				return addrPacket(qname, "A", "192.0.2.1")
			case "ns2.example":
				return addrPacket(qname, "A", "192.0.2.2")
			}
		case "AAAA":
			switch strings.ToLower(qname) {
			case "ns1.example":
				return addrPacket(qname, "AAAA", "2001:db8::1")
			case "ns2.example":
				return addrPacket(qname, "AAAA", "2001:db8::2")
			}
		}
		return packet.Packet{}
	})

	allNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{authNS}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "NS":
			return []packet.Packet{nsPacket(name, []string{"ns1.example", "ns2.example"})}, nil
		case "A":
			switch strings.ToLower(name) {
			case "ns1.example":
				return []packet.Packet{addrPacket(name, "A", "192.0.2.1")}, nil
			case "ns2.example":
				return []packet.Packet{addrPacket(name, "A", "192.0.2.2")}, nil
			}
		case "AAAA":
			switch strings.ToLower(name) {
			case "ns1.example":
				return []packet.Packet{addrPacket(name, "AAAA", "2001:db8::1")}, nil
			case "ns2.example":
				return []packet.Packet{addrPacket(name, "AAAA", "2001:db8::2")}, nil
			}
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if !hasEntryTag(entries, "ADDRESSES_MATCH") {
		t.Fatalf("expected ADDRESSES_MATCH")
	}
}

func TestConsistency05ChildZoneLame(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := allNSNames
	origM45 := allNameservers
	origParent := queryParentAll
	t.Cleanup(func() {
		allNSNames = origM23
		allNameservers = origM45
		queryParentAll = origParent
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}

	nonAAPacket := func() packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	}

	authNS := newNameserver(t, ctx, "auth.example", "192.0.2.53", func(_ string, _ string) packet.Packet {
		return nonAAPacket()
	})

	allNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{authNS}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, _ string, _ string) ([]packet.Packet, error) {
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if !hasEntryTag(entries, "CHILD_NS_FAILED") {
		t.Fatalf("expected CHILD_NS_FAILED")
	}
	entry := firstEntryByTag(entries, "CHILD_NS_FAILED")
	if _, ok := entry.Args["arg_schema"]; ok {
		t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
	}
	if ns, ok := entry.Args["ns"].(string); !ok || ns != "auth.example" {
		t.Fatalf("expected CHILD_NS_FAILED ns=auth.example, got %#v", entry.Args["ns"])
	}
	if ns, _ := entry.Args["ns"].(string); strings.Contains(ns, "/") {
		t.Fatalf("expected nameserver-only ns argument, got %q", ns)
	}
	if address, ok := entry.Args["address"].(string); !ok || address != "192.0.2.53" {
		t.Fatalf("expected CHILD_NS_FAILED address=192.0.2.53, got %#v", entry.Args["address"])
	}
	if !hasEntryTag(entries, "CHILD_ZONE_LAME") {
		t.Fatalf("expected CHILD_ZONE_LAME")
	}
}

func TestConsistency05InBailiwickMismatch(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := allNSNames
	origM45 := allNameservers
	origParent := queryParentAll
	t.Cleanup(func() {
		allNSNames = origM23
		allNameservers = origM45
		queryParentAll = origParent
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}

	authNS := newNameserver(t, ctx, "auth.example", "192.0.2.53", func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "A") && strings.EqualFold(qname, "ns1.example") {
			return addrPacket(qname, "A", "192.0.2.2")
		}
		return packet.Packet{}
	})

	allNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{authNS}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "NS":
			return []packet.Packet{nsPacket(name, []string{"ns1.example"})}, nil
		case "A":
			if strings.EqualFold(name, "ns1.example") {
				return []packet.Packet{addrPacket(name, "A", "192.0.2.1")}, nil
			}
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if !hasEntryTag(entries, "IN_BAILIWICK_ADDR_MISMATCH") {
		t.Fatalf("expected IN_BAILIWICK_ADDR_MISMATCH")
	}
	mismatch := firstEntryByTag(entries, "IN_BAILIWICK_ADDR_MISMATCH")
	if mismatch == nil {
		t.Fatalf("missing IN_BAILIWICK_ADDR_MISMATCH entry")
	}
	parent := serverEndpointsAtKey(mismatch.Args, "parent_servers")
	if len(parent) != 1 || parent[0] != "ns1.example/192.0.2.1" {
		t.Fatalf("expected parent_servers [ns1.example/192.0.2.1], got %v", parent)
	}
	zone := serverEndpointsAtKey(mismatch.Args, "zone_servers")
	if len(zone) != 1 || zone[0] != "ns1.example/192.0.2.2" {
		t.Fatalf("expected zone_servers [ns1.example/192.0.2.2], got %v", zone)
	}
	if _, ok := mismatch.Args["parent_addresses"]; ok {
		t.Fatalf("legacy key parent_addresses should not be present: %#v", mismatch.Args)
	}
	if _, ok := mismatch.Args["zone_addresses"]; ok {
		t.Fatalf("legacy key zone_addresses should not be present: %#v", mismatch.Args)
	}
	if !hasEntryTag(entries, "EXTRA_ADDRESS_CHILD") {
		t.Fatalf("expected EXTRA_ADDRESS_CHILD")
	}
	entry := firstEntryByTag(entries, "EXTRA_ADDRESS_CHILD")
	if entry == nil {
		t.Fatalf("missing EXTRA_ADDRESS_CHILD entry")
	}
	addresses, ok := entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 1 || addresses[0] != "192.0.2.2" {
		t.Fatalf("expected typed addresses [192.0.2.2], got %#v", entry.Args["addresses"])
	}
	if _, ok := entry.Args["ns_ip_list"]; ok {
		t.Fatalf("legacy key ns_ip_list should not be present: %#v", entry.Args)
	}
}

func TestConsistency05DisjointParentChildNSDoesNotReportLame(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := allNSNames
	origM45 := allNameservers
	origParent := queryParentAll
	t.Cleanup(func() {
		allNSNames = origM23
		allNameservers = origM45
		queryParentAll = origParent
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example")}, nil
	}

	newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, "example") && strings.EqualFold(qtype, "NS") {
			return nsPacket(qname, []string{"ns2.example"})
		}
		if strings.EqualFold(qname, "ns1.example") {
			return nxdomainPacket(qname)
		}
		if strings.EqualFold(qname, "ns2.example") {
			switch strings.ToUpper(qtype) {
			case "A":
				return addrPacket(qname, "A", "192.0.2.2")
			case "AAAA":
				return addrPacket(qname, "AAAA", "2001:db8::2")
			}
		}
		return packet.Packet{}
	})

	allNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "NS":
			return []packet.Packet{nsPacket(name, []string{"ns1.example"})}, nil
		case "A":
			if strings.EqualFold(name, "ns1.example") {
				return []packet.Packet{addrPacket(name, "A", "192.0.2.1")}, nil
			}
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if hasEntryTag(entries, "CHILD_ZONE_LAME") {
		t.Fatalf("did not expect CHILD_ZONE_LAME")
	}
	if !hasEntryTag(entries, "IN_BAILIWICK_ADDR_MISMATCH") {
		t.Fatalf("expected IN_BAILIWICK_ADDR_MISMATCH")
	}
	if !hasEntryTag(entries, "EXTRA_ADDRESS_CHILD") {
		t.Fatalf("expected EXTRA_ADDRESS_CHILD")
	}
}

func TestConsistency05OutOfBailiwickMismatch(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM23 := allNSNames
	origM45 := allNameservers
	origParent := queryParentAll
	origRecurse := recurse
	t.Cleanup(func() {
		allNSNames = origM23
		allNameservers = origM45
		queryParentAll = origParent
		recurse = origRecurse
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{}, nil
	}
	allNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "NS":
			return []packet.Packet{nsPacket(name, []string{"ns1.other"})}, nil
		case "A":
			if strings.EqualFold(name, "ns1.other") {
				return []packet.Packet{addrPacket(name, "A", "192.0.2.1")}, nil
			}
		}
		return []packet.Packet{}, nil
	}

	recurse = func(_ context.Context, _ *zone.Zone, _ string, _ string) (packet.Packet, error) {
		return packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	if !hasEntryTag(entries, "OUT_OF_BAILIWICK_ADDR_MISMATCH") {
		t.Fatalf("expected OUT_OF_BAILIWICK_ADDR_MISMATCH")
	}
	mismatch := firstEntryByTag(entries, "OUT_OF_BAILIWICK_ADDR_MISMATCH")
	if mismatch == nil {
		t.Fatalf("missing OUT_OF_BAILIWICK_ADDR_MISMATCH entry")
	}
	parent := serverEndpointsAtKey(mismatch.Args, "parent_servers")
	if len(parent) != 1 || parent[0] != "ns1.other/192.0.2.1" {
		t.Fatalf("expected parent_servers [ns1.other/192.0.2.1], got %v", parent)
	}
	if zone := serverEndpointsAtKey(mismatch.Args, "zone_servers"); len(zone) != 0 {
		t.Fatalf("expected empty zone_servers for OOB mismatch, got %v", zone)
	}
	if _, ok := mismatch.Args["parent_addresses"]; ok {
		t.Fatalf("legacy key parent_addresses should not be present: %#v", mismatch.Args)
	}
	if _, ok := mismatch.Args["zone_addresses"]; ok {
		t.Fatalf("legacy key zone_addresses should not be present: %#v", mismatch.Args)
	}
}

// A glueless out-of-bailiwick delegation must yield ADDRESSES_MATCH, not a
// spurious OUT_OF_BAILIWICK_ADDR_MISMATCH. gonemaster only compares glue the
// parent actually returns, so an empty parent side never fabricates localhost
// glue. Mirrors upstream consistency05 scenarios ADDRESSES-MATCH-8/9
// (zonemaster-engine#1537).
func TestConsistency05GluelessOOBAddressesMatch(t *testing.T) {
	addrHandler := func(name, v4, v6 string) func(string, string) packet.Packet {
		return func(qname, qtype string) packet.Packet {
			if !strings.EqualFold(qname, name) {
				return packet.Packet{}
			}
			switch strings.ToUpper(qtype) {
			case "A":
				return addrPacket(qname, "A", v4)
			case "AAAA":
				return addrPacket(qname, "AAAA", v6)
			}
			return packet.Packet{}
		}
	}

	scenarios := []struct {
		name string
		zone string
		ns41 string
		ns42 string
	}{
		{"ADDRESSES-MATCH-8", "child.a.b.addresses-match-8.consistency05.xa", "ns41.child.a.b.addresses-match-8.consistency05.xb", "ns42.child.a.b.addresses-match-8.consistency05.xb"},
		{"ADDRESSES-MATCH-9", "child.addresses-match-9.consistency05.xa", "ns41.child.addresses-match-9.consistency05.xb", "ns42.child.addresses-match-9.consistency05.xb"},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			ctx := testCtx()
			t.Cleanup(profile.ResetEffective)

			util.SetLogger(logger.New())
			t.Cleanup(func() { util.SetLogger(nil) })

			origNames := allNSNames
			origNS := allNameservers
			origParent := queryParentAll
			origRecurse := recurse
			t.Cleanup(func() {
				allNSNames = origNames
				allNameservers = origNS
				queryParentAll = origParent
				recurse = origRecurse
			})

			// Child NS names are out-of-bailiwick under the xb tree.
			allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
				return []dnsname.Name{dnsname.New(sc.ns41), dnsname.New(sc.ns42)}, nil
			}

			ns41 := newNameserver(t, ctx, sc.ns41, "127.14.5.41", addrHandler(sc.ns41, "127.14.5.41", "fda1:b2:c3:0:127:14:5:41"))
			ns42 := newNameserver(t, ctx, sc.ns42, "127.14.5.42", addrHandler(sc.ns42, "127.14.5.42", "fda1:b2:c3:0:127:14:5:42"))
			allNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
				return []nameserver.Nameserver{ns41, ns42}, nil
			}

			// Glueless delegation: the parent returns NS records but no glue.
			queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
				if strings.EqualFold(qtype, "NS") {
					return []packet.Packet{nsPacket(name, []string{sc.ns41, sc.ns42})}, nil
				}
				return []packet.Packet{}, nil
			}

			recurse = func(_ context.Context, _ *zone.Zone, _ string, _ string) (packet.Packet, error) {
				return packet.Packet{}, nil
			}

			z := zone.Zone{Name: dnsname.New(sc.zone)}
			entries, err := Consistency05(ctx, &z)
			if err != nil {
				t.Fatalf("consistency05: %v", err)
			}
			if !hasEntryTag(entries, "ADDRESSES_MATCH") {
				t.Fatalf("expected ADDRESSES_MATCH for glueless OOB delegation")
			}
			if hasEntryTag(entries, "OUT_OF_BAILIWICK_ADDR_MISMATCH") {
				t.Fatalf("unexpected OUT_OF_BAILIWICK_ADDR_MISMATCH for glueless OOB delegation")
			}
		})
	}
}

func TestConsistency06MultipleMnames(t *testing.T) {
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

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "mname1.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string) packet.Packet {
		if strings.EqualFold(qtype, "SOA") {
			return soaPacket("example", 1, "mname2.example", "hostmaster.example", 3600, 600, 86400, 60)
		}
		return packet.Packet{}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns2}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency06(ctx, &z)
	if err != nil {
		t.Fatalf("consistency06: %v", err)
	}
	if !hasEntryTag(entries, "MULTIPLE_SOA_MNAMES") {
		t.Fatalf("expected MULTIPLE_SOA_MNAMES")
	}
	if !hasEntryTag(entries, "SOA_MNAME") {
		t.Fatalf("expected SOA_MNAME")
	}
	entry := firstEntryByTag(entries, "SOA_MNAME")
	if entry == nil {
		t.Fatalf("missing SOA_MNAME entry")
	}
	if _, ok := entry.Args["servers"]; !ok {
		t.Fatalf("expected typed servers in SOA_MNAME args: %#v", entry.Args)
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
}

func testCtx() context.Context {
	return nameserver.WithCache(context.Background(), nameserver.NewCacheStore())
}

func newNameserver(t *testing.T, ctx context.Context, name string, ip string, handler func(qname string, qtype string) packet.Packet) nameserver.Nameserver {
	t.Helper()

	ns, err := nameserver.NewWithContext(ctx, name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(_ string, _ string) packet.Packet {
			return packet.Packet{}
		}
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype), nil
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

func firstServerName(args map[string]any) string {
	if args == nil {
		return ""
	}
	switch servers := args["servers"].(type) {
	case []map[string]any:
		if len(servers) == 0 {
			return ""
		}
		name, _ := servers[0]["ns"].(string)
		return name
	case []any:
		if len(servers) == 0 {
			return ""
		}
		item, _ := servers[0].(map[string]any)
		name, _ := item["ns"].(string)
		return name
	default:
		return ""
	}
}

func serverEndpointsAtKey(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok {
		return nil
	}
	toEndpoint := func(ns string, address string) string {
		ns = strings.TrimSpace(ns)
		address = strings.TrimSpace(address)
		switch {
		case ns != "" && address != "":
			return ns + "/" + address
		case ns != "":
			return ns
		case address != "":
			return address
		default:
			return ""
		}
	}
	var out []string
	switch items := raw.(type) {
	case []map[string]any:
		for _, item := range items {
			ns, _ := item["ns"].(string)
			address, _ := item["address"].(string)
			if endpoint := toEndpoint(ns, address); endpoint != "" {
				out = append(out, endpoint)
			}
		}
	case []any:
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			ns, _ := item["ns"].(string)
			address, _ := item["address"].(string)
			if endpoint := toEndpoint(ns, address); endpoint != "" {
				out = append(out, endpoint)
			}
		}
	}
	sort.Strings(out)
	return out
}

func soaPacket(owner string, serial uint32, mname string, rname string, refresh uint32, retry uint32, expire uint32, minimum uint32) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = dnsutil.Fqdn(mname)
	soaRR.Mbox = dnsutil.Fqdn(rname)
	soaRR.Serial = serial
	soaRR.Refresh = refresh
	soaRR.Retry = retry
	soaRR.Expire = expire
	soaRR.Minttl = minimum
	msg.Answer = []dns.RR{soaRR}
	return packet.Packet{Msg: msg}
}

func nsPacket(owner string, nsNames []string) packet.Packet {
	return nsPacketTTL(owner, nsNames, 60)
}

func nsPacketTTL(owner string, nsNames []string, ttl uint32) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	for _, nsName := range nsNames {
		nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: ttl}}
		nsRR.Ns = dnsutil.Fqdn(nsName)
		msg.Answer = append(msg.Answer, nsRR)
	}
	return packet.Packet{Msg: msg}
}

func addrPacket(name string, qtype string, address string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true

	addr, err := netip.ParseAddr(address)
	if err != nil {
		return packet.Packet{}
	}
	switch strings.ToUpper(qtype) {
	case "A":
		aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}}
		aRR.Addr = addr.Unmap()
		msg.Answer = []dns.RR{aRR}
	case "AAAA":
		aaaaRR := &dns.AAAA{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}}
		aaaaRR.Addr = addr
		msg.Answer = []dns.RR{aaaaRR}
	default:
		return packet.Packet{}
	}
	return packet.Packet{Msg: msg}
}

func nxdomainPacket(name string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeNameError
	msg.Authoritative = true
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = dnsutil.Fqdn("ns1.example")
	soaRR.Mbox = dnsutil.Fqdn("hostmaster.example")
	msg.Ns = []dns.RR{soaRR}
	return packet.Packet{Msg: msg}
}
