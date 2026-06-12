package nameserver

import (
	"context"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

// QueryCacheMaxEntries caps the number of cached DNS responses per nameserver
// address. When the limit is reached, the oldest entries are evicted. 0 means
// unbounded (not recommended for long-lived server processes).
var QueryCacheMaxEntries = 256

type queryCache struct {
	mu           sync.Mutex
	data         map[string]*packet.Packet
	order        []string // FIFO insertion order for bounded eviction
	met          *cacheMetrics
	inflight     map[string]*inflightQuery
	observers    map[*cacheMetrics]struct{}
	onWaiterJoin func() // optional; called without lock when a waiter joins an existing inflight entry
}

type errorCache struct {
	mu        sync.Mutex
	data      map[string]time.Time
	met       *cacheMetrics
	observers map[*cacheMetrics]struct{}
}

type cacheMetrics struct {
	hits      uint64
	misses    uint64
	evictions uint64
}

// CacheMetrics exposes aggregate cache hit/miss/eviction counters.
type CacheMetrics struct {
	// Hits is the number of cache hits observed.
	Hits uint64
	// Misses is the number of cache misses observed.
	Misses uint64
	// Evictions is the number of cached entries evicted.
	Evictions uint64
}

func (m *cacheMetrics) hit() {
	if m == nil {
		return
	}
	atomic.AddUint64(&m.hits, 1)
}

func (m *cacheMetrics) miss() {
	if m == nil {
		return
	}
	atomic.AddUint64(&m.misses, 1)
}

func (m *cacheMetrics) evict(n int) {
	if m == nil || n <= 0 {
		return
	}
	atomic.AddUint64(&m.evictions, uint64(n))
}

func (m *cacheMetrics) snapshot() CacheMetrics {
	if m == nil {
		return CacheMetrics{}
	}
	return CacheMetrics{
		Hits:      atomic.LoadUint64(&m.hits),
		Misses:    atomic.LoadUint64(&m.misses),
		Evictions: atomic.LoadUint64(&m.evictions),
	}
}

func (c *errorCache) shouldSkip(key string) (bool, time.Duration) {
	if c == nil {
		return false, 0
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.observeMissLocked()
		return false, 0
	}
	expiry, ok := c.data[key]
	if !ok {
		c.observeMissLocked()
		return false, 0
	}
	if now.After(expiry) {
		delete(c.data, key)
		c.observeEvictLocked(1)
		c.observeMissLocked()
		return false, 0
	}
	c.observeHitLocked()
	return true, expiry.Sub(now)
}

func (c *errorCache) set(key string, ttl time.Duration) {
	if c == nil || ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string]time.Time{}
	}
	now := time.Now()
	// Sweep expired entries opportunistically: the wide cache key admits
	// many distinct entries per address, and stale ones are otherwise only
	// reaped lazily on shouldSkip lookups for that exact key.
	evicted := 0
	for k, expiry := range c.data {
		if now.After(expiry) {
			delete(c.data, k)
			evicted++
		}
	}
	if evicted > 0 {
		c.observeEvictLocked(evicted)
	}
	c.data[key] = now.Add(ttl)
}

func (c *errorCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.observeEvictLocked(len(c.data))
	c.data = map[string]time.Time{}
	c.mu.Unlock()
}

func (c *queryCache) get(key string) (*packet.Packet, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.observeMissLocked()
		return nil, false
	}
	value, ok := c.data[key]
	if ok {
		c.observeHitLocked()
	} else {
		c.observeMissLocked()
	}
	return value, ok
}

func (c *queryCache) set(key string, value *packet.Packet) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string]*packet.Packet{}
	}
	if _, exists := c.data[key]; !exists {
		c.order = append(c.order, key)
	}
	c.data[key] = value

	// Evict oldest entries if over the cap.
	if max := QueryCacheMaxEntries; max > 0 && len(c.data) > max {
		drop := len(c.data) - max
		for i := 0; i < drop && i < len(c.order); i++ {
			oldKey := c.order[i]
			delete(c.data, oldKey)
		}
		c.order = c.order[drop:]
		c.observeEvictLocked(drop)
	}
}

func (c *queryCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.observeEvictLocked(len(c.data))
	c.data = map[string]*packet.Packet{}
	c.order = nil
	c.inflight = map[string]*inflightQuery{}
	c.mu.Unlock()
}

// clearData evicts cached packets but keeps the inflight map so concurrent
// query coalescing continues to work.
func (c *queryCache) clearData() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.observeEvictLocked(len(c.data))
	c.data = map[string]*packet.Packet{}
	c.order = nil
	c.mu.Unlock()
}

func (c *queryCache) addObserver(observer *cacheMetrics) {
	if c == nil || observer == nil {
		return
	}
	c.mu.Lock()
	if c.observers == nil {
		c.observers = map[*cacheMetrics]struct{}{}
	}
	c.observers[observer] = struct{}{}
	c.mu.Unlock()
}

func (c *queryCache) removeObserver(observer *cacheMetrics) {
	if c == nil || observer == nil {
		return
	}
	c.mu.Lock()
	delete(c.observers, observer)
	c.mu.Unlock()
}

func (c *queryCache) observeHitLocked() {
	if c.met != nil {
		c.met.hit()
	}
	for observer := range c.observers {
		observer.hit()
	}
}

func (c *queryCache) observeMissLocked() {
	if c.met != nil {
		c.met.miss()
	}
	for observer := range c.observers {
		observer.miss()
	}
}

func (c *queryCache) observeEvictLocked(n int) {
	if c.met != nil {
		c.met.evict(n)
	}
	for observer := range c.observers {
		observer.evict(n)
	}
}

func (c *errorCache) addObserver(observer *cacheMetrics) {
	if c == nil || observer == nil {
		return
	}
	c.mu.Lock()
	if c.observers == nil {
		c.observers = map[*cacheMetrics]struct{}{}
	}
	c.observers[observer] = struct{}{}
	c.mu.Unlock()
}

func (c *errorCache) removeObserver(observer *cacheMetrics) {
	if c == nil || observer == nil {
		return
	}
	c.mu.Lock()
	delete(c.observers, observer)
	c.mu.Unlock()
}

func (c *errorCache) observeHitLocked() {
	if c.met != nil {
		c.met.hit()
	}
	for observer := range c.observers {
		observer.hit()
	}
}

func (c *errorCache) observeMissLocked() {
	if c.met != nil {
		c.met.miss()
	}
	for observer := range c.observers {
		observer.miss()
	}
}

func (c *errorCache) observeEvictLocked(n int) {
	if c.met != nil {
		c.met.evict(n)
	}
	for observer := range c.observers {
		observer.evict(n)
	}
}

type inflightQuery struct {
	done chan struct{}
	resp *packet.Packet
	err  error
}

func (c *queryCache) waitOrRegister(key string) (*inflightQuery, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	if c.inflight == nil {
		c.inflight = map[string]*inflightQuery{}
	}
	if inflight, ok := c.inflight[key]; ok {
		hook := c.onWaiterJoin
		c.mu.Unlock()
		if hook != nil {
			hook()
		}
		return inflight, true
	}
	inflight := &inflightQuery{done: make(chan struct{})}
	c.inflight[key] = inflight
	c.mu.Unlock()
	return inflight, false
}

func (c *queryCache) finish(key string, resp *packet.Packet, err error) {
	if c == nil {
		return
	}
	c.mu.Lock()
	inflight := c.inflight[key]
	if inflight != nil {
		inflight.resp = resp
		inflight.err = err
		close(inflight.done)
		delete(c.inflight, key)
	}
	c.mu.Unlock()
}

type delegation struct {
	authority  []dns.RR
	additional []dns.RR
}

// DSData represents parameters for a fake DS record.
type DSData struct {
	// KeyTag is the DS key tag value.
	KeyTag uint16
	// Algorithm is the DNSSEC algorithm number.
	Algorithm uint8
	// DigestType is the DS digest type number.
	DigestType uint8
	// Digest is the uppercase hexadecimal digest text.
	Digest string
}

type nsState struct {
	cache           *queryCache
	errorCache      *errorCache
	concurrencyCap  *nameserverConcurrencyCap
	fakeDelegations map[string]delegation
	fakeDS          map[string][]dns.RR
	blacklisted     map[bool]bool
	fastFail        fastFailTracker
	latency         latencyTracker
	queryFunc       func(ctx context.Context, name string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error)
	axfrFunc        func(ctx context.Context, domain string, callback func(dns.RR) bool, class string) error
}

// CacheStore keeps nameserver objects and per-address query/error caches.
type CacheStore struct {
	mu                sync.Mutex
	objectCache       map[string]map[string]*Nameserver
	cacheByAddress    map[string]*queryCache
	errorCacheByAddr  map[string]*errorCache
	concurrencyByAddr map[string]*nameserverConcurrencyCap
	observedQueries   map[string]*queryCache
	observedErrors    map[string]*errorCache
	addrLastAccess    map[string]time.Time // when each address was last used
	warmAddrTTL       time.Duration        // evict addresses idle longer than this
	sharedParent      *CacheStore
	queryMetrics      cacheMetrics
	errorMetrics      cacheMetrics
	queryTimes        map[string][]time.Duration
}

// NewCacheStore creates an empty nameserver cache store.
func NewCacheStore() *CacheStore {
	return &CacheStore{
		objectCache:       map[string]map[string]*Nameserver{},
		cacheByAddress:    map[string]*queryCache{},
		errorCacheByAddr:  map[string]*errorCache{},
		concurrencyByAddr: map[string]*nameserverConcurrencyCap{},
		observedQueries:   map[string]*queryCache{},
		observedErrors:    map[string]*errorCache{},
		addrLastAccess:    map[string]time.Time{},
		queryTimes:        map[string][]time.Duration{},
	}
}

// SetWarmAddrTTL sets the maximum idle time for warmed addresses. Addresses
// not accessed within this duration are evicted during MergeWarmDataFrom.
func (c *CacheStore) SetWarmAddrTTL(ttl time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.warmAddrTTL = ttl
	c.mu.Unlock()
}

// RecordQueryTime appends a query duration for the given nameserver key.
func (c *CacheStore) RecordQueryTime(key string, d time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.queryTimes[key] = append(c.queryTimes[key], d)
	c.mu.Unlock()
}

// QueryTimings returns a copy of the accumulated per-nameserver query times.
func (c *CacheStore) QueryTimings() map[string][]time.Duration {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string][]time.Duration, len(c.queryTimes))
	for k, v := range c.queryTimes {
		cp := make([]time.Duration, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

// touchAddrLocked updates the last-access time for addr. Must be called with
// c.mu held.
func (c *CacheStore) touchAddrLocked(addr string) {
	if c.addrLastAccess != nil {
		c.addrLastAccess[addr] = time.Now()
	}
}

func (c *CacheStore) cacheForAddress(addr string) *queryCache {
	cache, _ := c.cacheForAddressWithStatus(addr)
	return cache
}

func (c *CacheStore) cacheForAddressWithStatus(addr string) (*queryCache, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	if cache := c.cacheByAddress[addr]; cache != nil {
		c.touchAddrLocked(addr)
		c.mu.Unlock()
		return cache, false
	}
	parent := c.sharedParent
	c.mu.Unlock()

	var parentCache *queryCache
	if parent != nil {
		parentCache = parent.cacheForAddress(addr)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if cache := c.cacheByAddress[addr]; cache != nil {
		c.touchAddrLocked(addr)
		return cache, false
	}
	if parentCache != nil {
		parentCache.addObserver(&c.queryMetrics)
		c.cacheByAddress[addr] = parentCache
		c.observedQueries[addr] = parentCache
		c.touchAddrLocked(addr)
		return parentCache, false
	}
	cache := &queryCache{data: map[string]*packet.Packet{}, met: &c.queryMetrics}
	c.cacheByAddress[addr] = cache
	c.touchAddrLocked(addr)
	return cache, true
}

// errorCacheForAddress returns a per-store error cache. The error cache
// is intentionally not shared via the parent chain: a transient failure
// in one run must not blackout the address for concurrent or future runs.
func (c *CacheStore) errorCacheForAddress(addr string) *errorCache {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cache := c.errorCacheByAddr[addr]; cache != nil {
		c.touchAddrLocked(addr)
		return cache
	}
	cache := &errorCache{data: map[string]time.Time{}, met: &c.errorMetrics}
	c.errorCacheByAddr[addr] = cache
	c.touchAddrLocked(addr)
	return cache
}

func (c *CacheStore) concurrencyCapForAddress(addr string) *nameserverConcurrencyCap {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if cap := c.concurrencyByAddr[addr]; cap != nil {
		c.mu.Unlock()
		return cap
	}
	parent := c.sharedParent
	c.mu.Unlock()

	var parentCap *nameserverConcurrencyCap
	if parent != nil {
		parentCap = parent.concurrencyCapForAddress(addr)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if cap := c.concurrencyByAddr[addr]; cap != nil {
		return cap
	}
	if parentCap != nil {
		c.concurrencyByAddr[addr] = parentCap
		return parentCap
	}
	cap := &nameserverConcurrencyCap{}
	c.concurrencyByAddr[addr] = cap
	return cap
}

func (c *CacheStore) cachedNameserver(nameKey string, addr string) *Nameserver {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	byName := c.objectCache[nameKey]
	if byName == nil {
		return nil
	}
	return byName[addr]
}

func (c *CacheStore) storeNameserver(nameKey string, addr string, ns *Nameserver) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.objectCache[nameKey] == nil {
		c.objectCache[nameKey] = map[string]*Nameserver{}
	}
	c.objectCache[nameKey][addr] = ns
}

// SnapshotForRun returns a run-local cache store that reuses warmed query/error
// caches from c while starting with an empty nameserver object cache.
func (c *CacheStore) SnapshotForRun() *CacheStore {
	if c == nil {
		return NewCacheStore()
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := &CacheStore{
		objectCache:       map[string]map[string]*Nameserver{},
		cacheByAddress:    make(map[string]*queryCache, len(c.cacheByAddress)),
		errorCacheByAddr:  map[string]*errorCache{},
		concurrencyByAddr: make(map[string]*nameserverConcurrencyCap, len(c.concurrencyByAddr)),
		observedQueries:   map[string]*queryCache{},
		observedErrors:    map[string]*errorCache{},
		addrLastAccess:    map[string]time.Time{},
		sharedParent:      c,
		queryTimes:        map[string][]time.Duration{},
	}
	return snapshot
}

// DetachSharedMetricObservers stops forwarding shared-parent cache metrics into
// this store. It should be called when a SnapshotForRun-derived store is no
// longer in use.
func (c *CacheStore) DetachSharedMetricObservers() {
	if c == nil {
		return
	}
	c.mu.Lock()
	queryObservers := make([]*queryCache, 0, len(c.observedQueries))
	for _, cache := range c.observedQueries {
		queryObservers = append(queryObservers, cache)
	}
	errorObservers := make([]*errorCache, 0, len(c.observedErrors))
	for _, cache := range c.observedErrors {
		errorObservers = append(errorObservers, cache)
	}
	c.observedQueries = map[string]*queryCache{}
	c.observedErrors = map[string]*errorCache{}
	c.mu.Unlock()

	for _, cache := range queryObservers {
		cache.removeObserver(&c.queryMetrics)
	}
	for _, cache := range errorObservers {
		cache.removeObserver(&c.errorMetrics)
	}
}

// MergeWarmDataFrom merges warmed query caches and concurrency caps from
// other into c. Error caches are intentionally NOT merged: they reflect
// transient run-local failures and must not poison subsequent runs.
//
// Nameserver object instances are also not merged to avoid sharing
// mutable adaptation state across runs. Before merging, addresses in c
// that have not been accessed within warmAddrTTL are evicted.
func (c *CacheStore) MergeWarmDataFrom(other *CacheStore) {
	if c == nil || other == nil || c == other {
		return
	}

	other.mu.Lock()
	queryByAddress := make(map[string]*queryCache, len(other.cacheByAddress))
	maps.Copy(queryByAddress, other.cacheByAddress)
	concurrencyByAddress := make(map[string]*nameserverConcurrencyCap, len(other.concurrencyByAddr))
	maps.Copy(concurrencyByAddress, other.concurrencyByAddr)
	otherAccess := make(map[string]time.Time, len(other.addrLastAccess))
	maps.Copy(otherAccess, other.addrLastAccess)
	other.mu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	for addr, t := range otherAccess {
		if existing, ok := c.addrLastAccess[addr]; !ok || t.After(existing) {
			c.addrLastAccess[addr] = t
		}
	}

	c.evictStaleAddrsLocked()

	for addr, cache := range queryByAddress {
		if cache == nil {
			continue
		}
		if _, ok := c.cacheByAddress[addr]; ok {
			continue
		}
		cache.mu.Lock()
		cache.met = &c.queryMetrics
		cache.mu.Unlock()
		c.cacheByAddress[addr] = cache
		if _, ok := c.addrLastAccess[addr]; !ok {
			c.addrLastAccess[addr] = time.Now()
		}
	}
	for addr, cap := range concurrencyByAddress {
		if cap == nil {
			continue
		}
		if _, ok := c.concurrencyByAddr[addr]; ok {
			continue
		}
		c.concurrencyByAddr[addr] = cap
	}
}

// evictStaleAddrsLocked clears cached packet data from addresses that haven't
// been accessed within warmAddrTTL. The cache structures themselves are kept
// alive so inflight coalescing and concurrency caps continue to work across
// concurrent snapshots. Must be called with c.mu held.
func (c *CacheStore) evictStaleAddrsLocked() {
	if c.warmAddrTTL <= 0 || len(c.addrLastAccess) == 0 {
		return
	}
	cutoff := time.Now().Add(-c.warmAddrTTL)
	for addr, lastAccess := range c.addrLastAccess {
		if lastAccess.Before(cutoff) {
			if cache := c.cacheByAddress[addr]; cache != nil {
				cache.clearData()
			}
			if cache := c.errorCacheByAddr[addr]; cache != nil {
				cache.clear()
			}
			delete(c.addrLastAccess, addr)
		}
	}
}

// Empty clears nameserver object caches and query caches.
func (c *CacheStore) Empty() {
	if c == nil {
		return
	}
	c.mu.Lock()
	for _, cache := range c.cacheByAddress {
		cache.clear()
	}
	for _, cache := range c.errorCacheByAddr {
		cache.clear()
	}
	c.cacheByAddress = map[string]*queryCache{}
	c.errorCacheByAddr = map[string]*errorCache{}
	c.objectCache = map[string]map[string]*Nameserver{}
	c.addrLastAccess = map[string]time.Time{}
	c.mu.Unlock()
}

// QueryMetrics returns aggregated metrics for query caches.
func (c *CacheStore) QueryMetrics() CacheMetrics {
	if c == nil {
		return CacheMetrics{}
	}
	return c.queryMetrics.snapshot()
}

// ErrorMetrics returns aggregated metrics for error caches.
func (c *CacheStore) ErrorMetrics() CacheMetrics {
	if c == nil {
		return CacheMetrics{}
	}
	return c.errorMetrics.snapshot()
}

// AddressCacheCount returns the number of per-address query caches accessible
// from c, including those inherited from the parent chain.
func (c *CacheStore) AddressCacheCount() int {
	if c == nil {
		return 0
	}
	seen := map[string]bool{}
	for cur := c; cur != nil; {
		cur.mu.Lock()
		for addr := range cur.cacheByAddress {
			seen[addr] = true
		}
		parent := cur.sharedParent
		cur.mu.Unlock()
		cur = parent
	}
	return len(seen)
}

// ErrorCacheCount returns the number of per-address error caches accessible
// from c, including those inherited from the parent chain.
func (c *CacheStore) ErrorCacheCount() int {
	if c == nil {
		return 0
	}
	seen := map[string]bool{}
	for cur := c; cur != nil; {
		cur.mu.Lock()
		for addr := range cur.errorCacheByAddr {
			seen[addr] = true
		}
		parent := cur.sharedParent
		cur.mu.Unlock()
		cur = parent
	}
	return len(seen)
}

// NameserverObjectCount returns the number of cached nameserver objects held by c
// (does not include parent chain - object caches are per-run only).
func (c *CacheStore) NameserverObjectCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, byAddr := range c.objectCache {
		total += len(byAddr)
	}
	return total
}

// SetWaiterJoinHookForAddr installs a hook that is called (without lock) each
// time waitOrRegister finds an existing inflight entry for the query cache
// associated with addr. Intended for use in tests to detect inflight coalescing.
func (c *CacheStore) SetWaiterJoinHookForAddr(addr string, f func()) {
	qc := c.cacheForAddress(addr)
	if qc == nil {
		return
	}
	qc.mu.Lock()
	qc.onWaiterJoin = f
	qc.mu.Unlock()
}

func (ns *Nameserver) ensureState() {
	if ns == nil || ns.state != nil {
		return
	}
	cache := ns.cache
	if cache == nil {
		panic("nameserver: ensureState on a Nameserver with nil cache; construct via NewWithContext or NewWithCache")
	}
	addrKey := ns.Address.String()
	ns.state = &nsState{
		cache:           cache.cacheForAddress(addrKey),
		errorCache:      cache.errorCacheForAddress(addrKey),
		concurrencyCap:  cache.concurrencyCapForAddress(addrKey),
		fakeDelegations: map[string]delegation{},
		fakeDS:          map[string][]dns.RR{},
		blacklisted:     map[bool]bool{},
	}
}

// SetQueryHook overrides the network query path (useful for tests).
func (ns *Nameserver) SetQueryHook(hook func(ctx context.Context, name string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error)) {
	if ns == nil {
		return
	}
	ns.ensureState()
	ns.state.queryFunc = hook
}

// SetAXFRHook overrides the AXFR network path (useful for tests).
func (ns *Nameserver) SetAXFRHook(hook func(ctx context.Context, domain string, callback func(dns.RR) bool, class string) error) {
	if ns == nil {
		return
	}
	ns.ensureState()
	ns.state.axfrFunc = hook
}
