package nameserver

import (
	"context"
	"sync"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

type queryCache struct {
	mu   sync.Mutex
	data map[string]*packet.Packet
}

type errorCache struct {
	mu   sync.Mutex
	data map[string]time.Time
}

func (c *errorCache) shouldSkip(key string) (bool, time.Duration) {
	if c == nil {
		return false, 0
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		return false, 0
	}
	expiry, ok := c.data[key]
	if !ok {
		return false, 0
	}
	if now.After(expiry) {
		delete(c.data, key)
		return false, 0
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
		return nil, false
	}
	value, ok := c.data[key]
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
	c.data = map[string]*packet.Packet{}
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
	fakeDelegations map[string]delegation
	fakeDS          map[string][]dns.RR
	blacklisted     map[bool]bool
	queryFunc       func(ctx context.Context, name string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error)
	axfrFunc        func(ctx context.Context, domain string, callback func(dns.RR) bool, class string) error
}

type CacheStore struct {
	mu               sync.Mutex
	objectCache      map[string]map[string]*Nameserver
	cacheByAddress   map[string]*queryCache
	errorCacheByAddr map[string]*errorCache
}

func NewCacheStore() *CacheStore {
	return &CacheStore{
		objectCache:      map[string]map[string]*Nameserver{},
		cacheByAddress:   map[string]*queryCache{},
		errorCacheByAddr: map[string]*errorCache{},
	}
}

func (c *CacheStore) cacheForAddress(addr string) *queryCache {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cacheByAddress[addr] == nil {
		c.cacheByAddress[addr] = &queryCache{data: map[string]*packet.Packet{}}
	}
	return c.cacheByAddress[addr]
}

func (c *CacheStore) errorCacheForAddress(addr string) *errorCache {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.errorCacheByAddr[addr] == nil {
		c.errorCacheByAddr[addr] = &errorCache{data: map[string]time.Time{}}
	}
	return c.errorCacheByAddr[addr]
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
	c.mu.Unlock()
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
