package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

const defaultProfileOverrideCacheMaxEntries = 256
const defaultProfileOverrideCacheTTL = 30 * time.Minute

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

type profileOverrideCache struct {
	mu         sync.Mutex
	entries    map[string]profileOverrideCacheEntry
	maxEntries int
	ttl        time.Duration
}

func newProfileOverrideCache(maxEntries int, ttl time.Duration) *profileOverrideCache {
	if maxEntries < 1 {
		maxEntries = defaultProfileOverrideCacheMaxEntries
	}
	if ttl <= 0 {
		ttl = defaultProfileOverrideCacheTTL
	}
	return &profileOverrideCache{
		entries:    map[string]profileOverrideCacheEntry{},
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
	if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		delete(c.entries, key)
		stalePath = entry.path
		c.mu.Unlock()
		if stalePath != "" {
			_ = os.Remove(stalePath)
		}
		return "", false
	}
	entry.lastUsed = now
	entry.expiresAt = now.Add(c.ttl)
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
		if entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
			entry.lastUsed = now
			entry.expiresAt = now.Add(c.ttl)
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
		expiresAt: now.Add(c.ttl),
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
	c.entries = map[string]profileOverrideCacheEntry{}
	c.mu.Unlock()
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func applyProfileOverrides(req *engine.RunRequest, overrides map[string]any, baseProfile string) (func(), error) {
	return applyProfileOverridesWithCache(req, overrides, baseProfile, nil)
}

func applyProfileOverridesWithCache(req *engine.RunRequest, overrides map[string]any, baseProfile string, cache *profileOverrideCache) (func(), error) {
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

	cacheKey, err := profileOverrideCacheKey(baseProfile, payload)
	if err != nil {
		return nil, err
	}
	if cache != nil {
		if cachedPath, ok := cache.Get(cacheKey); ok {
			req.Profile = cachedPath
			return nil, nil
		}
	}

	base := profile.New()
	if baseProfile != "" {
		data, err := os.ReadFile(baseProfile)
		if err != nil {
			return nil, err
		}
		base, err = profile.FromYAML(string(data))
		if err != nil {
			return nil, err
		}
	}
	overrideProfile, err := profile.FromJSON(string(payload))
	if err != nil {
		return nil, err
	}
	if err := base.Merge(overrideProfile); err != nil {
		return nil, err
	}
	merged, err := base.ToJSON()
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "gonemaster-profile-*.json")
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Write([]byte(merged)); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return nil, err
	}

	req.Profile = tmp.Name()
	if cache != nil {
		state, cachedPath := cache.Put(cacheKey, tmp.Name())
		switch state {
		case profileCachePutInserted:
			// Cache owns this file and cleans it up when the server stops.
			return nil, nil
		case profileCachePutExists:
			_ = os.Remove(tmp.Name())
			req.Profile = cachedPath
			return nil, nil
		}
	}
	return func() { _ = os.Remove(tmp.Name()) }, nil
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
