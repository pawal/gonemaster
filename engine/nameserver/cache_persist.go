package nameserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

const (
	// PacketCacheFileFormat identifies the on-disk packet cache file format.
	PacketCacheFileFormat = "gonemaster.packet-cache"
	// PacketCacheFileVersion is the current packet cache file version.
	PacketCacheFileVersion = 1
)

// PacketCacheFile is the portable representation of nameserver query cache data.
type PacketCacheFile struct {
	// Format identifies the on-disk file format.
	Format string `json:"format"`
	// Version identifies the schema version for compatibility checks.
	Version int `json:"version"`
	// Entries contains the serialized per-query cache entries.
	Entries []PacketCacheEntry `json:"entries"`
}

// PacketCacheEntry stores one cached query result for one nameserver address.
type PacketCacheEntry struct {
	// Address is the nameserver address that owns the cache entry.
	Address string `json:"address"`
	// Key is the normalized packet cache lookup key.
	Key string `json:"key"`
	// Message is the base64-encoded wire-format DNS message.
	Message string `json:"message,omitempty"`
	// AnswerFrom records the responder address captured with the packet.
	AnswerFrom string `json:"answer_from,omitempty"`
	// NoMessage marks a cached nil response entry.
	NoMessage bool `json:"no_message,omitempty"`
}

// ExportPacketCache returns a deterministic snapshot of query cache entries.
func (c *CacheStore) ExportPacketCache() (PacketCacheFile, error) {
	out := PacketCacheFile{
		Format:  PacketCacheFileFormat,
		Version: PacketCacheFileVersion,
		Entries: []PacketCacheEntry{},
	}
	if c == nil {
		return out, nil
	}

	c.mu.Lock()
	cacheByAddress := make(map[string]*queryCache, len(c.cacheByAddress))
	for address, cache := range c.cacheByAddress {
		cacheByAddress[address] = cache
	}
	c.mu.Unlock()

	addresses := make([]string, 0, len(cacheByAddress))
	for address := range cacheByAddress {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)

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
			entry := PacketCacheEntry{
				Address: address,
				Key:     key,
			}
			value := cache.data[key]
			if value == nil || value.Msg == nil {
				entry.NoMessage = true
				out.Entries = append(out.Entries, entry)
				continue
			}
			if err := value.Msg.Pack(); err != nil {
				cache.mu.Unlock()
				return PacketCacheFile{}, fmt.Errorf("pack packet cache entry (%s, %s): %w", address, key, err)
			}
			entry.Message = base64.StdEncoding.EncodeToString(value.Msg.Data)
			if value.AnswerFrom != "" {
				entry.AnswerFrom = value.AnswerFrom
			}
			out.Entries = append(out.Entries, entry)
		}
		cache.mu.Unlock()
	}

	return out, nil
}

// ImportPacketCache merges packet cache entries into the current cache store.
func (c *CacheStore) ImportPacketCache(input PacketCacheFile) error {
	if c == nil {
		return fmt.Errorf("cache store is nil")
	}
	if strings.TrimSpace(input.Format) == "" {
		return fmt.Errorf("packet cache format is required")
	}
	if input.Format != PacketCacheFileFormat {
		return fmt.Errorf("unsupported packet cache format %q", input.Format)
	}
	if input.Version != PacketCacheFileVersion {
		return fmt.Errorf("unsupported packet cache version %d", input.Version)
	}

	for idx, entry := range input.Entries {
		address, err := normalizePacketCacheAddress(entry.Address)
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
		if strings.TrimSpace(entry.Message) == "" {
			return fmt.Errorf("entry %d: message is required when no_message is false", idx)
		}
		wire, err := base64.StdEncoding.DecodeString(entry.Message)
		if err != nil {
			return fmt.Errorf("entry %d: decode message: %w", idx, err)
		}
		msg := new(dns.Msg)
		msg.Data = wire
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

// SavePacketCache writes packet cache entries to path in JSON format.
func (c *CacheStore) SavePacketCache(path string) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return fmt.Errorf("packet cache save path is required")
	}
	payload, err := c.ExportPacketCache()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return err
	}
	return nil
}

// RestorePacketCache reads packet cache entries from path and imports them.
func (c *CacheStore) RestorePacketCache(path string) error {
	source := strings.TrimSpace(path)
	if source == "" {
		return fmt.Errorf("packet cache restore path is required")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	var payload PacketCacheFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	return c.ImportPacketCache(payload)
}

func normalizePacketCacheAddress(value string) (string, error) {
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
