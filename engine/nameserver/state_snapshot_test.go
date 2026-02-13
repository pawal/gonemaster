package nameserver

import "testing"

func TestCacheStoreSnapshotForRunSharesWarmAddressCaches(t *testing.T) {
	base := NewCacheStore()
	ns, err := NewWithCache(base, "ns.example", "192.0.2.10", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if ns.state == nil || ns.state.cache == nil || ns.state.errorCache == nil {
		t.Fatalf("expected initialized nameserver state")
	}

	snapshot := base.SnapshotForRun()
	if snapshot == nil {
		t.Fatalf("expected snapshot")
	}
	if got := snapshot.AddressCacheCount(); got != 1 {
		t.Fatalf("snapshot address cache count = %d, want 1", got)
	}
	if got := snapshot.ErrorCacheCount(); got != 1 {
		t.Fatalf("snapshot error cache count = %d, want 1", got)
	}
	if got := snapshot.NameserverObjectCount(); got != 0 {
		t.Fatalf("snapshot nameserver object count = %d, want 0", got)
	}
}

func TestCacheStoreMergeWarmDataFromPreservesObjectIsolation(t *testing.T) {
	base := NewCacheStore()
	snapshot := base.SnapshotForRun()
	if _, err := NewWithCache(snapshot, "ns.example", "192.0.2.11", nil); err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if got := snapshot.NameserverObjectCount(); got != 1 {
		t.Fatalf("snapshot nameserver object count = %d, want 1", got)
	}

	base.MergeWarmDataFrom(snapshot)

	if got := base.AddressCacheCount(); got != 1 {
		t.Fatalf("base address cache count = %d, want 1", got)
	}
	if got := base.ErrorCacheCount(); got != 1 {
		t.Fatalf("base error cache count = %d, want 1", got)
	}
	if got := base.NameserverObjectCount(); got != 0 {
		t.Fatalf("base nameserver object count = %d, want 0", got)
	}
}

func TestCacheStoreMergeWarmDataFromNilNoop(t *testing.T) {
	base := NewCacheStore()
	base.MergeWarmDataFrom(nil)
	if got := base.AddressCacheCount(); got != 0 {
		t.Fatalf("base address cache count = %d, want 0", got)
	}
}
