package consistency

import (
	"context"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
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
	tctest.RequireTags(t, entries, "MULTIPLE_SOA_SERIALS", "SOA_SERIAL_VARIATION", "SOA_SERIAL")
	entry := tctest.RequireTag(t, entries, "SOA_SERIAL")
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

	entry := tctest.RequireTag(t, entries, "SOA_SERIAL_VARIATION")
	if got := entry.Args["serial_min"]; got != "9" {
		t.Fatalf("expected serial_min=9 (oldest), got %#v", got)
	}
	if got := entry.Args["serial_max"]; got != "100" {
		t.Fatalf("expected serial_max=100 (newest), got %#v", got)
	}
	behind := tctest.EndpointsAt(entry.Args, "servers_behind")
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

	entry := tctest.RequireTag(t, entries, "SOA_SERIAL_VARIATION")
	if got := entry.Args["serial_min"]; got != "4294967294" {
		t.Fatalf("expected serial_min=4294967294 (oldest under RFC 1982), got %#v", got)
	}
	if got := entry.Args["serial_max"]; got != "1" {
		t.Fatalf("expected serial_max=1 (newest under RFC 1982), got %#v", got)
	}
	behind := tctest.EndpointsAt(entry.Args, "servers_behind")
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
	tctest.RequireTags(t, entries, "MULTIPLE_SOA_RNAMES", "SOA_RNAME")
	entry := tctest.RequireTag(t, entries, "SOA_RNAME")
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
	tctest.RequireTags(t, entries, "MULTIPLE_SOA_TIME_PARAMETER_SET", "SOA_TIME_PARAMETER_SET")
	entry := tctest.RequireTag(t, entries, "SOA_TIME_PARAMETER_SET")
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
	tctest.RequireTags(t, entries, "MULTIPLE_NS_SET", "NS_SET")
	entry := tctest.RequireTag(t, entries, "NS_SET")
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
	entry := tctest.RequireTag(t, entries, "ONE_NS_SET")
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
	tctest.RequireNoTag(t, entries, "INCONSISTENT_NS_TTL")
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
		if name := tctest.FirstServerName(entry.Args); name != "" {
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

	tctest.RequireTags(t, entries, "ONE_NS_SET")
	entry := tctest.RequireTag(t, entries, "INCONSISTENT_NS_TTL")
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
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{nsPacketWithGlue(name, map[string][]string{
				"ns1.example": {"192.0.2.1", "2001:db8::1"},
				"ns2.example": {"192.0.2.2", "2001:db8::2"},
			})}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
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
	entry := tctest.RequireTag(t, entries, "CHILD_NS_FAILED")
	tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "auth.example", Address: "192.0.2.53"})
	tctest.RequireTags(t, entries, "CHILD_ZONE_LAME")
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
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{nsPacketWithGlue(name, map[string][]string{
				"ns1.example": {"192.0.2.1"},
			})}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	mismatch := tctest.RequireTag(t, entries, "IN_DOMAIN_ADDR_MISMATCH")
	if got := mismatch.Args["ns"]; got != "ns1.example" {
		t.Fatalf("expected ns=ns1.example, got %#v", got)
	}
	parent := tctest.EndpointsAt(mismatch.Args, "parent_servers")
	if len(parent) != 1 || parent[0] != "ns1.example/192.0.2.1" {
		t.Fatalf("expected parent_servers [ns1.example/192.0.2.1], got %v", parent)
	}
	zone := tctest.EndpointsAt(mismatch.Args, "zone_servers")
	if len(zone) != 1 || zone[0] != "ns1.example/192.0.2.2" {
		t.Fatalf("expected zone_servers [ns1.example/192.0.2.2], got %v", zone)
	}
	if _, ok := mismatch.Args["parent_addresses"]; ok {
		t.Fatalf("legacy key parent_addresses should not be present: %#v", mismatch.Args)
	}
	if _, ok := mismatch.Args["zone_addresses"]; ok {
		t.Fatalf("legacy key zone_addresses should not be present: %#v", mismatch.Args)
	}
	entry := tctest.RequireTag(t, entries, "EXTRA_ADDRESS_CHILD")
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
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{nsPacketWithGlue(name, map[string][]string{
				"ns1.example": {"192.0.2.1"},
			})}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "CHILD_ZONE_LAME")
	// The parent glues ns1 and the child answers NXDOMAIN for it, so the
	// child serves no address for a glued name. That is a missing record,
	// not glue pointing at the wrong address.
	missing := tctest.RequireTag(t, entries, "MISSING_ADDRESS_CHILD")
	if got := missing.Args["ns"]; got != "ns1.example" {
		t.Fatalf("expected ns=ns1.example, got %#v", got)
	}
	tctest.RequireNoTag(t, entries, "IN_DOMAIN_ADDR_MISMATCH")
	// ns2 is served only by the child and carries no glue, so it is outside
	// the address comparison. The parent/child NS disagreement is reported
	// by the NS set comparison, not as an extra address here.
	tctest.RequireNoTag(t, entries, "EXTRA_ADDRESS_CHILD")
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
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{nsPacketWithGlue(name, map[string][]string{
				"ns1.other": {"192.0.2.1"},
			})}, nil
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
	mismatch := tctest.RequireTag(t, entries, "NOT_IN_DOMAIN_ADDR_MISMATCH")
	parent := tctest.EndpointsAt(mismatch.Args, "parent_servers")
	if len(parent) != 1 || parent[0] != "ns1.other/192.0.2.1" {
		t.Fatalf("expected parent_servers [ns1.other/192.0.2.1], got %v", parent)
	}
	if zone := tctest.EndpointsAt(mismatch.Args, "zone_servers"); len(zone) != 0 {
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
// spurious NOT_IN_DOMAIN_ADDR_MISMATCH. gonemaster only compares glue the
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

			// Glueless delegation: the parent refers but carries no glue.
			queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
				if strings.EqualFold(qtype, "NS") {
					return []packet.Packet{referralPacket(name, []string{sc.ns41, sc.ns42})}, nil
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
			tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
			tctest.RequireNoTag(t, entries, "NOT_IN_DOMAIN_ADDR_MISMATCH")
		})
	}
}

// A parent that answers direct out-of-domain address queries with loopback (a
// catch-all or wildcard zone) must not poison the glue set. Glue is read only
// from the referral additional section, so a glueless referral yields no
// out-of-domain glue and no spurious mismatch, even though the parent would
// answer ns1.other/A with 127.0.0.1 (zonemaster-engine#1537).
func TestConsistency05OutOfDomainParentLoopbackNoMismatch(t *testing.T) {
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

	// Glueless referral, but the parent answers direct out-of-domain address
	// queries with loopback. The referral additional section carries no glue.
	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "NS":
			return []packet.Packet{referralPacket(name, []string{"ns1.other"})}, nil
		case "A":
			if strings.EqualFold(name, "ns1.other") {
				return []packet.Packet{addrPacket(name, "A", "127.0.0.1")}, nil
			}
		case "AAAA":
			if strings.EqualFold(name, "ns1.other") {
				return []packet.Packet{addrPacket(name, "AAAA", "::1")}, nil
			}
		}
		return []packet.Packet{}, nil
	}

	// Public resolution returns the real addresses, not loopback.
	recurse = func(_ context.Context, _ *zone.Zone, _ string, qtype string) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			return addrPacket("ns1.other", "A", "192.0.2.9"), nil
		case "AAAA":
			return addrPacket("ns1.other", "AAAA", "2001:db8::9"), nil
		}
		return packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "NOT_IN_DOMAIN_ADDR_MISMATCH")
	tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
}

// stubConsistency05Child installs the child-side stubs shared by the
// delegation NS-set tests: the child zone knows ns1.example and ns2.example,
// and one authoritative child server answers their A/AAAA lookups with the
// standard test addresses. queryParentAll is restored on cleanup as well;
// each test installs its own parent responses.
func stubConsistency05Child(t *testing.T, ctx context.Context) {
	t.Helper()

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
}

// fullTestGlue is the glue set matching what stubConsistency05Child's
// authoritative child server answers.
func fullTestGlue() map[string][]string {
	return map[string][]string{
		"ns1.example": {"192.0.2.1", "2001:db8::1"},
		"ns2.example": {"192.0.2.2", "2001:db8::2"},
	}
}

// glueParentPacket builds one parent referral response with glue and a
// parent server identity.
func glueParentPacket(owner string, glue map[string][]string, answerFrom string) packet.Packet {
	p := nsPacketWithGlue(owner, glue)
	p.AnswerFrom = answerFrom
	return p
}

func TestConsistency05DelegationNSSetConsistentParents(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, fullTestGlue(), "192.0.2.101"),
				glueParentPacket(name, fullTestGlue(), "192.0.2.102"),
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	// Two parents serving the identical delegation must stay silent.
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET", "DELEGATION_NS_SET")
	tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
}

func TestConsistency05DelegationNSSetInconsistentParents(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	// Parent one serves the full delegation plus a glueless out-of-domain
	// name; parent two only knows ns1. Both respond, so both are part of
	// the comparison and two distinct sets must be reported.
	glueOne := fullTestGlue()
	glueOne["ns3.other.test"] = nil
	glueTwo := map[string][]string{
		"ns1.example": {"192.0.2.1", "2001:db8::1"},
	}
	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, glueOne, "192.0.2.101"),
				glueParentPacket(name, glueTwo, "192.0.2.102"),
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}

	multiple := tctest.RequireTag(t, entries, "MULTIPLE_DELEGATION_NS_SET")
	if count, ok := multiple.Args["count"].(int); !ok || count != 2 {
		t.Fatalf("expected count=2, got %#v", multiple.Args["count"])
	}
	// ns1 is served by both parents, so only the two names parent two
	// omits are named as the disagreement.
	names, ok := multiple.Args["ns_names"].([]string)
	if !ok {
		t.Fatalf("expected ns_names, got %#v", multiple.Args["ns_names"])
	}
	if len(names) != 2 || names[0] != "ns2.example" || names[1] != "ns3.other.test" {
		t.Fatalf("unexpected ns_names: %#v", names)
	}

	var sets []*logger.Entry
	for _, entry := range entries {
		if entry != nil && entry.Tag == "DELEGATION_NS_SET" {
			sets = append(sets, entry)
		}
	}
	if len(sets) != 2 {
		t.Fatalf("expected 2 DELEGATION_NS_SET entries, got %d", len(sets))
	}

	// First set (first-seen order): the three delegated names, served by
	// parent one. Elements are names only, so the glue-backed and the
	// glueless name are rendered alike.
	first, ok := sets[0].Args["ns_set_servers"].([]map[string]any)
	if !ok || len(first) != 3 {
		t.Fatalf("expected 3 typed ns_set_servers elements, got %#v", sets[0].Args["ns_set_servers"])
	}
	if first[0]["ns"] != "ns1.example" {
		t.Fatalf("unexpected first element: %#v", first[0])
	}
	if first[2]["ns"] != "ns3.other.test" {
		t.Fatalf("expected ns3.other.test last, got %#v", first[2])
	}
	for _, element := range first {
		if addr, ok := element["address"].(string); ok && addr != "" {
			t.Fatalf("delegation set element must carry no address, got %#v", element)
		}
	}
	servers, ok := sets[0].Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["address"] != "192.0.2.101" {
		t.Fatalf("expected first set served by 192.0.2.101, got %#v", sets[0].Args["servers"])
	}

	// Second set: only ns1, served by parent two.
	second, ok := sets[1].Args["ns_set_servers"].([]map[string]any)
	if !ok || len(second) != 1 {
		t.Fatalf("expected 1 typed ns_set_servers element, got %#v", sets[1].Args["ns_set_servers"])
	}
	servers, ok = sets[1].Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["address"] != "192.0.2.102" {
		t.Fatalf("expected second set served by 192.0.2.102, got %#v", sets[1].Args["servers"])
	}
}

func TestConsistency05DelegationNSSetGlueDifference(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	// Same NS names on both parents, one glue address differs. The
	// delegation is the NS name set, so this is one set and no warning.
	// Differing addresses for the same name are an address fault, reported
	// by the glue comparison, not a delegation disagreement.
	glueTwo := map[string][]string{
		"ns1.example": {"192.0.2.1", "2001:db8::1"},
		"ns2.example": {"192.0.2.99", "2001:db8::2"},
	}
	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, fullTestGlue(), "192.0.2.101"),
				glueParentPacket(name, glueTwo, "192.0.2.102"),
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET", "DELEGATION_NS_SET")
}

// The reported false positive: every root serves the same ten NS names for
// se, but each trims the additional section its own way at 512 bytes, so the
// four observed glue shapes used to read as four delegations. Group sizes are
// taken from the recorded run: 20, 16, 13 and 15 address records.
func TestConsistency05DelegationNSSetTrimmedGlueIsOneDelegation(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	full := fullTestGlue()
	ipv4Only := map[string][]string{
		"ns1.example": {"192.0.2.1"},
		"ns2.example": {"192.0.2.2"},
	}
	ipv6Only := map[string][]string{
		"ns1.example": {"2001:db8::1"},
		"ns2.example": {"2001:db8::2"},
	}
	// The harshest trim: one name loses every address, as seven root
	// addresses do for four of the thirteen gtld-servers.net names.
	partial := map[string][]string{
		"ns1.example": {"192.0.2.1", "2001:db8::1"},
		"ns2.example": nil,
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, full, "192.0.2.101"),
				glueParentPacket(name, ipv4Only, "192.0.2.102"),
				glueParentPacket(name, ipv6Only, "192.0.2.103"),
				glueParentPacket(name, partial, "192.0.2.104"),
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET")
	// Every parent carries both names, so the union of glue still matches
	// the child and the address comparison stays clean.
	tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
}

func TestConsistency05DelegationNSSetIgnoresNonRespondingParent(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	// The middle parent never responds (zero packet). A server that does
	// not respond is not part of any set, so the two agreeing parents make
	// the delegation consistent and nothing may be emitted.
	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, fullTestGlue(), "192.0.2.101"),
				{},
				glueParentPacket(name, fullTestGlue(), "192.0.2.103"),
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET", "DELEGATION_NS_SET")
}

func TestConsistency05DelegationNSSetIgnoresParentWithoutNSRecords(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	// One parent responds but without NS records for the child zone. Such
	// a response contributes no set either, so the comparison sees one
	// distinct set and stays silent.
	emptyResponse := func() packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg, AnswerFrom: "192.0.2.102"}
	}
	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, fullTestGlue(), "192.0.2.101"),
				emptyResponse(),
				glueParentPacket(name, fullTestGlue(), "192.0.2.103"),
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET", "DELEGATION_NS_SET")
}

// Without an advertised EDNS payload size the parent caps the referral at 512
// bytes and trims glue, which leaves the glue union dependent on each parent
// implementation trimming differently. RFC 9471 section 3.1 requires a
// compliant parent to either fit all in-domain glue or set TC, and at 1232
// the referral fits.
func TestParentReferralQueryAdvertisesFullPayload(t *testing.T) {
	opts := parentReferralQueryOptions()
	if opts == nil || opts.EDNSSize == nil {
		t.Fatalf("expected an advertised EDNS payload size, got %#v", opts)
	}
	if *opts.EDNSSize != constants.EDNSUDPPayloadDNSSECDefault {
		t.Fatalf("EDNSSize = %d, want %d", *opts.EDNSSize, constants.EDNSUDPPayloadDNSSECDefault)
	}
	if *opts.EDNSSize <= constants.EDNSUDPPayloadDefault {
		t.Fatalf("advertised size %d does not exceed the 512-byte default", *opts.EDNSSize)
	}
}

// Wrong glue is reported per name, so a zone with several faulty names gets
// one entry each, naming only that name's unconfirmed addresses. This is the
// change from a single aggregate entry that dumped every address at once.
func TestConsistency05InBailiwickMismatchIsPerName(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origNames := allNSNames
	origNS := allNameservers
	origParent := queryParentAll
	t.Cleanup(func() {
		allNSNames = origNames
		allNameservers = origNS
		queryParentAll = origParent
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	}

	// The child serves .11 and .12; the parent glues .1 and .2 instead, so
	// both names carry glue the child does not confirm.
	authNS := newNameserver(t, ctx, "auth.example", "192.0.2.53", func(qname string, qtype string) packet.Packet {
		if !strings.EqualFold(qtype, "A") {
			return packet.Packet{}
		}
		switch strings.ToLower(qname) {
		case "ns1.example":
			return addrPacket(qname, "A", "192.0.2.11")
		case "ns2.example":
			return addrPacket(qname, "A", "192.0.2.12")
		}
		return packet.Packet{}
	})
	allNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{authNS}, nil
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{glueParentPacket(name, map[string][]string{
				"ns1.example": {"192.0.2.1"},
				"ns2.example": {"192.0.2.2"},
			}, "192.0.2.101")}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}

	var mismatches []*logger.Entry
	for _, entry := range entries {
		if entry != nil && entry.Tag == "IN_DOMAIN_ADDR_MISMATCH" {
			mismatches = append(mismatches, entry)
		}
	}
	if len(mismatches) != 2 {
		t.Fatalf("expected one entry per affected name, got %d", len(mismatches))
	}

	want := map[string]string{"ns1.example": "ns1.example/192.0.2.1", "ns2.example": "ns2.example/192.0.2.2"}
	for _, entry := range mismatches {
		nsName, _ := entry.Args["ns"].(string)
		wantEndpoint, ok := want[nsName]
		if !ok {
			t.Fatalf("unexpected ns %#v", entry.Args["ns"])
		}
		delete(want, nsName)
		parent := tctest.EndpointsAt(entry.Args, "parent_servers")
		if len(parent) != 1 || parent[0] != wantEndpoint {
			t.Fatalf("expected parent_servers [%s], got %v", wantEndpoint, parent)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing entries for %v", want)
	}
}

// A name whose glue every parent trimmed cannot be told from one that never
// had glue, so it stays out of the address comparison. Reporting it would
// turn lawful trimming into a finding, and Delegation01 already reports glue
// missing from the delegation.
func TestConsistency05NameWithoutGlueIsNotCompared(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origNames := allNSNames
	origNS := allNameservers
	origParent := queryParentAll
	t.Cleanup(func() {
		allNSNames = origNames
		allNameservers = origNS
		queryParentAll = origParent
	})

	allNSNames = func(_ context.Context, _ *zone.Zone) ([]dnsname.Name, error) {
		return []dnsname.Name{dnsname.New("ns1.example"), dnsname.New("ns2.example")}, nil
	}

	authNS := newNameserver(t, ctx, "auth.example", "192.0.2.53", func(qname string, qtype string) packet.Packet {
		if !strings.EqualFold(qtype, "A") {
			return packet.Packet{}
		}
		switch strings.ToLower(qname) {
		case "ns1.example":
			return addrPacket(qname, "A", "192.0.2.1")
		case "ns2.example":
			return addrPacket(qname, "A", "192.0.2.2")
		}
		return packet.Packet{}
	})
	allNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{authNS}, nil
	}

	// The referral delegates both names but glues only ns1. The child
	// serves an address for ns2 that no glue mentions.
	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{glueParentPacket(name, map[string][]string{
				"ns1.example": {"192.0.2.1"},
				"ns2.example": nil,
			}, "192.0.2.101")}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "EXTRA_ADDRESS_CHILD", "IN_DOMAIN_ADDR_MISMATCH", "MISSING_ADDRESS_CHILD")
	tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
}

// Trimming removes records from the union but never adds any, so a glue
// subset spread across parents still reconstructs the full set and must not
// produce an address finding.
func TestConsistency05TrimmedGlueUnionMatchesChild(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	// No parent carries the whole delegation glue; together they do.
	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, map[string][]string{
					"ns1.example": {"192.0.2.1", "2001:db8::1"},
					"ns2.example": nil,
				}, "192.0.2.101"),
				glueParentPacket(name, map[string][]string{
					"ns1.example": nil,
					"ns2.example": {"192.0.2.2", "2001:db8::2"},
				}, "192.0.2.102"),
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	for _, tag := range []string{"IN_DOMAIN_ADDR_MISMATCH", "MISSING_ADDRESS_CHILD", "EXTRA_ADDRESS_CHILD", "MULTIPLE_DELEGATION_NS_SET"} {
		tctest.RequireNoTag(t, entries, tag)
	}
	tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
}

// ns_names names the disputed names only: the union of the observed sets
// minus their intersection. Names every parent serves are not the problem and
// would only pad the message.
func TestDisagreeingNSNames(t *testing.T) {
	cases := []struct {
		name    string
		setKeys []string
		want    []string
	}{
		{
			name:    "one parent omits a name",
			setKeys: []string{"ns1.example;ns2.example", "ns1.example"},
			want:    []string{"ns2.example"},
		},
		{
			name:    "disjoint sets put every name in dispute",
			setKeys: []string{"ns1.example", "ns2.example"},
			want:    []string{"ns1.example", "ns2.example"},
		},
		{
			name:    "shared names are excluded, result is sorted",
			setKeys: []string{"a.example;shared.example", "shared.example;b.example"},
			want:    []string{"a.example", "b.example"},
		},
		{
			name:    "three sets, a name missing from just one is in dispute",
			setKeys: []string{"ns1.example;ns2.example", "ns1.example;ns2.example;ns3.example", "ns1.example;ns2.example"},
			want:    []string{"ns3.example"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := disagreeingNSNames(tc.setKeys)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// The warning is emitted if and only if ns_names is non-empty: distinct sets
// must differ by at least one name, and identical sets collapse to one key.
func TestConsistency05MultipleDelegationNSSetImpliesNSNames(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	cases := []struct {
		name        string
		second      map[string][]string
		wantWarning bool
	}{
		{
			name:        "same names, trimmed glue",
			second:      map[string][]string{"ns1.example": {"192.0.2.1"}, "ns2.example": nil},
			wantWarning: false,
		},
		{
			name:        "a name is missing",
			second:      map[string][]string{"ns1.example": {"192.0.2.1", "2001:db8::1"}},
			wantWarning: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
				if strings.EqualFold(qtype, "NS") {
					return []packet.Packet{
						glueParentPacket(name, fullTestGlue(), "192.0.2.101"),
						glueParentPacket(name, tc.second, "192.0.2.102"),
					}, nil
				}
				return []packet.Packet{}, nil
			}

			z := zone.Zone{Name: dnsname.New("example")}
			entries, err := Consistency05(ctx, &z)
			if err != nil {
				t.Fatalf("consistency05: %v", err)
			}

			multiple := tctest.First(entries, "MULTIPLE_DELEGATION_NS_SET")
			if got := multiple != nil; got != tc.wantWarning {
				t.Fatalf("warning emitted = %v, want %v", got, tc.wantWarning)
			}
			if multiple == nil {
				return
			}
			names, ok := multiple.Args["ns_names"].([]string)
			if !ok || len(names) == 0 {
				t.Fatalf("warning must carry a non-empty ns_names, got %#v", multiple.Args["ns_names"])
			}
		})
	}
}

// A parent that also serves the child zone answers authoritatively instead of
// referring. arpa is the live case: the root servers serve it directly, and
// each trims the additional section of that answer differently. Such a
// response is not a referral and must not take part in the comparison.
func TestConsistency05DelegationNSSetIgnoresAuthoritativeParent(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	// Two parents answer from the answer section with AA set, carrying
	// differently trimmed glue. Both are ignored, so nothing is compared.
	authoritative := func(owner string, glue map[string][]string, answerFrom string) packet.Packet {
		names := make([]string, 0, len(glue))
		for name := range glue {
			names = append(names, name)
		}
		sort.Strings(names)
		p := nsPacket(owner, names)
		for _, name := range names {
			for _, address := range glue[name] {
				ip, err := netip.ParseAddr(address)
				if err != nil {
					continue
				}
				hdr := dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}
				if ip.Is4() {
					aRR := &dns.A{Hdr: hdr}
					aRR.Addr = ip.Unmap()
					p.Msg.Extra = append(p.Msg.Extra, aRR)
				} else {
					aaaaRR := &dns.AAAA{Hdr: hdr}
					aaaaRR.Addr = ip
					p.Msg.Extra = append(p.Msg.Extra, aaaaRR)
				}
			}
		}
		p.AnswerFrom = answerFrom
		return p
	}
	trimmed := map[string][]string{
		"ns1.example": {"192.0.2.1"},
		"ns2.example": {"192.0.2.2", "2001:db8::2"},
	}

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				authoritative(name, fullTestGlue(), "192.0.2.101"),
				authoritative(name, trimmed, "192.0.2.102"),
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET", "DELEGATION_NS_SET")
}

// RFC 2181 section 9 lets a truncated response carry the partial RRset that
// did not fit. With resolver.defaults.fallback disabled the transport hands
// that response back as is, so the testcase must skip it rather than read a
// short NS RRset as a different delegation.
func TestConsistency05DelegationNSSetIgnoresTruncatedResponse(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	truncated := glueParentPacket("example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	}, "192.0.2.102")
	truncated.Msg.Truncated = true

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, fullTestGlue(), "192.0.2.101"),
				truncated,
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET")
	// The truncated response contributes no glue either, so the remaining
	// parent's complete glue still matches the child.
	tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
}

// An upward referral answers with the root NS RRset instead of delegating the
// queried zone. The owner name does not match, so it is not a usable referral.
func TestConsistency05DelegationNSSetIgnoresUpwardReferral(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	upward := referralPacket(".", []string{"a.root-servers.net", "b.root-servers.net"})
	upward.AnswerFrom = "192.0.2.102"

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, fullTestGlue(), "192.0.2.101"),
				upward,
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET")
}

// The delegation is the authority-section NS RRset. NS records that appear
// elsewhere in a referral are not part of it, so a parent that repeats or
// pads NS records in the additional section still serves one delegation.
func TestConsistency05DelegationNSSetReadsAuthoritySectionOnly(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	padded := glueParentPacket("example", fullTestGlue(), "192.0.2.102")
	extraNS := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	extraNS.Ns = dnsutil.Fqdn("ns3.example")
	padded.Msg.Extra = append(padded.Msg.Extra, extraNS)

	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			return []packet.Packet{
				glueParentPacket(name, fullTestGlue(), "192.0.2.101"),
				padded,
			}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "MULTIPLE_DELEGATION_NS_SET")
}

// Glue is only credible for names the same response delegates to. An address
// record for an unrelated owner in the additional section is not glue.
func TestConsistency05GlueIgnoresOwnerOutsideAuthoritySet(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	stubConsistency05Child(t, ctx)

	// The referral delegates ns1 and ns2 but also carries an address record
	// for an undelegated name. Treating that as glue would report it as
	// unconfirmed by the child.
	queryParentAll = func(_ context.Context, _ *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
		if strings.EqualFold(qtype, "NS") {
			p := glueParentPacket(name, fullTestGlue(), "192.0.2.101")
			strayRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn("stray.example"), Class: dns.ClassINET, TTL: 60}}
			strayRR.Addr = netip.MustParseAddr("192.0.2.66")
			p.Msg.Extra = append(p.Msg.Extra, strayRR)
			return []packet.Packet{p}, nil
		}
		return []packet.Packet{}, nil
	}

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Consistency05(ctx, &z)
	if err != nil {
		t.Fatalf("consistency05: %v", err)
	}
	tctest.RequireNoTag(t, entries, "IN_DOMAIN_ADDR_MISMATCH")
	tctest.RequireTags(t, entries, "ADDRESSES_MATCH")
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
	tctest.RequireTags(t, entries, "MULTIPLE_SOA_MNAMES", "SOA_MNAME")
	entry := tctest.RequireTag(t, entries, "SOA_MNAME")
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

// referralPacket builds a parent referral for owner: NS records in the
// authority section, an empty answer section and AA clear. This is what a
// delegating parent actually sends, as opposed to nsPacket, which models an
// authoritative answer from a server that holds the zone itself.
func referralPacket(owner string, nsNames []string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	for _, nsName := range nsNames {
		nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
		nsRR.Ns = dnsutil.Fqdn(nsName)
		msg.Ns = append(msg.Ns, nsRR)
	}
	return packet.Packet{Msg: msg}
}

// nsPacketWithGlue builds a referral carrying NS records plus glue (A/AAAA)
// in the additional section, keyed by owner name.
func nsPacketWithGlue(owner string, glue map[string][]string) packet.Packet {
	names := make([]string, 0, len(glue))
	for name := range glue {
		names = append(names, name)
	}
	sort.Strings(names)

	p := referralPacket(owner, names)
	for _, name := range names {
		for _, address := range glue[name] {
			ip, err := netip.ParseAddr(address)
			if err != nil {
				continue
			}
			hdr := dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}
			if ip.Is4() {
				aRR := &dns.A{Hdr: hdr}
				aRR.Addr = ip.Unmap()
				p.Msg.Extra = append(p.Msg.Extra, aRR)
			} else {
				aaaaRR := &dns.AAAA{Hdr: hdr}
				aaaaRR.Addr = ip
				p.Msg.Extra = append(p.Msg.Extra, aaaaRR)
			}
		}
	}
	return p
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
