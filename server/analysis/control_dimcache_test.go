package analysis

import (
	"testing"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

// countingDimStore records how often each dimension upsert reached the
// store, so the tests can tell a cache hit from a real write.
type countingDimStore struct {
	WriteStore
	nameserverCalls int
	nextID          int64
}

func (s *countingDimStore) UpsertAnalysisNameserver(name string, seenAt time.Time) (serverpkg.AnalysisNameserver, error) {
	s.nameserverCalls++
	s.nextID++
	return serverpkg.AnalysisNameserver{ID: s.nextID, Name: name, FirstSeenAt: seenAt, LastSeenAt: seenAt}, nil
}

func TestCachingWriteStoreWithholdsIDsUntilCommit(t *testing.T) {
	// The write transaction can be retried after a contention abort, and a
	// rolled back attempt's auto-increment IDs name rows that no longer
	// exist. Until the transaction commits, those IDs must stay invisible
	// to any other run sharing the cache.
	cache := newRebuildDimCache()
	inner := &countingDimStore{}
	attempt := newCachingWriteStore(inner, cache)

	seen := time.Now().UTC()
	ns, err := attempt.UpsertAnalysisNameserver("ns1.example", seen)
	if err != nil {
		t.Fatalf("UpsertAnalysisNameserver: %v", err)
	}
	if ns.ID == 0 {
		t.Fatal("expected an ID from the inner store")
	}

	cache.mu.Lock()
	_, published := cache.nameservers["ns1.example"]
	cache.mu.Unlock()
	if published {
		t.Fatal("an uncommitted ID must not be published to the shared cache")
	}

	// A concurrent run on the same cache must not see the pending ID and
	// therefore has to do its own upsert.
	other := newCachingWriteStore(inner, cache)
	if _, err := other.UpsertAnalysisNameserver("ns1.example", seen); err != nil {
		t.Fatalf("second attempt upsert: %v", err)
	}
	if inner.nameserverCalls != 2 {
		t.Fatalf("inner upserts = %d, want 2 (the pending ID must not be shared)", inner.nameserverCalls)
	}
}

func TestCachingWriteStoreReusesStagedIDWithinAttempt(t *testing.T) {
	// Within one transaction the same nameserver can appear on several
	// endpoints. Those repeats must still be deduplicated, otherwise the
	// staging change would undo the speedup the cache exists for.
	cache := newRebuildDimCache()
	inner := &countingDimStore{}
	attempt := newCachingWriteStore(inner, cache)

	seen := time.Now().UTC()
	first, err := attempt.UpsertAnalysisNameserver("ns1.example", seen)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second, err := attempt.UpsertAnalysisNameserver("ns1.example", seen)
	if err != nil {
		t.Fatalf("repeat upsert: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("repeat returned ID %d, want the staged %d", second.ID, first.ID)
	}
	if inner.nameserverCalls != 1 {
		t.Fatalf("inner upserts = %d, want 1", inner.nameserverCalls)
	}
}

func TestCachingWriteStorePublishesOnCommitStaged(t *testing.T) {
	// Once the transaction commits the IDs are real, so later runs should
	// hit the cache instead of the database.
	cache := newRebuildDimCache()
	inner := &countingDimStore{}
	committed := newCachingWriteStore(inner, cache)

	seen := time.Now().UTC()
	written, err := committed.UpsertAnalysisNameserver("ns1.example", seen)
	if err != nil {
		t.Fatalf("UpsertAnalysisNameserver: %v", err)
	}
	committed.CommitStaged()

	later := newCachingWriteStore(inner, cache)
	got, err := later.UpsertAnalysisNameserver("ns1.example", seen)
	if err != nil {
		t.Fatalf("later upsert: %v", err)
	}
	if got.ID != written.ID {
		t.Fatalf("cached ID = %d, want %d", got.ID, written.ID)
	}
	if inner.nameserverCalls != 1 {
		t.Fatalf("inner upserts = %d, want 1 (the second lookup should hit the cache)", inner.nameserverCalls)
	}
}
