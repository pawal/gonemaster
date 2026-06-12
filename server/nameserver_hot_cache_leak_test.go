package server

import (
	"context"
	"fmt"
	"net/netip"
	"runtime"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/rdata"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// TestHotCacheMemoryBounded simulates a large batch where each job introduces
// unique nameserver addresses. Memory should stabilise once stale addresses
// are evicted, not grow linearly with job count.
func TestHotCacheMemoryBounded(t *testing.T) {
	ttl := 500 * time.Millisecond
	hc := newNameserverHotCache(8, ttl)

	dummyPacket := func(addr string) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.Header{Name: "x.example.", Class: dns.ClassINET, TTL: 60},
				A:   rdata.A{Addr: netip.MustParseAddr(addr)},
			},
		}
		return packet.Packet{Msg: msg}
	}

	// Simulate 200 jobs, each touching one shared root address + one unique address.
	for i := range 200 {
		runCache, release := hc.Lease("batch")

		// Shared root nameserver (always warm).
		rootNS, err := nameserver.NewWithCache(runCache, "root.example", "198.41.0.4", nil)
		if err != nil {
			t.Fatalf("job %d root: %v", i, err)
		}
		rootNS.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			return dummyPacket("198.41.0.4"), nil
		})
		rootNS.QueryWithOptions(context.Background(), "example.", "A", nil)

		// Unique per-domain nameserver.
		uniqueAddr := fmt.Sprintf("10.%d.%d.%d", (i/65536)%256, (i/256)%256, i%256+1)
		uniqueNS, err := nameserver.NewWithCache(runCache, fmt.Sprintf("ns%d.example", i), uniqueAddr, nil)
		if err != nil {
			t.Fatalf("job %d unique: %v", i, err)
		}
		uniqueNS.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			return dummyPacket(uniqueAddr), nil
		})
		uniqueNS.QueryWithOptions(context.Background(), fmt.Sprintf("ns%d.example.", i), "A", nil)

		release()
	}

	// Force GC and measure.
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Wait for TTL to expire, then run one more job to trigger eviction.
	time.Sleep(ttl + 100*time.Millisecond)
	runCache, release := hc.Lease("batch")
	rootNS, _ := nameserver.NewWithCache(runCache, "root.example", "198.41.0.4", nil)
	rootNS.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return dummyPacket("198.41.0.4"), nil
	})
	rootNS.QueryWithOptions(context.Background(), "example.", "A", nil)
	release()

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	// Check: the base cache should only have the root address's data, not all 200.
	finalRun, finalRelease := hc.Lease("batch")
	defer finalRelease()
	addrCount := finalRun.AddressCacheCount()

	t.Logf("heap before eviction: %d KB, after: %d KB, base addresses: %d",
		before.HeapInuse/1024, after.HeapInuse/1024, addrCount)

	// We expect a small number of warmed addresses (root + maybe a few recent),
	// not all 200+ unique addresses.
	if addrCount > 20 {
		t.Fatalf("expected most stale addresses evicted, but base still has %d", addrCount)
	}
}

// TestHotCacheHeapGrowthWithForcedGC mirrors a memprobe pattern: run
// many jobs sequentially, call runtime.GC() after each, and check
// whether HeapInuse stabilises. If the heap grows linearly with job
// count, we have a true retention leak (not GC lag).
func TestHotCacheHeapGrowthWithForcedGC(t *testing.T) {
	if testing.Short() {
		t.Skip("long-running memprobe")
	}

	ttl := 250 * time.Millisecond
	hc := newNameserverHotCache(8, ttl)

	dummyPacket := func(addr string) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.Header{Name: "x.example.", Class: dns.ClassINET, TTL: 60},
				A:   rdata.A{Addr: netip.MustParseAddr(addr)},
			},
		}
		return packet.Packet{Msg: msg}
	}

	const (
		totalJobs       = 500
		addrsPerJob     = 5
		checkpointEvery = 50
	)

	type checkpoint struct {
		job       int
		heapInuse uint64
		heapAlloc uint64
		objects   uint64
		baseAddrs int
	}
	var checkpoints []checkpoint

	for job := range totalJobs {
		runCache, release := hc.Lease("batch")

		// Always hit the root (kept warm).
		rootNS, _ := nameserver.NewWithCache(runCache, "root.example", "198.41.0.4", nil)
		rootNS.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			return dummyPacket("198.41.0.4"), nil
		})
		rootNS.QueryWithOptions(context.Background(), "example.", "A", nil)

		// Touch several unique addresses per job (simulating per-domain nameservers).
		for k := range addrsPerJob {
			idx := job*addrsPerJob + k
			addr := fmt.Sprintf("10.%d.%d.%d", (idx/62500)%250, (idx/250)%250, idx%250+1)
			ns, err := nameserver.NewWithCache(runCache, fmt.Sprintf("ns%d-%d.example", job, k), addr, nil)
			if err != nil {
				t.Fatalf("new nameserver %d-%d: %v", job, k, err)
			}
			ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
				return dummyPacket(addr), nil
			})
			ns.QueryWithOptions(context.Background(), fmt.Sprintf("q%d-%d.example.", job, k), "A", nil)
		}

		release()

		// Sleep so TTLs have a chance to expire regularly.
		if job%10 == 9 {
			time.Sleep(ttl + 50*time.Millisecond)
		}

		if (job+1)%checkpointEvery == 0 {
			// Take the base's warmed address count before forcing GC.
			probeRun, probeRelease := hc.Lease("batch")
			baseAddrs := probeRun.AddressCacheCount()
			probeRelease()

			runtime.GC()
			runtime.GC() // run twice: ensures finalisers + full sweep
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			checkpoints = append(checkpoints, checkpoint{
				job:       job + 1,
				heapInuse: m.HeapInuse,
				heapAlloc: m.HeapAlloc,
				objects:   m.HeapObjects,
				baseAddrs: baseAddrs,
			})
		}
	}

	for _, cp := range checkpoints {
		t.Logf("after %3d jobs: HeapInuse=%7d KB  HeapAlloc=%7d KB  Objects=%8d  baseAddrs=%d",
			cp.job, cp.heapInuse/1024, cp.heapAlloc/1024, cp.objects, cp.baseAddrs)
	}

	// Compare middle to end: if we're in steady state, heap growth should be
	// small. A real leak would show linear growth across checkpoints.
	if len(checkpoints) >= 3 {
		mid := checkpoints[len(checkpoints)/2]
		end := checkpoints[len(checkpoints)-1]
		growth := int64(end.heapInuse) - int64(mid.heapInuse)
		ratio := float64(end.heapInuse) / float64(mid.heapInuse)
		t.Logf("steady-state growth: mid=%d KB end=%d KB growth=%+d KB (%.2fx)",
			mid.heapInuse/1024, end.heapInuse/1024, growth/1024, ratio)
		if ratio > 1.5 {
			t.Fatalf("heap grew %.2fx between checkpoint %d and %d (expected <1.5x)",
				ratio, mid.job, end.job)
		}
	}
}

// TestHotCacheHeapGrowthWithoutForcedGC shows the worker-level allocation
// pattern under the default GC pacer (GOGC=100). If this grows unboundedly
// while the forced-GC variant stays flat, the apparent "leak" is just GC
// pacing - fix with GOGC/GOMEMLIMIT, not code changes.
func TestHotCacheHeapGrowthWithoutForcedGC(t *testing.T) {
	if testing.Short() {
		t.Skip("long-running memprobe")
	}

	ttl := 250 * time.Millisecond
	hc := newNameserverHotCache(8, ttl)

	dummyPacket := func(addr string) packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.Header{Name: "x.example.", Class: dns.ClassINET, TTL: 60},
				A:   rdata.A{Addr: netip.MustParseAddr(addr)},
			},
		}
		return packet.Packet{Msg: msg}
	}

	const (
		totalJobs       = 500
		addrsPerJob     = 5
		checkpointEvery = 50
	)

	for job := range totalJobs {
		runCache, release := hc.Lease("batch")

		rootNS, _ := nameserver.NewWithCache(runCache, "root.example", "198.41.0.4", nil)
		rootNS.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			return dummyPacket("198.41.0.4"), nil
		})
		rootNS.QueryWithOptions(context.Background(), "example.", "A", nil)

		for k := range addrsPerJob {
			idx := job*addrsPerJob + k
			addr := fmt.Sprintf("10.%d.%d.%d", (idx/62500)%250, (idx/250)%250, idx%250+1)
			ns, err := nameserver.NewWithCache(runCache, fmt.Sprintf("ns%d-%d.example", job, k), addr, nil)
			if err != nil {
				t.Fatalf("new nameserver %d-%d: %v", job, k, err)
			}
			ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
				return dummyPacket(addr), nil
			})
			ns.QueryWithOptions(context.Background(), fmt.Sprintf("q%d-%d.example.", job, k), "A", nil)
		}

		release()

		if job%10 == 9 {
			time.Sleep(ttl + 50*time.Millisecond)
		}

		if (job+1)%checkpointEvery == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m) // no forced GC
			t.Logf("after %3d jobs (no GC): HeapInuse=%7d KB  HeapAlloc=%7d KB  Sys=%7d KB  NumGC=%d",
				job+1, m.HeapInuse/1024, m.HeapAlloc/1024, m.Sys/1024, m.NumGC)
		}
	}
}
