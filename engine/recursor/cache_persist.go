package recursor

import (
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// NameserverRef identifies a nameserver host+address pair.
type NameserverRef struct {
	Name    string
	Address string
}

// CacheEntry is a portable representation of a single recursor cache record.
type CacheEntry struct {
	Name        string
	QType       string
	QClass      string
	Nameservers []NameserverRef
	Message     []byte
}

// ExportCacheEntries returns a deterministic snapshot of the recursor cache.
func (r *Recursor) ExportCacheEntries() ([]CacheEntry, error) {
	if r == nil {
		return nil, nil
	}

	r.cacheMu.Lock()
	keys := make([]string, 0, len(r.recurseCache))
	for key := range r.recurseCache {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	type flatEntry struct {
		key    string
		qtype  string
		qclass string
		pkt    *packet.Packet
	}
	flat := make([]flatEntry, 0)
	for _, key := range keys {
		byType := r.recurseCache[key]
		types := slices.Sorted(maps.Keys(byType))
		for _, qtype := range types {
			byClass := byType[qtype]
			classes := slices.Sorted(maps.Keys(byClass))
			for _, qclass := range classes {
				entry := byClass[qclass]
				if entry == nil || entry.resp == nil || entry.resp.Msg == nil {
					continue
				}
				flat = append(flat, flatEntry{key: key, qtype: qtype, qclass: qclass, pkt: entry.resp})
			}
		}
	}
	r.cacheMu.Unlock()

	entries := make([]CacheEntry, 0, len(flat))
	for _, f := range flat {
		name, refs, err := parseCacheKey(f.key)
		if err != nil {
			return nil, fmt.Errorf("export recursor cache: %w", err)
		}
		if err := f.pkt.Msg.Pack(); err != nil {
			return nil, fmt.Errorf("pack recursor cache entry (%s %s %s): %w", name, f.qtype, f.qclass, err)
		}
		entries = append(entries, CacheEntry{
			Name:        name,
			QType:       f.qtype,
			QClass:      f.qclass,
			Nameservers: refs,
			Message:     append([]byte(nil), f.pkt.Msg.Data...),
		})
	}
	return entries, nil
}

// ImportCacheEntries populates the recursor cache from the supplied entries.
func (r *Recursor) ImportCacheEntries(entries []CacheEntry) error {
	if r == nil {
		return fmt.Errorf("recursor is nil")
	}

	for idx, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return fmt.Errorf("entry %d: name is required", idx)
		}
		qtype := strings.ToUpper(strings.TrimSpace(entry.QType))
		if qtype == "" {
			return fmt.Errorf("entry %d: qtype is required", idx)
		}
		qclass := strings.ToUpper(strings.TrimSpace(entry.QClass))
		if qclass == "" {
			qclass = "IN"
		}
		if len(entry.Message) == 0 {
			return fmt.Errorf("entry %d: message is required", idx)
		}

		nsList, err := refsToNameservers(entry.Nameservers)
		if err != nil {
			return fmt.Errorf("entry %d: %w", idx, err)
		}

		msg := new(dns.Msg)
		msg.Data = append([]byte(nil), entry.Message...)
		if err := msg.Unpack(); err != nil {
			return fmt.Errorf("entry %d: unpack message: %w", idx, err)
		}

		key := cacheNameKey(dnsname.New(name), nsList)
		r.cacheStore(key, qtype, qclass, packet.Packet{Msg: msg})
	}
	return nil
}

func refsToNameservers(refs []NameserverRef) ([]nameserver.Nameserver, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	out := make([]nameserver.Nameserver, 0, len(refs))
	for idx, ref := range refs {
		nsName := strings.TrimSpace(ref.Name)
		if nsName == "" {
			return nil, fmt.Errorf("nameserver %d: name is required", idx)
		}
		addrStr := strings.TrimSpace(ref.Address)
		if addrStr == "" {
			return nil, fmt.Errorf("nameserver %d: address is required", idx)
		}
		addr, err := netip.ParseAddr(addrStr)
		if err != nil {
			return nil, fmt.Errorf("nameserver %d: invalid address %q: %w", idx, addrStr, err)
		}
		out = append(out, nameserver.Nameserver{
			Name:    dnsname.New(nsName),
			Address: addr,
		})
	}
	return out, nil
}

// parseCacheKey inverts cacheNameKey.
func parseCacheKey(key string) (string, []NameserverRef, error) {
	switch {
	case strings.HasPrefix(key, "root|"):
		return strings.TrimPrefix(key, "root|"), nil, nil
	case strings.HasPrefix(key, "ns|"):
		body := strings.TrimPrefix(key, "ns|")
		sep := strings.LastIndex(body, "|")
		if sep < 0 {
			return "", nil, fmt.Errorf("invalid ns cache key %q", key)
		}
		nsPart := body[:sep]
		name := body[sep+1:]
		parts := strings.Split(nsPart, ",")
		refs := make([]NameserverRef, 0, len(parts))
		for _, part := range parts {
			at := strings.LastIndex(part, "@")
			if at < 0 {
				return "", nil, fmt.Errorf("invalid nameserver token %q in key %q", part, key)
			}
			refs = append(refs, NameserverRef{
				Name:    part[:at],
				Address: part[at+1:],
			})
		}
		return name, refs, nil
	default:
		return "", nil, fmt.Errorf("unknown cache key prefix in %q", key)
	}
}
