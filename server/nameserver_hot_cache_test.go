package server

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestNameserverHotCacheLeaseMergesWarmData(t *testing.T) {
	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	cache := newNameserverHotCache(8, time.Minute)
	cache.now = func() time.Time { return now }

	runCache, release := cache.Lease("alpha")
	if runCache == nil {
		t.Fatalf("expected run cache")
	}
	if got := runCache.AddressCacheCount(); got != 0 {
		t.Fatalf("run cache address count = %d, want 0", got)
	}
	if _, err := nameserver.NewWithCache(runCache, "ns1.example", "192.0.2.10", nil); err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if got := runCache.AddressCacheCount(); got != 1 {
		t.Fatalf("run cache address count = %d, want 1", got)
	}
	if got := runCache.NameserverObjectCount(); got != 1 {
		t.Fatalf("run cache object count = %d, want 1", got)
	}
	release()

	nextRunCache, nextRelease := cache.Lease("alpha")
	defer nextRelease()
	if got := nextRunCache.AddressCacheCount(); got != 1 {
		t.Fatalf("next run cache address count = %d, want 1", got)
	}
	if got := nextRunCache.ErrorCacheCount(); got != 1 {
		t.Fatalf("next run cache error count = %d, want 1", got)
	}
	if got := nextRunCache.NameserverObjectCount(); got != 0 {
		t.Fatalf("next run cache object count = %d, want 0", got)
	}
}

func TestNameserverHotCacheLeaseTTLInvalidatesEntry(t *testing.T) {
	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	cache := newNameserverHotCache(8, 10*time.Second)
	cache.now = func() time.Time { return now }

	runCache, release := cache.Lease("alpha")
	if _, err := nameserver.NewWithCache(runCache, "ns1.example", "192.0.2.20", nil); err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	release()

	now = now.Add(11 * time.Second)
	nextRunCache, nextRelease := cache.Lease("alpha")
	defer nextRelease()
	if got := nextRunCache.AddressCacheCount(); got != 0 {
		t.Fatalf("expired entry should not be reused, address count = %d", got)
	}
}

func TestNameserverHotCacheLeaseDistinctKeysIsolated(t *testing.T) {
	cache := newNameserverHotCache(8, time.Minute)

	runCacheA, releaseA := cache.Lease("alpha")
	if _, err := nameserver.NewWithCache(runCacheA, "ns1.example", "192.0.2.30", nil); err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	releaseA()

	runCacheB, releaseB := cache.Lease("beta")
	defer releaseB()
	if got := runCacheB.AddressCacheCount(); got != 0 {
		t.Fatalf("beta cache should start cold, address count = %d", got)
	}
}

func TestNameserverHotCacheLeaseEvictsOldestOnCapacity(t *testing.T) {
	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	cache := newNameserverHotCache(1, time.Minute)
	cache.now = func() time.Time { return now }

	runA, releaseA := cache.Lease("alpha")
	if _, err := nameserver.NewWithCache(runA, "ns1.example", "192.0.2.40", nil); err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	releaseA()

	now = now.Add(time.Second)
	runB, releaseB := cache.Lease("beta")
	if _, err := nameserver.NewWithCache(runB, "ns2.example", "192.0.2.41", nil); err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	releaseB()

	now = now.Add(time.Second)
	runA2, releaseA2 := cache.Lease("alpha")
	defer releaseA2()
	if got := runA2.AddressCacheCount(); got != 0 {
		t.Fatalf("expected alpha to be evicted, address count = %d", got)
	}
}

func TestNameserverHotCacheKeyUsesEffectiveProfileInputs(t *testing.T) {
	timeoutA := 2
	timeoutB := 5
	reqA := engine.RunRequest{
		Profile: "profile-a.json",
		Timeout: &timeoutA,
	}
	reqASame := engine.RunRequest{
		Profile: "profile-a.json",
		Timeout: &timeoutA,
	}
	reqB := engine.RunRequest{
		Profile: "profile-a.json",
		Timeout: &timeoutB,
	}

	keyA := nameserverHotCacheKey(reqA)
	keyASame := nameserverHotCacheKey(reqASame)
	keyB := nameserverHotCacheKey(reqB)

	if keyA != keyASame {
		t.Fatalf("expected equivalent requests to produce the same key")
	}
	if keyA == keyB {
		t.Fatalf("expected different timeout override to produce a different key")
	}
}

func TestNameserverHotCacheLeasesCoalesceInflightQueries(t *testing.T) {
	cache := newNameserverHotCache(8, time.Minute)
	runCacheA, releaseA := cache.Lease("alpha")
	defer releaseA()
	runCacheB, releaseB := cache.Lease("alpha")
	defer releaseB()

	nsA, err := nameserver.NewWithCache(runCacheA, "ns.example", "192.0.2.88", nil)
	if err != nil {
		t.Fatalf("new nameserver A: %v", err)
	}
	nsB, err := nameserver.NewWithCache(runCacheB, "ns.example", "192.0.2.88", nil)
	if err != nil {
		t.Fatalf("new nameserver B: %v", err)
	}

	var callsA atomic.Int32
	var callsB atomic.Int32
	started := make(chan struct{}, 1)
	releaseNetwork := make(chan struct{})

	queryHook := func(counter *atomic.Int32) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			counter.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			<-releaseNetwork

			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			msg.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "hot-cache.example.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: net.IPv4(192, 0, 2, 88),
				},
			}
			return packet.Packet{Msg: msg}, nil
		}
	}

	nsA.SetQueryHook(queryHook(&callsA))
	nsB.SetQueryHook(queryHook(&callsB))

	errCh := make(chan error, 2)
	go func() {
		_, err := nsA.QueryWithOptions(context.Background(), "hot-cache.example", "A", nil)
		errCh <- err
	}()
	go func() {
		_, err := nsB.QueryWithOptions(context.Background(), "hot-cache.example", "A", nil)
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected one network query to start")
	}
	close(releaseNetwork)

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("query %d error: %v", i+1, err)
		}
	}

	totalCalls := callsA.Load() + callsB.Load()
	if totalCalls != 1 {
		t.Fatalf("expected one deduplicated network call, got %d (A=%d B=%d)", totalCalls, callsA.Load(), callsB.Load())
	}
}
