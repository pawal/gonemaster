package extdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testClock is a manually advanced clock so TTLs can be crossed without
// sleeping.
type testClock struct {
	mu sync.Mutex
	at time.Time
}

func newTestClock() *testClock {
	return &testClock{at: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.at = c.at.Add(d)
	c.mu.Unlock()
}

// waitFor polls cond until it holds or the test gives up.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// newTestProvider wires a provider to srv with the transport-level guards
// relaxed, and runs its fetch worker for the duration of the test.
func newTestProvider(t *testing.T, srv *httptest.Server, clock *testClock, tune func(*Config)) *Provider {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.allowInsecure = true
	cfg.now = clock.Now
	cfg.Transport = srv.Client().Transport
	cfg.Sources = Sources{
		IANATLDs:      srv.URL + "/tlds",
		RDAPBootstrap: srv.URL + "/bootstrap",
	}
	if tune != nil {
		tune(&cfg)
	}
	p := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.worker(ctx)
	return p
}

// seedBootstrapAt puts a hand-built bootstrap in the cache so record fetches
// resolve to the test server instead of a registry.
func seedBootstrapAt(t *testing.T, p *Provider, base string) {
	t.Helper()
	p.store(datasetKey(datasetRDAPBootstrap), kindDataset, Item{
		Value:     &RDAPBootstrap{byTLD: map[string][]string{"se": {base}}},
		FetchedAt: p.now(),
	}, "", "")
}

// fetchedAt reads an entry's fetch time without scheduling anything.
func fetchedAt(p *Provider, key string) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	elem, ok := p.entries[key]
	if !ok {
		return time.Time{}
	}
	return elem.Value.(*cacheEntry).item.FetchedAt
}

// fixtureServer serves the dataset and RDAP fixtures, counting hits per path.
func fixtureServer(t *testing.T) (*httptest.Server, func(string) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	mux := http.NewServeMux()
	count := func(r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
	}
	mux.HandleFunc("/tlds", func(w http.ResponseWriter, r *http.Request) {
		count(r)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(readFixture(t, "tlds-alpha-by-domain.txt"))
	})
	mux.HandleFunc("/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		count(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readFixture(t, "rdap-dns.json"))
	})
	mux.HandleFunc("/rdap/domain/example.se", func(w http.ResponseWriter, r *http.Request) {
		count(r)
		w.Header().Set("Content-Type", "application/rdap+json")
		_, _ = w.Write(readFixture(t, "rdap-domain-cctld.json"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[path]
	}
}

func TestLookupDisabledProviderServesNothing(t *testing.T) {
	p := New(DefaultConfig())
	item, state := p.Lookup(datasetKey(datasetIANATLDs))
	if state != StateDisabled || item.Value != nil {
		t.Fatalf("state/value = %q/%v, want disabled and nothing", state, item.Value)
	}
	if p.Enabled() {
		t.Error("Enabled() = true on a disabled provider")
	}
	if status := p.Status(); status.Enabled {
		t.Error("Status().Enabled = true on a disabled provider")
	}
}

func TestLookupUnknownKey(t *testing.T) {
	p := enabledProvider()
	if _, state := p.Lookup("nonsense:key"); state != StateUnavailable {
		t.Fatalf("state = %q, want unavailable for an unknown key kind", state)
	}
	if _, state := p.Lookup(datasetKey("no_such_dataset")); state != StateUnavailable {
		t.Fatalf("state = %q, want unavailable for an unknown dataset", state)
	}
}

func TestLookupMissEnqueuesOnceUnderConcurrency(t *testing.T) {
	srv, hits := fixtureServer(t)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, nil)

	key := datasetKey(datasetIANATLDs)
	if _, state := p.Lookup(key); state != StatePending {
		t.Fatalf("first state = %q, want pending", state)
	}
	// Hammer the same key while the single fetch is in flight.
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				p.Lookup(key)
			}
		}()
	}
	wg.Wait()

	waitFor(t, "the dataset to load", func() bool {
		_, state := p.Lookup(key)
		return state == StateFresh
	})
	if got := hits("/tlds"); got != 1 {
		t.Fatalf("fetches = %d, want 1: concurrent lookups must share one fetch", got)
	}
	list, state := p.TLDs()
	if state != StateFresh || !list.Has("se") {
		t.Fatalf("TLDs() = %v/%q, want a loaded list", list, state)
	}
}

func TestLookupStaleServesAndRefreshes(t *testing.T) {
	srv, hits := fixtureServer(t)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, func(c *Config) { c.RefreshInterval = time.Hour })

	key := datasetKey(datasetIANATLDs)
	p.Lookup(key)
	waitFor(t, "the first load", func() bool {
		_, state := p.Lookup(key)
		return state == StateFresh
	})

	clock.Advance(2 * time.Hour)
	item, state := p.Lookup(key)
	if state != StateStale {
		t.Fatalf("state = %q, want stale past the refresh interval", state)
	}
	if item.Value == nil {
		t.Fatal("a stale lookup must still serve the cached value")
	}
	// Poll the entry rather than Lookup, which would re-enqueue while stale.
	waitFor(t, "the refresh", func() bool { return fetchedAt(p, key).Equal(clock.Now()) })
	if got := hits("/tlds"); got != 2 {
		t.Fatalf("fetches = %d, want 2: one initial load and one refresh", got)
	}
	if _, state := p.Lookup(key); state != StateFresh {
		t.Fatalf("state after refresh = %q, want fresh", state)
	}
}

func TestLookupNegativeTTL(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, func(c *Config) { c.NegativeTTL = 10 * time.Minute })

	key := datasetKey(datasetIANATLDs)
	p.Lookup(key)
	waitFor(t, "the failed fetch", func() bool { return attempts.Load() == 1 })
	waitFor(t, "the failure to be recorded", func() bool {
		_, state := p.Lookup(key)
		return state == StateUnavailable
	})
	// Inside the negative TTL nothing is retried.
	for range 5 {
		p.Lookup(key)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1 inside the negative TTL", got)
	}

	clock.Advance(11 * time.Minute)
	if _, state := p.Lookup(key); state != StatePending {
		t.Fatalf("state = %q, want pending once the negative TTL expires", state)
	}
	waitFor(t, "the retry", func() bool { return attempts.Load() == 2 })

	if status := p.Status(); status.LastError == "" || status.LastErrorAt == nil {
		t.Fatalf("status = %+v, want the last fetch error reported", status)
	}
}

func TestStaleRefreshBacksOffAfterAFailure(t *testing.T) {
	var attempts atomic.Int32
	mux := http.NewServeMux()
	// Only the TLD list is counted, so the bootstrap fetches the status
	// check schedules do not disturb the tally.
	mux.HandleFunc("/tlds", func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(readFixture(t, "tlds-alpha-by-domain.txt"))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	})
	mux.HandleFunc("/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, func(c *Config) {
		c.RefreshInterval = time.Hour
		c.NegativeTTL = 10 * time.Minute
	})

	key := datasetKey(datasetIANATLDs)
	p.Lookup(key)
	waitFor(t, "the first load", func() bool { return !fetchedAt(p, key).IsZero() })

	clock.Advance(2 * time.Hour)
	p.Lookup(key)
	waitFor(t, "the failing refresh", func() bool { return attempts.Load() == 2 })

	// Repeated views must not re-contact a source that just failed.
	for range 10 {
		item, state := p.Lookup(key)
		if state != StateStale || item.Value == nil {
			t.Fatalf("state/value = %q/%v, want the cached value served as stale", state, item.Value)
		}
	}
	waitFor(t, "the queue to drain", func() bool { return p.Status().QueueDepth == 0 })
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2 inside the negative TTL", got)
	}

	clock.Advance(11 * time.Minute)
	p.Lookup(key)
	waitFor(t, "the retry once the negative TTL expires", func() bool { return attempts.Load() == 3 })
}

func TestTokenBucketDropsFetchesPastTheLimit(t *testing.T) {
	srv, hits := fixtureServer(t)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, func(c *Config) { c.MaxRequestsPerMinute = 1 })

	p.Lookup(datasetKey(datasetIANATLDs))
	waitFor(t, "the first dataset", func() bool { return hits("/tlds") == 1 })

	// The bucket is empty, so the second dataset is dropped rather than fetched.
	waitFor(t, "the queue to drain", func() bool {
		p.Lookup(datasetKey(datasetRDAPBootstrap))
		return p.Status().QueueDepth == 0
	})
	if got := hits("/bootstrap"); got != 0 {
		t.Fatalf("bootstrap fetches = %d, want 0 while the bucket is empty", got)
	}
	if _, state := p.Lookup(datasetKey(datasetRDAPBootstrap)); state != StatePending {
		t.Fatalf("state = %q, want pending: a dropped fetch leaves no entry", state)
	}

	clock.Advance(time.Minute)
	p.Lookup(datasetKey(datasetRDAPBootstrap))
	waitFor(t, "the refilled bucket to allow the fetch", func() bool { return hits("/bootstrap") == 1 })
}

func TestRecordLookupStoresSummary(t *testing.T) {
	srv, hits := fixtureServer(t)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, nil)
	seedBootstrapAt(t, p, srv.URL+"/rdap/")

	if _, _, state := p.RDAPDomain("example.se"); state != StatePending {
		t.Fatalf("state = %q, want pending on a cache miss", state)
	}
	waitFor(t, "the RDAP record", func() bool {
		_, _, state := p.RDAPDomain("example.se")
		return state == StateFresh
	})
	summary, fetchedAt, _ := p.RDAPDomain("EXAMPLE.SE.")
	if fetchedAt.IsZero() {
		t.Error("fetch time = zero, want the time the record was stored")
	}
	if summary == nil || summary.Registrar == "" {
		t.Fatalf("summary = %+v, want a parsed record for the normalized name", summary)
	}
	if got := hits("/rdap/domain/example.se"); got != 1 {
		t.Fatalf("record fetches = %d, want 1", got)
	}
	if status := p.Status(); status.CachedRecords != 1 {
		t.Fatalf("cached_records = %d, want 1", status.CachedRecords)
	}
}

func TestRecordLookupWithoutBootstrapEntryNeverFetches(t *testing.T) {
	srv, hits := fixtureServer(t)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, nil)
	seedBootstrapAt(t, p, srv.URL+"/rdap/")

	if _, _, state := p.RDAPDomain("example.com"); state != StateUnavailable {
		t.Fatalf("state = %q, want unavailable when no registry serves the TLD", state)
	}
	if got := hits("/rdap/domain/example.com"); got != 0 {
		t.Fatalf("fetches = %d, want 0", got)
	}
}

func TestEvictionCapsRecordsAndKeepsDatasets(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.MaxCachedRecords = 2
	p := New(cfg)
	p.store(datasetKey(datasetIANATLDs), kindDataset, Item{Value: &TLDList{}, FetchedAt: p.now()}, "", "")
	for _, name := range []string{"a.se", "b.se", "c.se"} {
		p.store(RDAPDomainKey(name), kindRecord, Item{Value: &RDAPDomainSummary{LDHName: name}, FetchedAt: p.now()}, "", "")
	}
	p.mu.Lock()
	records, entries := p.records, len(p.entries)
	p.mu.Unlock()
	if records != 2 {
		t.Fatalf("records = %d, want the cap of 2", records)
	}
	// The dataset survives eviction; the oldest record does not.
	if entries != 3 {
		t.Fatalf("entries = %d, want 2 records plus the pinned dataset", entries)
	}
	if _, ok := p.cachedValue(RDAPDomainKey("a.se")); ok {
		t.Error("the least recently used record was not evicted")
	}
	if _, ok := p.cachedValue(datasetKey(datasetIANATLDs)); !ok {
		t.Error("a dataset must never be evicted")
	}
}

func TestStartPrimesDatasetsAndStopsWithContext(t *testing.T) {
	srv, hits := fixtureServer(t)
	clock := newTestClock()
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.allowInsecure = true
	cfg.now = clock.Now
	cfg.Transport = srv.Client().Transport
	cfg.Sources = Sources{IANATLDs: srv.URL + "/tlds", RDAPBootstrap: srv.URL + "/bootstrap"}
	p := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	waitFor(t, "both datasets to load", func() bool {
		return !fetchedAt(p, datasetKey(datasetIANATLDs)).IsZero() &&
			!fetchedAt(p, datasetKey(datasetRDAPBootstrap)).IsZero()
	})
	if hits("/tlds") != 1 || hits("/bootstrap") != 1 {
		t.Fatalf("fetches = %d/%d, want one each at startup", hits("/tlds"), hits("/bootstrap"))
	}

	status := p.Status()
	if len(status.Datasets) != 2 {
		t.Fatalf("status datasets = %+v, want both", status.Datasets)
	}
	for _, ds := range status.Datasets {
		if ds.State != string(StateFresh) || ds.FetchedAt == nil || ds.Entries == 0 {
			t.Errorf("dataset %s = %+v, want a fresh loaded entry", ds.Name, ds)
		}
	}
	if status.Datasets[0].Version != "2026090700" {
		t.Errorf("tld list version = %q, want the file serial", status.Datasets[0].Version)
	}

	cancel()
	// After shutdown the worker is gone, so nothing new is fetched.
	clock.Advance(48 * time.Hour)
	p.Lookup(datasetKey(datasetIANATLDs))
	time.Sleep(50 * time.Millisecond)
	if got := hits("/tlds"); got != 1 {
		t.Fatalf("fetches after cancel = %d, want 1", got)
	}
}

func TestStartOnDisabledProviderDoesNothing(t *testing.T) {
	srv, hits := fixtureServer(t)
	cfg := DefaultConfig()
	cfg.Transport = srv.Client().Transport
	cfg.Sources = Sources{IANATLDs: srv.URL + "/tlds", RDAPBootstrap: srv.URL + "/bootstrap"}
	p := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	if got := hits("/tlds"); got != 0 {
		t.Fatalf("fetches = %d, want 0 while disabled", got)
	}
}
