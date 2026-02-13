package nameserver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
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
		msg.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "example.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: net.IPv4(192, 0, 2, 5),
			},
		}
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

func TestQueryCacheDoesNotStoreErrors(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.250", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	opts := &QueryOptions{BlacklistingDisabled: true}
	_, err = ns.QueryWithOptions(ctx, "example", "SOA", opts)
	if err == nil {
		t.Fatalf("expected error on first query")
	}
	_, err = ns.QueryWithOptions(ctx, "example", "SOA", opts)
	if err == nil {
		t.Fatalf("expected error on second query")
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls without cached error, got %d", calls)
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
	if ns.state.blacklist.isBlocked(false, time.Now()) {
		t.Fatalf("expected UDP not to be blacklisted on context cancellation")
	}
}

func TestSOATimeoutBurstBlacklistsTemporarily(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.201", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, _ := testContext(t)
	timeout := 60 * time.Millisecond
	opts := &QueryOptions{Timeout: &timeout}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	_, err = ns.QueryWithOptions(ctx, "example1", "SOA", opts)
	if err == nil {
		t.Fatalf("expected first timeout error")
	}
	_, err = ns.QueryWithOptions(ctx, "example2", "SOA", opts)
	if err == nil {
		t.Fatalf("expected second timeout error")
	}
	_, err = ns.QueryWithOptions(ctx, "example3", "SOA", opts)
	if err != nil {
		t.Fatalf("expected blacklisted query to be skipped without error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 network calls before temporary blacklist, got %d", calls)
	}
}

func TestSOATemporaryBlacklistExpires(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.202", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, _ := testContext(t)
	timeout := 50 * time.Millisecond
	opts := &QueryOptions{Timeout: &timeout}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	_, _ = ns.QueryWithOptions(ctx, "example1", "SOA", opts)
	_, _ = ns.QueryWithOptions(ctx, "example2", "SOA", opts)
	_, _ = ns.QueryWithOptions(ctx, "example3", "SOA", opts)
	if calls != 2 {
		t.Fatalf("expected immediate third query to be skipped while blacklisted, got %d calls", calls)
	}

	time.Sleep(blacklistMinTTL + 80*time.Millisecond)
	_, _ = ns.QueryWithOptions(ctx, "example4", "SOA", opts)
	if calls != 3 {
		t.Fatalf("expected blacklist to expire and allow new network call, got %d calls", calls)
	}
}

func TestFastFailDisabledByDefault(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.203", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	if prof.Resolver.Defaults.FastFailTimeoutCount != 0 {
		t.Fatalf("expected default fast-fail timeout count to be 0")
	}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	for i := 0; i < 4; i++ {
		_, _ = ns.QueryWithOptions(ctx, fmt.Sprintf("example%d", i), "A", nil)
	}
	if calls != 4 {
		t.Fatalf("expected no fast-fail skipping by default, got %d calls", calls)
	}
}

func TestFastFailSkipsAfterConfiguredTimeoutThreshold(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.204", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.FastFailTimeoutCount = 2

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	_, err = ns.QueryWithOptions(ctx, "example1", "A", nil)
	if err == nil {
		t.Fatalf("expected timeout error on first call")
	}
	_, err = ns.QueryWithOptions(ctx, "example2", "A", nil)
	if err == nil {
		t.Fatalf("expected timeout error on second call")
	}
	_, err = ns.QueryWithOptions(ctx, "example3", "A", nil)
	if err != nil {
		t.Fatalf("expected third call to be skipped by fast-fail, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected only 2 network calls before fast-fail skip, got %d", calls)
	}
}

func TestFastFailDoesNotCountNonTimeoutErrors(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.205", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.FastFailTimeoutCount = 2

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("connection refused")
	})

	for i := 0; i < 4; i++ {
		_, _ = ns.QueryWithOptions(ctx, fmt.Sprintf("example%d", i), "A", nil)
	}
	if calls != 4 {
		t.Fatalf("expected non-timeout errors not to trigger fast-fail skip, got %d calls", calls)
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

	_, err = ns.QueryWithOptions(ctx, "example", "A", opts)
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

func TestAXFRHook(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.22", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	rr1, err := dns.NewRR("example. 60 IN SOA ns.example. hostmaster.example. 1 3600 600 86400 60")
	if err != nil {
		t.Fatalf("soa rr: %v", err)
	}
	rr2, err := dns.NewRR("example. 60 IN A 192.0.2.10")
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
	if !strings.Contains(err.Error(), "External AXFR query") {
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
	ns, err := New("ns.example", "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	log := logger.New()
	ctx := logger.WithContext(context.Background(), log)

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
			if name, ok := loggedArgs["name"]; !ok || name != "example.com" {
				t.Errorf("expected name=example.com, got %v", name)
			}
			if qtype, ok := loggedArgs["type"]; !ok || qtype != "SOA" {
				t.Errorf("expected type=SOA, got %v", qtype)
			}
			if ip, ok := loggedArgs["ip"]; !ok || ip != "127.0.0.1" {
				t.Errorf("expected ip=127.0.0.1, got %v", ip)
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

func TestRateLimitPacingDelaysQueryDispatch(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.70", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.RateLimitPacingEnabled = true
	prof.Resolver.Defaults.RateLimitPacingMinMS = 80
	prof.Resolver.Defaults.RateLimitPacingMaxMS = 80

	ns.state.rateLimitPacing.jitterFn = func() float64 { return 0.5 }
	ns.state.rateLimitPacing.observeResult(false, rateLimitSignalConnectionError, time.Now())

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	start := time.Now()
	_, err = ns.QueryWithOptions(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("query with pacing delay: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 65*time.Millisecond {
		t.Fatalf("expected paced delay >=65ms, got %v", elapsed)
	}
	if calls != 1 {
		t.Fatalf("expected one network call after delay, got %d", calls)
	}
}

func TestRateLimitPacingSkipsWhenDelayExceedsTimeoutBudget(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.71", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.RateLimitPacingEnabled = true
	prof.Resolver.Defaults.RateLimitPacingMinMS = 200
	prof.Resolver.Defaults.RateLimitPacingMaxMS = 200

	ns.state.rateLimitPacing.jitterFn = func() float64 { return 0.5 }
	ns.state.rateLimitPacing.observeResult(false, rateLimitSignalConnectionError, time.Now())

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	timeout := 20 * time.Millisecond
	resp, err := ns.QueryWithOptions(ctx, "example", "A", &QueryOptions{Timeout: &timeout})
	if err != nil {
		t.Fatalf("query with paced skip: %v", err)
	}
	if resp.Msg != nil {
		t.Fatalf("expected paced query skip to return empty response")
	}
	if calls != 0 {
		t.Fatalf("expected paced query skip before network dispatch, got %d calls", calls)
	}
}

func TestRateLimitPacingIsPerNameserver(t *testing.T) {
	nsSlow, err := New("ns-slow.example", "192.0.2.72", nil)
	if err != nil {
		t.Fatalf("new slow nameserver: %v", err)
	}
	nsFast, err := New("ns-fast.example", "192.0.2.73", nil)
	if err != nil {
		t.Fatalf("new fast nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.RateLimitPacingEnabled = true
	prof.Resolver.Defaults.RateLimitPacingMinMS = 150
	prof.Resolver.Defaults.RateLimitPacingMaxMS = 150

	nsSlow.state.rateLimitPacing.jitterFn = func() float64 { return 0.5 }
	nsSlow.state.rateLimitPacing.observeResult(false, rateLimitSignalConnectionError, time.Now())

	nsSlow.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})
	nsFast.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	var slowElapsed time.Duration
	var fastElapsed time.Duration
	var slowErr error
	var fastErr error

	go func() {
		defer wg.Done()
		<-start
		begin := time.Now()
		_, slowErr = nsSlow.QueryWithOptions(ctx, "slow.example", "A", nil)
		slowElapsed = time.Since(begin)
	}()

	go func() {
		defer wg.Done()
		<-start
		begin := time.Now()
		_, fastErr = nsFast.QueryWithOptions(ctx, "fast.example", "A", nil)
		fastElapsed = time.Since(begin)
	}()

	close(start)
	wg.Wait()

	if slowErr != nil {
		t.Fatalf("slow query error: %v", slowErr)
	}
	if fastErr != nil {
		t.Fatalf("fast query error: %v", fastErr)
	}
	if slowElapsed < 120*time.Millisecond {
		t.Fatalf("expected slow nameserver to be paced, got %v", slowElapsed)
	}
	if fastElapsed > 80*time.Millisecond {
		t.Fatalf("expected fast nameserver to run without pacing delay, got %v", fastElapsed)
	}
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
