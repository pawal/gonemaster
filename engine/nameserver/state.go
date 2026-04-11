package nameserver

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

type queryCache struct {
	mu       sync.Mutex
	data     map[string]*packet.Packet
	met      *cacheMetrics
	inflight map[string]*inflightQuery
}

type errorCache struct {
	mu   sync.Mutex
	data map[string]time.Time
	met  *cacheMetrics
}

type cacheMetrics struct {
	hits      uint64
	misses    uint64
	evictions uint64
}

// CacheMetrics exposes aggregate cache hit/miss/eviction counters.
type CacheMetrics struct {
	Hits      uint64
	Misses    uint64
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
		if c.met != nil {
			c.met.miss()
		}
		return false, 0
	}
	expiry, ok := c.data[key]
	if !ok {
		if c.met != nil {
			c.met.miss()
		}
		return false, 0
	}
	if now.After(expiry) {
		delete(c.data, key)
		if c.met != nil {
			c.met.evict(1)
			c.met.miss()
		}
		return false, 0
	}
	if c.met != nil {
		c.met.hit()
	}
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
	c.data[key] = time.Now().Add(ttl)
}

func (c *errorCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.met != nil {
		c.met.evict(len(c.data))
	}
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
		if c.met != nil {
			c.met.miss()
		}
		return nil, false
	}
	value, ok := c.data[key]
	if c.met != nil {
		if ok {
			c.met.hit()
		} else {
			c.met.miss()
		}
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
	c.data[key] = value
}

func (c *queryCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.met != nil {
		c.met.evict(len(c.data))
	}
	c.data = map[string]*packet.Packet{}
	c.inflight = map[string]*inflightQuery{}
	c.mu.Unlock()
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
	defer c.mu.Unlock()
	if c.inflight == nil {
		c.inflight = map[string]*inflightQuery{}
	}
	if inflight, ok := c.inflight[key]; ok {
		return inflight, true
	}
	inflight := &inflightQuery{done: make(chan struct{})}
	c.inflight[key] = inflight
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
	KeyTag     uint16
	Algorithm  uint8
	DigestType uint8
	Digest     string
}

type nsState struct {
	cache           *queryCache
	errorCache      *errorCache
	concurrencyCap  *nameserverConcurrencyCap
	fakeDelegations map[string]delegation
	fakeDS          map[string][]dns.RR
	blacklisted     map[bool]bool
	adaptiveTimeout adaptiveTimeoutTracker
	fastFail        fastFailTracker
	rateLimitPacing rateLimitPacingTracker
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
	sharedParent      *CacheStore
	queryMetrics      cacheMetrics
	errorMetrics      cacheMetrics
}

// NewCacheStore creates an empty nameserver cache store.
func NewCacheStore() *CacheStore {
	return &CacheStore{
		objectCache:       map[string]map[string]*Nameserver{},
		cacheByAddress:    map[string]*queryCache{},
		errorCacheByAddr:  map[string]*errorCache{},
		concurrencyByAddr: map[string]*nameserverConcurrencyCap{},
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
		return cache, false
	}
	if parentCache != nil {
		c.cacheByAddress[addr] = parentCache
		return parentCache, false
	}
	cache := &queryCache{data: map[string]*packet.Packet{}, met: &c.queryMetrics}
	c.cacheByAddress[addr] = cache
	return cache, true
}

func (c *CacheStore) errorCacheForAddress(addr string) *errorCache {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if cache := c.errorCacheByAddr[addr]; cache != nil {
		c.mu.Unlock()
		return cache
	}
	parent := c.sharedParent
	c.mu.Unlock()

	var parentCache *errorCache
	if parent != nil {
		parentCache = parent.errorCacheForAddress(addr)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if cache := c.errorCacheByAddr[addr]; cache != nil {
		return cache
	}
	if parentCache != nil {
		c.errorCacheByAddr[addr] = parentCache
		return parentCache
	}
	cache := &errorCache{data: map[string]time.Time{}, met: &c.errorMetrics}
	c.errorCacheByAddr[addr] = cache
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
		errorCacheByAddr:  make(map[string]*errorCache, len(c.errorCacheByAddr)),
		concurrencyByAddr: make(map[string]*nameserverConcurrencyCap, len(c.concurrencyByAddr)),
		sharedParent:      c,
	}
	for addr, cache := range c.cacheByAddress {
		snapshot.cacheByAddress[addr] = cache
	}
	for addr, cache := range c.errorCacheByAddr {
		snapshot.errorCacheByAddr[addr] = cache
	}
	for addr, cap := range c.concurrencyByAddr {
		snapshot.concurrencyByAddr[addr] = cap
	}
	return snapshot
}

// MergeWarmDataFrom merges warmed query/error caches from other into c.
//
// Nameserver object instances are intentionally not merged to avoid sharing
// mutable adaptation state across runs.
func (c *CacheStore) MergeWarmDataFrom(other *CacheStore) {
	if c == nil || other == nil || c == other {
		return
	}

	other.mu.Lock()
	queryByAddress := make(map[string]*queryCache, len(other.cacheByAddress))
	for addr, cache := range other.cacheByAddress {
		queryByAddress[addr] = cache
	}
	errorByAddress := make(map[string]*errorCache, len(other.errorCacheByAddr))
	for addr, cache := range other.errorCacheByAddr {
		errorByAddress[addr] = cache
	}
	concurrencyByAddress := make(map[string]*nameserverConcurrencyCap, len(other.concurrencyByAddr))
	for addr, cap := range other.concurrencyByAddr {
		concurrencyByAddress[addr] = cap
	}
	other.mu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

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
	}
	for addr, cache := range errorByAddress {
		if cache == nil {
			continue
		}
		if _, ok := c.errorCacheByAddr[addr]; ok {
			continue
		}
		cache.mu.Lock()
		cache.met = &c.errorMetrics
		cache.mu.Unlock()
		c.errorCacheByAddr[addr] = cache
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

// AddressCacheCount returns the number of per-address query cache buckets.
func (c *CacheStore) AddressCacheCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cacheByAddress)
}

// ErrorCacheCount returns the number of per-address error cache buckets.
func (c *CacheStore) ErrorCacheCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.errorCacheByAddr)
}

// NameserverObjectCount returns the number of cached nameserver objects.
func (c *CacheStore) NameserverObjectCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, byAddress := range c.objectCache {
		total += len(byAddress)
	}
	return total
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
	c.concurrencyByAddr = map[string]*nameserverConcurrencyCap{}
	c.objectCache = map[string]map[string]*Nameserver{}
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

var defaultCache = NewCacheStore()

// DefaultCache returns the fallback cache store.
func DefaultCache() *CacheStore {
	return defaultCache
}

// EmptyCache clears the default nameserver cache store.
func EmptyCache() {
	defaultCache.Empty()
}

func (ns *Nameserver) ensureState() {
	if ns == nil || ns.state != nil {
		return
	}
	cache := ns.cache
	if cache == nil {
		cache = defaultCache
		ns.cache = cache
	}
	ns.state = &nsState{
		cache:           cache.cacheForAddress(ns.Address.String()),
		errorCache:      cache.errorCacheForAddress(ns.Address.String()),
		concurrencyCap:  cache.concurrencyCapForAddress(ns.Address.String()),
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
