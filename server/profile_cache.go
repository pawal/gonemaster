package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

const defaultProfileOverrideCacheMaxEntries = 256

// defaultProfileOverrideCacheTTL=0 disables time-based expiration and keeps
// profile variants warm until LRU eviction by maxEntries.
const defaultProfileOverrideCacheTTL = 0

var errProfileOverrideCacheClosed = errors.New("profile override cache closed")

type profileCachePutState int

const (
	profileCachePutInserted profileCachePutState = iota
	profileCachePutExists
	profileCachePutNotCached
)

type profileOverrideCacheEntry struct {
	path      string
	lastUsed  time.Time
	expiresAt time.Time
}

type profileCacheInflight struct {
	done chan struct{}
	path string
	err  error
}

type profileOverrideCache struct {
	mu         sync.Mutex
	entries    map[string]profileOverrideCacheEntry
	inflight   map[string]*profileCacheInflight
	maxEntries int
	ttl        time.Duration
}

func newProfileOverrideCache(maxEntries int, ttl time.Duration) *profileOverrideCache {
	if maxEntries < 1 {
		maxEntries = defaultProfileOverrideCacheMaxEntries
	}
	if ttl < 0 {
		ttl = defaultProfileOverrideCacheTTL
	}
	return &profileOverrideCache{
		entries:    map[string]profileOverrideCacheEntry{},
		inflight:   map[string]*profileCacheInflight{},
		maxEntries: maxEntries,
		ttl:        ttl,
	}
}

// Get returns a cached merged-profile path for key when present and not expired.
// Expired entries are removed and their temporary files are deleted.
func (c *profileOverrideCache) Get(key string) (string, bool) {
	if c == nil || key == "" {
		return "", false
	}
	now := time.Now().UTC()
	var stalePath string

	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		return "", false
	}
	if c.ttl > 0 && !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		delete(c.entries, key)
		stalePath = entry.path
		c.mu.Unlock()
		if stalePath != "" {
			_ = os.Remove(stalePath)
		}
		return "", false
	}
	entry.lastUsed = now
	entry.expiresAt = expiryTimeForTTL(now, c.ttl)
	c.entries[key] = entry
	c.mu.Unlock()

	return entry.path, true
}

// Put stores path for key and returns whether it was inserted, already existed,
// or skipped due to cache limits.
func (c *profileOverrideCache) Put(key string, path string) (profileCachePutState, string) {
	if c == nil || key == "" || path == "" {
		return profileCachePutNotCached, ""
	}

	now := time.Now().UTC()
	var removePaths []string

	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		if c.ttl <= 0 || entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
			entry.lastUsed = now
			entry.expiresAt = expiryTimeForTTL(now, c.ttl)
			c.entries[key] = entry
			c.mu.Unlock()
			return profileCachePutExists, entry.path
		}
		delete(c.entries, key)
		removePaths = append(removePaths, entry.path)
	}

	for len(c.entries) >= c.maxEntries {
		victimKey, victim, ok := c.oldestEntryLocked()
		if !ok {
			break
		}
		delete(c.entries, victimKey)
		removePaths = append(removePaths, victim.path)
	}
	if len(c.entries) >= c.maxEntries {
		c.mu.Unlock()
		for _, oldPath := range removePaths {
			if oldPath != "" && oldPath != path {
				_ = os.Remove(oldPath)
			}
		}
		return profileCachePutNotCached, ""
	}

	c.entries[key] = profileOverrideCacheEntry{
		path:      path,
		lastUsed:  now,
		expiresAt: expiryTimeForTTL(now, c.ttl),
	}
	c.mu.Unlock()

	for _, oldPath := range removePaths {
		if oldPath != "" && oldPath != path {
			_ = os.Remove(oldPath)
		}
	}
	return profileCachePutInserted, path
}

func (c *profileOverrideCache) oldestEntryLocked() (string, profileOverrideCacheEntry, bool) {
	var victimKey string
	var victim profileOverrideCacheEntry
	found := false

	for key, entry := range c.entries {
		if !found || entry.lastUsed.Before(victim.lastUsed) {
			victimKey = key
			victim = entry
			found = true
		}
	}
	return victimKey, victim, found
}

func expiryTimeForTTL(now time.Time, ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return now.Add(ttl)
}

func (c *profileOverrideCache) startInflight(key string) (*profileCacheInflight, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if pending, ok := c.inflight[key]; ok {
		return pending, false
	}
	pending := &profileCacheInflight{done: make(chan struct{})}
	c.inflight[key] = pending
	return pending, true
}

func (c *profileOverrideCache) finishInflight(key string, path string, err error) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	pending, ok := c.inflight[key]
	if ok {
		delete(c.inflight, key)
		pending.path = path
		pending.err = err
		close(pending.done)
	}
	c.mu.Unlock()
}

// Close removes all cached entries and deletes their temporary files.
func (c *profileOverrideCache) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	paths := make([]string, 0, len(c.entries))
	for _, entry := range c.entries {
		paths = append(paths, entry.path)
	}
	inflight := make([]*profileCacheInflight, 0, len(c.inflight))
	for _, pending := range c.inflight {
		inflight = append(inflight, pending)
	}
	c.entries = map[string]profileOverrideCacheEntry{}
	c.inflight = map[string]*profileCacheInflight{}
	c.mu.Unlock()
	for _, path := range paths {
		_ = os.Remove(path)
	}
	for _, pending := range inflight {
		pending.err = errProfileOverrideCacheClosed
		close(pending.done)
	}
}

func applyProfileOverrides(req *engine.RunRequest, overrides map[string]any, baseProfile string) (func(), error) {
	return applyProfileOverridesWithCache(req, overrides, baseProfile, nil)
}

var buildMergedProfileFileFunc = buildMergedProfileFile

func applyProfileOverridesWithCache(req *engine.RunRequest, overrides map[string]any, baseProfile string, cache *profileOverrideCache) (cleanup func(), err error) {
	if req == nil || len(overrides) == 0 {
		if req != nil && baseProfile != "" {
			req.Profile = baseProfile
		}
		return nil, nil
	}

	payload, err := json.Marshal(overrides)
	if err != nil {
		return nil, err
	}

	var cacheKey string
	sharedPath := ""
	if cache != nil {
		cacheKey, err = profileOverrideCacheKey(baseProfile, payload)
		if err != nil {
			return nil, err
		}
		for {
			if cachedPath, ok := cache.Get(cacheKey); ok {
				req.Profile = cachedPath
				return nil, nil
			}

			pending, owner := cache.startInflight(cacheKey)
			if owner {
				defer func() {
					cache.finishInflight(cacheKey, sharedPath, err)
				}()
				break
			}

			<-pending.done
			if pending.err != nil {
				return nil, pending.err
			}
			if pending.path != "" {
				req.Profile = pending.path
				return nil, nil
			}
		}
	}

	path, err := buildMergedProfileFileFunc(payload, baseProfile)
	if err != nil {
		return nil, err
	}

	req.Profile = path
	if cache != nil {
		state, cachedPath := cache.Put(cacheKey, path)
		switch state {
		case profileCachePutInserted:
			// Cache owns this file and cleans it up when the server stops.
			sharedPath = path
			return nil, nil
		case profileCachePutExists:
			_ = os.Remove(path)
			req.Profile = cachedPath
			sharedPath = cachedPath
			return nil, nil
		}
	}
	return func() { _ = os.Remove(path) }, nil
}

func buildMergedProfileFile(overridePayload []byte, baseProfile string) (string, error) {
	base := profile.New()
	if baseProfile != "" {
		data, err := os.ReadFile(baseProfile)
		if err != nil {
			return "", err
		}
		base, err = profile.FromYAML(string(data))
		if err != nil {
			return "", err
		}
	}
	overrideProfile, err := profile.FromJSON(string(overridePayload))
	if err != nil {
		return "", err
	}
	if err := base.Merge(overrideProfile); err != nil {
		return "", err
	}
	merged, err := base.ToJSON()
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "gonemaster-profile-*.json")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write([]byte(merged)); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func profileOverrideCacheKey(baseProfile string, overrideJSON []byte) (string, error) {
	var baseFingerprint string
	if baseProfile == "" {
		baseFingerprint = "<default>"
	} else {
		stat, err := os.Stat(baseProfile)
		if err != nil {
			return "", err
		}
		baseFingerprint = fmt.Sprintf("%s|%d|%d", baseProfile, stat.Size(), stat.ModTime().UTC().UnixNano())
	}

	hash := sha256.New()
	_, _ = hash.Write([]byte(baseFingerprint))
	_, _ = hash.Write([]byte{'\n'})
	_, _ = hash.Write(overrideJSON)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
