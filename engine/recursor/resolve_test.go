package recursor

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

func TestAddFakeAddressesDedupAndRemove(t *testing.T) {
	r := &Recursor{}
	if err := r.AddFakeAddresses("Example.COM", map[string][]string{
		"NS1.Example.COM": {"192.0.2.1", "192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns2.example.com": {},
	}); err != nil {
		t.Fatalf("add fake addresses (empty): %v", err)
	}
	if !r.HasFakeAddresses("example.com") {
		t.Fatalf("expected fake addresses for example.com")
	}
	addrs := r.GetFakeAddresses("EXAMPLE.COM", "ns1.example.com")
	if len(addrs) != 1 || addrs[0].String() != "192.0.2.1" {
		t.Fatalf("unexpected addresses: %#v", addrs)
	}
	names := r.GetFakeNames("example.com")
	found := false
	for _, name := range names {
		if name == "ns2.example.com" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ns2.example.com in fake names: %#v", names)
	}

	r.RemoveFakeAddresses("example.com")
	if r.HasFakeAddresses("example.com") {
		t.Fatalf("expected fake addresses removed")
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{"ns": {"bad"}}); err == nil {
		t.Fatalf("expected error for invalid IP")
	}
}

func TestRootServersSorted(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{
			".": {
				"b.root": {netip.MustParseAddr("192.0.2.2")},
				"a.root": {netip.MustParseAddr("192.0.2.1")},
			},
		},
		client: &transport.Client{},
	}

	servers, err := r.RootServers()
	if err != nil {
		t.Fatalf("root servers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 root servers, got %d", len(servers))
	}
	if servers[0].Name.String() != "a.root" || servers[1].Name.String() != "b.root" {
		t.Fatalf("unexpected root order: %s, %s", servers[0].Name.String(), servers[1].Name.String())
	}
}

func TestCacheStoreLookupAndClear(t *testing.T) {
	r := &Recursor{}
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	resp := packet.Packet{Msg: msg}

	r.cacheStore("example", "A", "IN", resp)
	cached, ok := r.cacheLookup("example", "A", "IN")
	if !ok || cached.Msg == nil {
		t.Fatalf("expected cached response")
	}

	r.cacheStore("empty", "A", "IN", packet.Packet{})
	if _, ok := r.cacheLookup("empty", "A", "IN"); !ok {
		t.Fatalf("expected cached nil response")
	}

	r.ClearCache()
	if _, ok := r.cacheLookup("example", "A", "IN"); ok {
		t.Fatalf("expected cache cleared")
	}
}

func TestGetNSFromUsesGlueAndLazy(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	msg := new(dns.Msg)
	msg.Ns = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "ns2.example.",
		},
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "ns1.example.",
		},
	}
	msg.Extra = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "ns1.example.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
			},
			A: net.IPv4(192, 0, 2, 53),
		},
	}

	resp := packet.Packet{Msg: msg}
	r := &Recursor{client: &transport.Client{}}
	queryers, err := r.getNSFrom(resp, nil)
	if err != nil {
		t.Fatalf("getNSFrom: %v", err)
	}
	if len(queryers) != 2 {
		t.Fatalf("expected 2 queryers, got %d", len(queryers))
	}

	first, ok := queryers[0].(nameserver.Nameserver)
	if !ok || first.Address.String() != "192.0.2.53" {
		t.Fatalf("unexpected first queryer: %#v", queryers[0])
	}
	if _, ok := queryers[1].(lazyNameserver); !ok {
		t.Fatalf("expected lazy nameserver, got %#v", queryers[1])
	}
}

func TestLazyNameserverMissingRecursor(t *testing.T) {
	lns := lazyNameserver{name: "ns1.example"}
	if _, err := lns.QueryWithClass(context.Background(), "example", "A", "IN"); err == nil {
		t.Fatalf("expected error for missing recursor")
	}
}

func TestGetAddressesForParallelAAndAAAA(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	if err := profile.Effective().Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{client: &transport.Client{}}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	aaaaStarted := make(chan struct{})
	rootNS, err := nameserver.New("root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			select {
			case <-aaaaStarted:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return packetWithA(name, netip.MustParseAddr("192.0.2.10")), nil
		case "AAAA":
			select {
			case <-aaaaStarted:
			default:
				close(aaaaStarted)
			}
			return packetWithAAAA(name, netip.MustParseAddr("2001:db8::1")), nil
		default:
			return packet.Packet{Msg: new(dns.Msg)}, nil
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	addrs, err := r.GetAddressesFor(ctx, "ns.example")
	if err != nil {
		t.Fatalf("get addresses: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
	if addrs[0].String() != "192.0.2.10" || addrs[1].String() != "2001:db8::1" {
		t.Fatalf("unexpected address order: %#v", addrs)
	}
}

func TestLazyNameserverParallelPrefersFirstAddress(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	defer profile.ResetEffective()

	if err := profile.Effective().Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{client: &transport.Client{}}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.New("root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			return packetWithARecords(name, []netip.Addr{
				netip.MustParseAddr("192.0.2.10"),
				netip.MustParseAddr("192.0.2.20"),
			}), nil
		case "AAAA":
			return packet.Packet{Msg: new(dns.Msg)}, nil
		default:
			return packet.Packet{Msg: new(dns.Msg)}, nil
		}
	})

	addr2Started := make(chan struct{})
	allowAddr1 := make(chan struct{})
	allowOnce := func() {
		select {
		case <-allowAddr1:
		default:
			close(allowAddr1)
		}
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.10", r.client)
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		select {
		case <-allowAddr1:
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
		resp := packetWithA(name, netip.MustParseAddr("192.0.2.10"))
		resp.AnswerFrom = "192.0.2.10"
		return resp, nil
	})

	ns2, err := nameserver.New("ns1.example", "192.0.2.20", r.client)
	if err != nil {
		t.Fatalf("new ns2: %v", err)
	}
	ns2.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		select {
		case <-addr2Started:
		default:
			close(addr2Started)
		}
		resp := packetWithA(name, netip.MustParseAddr("192.0.2.20"))
		resp.AnswerFrom = "192.0.2.20"
		return resp, nil
	})

	lns := lazyNameserver{name: "ns1.example", recursor: r, state: &recurseState{glue: map[string]map[netip.Addr]bool{}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resultCh := make(chan packet.Packet, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := lns.QueryWithClass(ctx, "example", "A", "IN")
		resultCh <- resp
		errCh <- err
	}()

	select {
	case <-addr2Started:
	case <-time.After(time.Second):
		allowOnce()
		t.Fatalf("expected second address to be queried")
	}
	allowOnce()

	resp := <-resultCh
	err = <-errCh
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.AnswerFrom != "192.0.2.10" {
		t.Fatalf("expected first address response, got %q", resp.AnswerFrom)
	}
}

func TestCollectCNAMEsAndAddresses(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   "www.example.",
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
			},
			Target: "alias.example.",
		},
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "alias.example.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
			},
			A: net.IPv4(192, 0, 2, 55),
		},
		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   "alias.example.",
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
			},
			AAAA: net.ParseIP("2001:db8::55"),
		},
	}

	resp := packet.Packet{Msg: msg}
	target := dnsname.New("www.example")
	cnames := map[string]bool{}
	collectCNAMEs(resp, target, cnames)
	if !cnames["alias.example"] {
		t.Fatalf("expected alias.example cname")
	}

	addrs := collectAddresses(resp, target, cnames)
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
	got := map[string]bool{}
	for _, addr := range addrs {
		got[addr.String()] = true
	}
	if !got["192.0.2.55"] || !got["2001:db8::55"] {
		t.Fatalf("unexpected addresses: %#v", got)
	}
}

func TestFirstSOAOwner(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
			},
		},
	}
	resp := packet.Packet{Msg: msg}
	if owner := firstSOAOwner(resp); owner != "example" {
		t.Fatalf("unexpected owner %q", owner)
	}

	if owner := firstSOAOwner(packet.Packet{Msg: new(dns.Msg)}); owner != "" {
		t.Fatalf("expected empty owner, got %q", owner)
	}
}

func TestRecurseOrderedUsesLIFO(t *testing.T) {
	defer profile.ResetEffective()
	if err := profile.Effective().Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}

	r := &Recursor{}
	called := make(chan string, 2)

	slowResp := packetWithA("example", netip.MustParseAddr("192.0.2.10"))
	slowResp.AnswerFrom = "slow"
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.11"))
	fastResp.AnswerFrom = "fast"

	slow := testQueryer{id: "slow", resp: slowResp, called: called}
	fast := testQueryer{id: "fast", resp: fastResp, called: called}

	state := &recurseState{ns: []queryer{fast, slow}}
	resp, _, err := r.recurse(context.Background(), "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "slow" {
		t.Fatalf("expected LIFO response from slow, got %q", resp.AnswerFrom)
	}

	first := <-called
	if first != "slow" {
		t.Fatalf("expected slow queried first, got %q", first)
	}
	select {
	case next := <-called:
		t.Fatalf("expected only one query, got %q", next)
	default:
	}
}

func TestRecurseUnorderedReturnsFastest(t *testing.T) {
	defer profile.ResetEffective()
	if err := profile.Effective().Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := profile.Effective().Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowResp := packetWithA("example", netip.MustParseAddr("192.0.2.20"))
	slowResp.AnswerFrom = "slow"
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.21"))
	fastResp.AnswerFrom = "fast"

	slow := testQueryer{id: "slow", resp: slowResp, delay: 80 * time.Millisecond}
	fast := testQueryer{id: "fast", resp: fastResp}

	state := &recurseState{ns: []queryer{fast, slow}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "fast" {
		t.Fatalf("expected fastest response from fast, got %q", resp.AnswerFrom)
	}
}

func packetWithA(name string, addr netip.Addr) packet.Packet {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			A: addr.AsSlice(),
		},
	}
	return packet.Packet{Msg: msg}
}

type testQueryer struct {
	id     string
	delay  time.Duration
	resp   packet.Packet
	called chan string
}

func (t testQueryer) QueryWithClass(ctx context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	if t.called != nil {
		t.called <- t.id
	}
	if t.delay > 0 {
		select {
		case <-time.After(t.delay):
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	}
	return t.resp, nil
}

func packetWithAAAA(name string, addr netip.Addr) packet.Packet {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			AAAA: addr.AsSlice(),
		},
	}
	return packet.Packet{Msg: msg}
}

func packetWithARecords(name string, addrs []netip.Addr) packet.Packet {
	msg := new(dns.Msg)
	for _, addr := range addrs {
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			A: addr.AsSlice(),
		})
	}
	return packet.Packet{Msg: msg}
}

func TestRedirectNameNoNS(t *testing.T) {
	resp := packet.Packet{Msg: new(dns.Msg)}
	if _, ok := redirectName(resp); ok {
		t.Fatalf("expected no redirect name")
	}
}
