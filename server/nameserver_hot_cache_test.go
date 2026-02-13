package server

import (
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
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
