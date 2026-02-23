package address

import (
	"context"
	"strings"
	"testing"
	"net/netip"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestAddress01DocumentationAddr(t *testing.T) {
	ctx := testContext(t)
	z := newZoneWithFakeAddresses(ctx, t, "example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	})

	entries, err := Address01(ctx, z)
	if err != nil {
		t.Fatalf("address01: %v", err)
	}
	if !hasEntryTag(entries, "A01_DOCUMENTATION_ADDR") {
		t.Fatalf("expected A01_DOCUMENTATION_ADDR")
	}
	if !hasEntryTag(entries, "A01_NO_GLOBALLY_REACHABLE_ADDR") {
		t.Fatalf("expected A01_NO_GLOBALLY_REACHABLE_ADDR")
	}
	if hasEntryTag(entries, "A01_GLOBALLY_REACHABLE_ADDR") {
		t.Fatalf("did not expect A01_GLOBALLY_REACHABLE_ADDR")
	}
}

func TestAddress01NoNameServersFound(t *testing.T) {
	ctx := testContext(t)
	z := newZoneWithFakeAddresses(ctx, t, "example", map[string][]string{
		"ns.other": {},
	})

	entries, err := Address01(ctx, z)
	if err != nil {
		t.Fatalf("address01: %v", err)
	}
	if !hasEntryTag(entries, "A01_NO_NAME_SERVERS_FOUND") {
		t.Fatalf("expected A01_NO_NAME_SERVERS_FOUND")
	}
}

func TestAddress02NameserversIPWithReverse(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))

	z := newRootZoneWithHook(ctx, t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(qname, ptrName) && strings.EqualFold(qtype, "PTR") {
			return ptrPacket(qname, "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Address02(ctx, z)
	if err != nil {
		t.Fatalf("address02: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVERS_IP_WITH_REVERSE") {
		t.Fatalf("expected NAMESERVERS_IP_WITH_REVERSE")
	}
}

func TestAddress02NameserverIPWithoutReverse(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))

	z := newRootZoneWithHook(ctx, t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(qname, ptrName) && strings.EqualFold(qtype, "PTR") {
			return noAnswerPacket(qname, "PTR")
		}
		return packet.Packet{}
	})

	entries, err := Address02(ctx, z)
	if err != nil {
		t.Fatalf("address02: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_IP_WITHOUT_REVERSE") {
		t.Fatalf("expected NAMESERVER_IP_WITHOUT_REVERSE")
	}
}

func TestAddress03PTRMatch(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))

	z := newRootZoneWithHook(ctx, t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(qname, ptrName) && strings.EqualFold(qtype, "PTR") {
			return ptrPacket(qname, "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Address03(ctx, z)
	if err != nil {
		t.Fatalf("address03: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_IP_PTR_MATCH") {
		t.Fatalf("expected NAMESERVER_IP_PTR_MATCH")
	}
}

func TestAddress03PTRMismatch(t *testing.T) {
	ctx := testContext(t)
	ptrName := dnsutil.ReverseAddr(netip.MustParseAddr("192.0.2.1"))

	z := newRootZoneWithHook(ctx, t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(qname, ptrName) && strings.EqualFold(qtype, "PTR") {
			return ptrPacket(qname, "ptr.example.")
		}
		return packet.Packet{}
	})

	entries, err := Address03(ctx, z)
	if err != nil {
		t.Fatalf("address03: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_IP_PTR_MISMATCH") {
		t.Fatalf("expected NAMESERVER_IP_PTR_MISMATCH")
	}
}

func TestAddress02ParallelPTRQueries(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
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
		if ip, ok := entry.Args["ns_ip"].(string); ok {
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
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
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
		if ip, ok := entry.Args["ns_ip"].(string); ok {
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

func newRootZoneWithHook(ctx context.Context, t *testing.T, handler func(qname string, qtype string) packet.Packet) *zone.Zone {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	ns, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype), nil
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	return &z
}

func newZoneWithFakeAddresses(ctx context.Context, t *testing.T, zoneName string, data map[string][]string) *zone.Zone {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(zoneName, data); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z, err := zone.NewWithRecursor(zoneName, r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	return &z
}

func nsPacket(zoneName string, nsName string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}}
	nsRR.Ns = dnsutil.Fqdn(nsName)
	msg.Answer = []dns.RR{nsRR}
	return packet.Packet{Msg: msg}
}

func ptrPacket(owner string, targets ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	for _, target := range targets {
		ptrRR := &dns.PTR{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
		ptrRR.Ptr = dnsutil.Fqdn(target)
		msg.Answer = append(msg.Answer, ptrRR)
	}
	return packet.Packet{Msg: msg}
}

func noAnswerPacket(owner string, qtype string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	if qtype == "" {
		qtype = "A"
	}
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.StringToType[strings.ToUpper(qtype)])
	return packet.Packet{Msg: msg}
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
