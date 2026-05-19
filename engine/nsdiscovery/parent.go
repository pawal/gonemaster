package nsdiscovery

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
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

var parentCache = struct {
	mu    sync.Mutex
	items map[string]parentCacheEntry
}{
	items: map[string]parentCacheEntry{},
}

// ClearParentNSCache clears the cache used by ParentNameservers.
func ClearParentNSCache() {
	parentCache.mu.Lock()
	parentCache.items = map[string]parentCacheEntry{}
	parentCache.mu.Unlock()
}

// ParentNameservers returns the parent zone's nameservers by walking the
// delegation chain from the root.
func ParentNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	r := z.Recursor()
	if r == nil {
		return nil, fmt.Errorf("missing recursor")
	}
	prof := profile.FromContext(ctx)

	if z.Name.String() == "." || r.HasFakeAddresses(z.Name.String()) {
		return []nameserver.Nameserver{}, nil
	}

	key := strings.ToLower(z.Name.String())
	parentCache.mu.Lock()
	if cached, ok := parentCache.items[key]; ok {
		parentCache.mu.Unlock()
		if !cached.defined {
			return nil, nil
		}
		return materializeParentServers(ctx, r.Client(), cached.servers), nil
	}
	parentCache.mu.Unlock()

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
						if err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
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
					cacheParent(key, nil, false)
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
									if err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
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
									if err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
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
				}
				break
			}
		}
	}

	if len(parentNS) == 0 {
		cacheParent(key, nil, false)
		return nil, nil
	}

	parentNS = uniqueSortedNameservers(parentNS)
	cacheParent(key, parentNS, true)
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

	keys := make([]string, 0, len(nsByIP))
	for key := range nsByIP {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]nameserver.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, nsByIP[key])
	}
	return out, nil
}

func cacheParent(key string, servers []nameserver.Nameserver, defined bool) {
	parentCache.mu.Lock()
	parentCache.items[key] = parentCacheEntry{defined: defined, servers: snapshotParentServers(servers)}
	parentCache.mu.Unlock()
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
