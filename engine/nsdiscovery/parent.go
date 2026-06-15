package nsdiscovery

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/transport"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

type parentCacheEntry struct {
	defined bool
	servers []parentCacheServer
}

type parentCacheServer struct {
	Name    string
	Address string
}

// Cache holds memoised ParentNameservers results for one engine run.
// Attach it to a context with [WithCache] so concurrent runs each see
// their own cache without sharing state.
type Cache struct {
	mu    sync.Mutex
	items map[string]parentCacheEntry
}

// NewCache returns an empty Cache ready to be attached via [WithCache].
func NewCache() *Cache {
	return &Cache{items: map[string]parentCacheEntry{}}
}

// Clear empties the cache.
func (c *Cache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.items = map[string]parentCacheEntry{}
	c.mu.Unlock()
}

// Len returns the number of cached entries.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *Cache) lookup(key string) (parentCacheEntry, bool) {
	if c == nil {
		return parentCacheEntry{}, false
	}
	c.mu.Lock()
	entry, ok := c.items[key]
	c.mu.Unlock()
	return entry, ok
}

func (c *Cache) store(key string, servers []nameserver.Nameserver, defined bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.items[key] = parentCacheEntry{defined: defined, servers: snapshotParentServers(servers)}
	c.mu.Unlock()
}

type cacheCtxKey struct{}

// WithCache returns a context carrying c, so [ParentNameservers] reuses
// already-walked parent chains within the run.
func WithCache(ctx context.Context, c *Cache) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, cacheCtxKey{}, c)
}

// cacheFromContext returns the Cache attached to ctx, or nil if none.
func cacheFromContext(ctx context.Context) *Cache {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.Value(cacheCtxKey{}).(*Cache); ok {
		return c
	}
	return nil
}

// ParentNameservers returns the nameservers of the parent zone, found by
// walking the delegation chain from the root down to the parent of z and
// collecting the parent's NS RRset and addresses.
//
// Results are memoised in the [Cache] attached to ctx via [WithCache] (one
// cache per engine run, isolated between runs). With no cache attached,
// every call re-walks the chain.
//
// For the root zone and zones whose names appear in the recursor's
// fake-address map (undelegated test setups), an empty slice and nil
// error are returned without caching.
//
// Errors are returned if z is nil, if z has no recursor, or if the chain
// walk encounters a definitive failure. An unreachable intermediate
// produces an empty result with no error - callers must check len().
func ParentNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	r := z.Recursor()
	if r == nil {
		return nil, fmt.Errorf("missing recursor")
	}
	prof := profile.FromContext(ctx)
	cache := cacheFromContext(ctx)

	if z.Name.String() == "." || r.HasFakeAddresses(z.Name.String()) {
		return []nameserver.Nameserver{}, nil
	}

	key := strings.ToLower(z.Name.String())
	if cached, ok := cache.lookup(key); ok {
		if !cached.defined {
			return nil, nil
		}
		return materializeParentServers(ctx, r.Client(), cached.servers), nil
	}

	root, err := r.RootServers(ctx)
	if err != nil {
		return nil, err
	}

	handled := map[string]map[string]bool{}
	remaining := map[string][]nameserver.Nameserver{
		".": root,
	}
	var parentNS []nameserver.Nameserver

	pushToRemaining := func(ns nameserver.Nameserver, zoneName string) {
		zoneNameObj := dnsname.New(zoneName)
		zoneKey := strings.ToLower(zoneNameObj.String())
		nsKey := strings.ToLower(ns.String())
		if handled[zoneKey] != nil && handled[zoneKey][nsKey] {
			return
		}
		for _, existing := range remaining[zoneKey] {
			if strings.EqualFold(existing.String(), ns.String()) {
				return
			}
		}
		remaining[zoneKey] = append(remaining[zoneKey], ns)
	}

	zLabels := z.Name.Labels()

	for len(remaining) > 0 {
		zoneKey := firstKey(remaining)
		servers := remaining[zoneKey]
		delete(remaining, zoneKey)

	serverLoop:
		for len(servers) > 0 {
			ns := servers[0]
			servers = servers[1:]

			addr := ns.Address.String()
			if handled[zoneKey] == nil {
				handled[zoneKey] = map[string]bool{}
			}
			nsKey := strings.ToLower(ns.String())
			if handled[zoneKey][nsKey] {
				for _, existing := range parentNS {
					if existing.Address.String() == addr && !strings.EqualFold(existing.String(), ns.String()) {
						parentNS = append(parentNS, ns)
						break
					}
				}
				continue
			}
			handled[zoneKey][nsKey] = true

			if ns.Address.Is4() && !prof.Net.IPv4 {
				continue
			}
			if ns.Address.Is6() && !prof.Net.IPv6 {
				continue
			}

			pSOA := queryPacket(ctx, ns, zoneKey, "SOA")
			if !validSOA(pSOA, dnsname.New(zoneKey)) {
				continue
			}

			pNS := queryPacket(ctx, ns, zoneKey, "NS")
			if !validNS(pNS, dnsname.New(zoneKey)) {
				continue
			}

			rrsNS := nsMapFromResponse(pNS, dnsname.New(zoneKey), "answer")
			for nsName := range rrsNS {
				if len(rrsNS[nsName]) == 0 {
					for _, qtype := range []string{"A", "AAAA"} {
						resp, err := r.Recurse(ctx, nsName, qtype, "IN")
						if err := recursor.IgnoreCNAMEError(err); err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
							continue
						}
						rrsNS[nsName] = append(rrsNS[nsName], collectAddrs(resp, qtype, dnsname.New(nsName))...)
					}
				}
				for _, addr := range uniqueAddrs(rrsNS[nsName]) {
					next, err := nameserver.NewWithContext(ctx, nsName, addr.String(), r.Client())
					if err != nil {
						continue
					}
					pushToRemaining(next, zoneKey)
				}
			}

			intermediate := dnsname.New(zoneKey)
			loopCount := 0
		loop:
			for {
				loopCount++
				if loopCount >= 1000 {
					cache.store(key, nil, false)
					return nil, nil
				}

				if len(intermediate.Labels()) >= len(zLabels) {
					break
				}

				idx := len(zLabels) - len(intermediate.Labels()) - 1
				intermediate = intermediate.Prepend(zLabels[idx])

				pSOA = queryPacket(ctx, ns, intermediate.String(), "SOA")
				if pSOA.Msg == nil {
					continue serverLoop
				}

				if validSOA(pSOA, intermediate) {
					if strings.EqualFold(intermediate.String(), z.Name.String()) {
						parentNS = append(parentNS, ns)
					} else {
						pNS = queryPacket(ctx, ns, intermediate.String(), "NS")
						if !validNS(pNS, intermediate) {
							continue
						}

						rrsNSBis := nsMapFromResponse(pNS, intermediate, "answer")
						for nsName := range rrsNSBis {
							if len(rrsNSBis[nsName]) == 0 {
								for _, qtype := range []string{"A", "AAAA"} {
									resp, err := r.Recurse(ctx, nsName, qtype, "IN")
									if err := recursor.IgnoreCNAMEError(err); err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
										continue
									}
									rrsNSBis[nsName] = append(rrsNSBis[nsName], collectAddrs(resp, qtype, dnsname.New(nsName))...)
								}
							}
							for _, addr := range uniqueAddrs(rrsNSBis[nsName]) {
								next, err := nameserver.NewWithContext(ctx, nsName, addr.String(), r.Client())
								if err != nil {
									continue
								}
								pushToRemaining(next, intermediate.String())
							}
						}
						continue loop
					}
				} else if pSOA.IsRedirect() && len(pSOA.GetRecordsForName("NS", intermediate, "authority")) > 0 {
					if strings.EqualFold(intermediate.String(), z.Name.String()) {
						parentNS = append(parentNS, ns)
					} else {
						rrsNSBis := nsMapFromResponse(pSOA, intermediate, "authority")
						for nsName := range rrsNSBis {
							if len(rrsNSBis[nsName]) == 0 {
								for _, qtype := range []string{"A", "AAAA"} {
									resp, err := r.Recurse(ctx, nsName, qtype, "IN")
									if err := recursor.IgnoreCNAMEError(err); err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
										continue
									}
									rrsNSBis[nsName] = append(rrsNSBis[nsName], collectAddrs(resp, qtype, dnsname.New(nsName))...)
								}
							}
							for _, addr := range uniqueAddrs(rrsNSBis[nsName]) {
								next, err := nameserver.NewWithContext(ctx, nsName, addr.String(), r.Client())
								if err != nil {
									continue
								}
								pushToRemaining(next, intermediate.String())
							}
						}
					}
				} else if pSOA.Rcode() == "NOERROR" && pSOA.AA() {
					if !strings.EqualFold(intermediate.String(), z.Name.String()) {
						continue loop
					}
				} else if pSOA.Rcode() == "NXDOMAIN" && pSOA.AA() && !strings.EqualFold(intermediate.String(), z.Name.String()) {
					// RFC 8020 contradiction: NXDOMAIN at an intermediate
					// empty non-terminal, but the same NS may still hold
					// a referral at z.Name. Probe directly; if the child
					// referral is present, accept this NS as the parent.
					// Basic01 reports this as B01_PARENT_NXDOMAIN_HIDES_DELEGATION.
					pChild := queryPacket(ctx, ns, z.Name.String(), "SOA")
					if pChild.IsRedirect() && len(pChild.GetRecordsForName("NS", z.Name, "authority")) > 0 {
						parentNS = append(parentNS, ns)
					}
				}
				break
			}
		}
	}

	if len(parentNS) == 0 {
		cache.store(key, nil, false)
		return nil, nil
	}

	parentNS = uniqueSortedNameservers(parentNS)
	cache.store(key, parentNS, true)
	return cloneNameservers(parentNS), nil
}

// parentNSIPs returns parent nameservers filtered to unique IPs.
func parentNSIPs(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	parent, err := ParentNameservers(ctx, z)
	if err != nil || parent == nil {
		return parent, err
	}

	nsByIP := map[string]nameserver.Nameserver{}
	for _, ns := range parent {
		ip := ns.Address.String()
		if _, ok := nsByIP[ip]; !ok {
			nsByIP[ip] = ns
		}
	}

	keys := slices.Sorted(maps.Keys(nsByIP))

	out := make([]nameserver.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, nsByIP[key])
	}
	return out, nil
}

func snapshotParentServers(list []nameserver.Nameserver) []parentCacheServer {
	if list == nil {
		return nil
	}
	out := make([]parentCacheServer, 0, len(list))
	for _, item := range list {
		addr := item.Address.String()
		if addr == "" {
			continue
		}
		out = append(out, parentCacheServer{
			Name:    item.Name.String(),
			Address: addr,
		})
	}
	return out
}

func materializeParentServers(ctx context.Context, client *transport.Client, list []parentCacheServer) []nameserver.Nameserver {
	if len(list) == 0 {
		return nil
	}
	out := make([]nameserver.Nameserver, 0, len(list))
	for _, item := range list {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Address) == "" {
			continue
		}
		ns, err := nameserver.NewWithContext(ctx, item.Name, item.Address, client)
		if err != nil {
			continue
		}
		out = append(out, ns)
	}
	return out
}
