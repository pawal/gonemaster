package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

const defaultNameserverHotCacheMaxEntries = 64

type nameserverHotCacheEntry struct {
	cache     *nameserver.CacheStore
	lastUsed  time.Time
	expiresAt time.Time
}

// nameserverHotCache keeps short-lived warmed nameserver query/error caches
// across jobs while issuing run-local cache snapshots per job.
type nameserverHotCache struct {
	mu         sync.Mutex
	entries    map[string]nameserverHotCacheEntry
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

func newNameserverHotCache(maxEntries int, ttl time.Duration) *nameserverHotCache {
	if maxEntries < 1 {
		maxEntries = defaultNameserverHotCacheMaxEntries
	}
	if ttl <= 0 {
		ttl = time.Duration(defaultCrossJobHotCacheTTLSeconds) * time.Second
	}
	return &nameserverHotCache{
		entries:    map[string]nameserverHotCacheEntry{},
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

// Lease returns a run-local cache view and a release function that merges
// warmed query/error data back into the hot cache.
func (c *nameserverHotCache) Lease(key string) (*nameserver.CacheStore, func()) {
	if c == nil {
		cache := nameserver.NewCacheStore()
		return cache, func() {}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "<default>"
	}
	now := c.nowUTC()

	c.mu.Lock()
	c.evictExpiredLocked(now)
	entry, ok := c.entries[key]
	if !ok {
		base := nameserver.NewCacheStore()
		base.SetWarmAddrTTL(c.ttl)
		entry = nameserverHotCacheEntry{
			cache: base,
		}
	}
	entry.lastUsed = now
	entry.expiresAt = now.Add(c.ttl)
	c.entries[key] = entry
	c.evictOverflowLocked()
	base := entry.cache
	c.mu.Unlock()

	runCache := base.SnapshotForRun()
	release := func() {
		base.MergeWarmDataFrom(runCache)
		runCache.DetachSharedMetricObservers()
		c.touch(key, base)
	}
	return runCache, release
}

func (c *nameserverHotCache) touch(key string, expected *nameserver.CacheStore) {
	if c == nil {
		return
	}
	now := c.nowUTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked(now)
	entry, ok := c.entries[key]
	if !ok || entry.cache != expected {
		return
	}
	entry.lastUsed = now
	entry.expiresAt = now.Add(c.ttl)
	c.entries[key] = entry
}

func (c *nameserverHotCache) nowUTC() time.Time {
	nowFn := time.Now
	if c != nil && c.now != nil {
		nowFn = c.now
	}
	return nowFn().UTC()
}

func (c *nameserverHotCache) evictExpiredLocked(now time.Time) {
	for key, entry := range c.entries {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

func (c *nameserverHotCache) evictOverflowLocked() {
	for len(c.entries) > c.maxEntries {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range c.entries {
			if oldestKey == "" || entry.lastUsed.Before(oldest) {
				oldestKey = key
				oldest = entry.lastUsed
			}
		}
		if oldestKey == "" {
			break
		}
		delete(c.entries, oldestKey)
	}
}

// SetTTL updates the TTL used for new and refreshed cache entries.
// Existing entries keep their current expiration but will use the new TTL
// on their next touch.
func (c *nameserverHotCache) SetTTL(ttl time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.ttl = ttl
	c.mu.Unlock()
}

func (c *nameserverHotCache) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = map[string]nameserverHotCacheEntry{}
	c.mu.Unlock()
}

func nameserverHotCacheKey(req engine.RunRequest) string {
	hash := sha256.New()
	writeHotCacheKeyField(hash, "profile", strings.TrimSpace(req.Profile))
	writeHotCacheKeyField(hash, "ipv4", boolPtrString(req.IPv4))
	writeHotCacheKeyField(hash, "ipv6", boolPtrString(req.IPv6))
	writeHotCacheKeyField(hash, "parallel", intPtrString(req.Parallel))
	writeHotCacheKeyField(hash, "unordered", boolPtrString(req.Unordered))
	writeHotCacheKeyField(hash, "error_cache_ttl", intPtrString(req.ErrorCacheTTL))
	writeHotCacheKeyField(hash, "positive_cache_ttl", intPtrString(req.PositiveCacheTTL))
	writeHotCacheKeyField(hash, "negative_cache_ttl", intPtrString(req.NegativeCacheTTL))
	writeHotCacheKeyField(hash, "timeout", intPtrString(req.Timeout))
	writeHotCacheKeyField(hash, "retry", intPtrString(req.Retry))
	writeHotCacheKeyField(hash, "retrans", intPtrString(req.Retrans))
	writeHotCacheKeyField(hash, "fallback", boolPtrString(req.Fallback))
	return hex.EncodeToString(hash.Sum(nil))
}

func writeHotCacheKeyField(hash interface{ Write([]byte) (int, error) }, name string, value string) {
	_, _ = hash.Write([]byte(name))
	_, _ = hash.Write([]byte("="))
	_, _ = hash.Write([]byte(value))
	_, _ = hash.Write([]byte("\n"))
}

func intPtrString(v *int) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *v)
}

func boolPtrString(v *bool) string {
	if v == nil {
		return "<nil>"
	}
	if *v {
		return "true"
	}
	return "false"
}
