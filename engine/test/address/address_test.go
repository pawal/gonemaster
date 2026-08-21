package address

import (
	"context"
	"errors"
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
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestAddress01DocumentationAddr(t *testing.T) {
	ctx := testContext(t)
	z := tctest.ZoneWithAddrs(t, "example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	})

	entries, err := Address01(ctx, z)
	if err != nil {
		t.Fatalf("address01: %v", err)
	}
	tctest.RequireTags(t, entries, "A01_DOCUMENTATION_ADDR", "A01_NO_GLOBALLY_REACHABLE_ADDR")
	tctest.RequireNoTag(t, entries, "A01_GLOBALLY_REACHABLE_ADDR")
	entry := tctest.RequireTag(t, entries, "A01_DOCUMENTATION_ADDR")
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("did not expect legacy ns_list key in args")
	}
	endpoints := tctest.ServerEndpoints(t, entry.Args)
	if len(endpoints) != 1 || endpoints[0] != "ns1.example/192.0.2.1" {
		t.Fatalf("expected typed servers [ns1.example/192.0.2.1], got %v", endpoints)
	}
}

func TestAddress01NoNameServersFound(t *testing.T) {
	ctx := testContext(t)
	z := tctest.ZoneWithAddrs(t, "example", map[string][]string{
		"ns.other": {},
	})

	entries, err := Address01(ctx, z)
	if err != nil {
		t.Fatalf("address01: %v", err)
	}
	tctest.RequireTags(t, entries, "A01_NO_NAME_SERVERS_FOUND")
}

// TestAddress01EmitsCNAMETagForUnresolvableNS verifies that when the
// delegation/zone NS discovery returns an address-less NSItem carrying a
// *recursor.CNAMEError, Address01 emits the matching CNAME_* tag. The same
// failing name appears in both the delegation and zone views, so the test
// also asserts the tag is logged exactly once (dedup by query_name).
func TestAddress01EmitsCNAMETagForUnresolvableNS(t *testing.T) {
	ctx := testContext(t)

	cnameErr := &recursor.CNAMEError{
		Reason: recursor.CNAMEUnresolved,
		Name:   "ns.outside.test",
		Target: "loop.outside.test",
		Detail: "loop",
	}
	delItems := []nsdiscovery.NSItem{
		{Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.1"), HasAddress: true},
		{Name: dnsname.New("ns.outside.test"), Err: cnameErr},
	}
	zoneItems := []nsdiscovery.NSItem{
		{Name: dnsname.New("ns.outside.test"), Err: cnameErr},
	}
	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return delItems, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return zoneItems, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Address01(ctx, &z)
	if err != nil {
		t.Fatalf("address01: %v", err)
	}

	count := 0
	for _, e := range entries {
		if e != nil && e.Tag == "CNAME_TARGET_UNRESOLVED" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one CNAME_TARGET_UNRESOLVED entry, got %d", count)
	}

	entry := tctest.RequireTag(t, entries, "CNAME_TARGET_UNRESOLVED")
	if got := entry.Args["query_name"]; got != "ns.outside.test" {
		t.Fatalf("query_name: got %#v, want ns.outside.test", got)
	}
	if got := entry.Args["cname_target"]; got != "loop.outside.test" {
		t.Fatalf("cname_target: got %#v, want loop.outside.test", got)
	}
}

func TestAddress02NameserversIPWithReverse(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))

	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(q.Name, ptrName) && strings.EqualFold(q.Type, "PTR") {
			return ptrPacket(q.Name, "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Address02(ctx, z)
	if err != nil {
		t.Fatalf("address02: %v", err)
	}
	tctest.RequireTags(t, entries, "NAMESERVERS_IP_WITH_REVERSE")
}

func TestAddress02NameserverIPWithoutReverse(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))

	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(q.Name, ptrName) && strings.EqualFold(q.Type, "PTR") {
			return noAnswerPacket(q.Name, "PTR")
		}
		return packet.Packet{}
	})

	entries, err := Address02(ctx, z)
	if err != nil {
		t.Fatalf("address02: %v", err)
	}
	tctest.RequireTags(t, entries, "NAMESERVER_IP_WITHOUT_REVERSE")
}

// TestAddress02ReverseThroughCNAME locks in handling of a reverse (PTR) lookup
// whose path goes through a CNAME, as permitted by RFC 2181 section 10.2. The
// reverse name is a CNAME to a target that carries the PTR. Address02 must
// follow the CNAME and report NAMESERVERS_IP_WITH_REVERSE, not
// NAMESERVER_IP_WITHOUT_REVERSE or NO_RESPONSE_PTR_QUERY. The target is kept
// in-bailiwick so the fake recursor resolves it without cross-zone delegation.
func TestAddress02ReverseThroughCNAME(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))
	const cnameTarget = "host.1.2.0.192.in-addr.arpa."

	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(q.Name, ptrName) && strings.EqualFold(q.Type, "PTR") {
			return cnamePacket(q.Name, cnameTarget)
		}
		if strings.EqualFold(q.Name, cnameTarget) && strings.EqualFold(q.Type, "PTR") {
			return ptrPacket(cnameTarget, "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Address02(ctx, z)
	if err != nil {
		t.Fatalf("address02: %v", err)
	}
	tctest.RequireTags(t, entries, "NAMESERVERS_IP_WITH_REVERSE")
	tctest.RequireNoTag(t, entries, "NAMESERVER_IP_WITHOUT_REVERSE", "NO_RESPONSE_PTR_QUERY")
}

// TestAddress02CNAMEFailure locks in the decided behavior for Address02: a
// *recursor.CNAMEError from glue/apex NS resolution is logged as a CNAME_*
// tag and the testcase continues (no abort), while any non-CNAME error still
// aborts the testcase unchanged.
func TestAddress02CNAMEFailure(t *testing.T) {
	t.Run("cname error logs tag and continues", func(t *testing.T) {
		ctx := testContext(t)

		cnameErr := &recursor.CNAMEError{
			Reason: recursor.CNAMEUnresolved,
			Name:   "ns.outside.test",
			Target: "loop.outside.test",
			Detail: "loop",
		}
		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, cnameErr
		})
		tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		})

		z := tctest.ZoneWithAddrs(t, "example", map[string][]string{"ns1.example": {"192.0.2.1"}})

		entries, err := Address02(ctx, z)
		if err != nil {
			t.Fatalf("address02 must continue past a CNAME failure, got error: %v", err)
		}
		tctest.RequireTags(t, entries, "CNAME_TARGET_UNRESOLVED")
	})

	t.Run("non-cname error still aborts", func(t *testing.T) {
		ctx := testContext(t)

		boom := errors.New("resolver failure")
		tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, boom
		})

		z := tctest.ZoneWithAddrs(t, "example", map[string][]string{"ns1.example": {"192.0.2.1"}})

		entries, err := Address02(ctx, z)
		if !errors.Is(err, boom) {
			t.Fatalf("expected the non-CNAME error to propagate, got: %v", err)
		}
		tctest.RequireNoTag(t, entries, "CNAME_TARGET_UNRESOLVED")
	})
}

// TestAddress03CNAMEFailureLogsTagAndContinues verifies Address03 emits the
// matching CNAME_* tag and continues when apex NS resolution fails with a
// *recursor.CNAMEError.
func TestAddress03CNAMEFailureLogsTagAndContinues(t *testing.T) {
	ctx := testContext(t)

	cnameErr := &recursor.CNAMEError{
		Reason: recursor.CNAMEChainTooLong,
		Name:   "ns.outside.test",
		Target: "deep.outside.test",
	}
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, cnameErr
	})

	z := tctest.ZoneWithAddrs(t, "example", map[string][]string{"ns1.example": {"192.0.2.1"}})

	entries, err := Address03(ctx, z)
	if err != nil {
		t.Fatalf("address03 must continue past a CNAME failure, got error: %v", err)
	}
	tctest.RequireTags(t, entries, "CNAME_CHAIN_TOO_LONG")
}

func TestAddress03PTRMatch(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))

	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(q.Name, ptrName) && strings.EqualFold(q.Type, "PTR") {
			return ptrPacket(q.Name, "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Address03(ctx, z)
	if err != nil {
		t.Fatalf("address03: %v", err)
	}
	tctest.RequireTags(t, entries, "NAMESERVER_IP_PTR_MATCH")
}

func TestAddress03PTRMismatch(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))

	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(q.Name, ptrName) && strings.EqualFold(q.Type, "PTR") {
			return ptrPacket(q.Name, "ptr.example.")
		}
		return packet.Packet{}
	})

	entries, err := Address03(ctx, z)
	if err != nil {
		t.Fatalf("address03: %v", err)
	}
	tctest.RequireTags(t, entries, "NAMESERVER_IP_PTR_MISMATCH")
}

func TestAddress02ParallelPTRQueries(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 2

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if strings.EqualFold(qtype, "PTR") {
			select {
			case started <- qname:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return noAnswerPacket(qname, "PTR"), nil
		}
		return packet.Packet{}, nil
	}

	ns1, err := nameserver.NewWithContext(baseCtx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook)

	ns2, err := nameserver.NewWithContext(baseCtx, "b.root", "192.0.2.2", r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook)

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var addrErr error
	go func() {
		entries, addrErr = Address02(ctx, &z)
		close(done)
	}()

	want := map[string]bool{}
	ptr1 := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))
	ptr2 := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.2"))
	want[ptr1] = true
	want[ptr2] = true

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel PTR queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if addrErr != nil {
			t.Fatalf("address02: %v", addrErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("address02 did not finish")
	}

	for name := range want {
		if !got[name] {
			t.Fatalf("missing PTR query for %q", name)
		}
	}

	var ips []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "NAMESERVER_IP_WITHOUT_REVERSE" {
			continue
		}
		if ip, ok := entry.Args["address"].(string); ok {
			ips = append(ips, ip)
		}
	}
	if len(ips) != 2 {
		t.Fatalf("expected 2 reverse-missing entries, got %v", ips)
	}
	if ips[0] != "192.0.2.1" || ips[1] != "192.0.2.2" {
		t.Fatalf("expected deterministic log order, got %v", ips)
	}
}

func TestAddress03ParallelPTRQueries(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 2

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if strings.EqualFold(qtype, "PTR") {
			select {
			case started <- qname:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return ptrPacket(qname, "ptr.example."), nil
		}
		return packet.Packet{}, nil
	}

	ns1, err := nameserver.NewWithContext(baseCtx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook)

	ns2, err := nameserver.NewWithContext(baseCtx, "b.root", "192.0.2.2", r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook)

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var addrErr error
	go func() {
		entries, addrErr = Address03(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel PTR queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if addrErr != nil {
			t.Fatalf("address03: %v", addrErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("address03 did not finish")
	}

	var ips []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "NAMESERVER_IP_PTR_MISMATCH" {
			continue
		}
		if ip, ok := entry.Args["address"].(string); ok {
			ips = append(ips, ip)
		}
	}
	if len(ips) != 2 {
		t.Fatalf("expected 2 mismatch entries, got %v", ips)
	}
	if ips[0] != "192.0.2.1" || ips[1] != "192.0.2.2" {
		t.Fatalf("expected deterministic log order, got %v", ips)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, _, _ := testhelpers.Context(t)
	return ctx
}

// These responses come from a recursor, so none of them set the AA bit.
func nsPacket(zoneName string, nsName string) packet.Packet {
	return tctest.Response(tctest.NotAuthoritative(),
		tctest.Answers(tctest.NSRR(zoneName, nsName)))
}

func ptrPacket(owner string, targets ...string) packet.Packet {
	rrs := make([]dns.RR, 0, len(targets))
	for _, target := range targets {
		rrs = append(rrs, tctest.PTRRR(owner, target))
	}
	return tctest.Response(tctest.NotAuthoritative(), tctest.Answers(rrs...))
}

func cnamePacket(owner string, target string) packet.Packet {
	return tctest.Response(tctest.NotAuthoritative(),
		tctest.Answers(tctest.CNAMERR(owner, target)))
}

func noAnswerPacket(owner string, qtype string) packet.Packet {
	if qtype == "" {
		qtype = "A"
	}
	return tctest.Response(tctest.NotAuthoritative(),
		tctest.Question(owner, dns.StringToType[strings.ToUpper(qtype)]))
}
