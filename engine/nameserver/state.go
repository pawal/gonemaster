package nameserver

import (
	"context"
	"sync"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/packet"
)

type queryCache struct {
	mu   sync.Mutex
	data map[string]*packet.Packet
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
	fakeDelegations map[string]delegation
	fakeDS          map[string][]dns.RR
	blacklisted     map[bool]bool
	queryFunc       func(ctx context.Context, name string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error)
	axfrFunc        func(ctx context.Context, domain string, callback func(dns.RR) bool, class string) error
}

var (
	cacheMu        sync.Mutex
	objectCache    = map[string]map[string]*Nameserver{}
	cacheByAddress = map[string]*queryCache{}
)

func cacheForAddress(addr string) *queryCache {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cacheByAddress[addr] == nil {
		cacheByAddress[addr] = &queryCache{data: map[string]*packet.Packet{}}
	}
	return cacheByAddress[addr]
}

func cachedNameserver(nameKey string, addr string) *Nameserver {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	byName := objectCache[nameKey]
	if byName == nil {
		return nil
	}
	return byName[addr]
}

func storeNameserver(nameKey string, addr string, ns *Nameserver) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if objectCache[nameKey] == nil {
		objectCache[nameKey] = map[string]*Nameserver{}
	}
	objectCache[nameKey][addr] = ns
}

// EmptyCache clears nameserver object caches and query caches.
func EmptyCache() {
	cacheMu.Lock()
	for _, cache := range cacheByAddress {
		cache.clear()
	}
	cacheByAddress = map[string]*queryCache{}
	objectCache = map[string]map[string]*Nameserver{}
	cacheMu.Unlock()
}

// SetQueryHook overrides the network query path (useful for tests).
func (ns *Nameserver) SetQueryHook(hook func(ctx context.Context, name string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error)) {
	if ns == nil {
		return
	}
	if ns.state == nil {
		ns.state = &nsState{}
	}
	ns.state.queryFunc = hook
}

// SetAXFRHook overrides the AXFR network path (useful for tests).
func (ns *Nameserver) SetAXFRHook(hook func(ctx context.Context, domain string, callback func(dns.RR) bool, class string) error) {
	if ns == nil {
		return
	}
	if ns.state == nil {
		ns.state = &nsState{}
	}
	ns.state.axfrFunc = hook
}
