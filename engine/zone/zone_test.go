package zone

import (
	"context"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

// newZone builds a zone served by r.
func newZone(t *testing.T, name string, r *recursor.Recursor) Zone {
	t.Helper()
	z, err := NewWithRecursor(name, r)
	if err != nil {
		t.Fatalf("new zone %s: %v", name, err)
	}
	return z
}

func TestZoneQueryOneSkipsDisabledIP(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = false
	prof.Net.IPv6 = true

	ns4 := nstest.NS(t, ctx, nil, "ns4.example", "192.0.2.1")
	ns6 := nstest.NS(t, ctx, nil, "ns6.example", "2001:db8::1")

	var calls4 int
	ns4.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		calls4++
		return packet.Packet{}, nil
	})
	ns6.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return dnstest.Response(dnstest.NotAuthoritative(),
			dnstest.Answers(dnstest.ARR("example", "192.0.2.9"))), nil
	})

	z := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{ns4, ns6},
		nsSet: true,
	}

	resp, err := z.QueryOne(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected response from IPv6 nameserver")
	}
	if calls4 != 0 {
		t.Fatalf("expected IPv4 nameserver skipped, got %d calls", calls4)
	}
}

func TestZoneQueryAllParallel(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 2
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, _ string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
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
			return dnstest.Response(dnstest.NotAuthoritative(), dnstest.Reply(),
				dnstest.Question("example", dns.TypeDNSKEY), dnstest.AnswerFrom(id)), nil
		}
	}

	ns1 := nstest.HookedNS(t, baseCtx, nil, "ns1.example", "192.0.2.50", hook("ns1"))
	ns2 := nstest.HookedNS(t, baseCtx, nil, "ns2.example", "192.0.2.51", hook("ns2"))

	z := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{ns1, ns2},
		nsSet: true,
	}

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var res []packet.Packet
	var qerr error
	go func() {
		res, qerr = z.QueryAll(ctx, "example", "DNSKEY", nil)
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
		if qerr != nil {
			t.Fatalf("queryall: %v", qerr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("queryall did not finish")
	}

	if len(res) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(res))
	}
	if res[0].AnswerFrom != "ns1" || res[1].AnswerFrom != "ns2" {
		t.Fatalf("expected ordered responses, got %q and %q", res[0].AnswerFrom, res[1].AnswerFrom)
	}
}

func TestZoneGlueUsesFakeAddresses(t *testing.T) {
	r := nstest.HintedRecursor(t, map[string]map[string][]string{
		"example": {"ns1.example": {"192.0.2.55"}},
	})

	z := Zone{
		Name:         dnsname.New("example"),
		recursor:     r,
		glueNames:    []dnsname.Name{dnsname.New("ns1.example")},
		glueNamesSet: true,
	}

	ctx, _, _ := testhelpers.Context(t)
	glue, err := z.Glue(ctx)
	if err != nil {
		t.Fatalf("glue: %v", err)
	}
	if len(glue) != 1 {
		t.Fatalf("expected 1 glue nameserver, got %d", len(glue))
	}
	if glue[0].Address.String() != "192.0.2.55" {
		t.Fatalf("unexpected glue address %s", glue[0].Address.String())
	}
}

func TestZoneNSNamesUndelegatedUsesFakeDelegation(t *testing.T) {
	r := nstest.Recursor(t, map[string]map[string][]string{
		"example": {
			"ns2.example.net": {},
			"ns1.example":     {"192.0.2.55"},
		},
	})

	z := newZone(t, "example", r)

	names, err := z.NSNames(context.Background())
	if err != nil {
		t.Fatalf("ns names: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0].String() != "ns1.example" || names[1].String() != "ns2.example.net" {
		t.Fatalf("unexpected names: %q, %q", names[0].String(), names[1].String())
	}
}

func TestZoneNSUndelegatedUsesProvidedGlueOnly(t *testing.T) {
	r := nstest.Recursor(t, map[string]map[string][]string{
		"example": {
			"ns2.example.net": {},
			"ns1.example":     {"192.0.2.55"},
		},
	})

	z := newZone(t, "example", r)

	ctx, _, _ := testhelpers.Context(t)
	nss, err := z.NS(ctx)
	if err != nil {
		t.Fatalf("ns: %v", err)
	}
	if len(nss) != 1 {
		t.Fatalf("expected 1 nameserver with address, got %d", len(nss))
	}
	if nss[0].String() != "ns1.example/192.0.2.55" {
		t.Fatalf("unexpected nameserver %q", nss[0].String())
	}
}

func TestZoneNewWithRecursorEmptyName(t *testing.T) {
	if _, err := NewWithRecursor("", nil); err == nil {
		t.Fatalf("expected error for empty zone name")
	}
}

func TestZoneParentRoot(t *testing.T) {
	z := Zone{Name: dnsname.New(".")}
	parent, err := z.Parent(context.Background())
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent != &z {
		t.Fatalf("expected parent to be root zone")
	}
}

func TestZoneParentMissingRecursor(t *testing.T) {
	z := Zone{Name: dnsname.New("example")}
	if _, err := z.Parent(context.Background()); err == nil {
		t.Fatalf("expected error for missing recursor")
	}
}

func TestZoneGlueNamesFromParent(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	parentNS := nstest.HookedNS(t, ctx, nil, "ns.parent.example", "192.0.2.10", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return dnstest.Response(dnstest.NotAuthoritative(), dnstest.Answers(
			dnstest.NSRRs("child.example", "NS1.Child.Example", "ns2.child.example", "ns1.child.example")...)), nil
	})

	parent := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{parentNS},
		nsSet: true,
	}
	z := Zone{
		Name:      dnsname.New("child.example"),
		parent:    &parent,
		parentSet: true,
	}

	names, err := z.GlueNames(ctx)
	if err != nil {
		t.Fatalf("glue names: %v", err)
	}
	want := []string{"ns1.child.example", "ns2.child.example"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d", len(want), len(names))
	}
	for i, name := range names {
		if name.String() != want[i] {
			t.Fatalf("unexpected name %q", name.String())
		}
	}
}

func TestZoneGlueAddressesFromParent(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	parentNS := nstest.HookedNS(t, ctx, nil, "ns.parent.example", "192.0.2.11", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return dnstest.Response(dnstest.NotAuthoritative(), dnstest.Additional(
			dnstest.ARR("ns1.child.example", "192.0.2.100"),
			dnstest.AAAARR("ns1.child.example", "2001:db8::100"))), nil
	})

	parent := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{parentNS},
		nsSet: true,
	}
	z := Zone{
		Name:      dnsname.New("child.example"),
		parent:    &parent,
		parentSet: true,
	}

	records, err := z.GlueAddresses(ctx)
	if err != nil {
		t.Fatalf("glue addresses: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 glue records, got %d", len(records))
	}
}

func TestZoneNSNamesRootSorted(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	r, err := recursor.New()
	if err != nil {
		t.Fatalf("new recursor: %v", err)
	}
	z := Zone{Name: dnsname.New("."), recursor: r}

	names, err := z.NSNames(ctx)
	if err != nil {
		t.Fatalf("ns names: %v", err)
	}
	if len(names) == 0 {
		t.Fatalf("expected root names")
	}
	for i := 1; i < len(names); i++ {
		prev := strings.ToLower(names[i-1].String())
		curr := strings.ToLower(names[i].String())
		if prev > curr {
			t.Fatalf("expected names sorted, got %q then %q", prev, curr)
		}
	}
}

func TestZoneQueryPersistentSelectsAnswer(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ns1 := nstest.HookedNS(t, ctx, nil, "ns1.example", "192.0.2.20", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return dnstest.Response(dnstest.NotAuthoritative(),
			dnstest.Answers(dnstest.NSRR("other.example", "ns.other.example"))), nil
	})
	ns2 := nstest.HookedNS(t, ctx, nil, "ns2.example", "192.0.2.21", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return dnstest.Response(dnstest.NotAuthoritative(),
			dnstest.Answers(dnstest.NSRR("example", "ns2.example"))), nil
	})

	z := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{ns1, ns2},
		nsSet: true,
	}

	resp, err := z.QueryPersistent(ctx, "example", "NS", nil)
	if err != nil {
		t.Fatalf("query persistent: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected response")
	}
	if len(resp.GetRecordsForName("NS", dnsname.New("example"), "answer")) == 0 {
		t.Fatalf("expected NS record for example")
	}
}

func TestZoneQueryPersistentAcceptsAuthority(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ns := nstest.HookedNS(t, ctx, nil, "ns1.example", "192.0.2.31", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		nsRR := &dns.NS{Hdr: dns.Header{Name: "child.example.", Class: dns.ClassINET}}
		nsRR.Ns = "ns1.child.example."
		msg.Ns = []dns.RR{nsRR}
		return packet.Packet{Msg: msg}, nil
	})

	z := Zone{
		Name:  dnsname.New("child.example"),
		ns:    []nameserver.Nameserver{ns},
		nsSet: true,
	}

	resp, err := z.QueryPersistent(ctx, "child.example", "NS", nil)
	if err != nil {
		t.Fatalf("query persistent: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected response")
	}
	if len(resp.GetRecordsForName("NS", dnsname.New("child.example"), "authority")) == 0 {
		t.Fatalf("expected NS record in authority section")
	}
}

func TestZoneIsInZone(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ns := nstest.HookedNS(t, ctx, nil, "ns1.example", "192.0.2.30", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Authoritative = true
		soaRR := &dns.SOA{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}
		soaRR.Ns = "ns1.example."
		soaRR.Mbox = "hostmaster.example."
		soaRR.Serial = 1
		soaRR.Refresh = 3600
		soaRR.Retry = 600
		soaRR.Expire = 86400
		soaRR.Minttl = 300
		msg.Answer = []dns.RR{soaRR}
		return packet.Packet{Msg: msg}, nil
	})

	z := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{ns},
		nsSet: true,
	}

	inZone, err := z.IsInZone(ctx, "www.example")
	if err != nil {
		t.Fatalf("is in zone: %v", err)
	}
	if !inZone {
		t.Fatalf("expected name to be in zone")
	}
}

// TestZoneApexNSNamesUnionsAcrossServers verifies that ApexNSNames queries
// every authoritative server and unions the NS RRsets (deduplicated,
// case-folded, sorted), unlike NSNames which stops at the first answer.
func TestZoneApexNSNamesUnionsAcrossServers(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.HintedRecursor(t, map[string]map[string][]string{
		"example.com": {
			"ns1.example.com": {"192.0.2.11"},
			"ns2.example.com": {"192.0.2.12"},
		},
	})

	// Both apex servers reply with overlapping but distinct NS sets.
	mkHook := func(names ...string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qname != "example.com" || qtype != "NS" {
				return packet.Packet{}, nil
			}
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			msg.Authoritative = true
			for _, n := range names {
				rr := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn("example.com"), Class: dns.ClassINET, TTL: 60}}
				rr.Ns = dnsutil.Fqdn(n)
				msg.Answer = append(msg.Answer, rr)
			}
			return packet.Packet{Msg: msg}, nil
		}
	}

	nstest.HookedNS(t, ctx, r, "ns1.example.com", "192.0.2.11", mkHook("NS1.Example.com.", "ns3.example.com."))
	nstest.HookedNS(t, ctx, r, "ns2.example.com", "192.0.2.12", mkHook("ns2.example.com.", "ns3.example.com."))

	z := newZone(t, "example.com", r)

	names, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("apex names: %v", err)
	}
	want := []string{"ns1.example.com", "ns2.example.com", "ns3.example.com"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d: %#v", len(want), len(names), names)
	}
	for i, n := range names {
		if n.String() != want[i] {
			t.Fatalf("at %d expected %q got %q", i, want[i], n.String())
		}
	}
}

// TestZoneApexNSNamesNilZone verifies the nil-zone guard.
func TestZoneApexNSNamesNilZone(t *testing.T) {
	var z *Zone
	if _, err := z.ApexNSNames(context.Background()); err == nil {
		t.Fatalf("expected error for nil zone")
	}
}

func TestCachedParentReturnsMemoizedState(t *testing.T) {
	// A nil zone and an unresolved zone both report "not resolved".
	var nilZone *Zone
	if p, ok := nilZone.CachedParent(); ok || p != nil {
		t.Fatalf("nil zone: got (%v, %v), want (nil, false)", p, ok)
	}
	unresolved := &Zone{Name: dnsname.New("example")}
	if p, ok := unresolved.CachedParent(); ok || p != nil {
		t.Fatalf("unresolved: got (%v, %v), want (nil, false)", p, ok)
	}

	// Once parent resolution has run the memoized value is returned as-is,
	// including the nil-parent-but-resolved case (parentSet true, parent nil).
	parent := &Zone{Name: dnsname.New("se")}
	resolved := &Zone{Name: dnsname.New("example.se"), parent: parent, parentSet: true}
	if p, ok := resolved.CachedParent(); !ok || p != parent {
		t.Fatalf("resolved: got (%v, %v), want (parent, true)", p, ok)
	}
	resolvedNil := &Zone{Name: dnsname.New("com"), parentSet: true}
	if p, ok := resolvedNil.CachedParent(); !ok || p != nil {
		t.Fatalf("resolved-nil: got (%v, %v), want (nil, true)", p, ok)
	}
}

func TestCachedNSReturnsMemoizedCopy(t *testing.T) {
	var nilZone *Zone
	if ns, ok := nilZone.CachedNS(); ok || ns != nil {
		t.Fatalf("nil zone: got (%v, %v), want (nil, false)", ns, ok)
	}
	unresolved := &Zone{Name: dnsname.New("example")}
	if ns, ok := unresolved.CachedNS(); ok || ns != nil {
		t.Fatalf("unresolved: got (%v, %v), want (nil, false)", ns, ok)
	}

	ctx, _, _ := testhelpers.Context(t)
	ns1 := nstest.NS(t, ctx, nil, "ns1.example", "192.0.2.1")
	z := &Zone{Name: dnsname.New("example"), ns: []nameserver.Nameserver{ns1}, nsSet: true}
	got, ok := z.CachedNS()
	if !ok || len(got) != 1 {
		t.Fatalf("resolved: got (%v, %v), want 1 nameserver, true", got, ok)
	}

	// The returned slice must be a copy: mutating it must not affect the zone.
	got[0] = nameserver.Nameserver{}
	again, _ := z.CachedNS()
	if again[0].AddressString() != "192.0.2.1" {
		t.Fatalf("CachedNS returned an aliased slice; zone state was mutated")
	}
}
