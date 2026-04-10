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
	blacklist       blacklistTracker
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
	defer c.mu.Unlock()
	if cache := c.cacheByAddress[addr]; cache != nil {
		return cache, false
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
	defer c.mu.Unlock()
	if c.errorCacheByAddr[addr] == nil {
		c.errorCacheByAddr[addr] = &errorCache{data: map[string]time.Time{}, met: &c.errorMetrics}
	}
	return c.errorCacheByAddr[addr]
}

func (c *CacheStore) concurrencyCapForAddress(addr string) *nameserverConcurrencyCap {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cap := c.concurrencyByAddr[addr]; cap != nil {
		return cap
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

// SnapshotForRun returns an independent run-local cache store.
// Stub: snapshot/merge plumbing removed for ablation measurement.
func (c *CacheStore) SnapshotForRun() *CacheStore {
	return NewCacheStore()
}

// MergeWarmDataFrom is a no-op stub; snapshot/merge plumbing removed for ablation measurement.
func (c *CacheStore) MergeWarmDataFrom(_ *CacheStore) {}

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
