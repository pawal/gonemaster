package server

import (
	"fmt"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

// warmHotCacheEntry runs one Lease+release cycle to populate a hot-cache key
// with n pre-warmed address entries, so subsequent Lease calls see a warmed base.
func warmHotCacheEntry(b *testing.B, hc *nameserverHotCache, key string, n int) {
	b.Helper()
	store, release := hc.Lease(key)
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("10.%d.%d.%d", (i/65536)%256, (i/256)%256, i%256)
		if _, err := nameserver.NewWithCache(store, fmt.Sprintf("ns%d.example", i), addr, nil); err != nil {
			b.Fatalf("NewWithCache addr %d: %v", i, err)
		}
	}
	release()
}

// BenchmarkHotCacheLease measures the full Lease+release cycle cost as the
// number of pre-warmed addresses in the hot-cache entry grows. This is the
// per-job overhead of the cross-job hot-cache feature in steady state.
func BenchmarkHotCacheLease(b *testing.B) {
	for _, n := range []int{0, 10, 100, 1000} {
		n := n
		b.Run(fmt.Sprintf("addrs%04d", n), func(b *testing.B) {
			hc := newNameserverHotCache(0, 0)
			warmHotCacheEntry(b, hc, "bench-key", n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, release := hc.Lease("bench-key")
				release()
			}
		})
	}
}
