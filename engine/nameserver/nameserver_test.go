package nameserver

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

func TestFakeDSResponse(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	err = ns.AddFakeDS("example", []DSData{{
		KeyTag:     1234,
		Algorithm:  8,
		DigestType: 2,
		Digest:     "ABCD",
	}})
	if err != nil {
		t.Fatalf("add fake DS: %v", err)
	}

	resp, err := ns.QueryWithOptions(context.Background(), "example", "DS", nil)
	if err != nil {
		t.Fatalf("query fake DS: %v", err)
	}
	if resp.Msg == nil || len(resp.Msg.Answer) != 1 {
		t.Fatalf("expected DS answer, got %#v", resp.Msg)
	}
	if _, ok := resp.Msg.Answer[0].(*dns.DS); !ok {
		t.Fatalf("expected DS record, got %T", resp.Msg.Answer[0])
	}
	if !resp.Msg.Authoritative {
		t.Fatalf("expected authoritative response")
	}
}

func TestFakeDelegationNS(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	err = ns.AddFakeDelegation("example", map[string][]string{
		"ns1.example.": {"192.0.2.2"},
	})
	if err != nil {
		t.Fatalf("add fake delegation: %v", err)
	}

	resp, err := ns.QueryWithOptions(context.Background(), "example", "NS", nil)
	if err != nil {
		t.Fatalf("query fake delegation: %v", err)
	}
	if resp.Msg == nil || len(resp.Msg.Answer) == 0 {
		t.Fatalf("expected delegation answer, got %#v", resp.Msg)
	}
	if _, ok := resp.Msg.Answer[0].(*dns.NS); !ok {
		t.Fatalf("expected NS record, got %T", resp.Msg.Answer[0])
	}
	if len(resp.Msg.Extra) == 0 {
		t.Fatalf("expected glue in additional section")
	}
}

func TestQueryCacheHit(t *testing.T) {
	cache := NewCacheStore()
	ns, err := NewWithCache(cache, "ns.example", "192.0.2.10", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		aRR := &dns.A{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 60}}
		aRR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 5})
		msg.Answer = []dns.RR{aRR}
		return packet.Packet{Msg: msg}, nil
	})

	_, err = ns.QueryWithOptions(context.Background(), "example", "A", nil)
	if err != nil {
		t.Fatalf("query 1: %v", err)
	}
	_, err = ns.QueryWithOptions(context.Background(), "example", "A", nil)
	if err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	metrics := cache.QueryMetrics()
	if metrics.Hits != 1 || metrics.Misses != 1 || metrics.Evictions != 0 {
		t.Fatalf("unexpected query cache metrics: %+v", metrics)
	}
}

func TestInflightQueryCoalescing(t *testing.T) {
	cache := NewCacheStore()
	ns, err := NewWithCache(cache, "ns.example", "192.0.2.51", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, _ := testContext(t)

	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	var calls int32
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		atomic.AddInt32(&calls, 1)
		startOnce.Do(func() { close(started) })
		<-release
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	var wg sync.WaitGroup
	wg.Add(2)
	var err1, err2 error
	go func() {
		defer wg.Done()
		_, err1 = ns.QueryWithOptions(ctx, "example", "A", nil)
	}()

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected first query to start")
	}

	go func() {
		defer wg.Done()
		_, err2 = ns.QueryWithOptions(ctx, "example", "A", nil)
	}()

	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected single inflight call, got %d", got)
	}

	close(release)
	wg.Wait()

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v %v", err1, err2)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one query call total, got %d", got)
	}
}

func TestCacheStoreIsolation(t *testing.T) {
	cacheA := NewCacheStore()
	cacheB := NewCacheStore()

	nsA, err := NewWithCache(cacheA, "ns.example", "192.0.2.30", nil)
	if err != nil {
		t.Fatalf("new nameserver A: %v", err)
	}
	nsB, err := NewWithCache(cacheB, "ns.example", "192.0.2.30", nil)
	if err != nil {
		t.Fatalf("new nameserver B: %v", err)
	}

	var callsA int
	nsA.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		callsA++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	var callsB int
	nsB.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		callsB++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	_, _ = nsA.QueryWithOptions(context.Background(), "example", "A", nil)
	_, _ = nsA.QueryWithOptions(context.Background(), "example", "A", nil)
	if callsA != 1 {
		t.Fatalf("expected cache hit in store A, got %d calls", callsA)
	}

	_, _ = nsB.QueryWithOptions(context.Background(), "example", "A", nil)
	if callsB != 1 {
		t.Fatalf("expected isolated cache store, got %d calls", callsB)
	}
}

func TestErrorCacheSkipsQueries(t *testing.T) {
	cache := NewCacheStore()
	ns, err := NewWithCache(cache, "ns.example", "192.0.2.15", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 60

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("network error")
	})

	_, err = ns.QueryWithOptions(ctx, "example1", "A", nil)
	if err == nil {
		t.Fatalf("expected error on first query")
	}
	_, err = ns.QueryWithOptions(ctx, "example2", "A", nil)
	if err != nil {
		t.Fatalf("expected error cache to suppress second query error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call due to error cache, got %d", calls)
	}

	metrics := cache.ErrorMetrics()
	if metrics.Hits != 1 || metrics.Misses != 1 || metrics.Evictions != 0 {
		t.Fatalf("unexpected error cache metrics: %+v", metrics)
	}
}

func TestErrorCacheTTLRespectsTimeoutBudget(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.31", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 60

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("network error")
	})

	timeout := 20 * time.Millisecond
	retry := 1
	opts := &QueryOptions{Timeout: &timeout, Retry: &retry}

	_, err = ns.QueryWithOptions(ctx, "example1", "A", opts)
	if err == nil {
		t.Fatalf("expected error on first query")
	}
	time.Sleep(60 * time.Millisecond)

	_, err = ns.QueryWithOptions(ctx, "example2", "A", opts)
	if err == nil {
		t.Fatalf("expected error on second query")
	}
	if calls != 2 {
		t.Fatalf("expected cache to expire based on timeout budget, got %d calls", calls)
	}
}

func TestQueryCacheStoresTimeoutsAsNoMessage(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.250", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	opts := &QueryOptions{BlacklistingDisabled: true}
	pkt, err := ns.QueryWithOptions(ctx, "example", "SOA", opts)
	if err == nil {
		t.Fatalf("expected error on first query")
	}
	if pkt.Msg != nil {
		t.Fatalf("expected nil msg on first timeout, got %v", pkt.Msg)
	}
	pkt, err = ns.QueryWithOptions(ctx, "example", "SOA", opts)
	if err != nil {
		t.Fatalf("expected cached no-response on second query, got error: %v", err)
	}
	if pkt.Msg != nil {
		t.Fatalf("expected cached no-response on second query, got msg: %v", pkt.Msg)
	}
	if calls != 1 {
		t.Fatalf("expected 1 network call after caching the timeout, got %d", calls)
	}
}

func TestQueryCacheDoesNotStoreContextCancelErrors(t *testing.T) {
	ctx, _ := testContext(t)
	cctx, cancel := context.WithCancel(ctx)

	ns, err := NewWithContext(cctx, "ns.example", "192.0.2.251", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		cancel()
		return packet.Packet{}, context.Canceled
	})

	opts := &QueryOptions{BlacklistingDisabled: true}
	_, _ = ns.QueryWithOptions(cctx, "example", "SOA", opts)
	// New context for the second call so it isn't short-circuited by ctx.Err().
	cctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})
	_, err = ns.QueryWithOptions(cctx2, "example", "SOA", opts)
	if err == nil {
		t.Fatalf("expected error on second query (cancellation must not have cached nil)")
	}
	if calls != 2 {
		t.Fatalf("expected 2 network calls, got %d (cancellation should not cache)", calls)
	}
}

func TestContextCanceledDoesNotBlacklist(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.200", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, context.Canceled
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = ns.QueryWithOptions(ctx, "example", "SOA", nil)

	if ns.state == nil {
		t.Fatalf("expected state to be initialized")
	}
	if ns.state.blacklisted[false] {
		t.Fatalf("expected UDP not to be blacklisted on context cancellation")
	}
}

func TestReachabilityCacheSkipsAcrossCaches(t *testing.T) {
	clearReachabilityCache()
	t.Cleanup(clearReachabilityCache)

	nsA, err := NewWithCache(NewCacheStore(), "ns.example", "192.0.2.44", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	nsB, err := NewWithCache(NewCacheStore(), "ns.example", "192.0.2.44", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NegativeCacheTTL = 60

	var callsA int
	nsA.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		callsA++
		return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
	})

	var callsB int
	nsB.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		callsB++
		return packet.Packet{}, nil
	})

	_, err = nsA.QueryWithOptions(ctx, "example", "A", nil)
	if err == nil {
		t.Fatalf("expected error on first query")
	}

	_, err = nsB.QueryWithOptions(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("expected reachability cache to skip error, got %v", err)
	}
	if callsA != 1 {
		t.Fatalf("expected first call to run, got %d", callsA)
	}
	if callsB != 0 {
		t.Fatalf("expected second call to be skipped by reachability cache, got %d", callsB)
	}

	metrics := reachabilityMetricsSnapshot()
	if metrics.Hits != 1 || metrics.Misses != 1 || metrics.Evictions != 0 {
		t.Fatalf("unexpected reachability metrics: %+v", metrics)
	}
}

func TestReachabilityCacheExpiresByBudget(t *testing.T) {
	clearReachabilityCache()
	t.Cleanup(clearReachabilityCache)

	ns, err := NewWithCache(NewCacheStore(), "ns.example", "192.0.2.55", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NegativeCacheTTL = 60

	timeout := 15 * time.Millisecond
	retry := 0
	opts := &QueryOptions{Timeout: &timeout, Retry: &retry}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
	})

	_, err = ns.QueryWithOptions(ctx, "example", "A", opts)
	if err == nil {
		t.Fatalf("expected error on first query")
	}

	time.Sleep(40 * time.Millisecond)

	// Different qname so per-query cache miss forces reachability path to run.
	_, err = ns.QueryWithOptions(ctx, "another.example", "A", opts)
	if err == nil {
		t.Fatalf("expected error after reachability cache expiry")
	}
	if calls != 2 {
		t.Fatalf("expected reachability cache to expire and re-query, got %d calls", calls)
	}
}

func TestQueryIPv4Disabled(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.11", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Net.IPv4 = false

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, nil
	})

	resp, err := ns.QueryWithOptions(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if resp.Msg != nil {
		t.Fatalf("expected no response when IPv4 disabled")
	}
	if calls != 0 {
		t.Fatalf("expected hook not called, got %d", calls)
	}
}

func TestClientForOptionsDefaults(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.12", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.Recurse = true
	prof.Resolver.Defaults.UseVC = true

	client, err := ns.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("client for nil opts: %v", err)
	}
	if client.RecursionDesired {
		t.Fatalf("expected recursion disabled by default")
	}
	if client.UseTCP {
		t.Fatalf("expected UDP by default")
	}

	on := true
	client, err = ns.clientForOptions(ctx, &QueryOptions{Recurse: &on, UseVC: &on})
	if err != nil {
		t.Fatalf("client for explicit opts: %v", err)
	}
	if !client.RecursionDesired {
		t.Fatalf("expected recursion enabled when requested")
	}
	if !client.UseTCP {
		t.Fatalf("expected TCP when requested")
	}
}

func TestClientForOptionsAppliesProfileSourceAddressByFamily(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Source4 = "192.0.2.88"
	prof.Resolver.Source6 = "2001:db8::88"

	ns4, err := New("ns4.example", "192.0.2.30", nil)
	if err != nil {
		t.Fatalf("new ipv4 nameserver: %v", err)
	}
	client4, err := ns4.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("client4: %v", err)
	}
	if client4.SourceIP != "192.0.2.88" {
		t.Fatalf("client4.SourceIP = %q, want 192.0.2.88", client4.SourceIP)
	}

	ns6, err := New("ns6.example", "2001:db8::30", nil)
	if err != nil {
		t.Fatalf("new ipv6 nameserver: %v", err)
	}
	client6, err := ns6.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("client6: %v", err)
	}
	if client6.SourceIP != "2001:db8::88" {
		t.Fatalf("client6.SourceIP = %q, want 2001:db8::88", client6.SourceIP)
	}

	explicit := &transport.Client{SourceIP: "192.0.2.199"}
	nsExplicit, err := New("ns-explicit.example", "192.0.2.31", explicit)
	if err != nil {
		t.Fatalf("new explicit nameserver: %v", err)
	}
	clientExplicit, err := nsExplicit.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("clientExplicit: %v", err)
	}
	if clientExplicit.SourceIP != "192.0.2.199" {
		t.Fatalf("expected explicit source ip to be preserved, got %q", clientExplicit.SourceIP)
	}
}

func TestAXFRHook(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.22", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	rr1, err := dns.New("example. 60 IN SOA ns.example. hostmaster.example. 1 3600 600 86400 60")
	if err != nil {
		t.Fatalf("soa rr: %v", err)
	}
	rr2, err := dns.New("example. 60 IN A 192.0.2.10")
	if err != nil {
		t.Fatalf("a rr: %v", err)
	}

	ns.SetAXFRHook(func(_ context.Context, domain string, callback func(dns.RR) bool, class string) error {
		if domain != "example" {
			return fmt.Errorf("unexpected domain %q", domain)
		}
		if class != "CH" {
			return fmt.Errorf("unexpected class %q", class)
		}
		callback(rr1)
		callback(rr2)
		return nil
	})

	var seen int
	err = ns.AXFR(context.Background(), "example", func(_ dns.RR) bool {
		seen++
		return true
	}, "CH")
	if err != nil {
		t.Fatalf("axfr hook: %v", err)
	}
	if seen != 2 {
		t.Fatalf("expected 2 records, got %d", seen)
	}
}

func TestAXFRNoNetwork(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.23", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.NoNetwork = true

	err = ns.AXFR(ctx, "example", nil, "")
	if err == nil {
		t.Fatalf("expected error when no_network is set")
	}
	if !strings.Contains(err.Error(), "external AXFR query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAXFRIPv4Disabled(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.24", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Net.IPv4 = false

	var called int
	ns.SetAXFRHook(func(_ context.Context, _ string, _ func(dns.RR) bool, _ string) error {
		called++
		return nil
	})

	err = ns.AXFR(ctx, "example", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 0 {
		t.Fatalf("expected hook not called, got %d", called)
	}
}

func TestEmptyCache(t *testing.T) {
	cache := NewCacheStore()
	ns, err := NewWithCache(cache, "ns.example", "192.0.2.25", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if ns.state == nil || ns.state.cache == nil {
		t.Fatalf("expected nameserver cache state")
	}

	cacheKey := "example-cache"
	ns.state.cache.set(cacheKey, &packet.Packet{Msg: new(dns.Msg)})
	if _, ok := ns.state.cache.get(cacheKey); !ok {
		t.Fatalf("expected cached packet")
	}

	nameKey := strings.ToLower(ns.Name.String())
	addrKey := ns.Address.String()
	if cached := cache.cachedNameserver(nameKey, addrKey); cached == nil {
		t.Fatalf("expected nameserver cached")
	}

	cache.Empty()

	if cached := cache.cachedNameserver(nameKey, addrKey); cached != nil {
		t.Fatalf("expected nameserver cache cleared")
	}
	if _, ok := ns.state.cache.get(cacheKey); ok {
		t.Fatalf("expected query cache cleared")
	}
}

func TestQueryLogging(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	ns, err := NewWithContext(ctx, "ns.example", "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	// Use a very short timeout since we expect network failure
	timeout := 10 * time.Millisecond
	opts := &QueryOptions{Timeout: &timeout}

	// This query will likely fail due to no server at 127.0.0.1:53, but logging happens before exchange
	_, _ = ns.QueryWithOptions(ctx, "example.com", "SOA", opts)

	var foundQuery bool
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		if entry.Tag == "EXTERNAL_QUERY" {
			foundQuery = true
			loggedArgs := entry.Args
			if name, ok := loggedArgs["query_name"]; !ok || name != "example.com" {
				t.Errorf("expected query_name=example.com, got %v", name)
			}
			if qtype, ok := loggedArgs["query_type"]; !ok || qtype != "SOA" {
				t.Errorf("expected query_type=SOA, got %v", qtype)
			}
			if qclass, ok := loggedArgs["query_class"]; !ok || qclass != "IN" {
				t.Errorf("expected query_class=IN, got %v", qclass)
			}
			if address, ok := loggedArgs["address"]; !ok || address != "127.0.0.1" {
				t.Errorf("expected address=127.0.0.1, got %v", address)
			}
			if _, ok := loggedArgs["ip"]; ok {
				t.Errorf("legacy key ip should not be present, got %v", loggedArgs["ip"])
			}
			if flags, ok := loggedArgs["flags"]; !ok || flags != "{\"class\":\"IN\"}" {
				t.Errorf("expected flags={\"class\":\"IN\"}, got %v", flags)
			}
			break
		}
	}

	if !foundQuery {
		t.Errorf("expected EXTERNAL_QUERY tag, got %v", log.Entries())
	}
}

func TestConstructorEmitsCreationLogs(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	if _, err := NewWithContext(ctx, "ns1.example", "192.0.2.77", nil); err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if _, err := NewWithContext(ctx, "ns2.example", "192.0.2.77", nil); err != nil {
		t.Fatalf("new nameserver with shared cache: %v", err)
	}

	var cacheCreated, cacheFetched, nsCreated int
	seenNS := map[string]bool{}
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		switch entry.Tag {
		case "CACHE_CREATED":
			cacheCreated++
		case "CACHE_FETCHED":
			cacheFetched++
		case "NS_CREATED":
			nsCreated++
			ns, _ := entry.Args["ns"].(string)
			address, _ := entry.Args["address"].(string)
			if ns == "" || address == "" {
				t.Fatalf("expected typed ns/address args in NS_CREATED, got %#v", entry.Args)
			}
			seenNS[ns] = true
			if _, ok := entry.Args["name"]; ok {
				t.Fatalf("legacy key name should not be present: %#v", entry.Args)
			}
		}
	}
	if cacheCreated != 1 {
		t.Fatalf("expected 1 CACHE_CREATED, got %d", cacheCreated)
	}
	if cacheFetched != 1 {
		t.Fatalf("expected 1 CACHE_FETCHED, got %d", cacheFetched)
	}
	if nsCreated != 2 {
		t.Fatalf("expected 2 NS_CREATED, got %d", nsCreated)
	}
	if !seenNS["ns1.example"] || !seenNS["ns2.example"] {
		t.Fatalf("expected NS_CREATED entries for ns1/ns2.example, got %#v", seenNS)
	}
}

func TestQueryEmitsQueryAndCachedReturn(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.80", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	var calls int
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, qclass string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.StringToType[qtype])
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	if _, err := ns.QueryWithOptions(ctx, "example", "A", nil); err != nil {
		t.Fatalf("query 1: %v", err)
	}
	if _, err := ns.QueryWithOptions(ctx, "example", "A", nil); err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 network call due to cache hit, got %d", calls)
	}

	var queryCount, cachedReturnCount int
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		switch entry.Tag {
		case "QUERY":
			queryCount++
		case "CACHED_RETURN":
			cachedReturnCount++
		}
	}
	if queryCount != 2 {
		t.Fatalf("expected 2 QUERY logs, got %d", queryCount)
	}
	if cachedReturnCount != 2 {
		t.Fatalf("expected 2 CACHED_RETURN logs, got %d", cachedReturnCount)
	}
}

func TestQueryLogsIPBlocked(t *testing.T) {
	ctx, prof := testContext(t)
	log := logger.FromContext(ctx)
	prof.Net.IPv4 = false

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.81", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if _, err := ns.QueryWithOptions(ctx, "example", "A", nil); err != nil {
		t.Fatalf("query with ipv4 disabled: %v", err)
	}

	for _, entry := range log.Entries() {
		if entry != nil && entry.Tag == "IPV4_BLOCKED" {
			return
		}
	}
	t.Fatalf("expected IPV4_BLOCKED log entry")
}

func TestBlacklistingEmitsTags(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.90", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	// First SOA query triggers blacklisting.
	_, _ = ns.QueryWithOptions(ctx, "example", "SOA", nil)
	// Different qname so the second query bypasses the per-query timeout cache
	// and reaches the IS_BLACKLISTED short-circuit.
	_, _ = ns.QueryWithOptions(ctx, "other.example", "SOA", nil)

	var hasBlacklisting, hasIsBlacklisted bool
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		switch entry.Tag {
		case "BLACKLISTING":
			hasBlacklisting = true
		case "IS_BLACKLISTED":
			hasIsBlacklisted = true
		}
	}
	if !hasBlacklisting {
		t.Fatalf("expected BLACKLISTING tag after failed SOA query")
	}
	if !hasIsBlacklisted {
		t.Fatalf("expected IS_BLACKLISTED tag on second query to blacklisted NS")
	}
}

func TestPacketBigEmitted(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.91", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		// Add enough records to exceed 4096 bytes.
		for i := range 250 {
			rr := &dns.A{Hdr: dns.Header{Name: fmt.Sprintf("host%d.example.", i), Class: dns.ClassINET, TTL: 300}}
			rr.Addr = netip.AddrFrom4([4]byte{192, 0, 2, byte(i % 256)})
			msg.Answer = append(msg.Answer, rr)
		}
		return packet.Packet{Msg: msg}, nil
	})

	_, err = ns.QueryWithOptions(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	for _, entry := range log.Entries() {
		if entry != nil && entry.Tag == "PACKET_BIG" {
			if size, ok := entry.Args["size"]; ok {
				if s, ok := size.(int); ok && s > 4096 {
					return
				}
			}
		}
	}
	t.Fatalf("expected PACKET_BIG tag for large response")
}

func testContext(t *testing.T) (context.Context, *profile.Profile) {
	t.Helper()
	prof, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	log := logger.New()
	ctx := context.Background()
	ctx = profile.WithContext(ctx, prof)
	ctx = logger.WithContext(ctx, log)
	ctx = WithCache(ctx, NewCacheStore())
	return ctx, prof
}
