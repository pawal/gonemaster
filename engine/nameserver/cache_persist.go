package nameserver

import (
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

// Entry is a portable representation of a single nameserver cache record.
type Entry struct {
	// Address is the nameserver address that owns the entry.
	Address string
	// Key is the normalized packet cache lookup key.
	Key string
	// Message is the wire-format DNS response packet.
	Message []byte
	// AnswerFrom records the responder address captured with the packet.
	AnswerFrom string
	// NoMessage marks a cached nil response entry.
	NoMessage bool
}

// ExportEntries returns a deterministic snapshot of the cache contents.
func (c *CacheStore) ExportEntries() ([]Entry, error) {
	if c == nil {
		return nil, nil
	}

	c.mu.Lock()
	cacheByAddress := make(map[string]*queryCache, len(c.cacheByAddress))
	maps.Copy(cacheByAddress, c.cacheByAddress)
	c.mu.Unlock()

	addresses := slices.Sorted(maps.Keys(cacheByAddress))

	entries := make([]Entry, 0)
	for _, address := range addresses {
		cache := cacheByAddress[address]
		if cache == nil {
			continue
		}
		cache.mu.Lock()
		keys := make([]string, 0, len(cache.data))
		for key := range cache.data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			entry := Entry{
				Address: address,
				Key:     key,
			}
			value := cache.data[key]
			if value == nil || value.Msg == nil {
				entry.NoMessage = true
				entries = append(entries, entry)
				continue
			}
			if err := value.Msg.Pack(); err != nil {
				cache.mu.Unlock()
				return nil, fmt.Errorf("pack nameserver cache entry (%s, %s): %w", address, key, err)
			}
			entry.Message = append([]byte(nil), value.Msg.Data...)
			entry.AnswerFrom = value.AnswerFrom
			entries = append(entries, entry)
		}
		cache.mu.Unlock()
	}

	return entries, nil
}

// ImportEntries merges the supplied entries into the cache store.
func (c *CacheStore) ImportEntries(entries []Entry) error {
	if c == nil {
		return fmt.Errorf("cache store is nil")
	}

	for idx, entry := range entries {
		address, err := normalizeCacheAddress(entry.Address)
		if err != nil {
			return fmt.Errorf("entry %d: %w", idx, err)
		}
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			return fmt.Errorf("entry %d: key is required", idx)
		}
		cache := c.cacheForAddress(address)
		if cache == nil {
			return fmt.Errorf("entry %d: cache unavailable for address %s", idx, address)
		}

		if entry.NoMessage {
			cache.set(key, nil)
			continue
		}
		if len(entry.Message) == 0 {
			return fmt.Errorf("entry %d: message is required when no_message is false", idx)
		}
		msg := new(dns.Msg)
		msg.Data = append([]byte(nil), entry.Message...)
		if err := msg.Unpack(); err != nil {
			return fmt.Errorf("entry %d: unpack message: %w", idx, err)
		}
		cache.set(key, &packet.Packet{
			Msg:        msg,
			AnswerFrom: strings.TrimSpace(entry.AnswerFrom),
		})
	}
	return nil
}

func normalizeCacheAddress(value string) (string, error) {
	address := strings.TrimSpace(value)
	if address == "" {
		return "", fmt.Errorf("address is required")
	}
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return "", fmt.Errorf("invalid address %q: %w", value, err)
	}
	return addr.String(), nil
}
