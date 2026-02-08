package recursor

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
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

	servers, err := r.RootServers(context.Background())
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

func TestParentSingleLabelFallsBackToRoot(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{},
		client:        &transport.Client{},
	}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root.test": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(context.Background(), "a.root.test", "192.0.2.1", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		nameObj := dnsname.New(name)
		name = nameObj.String()
		qtype = strings.ToUpper(qtype)
		switch {
		case name == "arpa" && qtype == "SOA":
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			msg.Ns = []dns.RR{
				&dns.NS{
					Hdr: dns.RR_Header{
						Name:   "arpa.",
						Rrtype: dns.TypeNS,
						Class:  dns.ClassINET,
					},
					Ns: "ns.arpa.test.",
				},
			}
			msg.Extra = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "ns.arpa.test.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
					},
					A: net.IPv4(192, 0, 2, 2),
				},
			}
			return packet.Packet{Msg: msg}, nil
		case name == "." && qtype == "SOA":
			// Simulate a transient parent-check failure. Parent() should still
			// fall back to root for single-label zones.
			return packet.Packet{}, errors.New("timeout")
		default:
			return packet.Packet{}, errors.New("unexpected query")
		}
	})

	arpaNS, err := nameserver.NewWithContext(context.Background(), "ns.arpa.test", "192.0.2.2", r.client)
	if err != nil {
		t.Fatalf("new arpa nameserver: %v", err)
	}
	arpaNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		nameObj := dnsname.New(name)
		name = nameObj.String()
		qtype = strings.ToUpper(qtype)
		if name != "arpa" || qtype != "SOA" {
			return packet.Packet{}, errors.New("unexpected query")
		}

		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.SOA{
				Hdr: dns.RR_Header{
					Name:   "arpa.",
					Rrtype: dns.TypeSOA,
					Class:  dns.ClassINET,
				},
				Ns:      "ns.arpa.test.",
				Mbox:    "hostmaster.arpa.",
				Serial:  1,
				Refresh: 3600,
				Retry:   600,
				Expire:  1209600,
				Minttl:  3600,
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	parent, _, err := r.Parent(context.Background(), "arpa")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent != "." {
		t.Fatalf("expected parent '.', got %q", parent)
	}
}

func TestParentSingleLabelNoTraceFallsBackToRoot(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{},
		client:        &transport.Client{},
	}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root.test": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(context.Background(), "a.root.test", "192.0.2.1", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		nameObj := dnsname.New(name)
		name = nameObj.String()
		qtype = strings.ToUpper(qtype)
		if name != "arpa" || qtype != "SOA" {
			return packet.Packet{}, errors.New("unexpected query")
		}

		// Authoritative answer directly from the current server produces no
		// referral trace. Parent() must still resolve arpa -> .
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.SOA{
				Hdr: dns.RR_Header{
					Name:   "arpa.",
					Rrtype: dns.TypeSOA,
					Class:  dns.ClassINET,
				},
				Ns:      "ns.arpa.test.",
				Mbox:    "hostmaster.arpa.",
				Serial:  1,
				Refresh: 3600,
				Retry:   600,
				Expire:  1209600,
				Minttl:  3600,
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	parent, _, err := r.Parent(context.Background(), "arpa")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent != "." {
		t.Fatalf("expected parent '.', got %q", parent)
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
	queryers, err := r.getNSFrom(context.Background(), resp, nil)
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
	baseCtx, prof, _ := testhelpers.Context(t)

	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}

	r := &Recursor{client: &transport.Client{}}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	aaaaStarted := make(chan struct{})
	rootNS, err := nameserver.NewWithContext(baseCtx, "root.test", "192.0.2.53", r.client)
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

	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
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
	baseCtx, prof, _ := testhelpers.Context(t)

	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}

	r := &Recursor{client: &transport.Client{}}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(baseCtx, "root.test", "192.0.2.53", r.client)
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

	ns1, err := nameserver.NewWithContext(baseCtx, "ns1.example", "192.0.2.10", r.client)
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

	ns2, err := nameserver.NewWithContext(baseCtx, "ns1.example", "192.0.2.20", r.client)
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
	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
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

func TestLazyNameserverConcurrentQueriesShareGlue(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

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
			return packetWithA(name, netip.MustParseAddr("192.0.2.30")), nil
		case "AAAA":
			return packetWithAAAA(name, netip.MustParseAddr("2001:db8::30")), nil
		default:
			return packet.Packet{Msg: new(dns.Msg)}, nil
		}
	})

	ns4, err := nameserver.New("ns.example", "192.0.2.30", r.client)
	if err != nil {
		t.Fatalf("new ns4: %v", err)
	}
	ns4.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("192.0.2.31")), nil
	})

	ns6, err := nameserver.New("ns.example", "2001:db8::30", r.client)
	if err != nil {
		t.Fatalf("new ns6: %v", err)
	}
	ns6.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("192.0.2.32")), nil
	})

	state := &recurseState{glue: map[string]map[netip.Addr]bool{}}
	lns := lazyNameserver{name: "ns.example", recursor: r, state: state}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := make(chan struct{})
	errCh := make(chan error, 64)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := lns.QueryWithClass(ctx, "example", "A", "IN")
			if err != nil && ctx.Err() == nil {
				errCh <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("query failed: %v", err)
	}

	state.ensureLock()
	state.lock()
	defer state.unlock()
	nameObj := dnsname.New("ns.example")
	nameKey := strings.ToLower(nameObj.String())
	if len(state.glue[nameKey]) == 0 {
		t.Fatalf("expected glue cached for ns.example")
	}
}

func TestGetNSFromConcurrentWithLazyNameserver(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

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
			return packetWithA(name, netip.MustParseAddr("192.0.2.40")), nil
		case "AAAA":
			return packetWithAAAA(name, netip.MustParseAddr("2001:db8::40")), nil
		default:
			return packet.Packet{Msg: new(dns.Msg)}, nil
		}
	})

	ns4, err := nameserver.New("ns.example", "192.0.2.40", r.client)
	if err != nil {
		t.Fatalf("new ns4: %v", err)
	}
	ns4.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("192.0.2.41")), nil
	})

	ns6, err := nameserver.New("ns.example", "2001:db8::40", r.client)
	if err != nil {
		t.Fatalf("new ns6: %v", err)
	}
	ns6.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("192.0.2.42")), nil
	})

	msg := new(dns.Msg)
	msg.Ns = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "ns.example.",
		},
	}
	msg.Extra = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "ns.example.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
			},
			A: net.IPv4(192, 0, 2, 40),
		},
		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   "ns.example.",
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
			},
			AAAA: net.ParseIP("2001:db8::40"),
		},
	}
	resp := packet.Packet{Msg: msg}

	state := &recurseState{glue: map[string]map[netip.Addr]bool{}}
	lns := lazyNameserver{name: "ns.example", recursor: r, state: state}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 20; i++ {
			if _, err := r.getNSFrom(context.Background(), resp, state); err != nil {
				errCh <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 20; i++ {
			if _, err := lns.QueryWithClass(ctx, "example", "A", "IN"); err != nil && ctx.Err() == nil {
				errCh <- err
				return
			}
		}
	}()
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent query failed: %v", err)
	}

	state.ensureLock()
	state.lock()
	defer state.unlock()
	nameObj := dnsname.New("ns.example")
	nameKey := strings.ToLower(nameObj.String())
	if len(state.glue[nameKey]) == 0 {
		t.Fatalf("expected glue cached for ns.example")
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
	ctx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}

	r := &Recursor{}

	slowResp := packetWithA("example", netip.MustParseAddr("192.0.2.10"))
	slowResp.AnswerFrom = "slow"
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.11"))
	fastResp.AnswerFrom = "fast"

	slow := testQueryer{id: "slow", resp: slowResp}
	fast := testQueryer{id: "fast", resp: fastResp}

	state := &recurseState{ns: []queryer{fast, slow}}
	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "slow" {
		t.Fatalf("expected LIFO response from slow, got %q", resp.AnswerFrom)
	}
}

func TestRecurseOrderedParallelStartsNextQuery(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}
	slowStarted := make(chan struct{})
	fastStarted := make(chan struct{})
	releaseSlow := make(chan struct{})

	slow := testQueryer{id: "slow", startCh: slowStarted, waitCh: releaseSlow}
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.71"))
	fastResp.AnswerFrom = "fast"
	fast := testQueryer{id: "fast", startCh: fastStarted, resp: fastResp}

	state := &recurseState{ns: []queryer{fast, slow}}
	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	done := make(chan struct{})
	var out packet.Packet
	var recurseErr error
	go func() {
		out, _, recurseErr = r.recurse(ctx, "example", "A", "IN", state)
		close(done)
	}()

	select {
	case <-slowStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected slow query to start")
	}
	select {
	case <-fastStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected fast query to start in parallel while slow is blocked")
	}

	close(releaseSlow)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("recurse did not finish")
	}
	if recurseErr != nil {
		t.Fatalf("recurse: %v", recurseErr)
	}
	if out.AnswerFrom != "fast" {
		t.Fatalf("expected fast response after slow miss, got %q", out.AnswerFrom)
	}
}

func TestRecurseOrderedParallelPreservesRedirectPriority(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}
	redirectResp := packetWithReferral("child.example", "ns.child.example")
	redirectResp.AnswerFrom = "redirect"
	speculativeResp := packetWithA("example", netip.MustParseAddr("192.0.2.80"))
	speculativeResp.AnswerFrom = "speculative"
	childResp := packetWithA("example", netip.MustParseAddr("192.0.2.81"))
	childResp.AnswerFrom = "child"

	state := &recurseState{
		ns: []queryer{
			testQueryer{id: "speculative", resp: speculativeResp},
			testQueryer{id: "redirect", resp: redirectResp},
		},
		nsFrom: func(_ context.Context, resp packet.Packet, _ *recurseState) ([]queryer, error) {
			if resp.AnswerFrom != "redirect" {
				t.Fatalf("expected redirect source, got %q", resp.AnswerFrom)
			}
			return []queryer{testQueryer{id: "child", resp: childResp}}, nil
		},
	}

	out, nextState, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if out.AnswerFrom != "child" {
		t.Fatalf("expected child answer after redirect, got %q", out.AnswerFrom)
	}
	if len(nextState.trace) == 0 {
		t.Fatalf("expected trace entry for redirect")
	}
	if nextState.trace[0].answerFrom != "redirect" {
		t.Fatalf("expected redirect trace source, got %q", nextState.trace[0].answerFrom)
	}
	if nextState.trace[0].zoneName != "child.example" {
		t.Fatalf("expected child.example trace, got %q", nextState.trace[0].zoneName)
	}
}

func TestRecurseOrderedParallelCandidateSelection(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}
	servfail := new(dns.Msg)
	servfail.Rcode = dns.RcodeServerFailure
	refused := new(dns.Msg)
	refused.Rcode = dns.RcodeRefused

	state := &recurseState{
		ns: []queryer{
			testQueryer{id: "final-miss", resp: packet.Packet{}},
			testQueryer{id: "refused", resp: packet.Packet{Msg: refused, AnswerFrom: "refused"}},
			testQueryer{id: "servfail", resp: packet.Packet{Msg: servfail, AnswerFrom: "servfail"}},
		},
	}

	out, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if out.Msg == nil || out.Rcode() != "REFUSED" {
		t.Fatalf("expected REFUSED candidate, got %#v", out.Msg)
	}
	if out.AnswerFrom != "refused" {
		t.Fatalf("expected latest candidate from refused, got %q", out.AnswerFrom)
	}
}

func TestRecurseOrderedParallelDropsSpeculativeLogs(t *testing.T) {
	baseCtx, prof, log := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}
	specStarted := make(chan struct{})
	specBlocked := make(chan struct{})

	redirectResp := packetWithReferral("child.example", "ns.child.example")
	redirectResp.AnswerFrom = "redirect"
	childResp := packetWithA("example", netip.MustParseAddr("192.0.2.91"))
	childResp.AnswerFrom = "child"

	state := &recurseState{
		ns: []queryer{
			loggingQueryer{
				id:      "spec",
				startCh: specStarted,
				waitCh:  specBlocked,
				resp:    packetWithA("example", netip.MustParseAddr("192.0.2.90")),
			},
			loggingQueryer{
				id:        "redirect",
				waitStart: specStarted,
				resp:      redirectResp,
			},
		},
		nsFrom: func(_ context.Context, _ packet.Packet, _ *recurseState) ([]queryer, error) {
			return []queryer{loggingQueryer{id: "child", resp: childResp}}, nil
		},
	}

	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	out, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if out.AnswerFrom != "child" {
		t.Fatalf("expected child answer after redirect, got %q", out.AnswerFrom)
	}

	tags := map[string]bool{}
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		tags[entry.Tag] = true
	}
	if !tags["TEST_QUERY_REDIRECT"] || !tags["TEST_QUERY_CHILD"] {
		t.Fatalf("expected redirect and child logs, got %v", tags)
	}
	if tags["TEST_QUERY_SPEC"] {
		t.Fatalf("expected speculative logs to be dropped, got %v", tags)
	}
}

func TestRecurseUnorderedReturnsFastest(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
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
	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "fast" {
		t.Fatalf("expected fastest response from fast, got %q", resp.AnswerFrom)
	}
}

func TestRecurseUnorderedCancelsSlowQuery(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowStarted := make(chan struct{})
	slowCanceled := make(chan struct{})
	slow := testQueryer{id: "slow", waitForCancel: true, cancelCh: slowCanceled, startCh: slowStarted}

	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.22"))
	fastResp.AnswerFrom = "fast"
	fast := testQueryer{id: "fast", resp: fastResp, waitCh: slowStarted}

	state := &recurseState{ns: []queryer{fast, slow}}
	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "fast" {
		t.Fatalf("expected fastest response from fast, got %q", resp.AnswerFrom)
	}

	select {
	case <-slowCanceled:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected slow query to be canceled")
	}
}

func TestRecurseUnorderedWaitsForRedirectBatchCleanup(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowStarted := make(chan struct{})
	slowCanceled := make(chan struct{})
	cancelGate := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(cancelGate)
	}()
	slow := testQueryer{
		id:            "slow",
		waitForCancel: true,
		cancelCh:      slowCanceled,
		cancelWaitCh:  cancelGate,
		startCh:       slowStarted,
	}

	referral := packetWithReferral("example", "ns1.example")
	referral.AnswerFrom = "redirect"
	redirect := testQueryer{id: "redirect", resp: referral, waitCh: slowStarted}

	failCh := make(chan string, 1)
	nextResp := packetWithA("example", netip.MustParseAddr("192.0.2.23"))
	nextResp.AnswerFrom = "next"
	next := testQueryer{id: "next", resp: nextResp, requireClosed: slowCanceled, failCh: failCh}

	state := &recurseState{
		ns: []queryer{redirect, slow},
		nsFrom: func(_ context.Context, _ packet.Packet, _ *recurseState) ([]queryer, error) {
			select {
			case <-slowCanceled:
			case <-time.After(200 * time.Millisecond):
				return nil, errors.New("redirect nsFrom called before cancel")
			}
			return []queryer{next}, nil
		},
	}

	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "next" {
		t.Fatalf("expected next response, got %q", resp.AnswerFrom)
	}

	select {
	case id := <-failCh:
		t.Fatalf("expected redirect batch cleanup before %s started", id)
	default:
	}
}

func TestGetAddressesForUnorderedSequential(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r, err := New()
	if err != nil {
		t.Fatalf("new recursor: %v", err)
	}
	r.RemoveFakeAddresses(".")
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	ns, err := nameserver.NewWithContext(baseCtx, "root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("nameserver: %v", err)
	}

	started := make(chan string, 2)
	blockA := make(chan struct{})
	blockAAAA := make(chan struct{})

	ns.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			started <- "A"
			<-blockA
			return packetWithA(name, netip.MustParseAddr("192.0.2.44")), nil
		case "AAAA":
			started <- "AAAA"
			<-blockAAAA
			return packetWithAAAA(name, netip.MustParseAddr("2001:db8::44")), nil
		default:
			return packet.Packet{}, nil
		}
	})

	ctx := baseCtx
	done := make(chan struct{})
	var addrs []netip.Addr
	var addrErr error
	go func() {
		addrs, addrErr = r.getAddressesFor(ctx, "ns.example", nil)
		close(done)
	}()

	first := <-started
	select {
	case second := <-started:
		t.Fatalf("expected sequential A/AAAA recursion, got %s and %s", first, second)
	case <-time.After(50 * time.Millisecond):
	}

	if first == "A" {
		close(blockA)
	} else {
		close(blockAAAA)
	}

	var second string
	select {
	case second = <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected second recursion to start")
	}

	if second == "A" {
		close(blockA)
	} else {
		close(blockAAAA)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("getAddressesFor timed out")
	}

	if addrErr != nil {
		t.Fatalf("getAddressesFor: %v", addrErr)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
}

func TestRecurseUnorderedDepthLimitsWorkers(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowStarted := make(chan struct{})
	fastStarted := make(chan struct{})
	blockSlow := make(chan struct{})

	slowResp := packetWithA("example", netip.MustParseAddr("192.0.2.60"))
	slowResp.AnswerFrom = "slow"
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.61"))
	fastResp.AnswerFrom = "fast"

	slow := testQueryer{id: "slow", resp: slowResp, startCh: slowStarted, waitCh: blockSlow}
	fast := testQueryer{id: "fast", resp: fastResp, startCh: fastStarted}

	state := &recurseState{ns: []queryer{slow, fast}}
	ctx := withUnorderedDepth(withUnorderedContext(baseCtx), 1)

	done := make(chan struct{})
	go func() {
		_, _, _ = r.recurse(ctx, "example", "A", "IN", state)
		close(done)
	}()

	select {
	case <-slowStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected slow query to start")
	}

	select {
	case <-fastStarted:
		t.Fatalf("expected fast query to wait for slow in nested unordered context")
	case <-time.After(50 * time.Millisecond):
	}

	close(blockSlow)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("expected recurse to finish")
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
	id            string
	delay         time.Duration
	resp          packet.Packet
	called        chan string
	waitForCancel bool
	cancelCh      chan struct{}
	cancelWaitCh  <-chan struct{}
	startCh       chan struct{}
	waitCh        <-chan struct{}
	requireClosed <-chan struct{}
	failCh        chan<- string
}

func (t testQueryer) QueryWithClass(ctx context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	if t.called != nil {
		t.called <- t.id
	}
	if t.startCh != nil {
		select {
		case <-t.startCh:
		default:
			close(t.startCh)
		}
	}
	if t.requireClosed != nil {
		select {
		case <-t.requireClosed:
		default:
			if t.failCh != nil {
				t.failCh <- t.id
			}
		}
	}
	if t.waitCh != nil {
		select {
		case <-t.waitCh:
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	}
	if t.waitForCancel {
		<-ctx.Done()
		if t.cancelWaitCh != nil {
			<-t.cancelWaitCh
		}
		if t.cancelCh != nil {
			close(t.cancelCh)
		}
		return packet.Packet{}, ctx.Err()
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

type loggingQueryer struct {
	id        string
	resp      packet.Packet
	startCh   chan struct{}
	waitStart <-chan struct{}
	waitCh    <-chan struct{}
}

func (q loggingQueryer) QueryWithClass(ctx context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	if q.waitStart != nil {
		select {
		case <-q.waitStart:
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	}
	if q.startCh != nil {
		select {
		case <-q.startCh:
		default:
			close(q.startCh)
		}
	}
	if log := logger.FromContext(ctx); log != nil {
		_, _ = log.Add("TEST_QUERY_"+strings.ToUpper(q.id), map[string]any{"id": q.id}, "System", "")
	}
	if q.waitCh != nil {
		select {
		case <-q.waitCh:
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	}
	return q.resp, nil
}

func packetWithReferral(zone string, nsName string) packet.Packet {
	msg := new(dns.Msg)
	msg.Ns = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zone),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			Ns: dns.Fqdn(nsName),
		},
	}
	return packet.Packet{Msg: msg}
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
