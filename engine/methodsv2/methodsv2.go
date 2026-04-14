package methodsv2

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// NSItem represents either a nameserver with an address or a bare nameserver name.
type NSItem struct {
	// Name is the canonical nameserver host name.
	Name dnsname.Name
	// Address is the nameserver IP address when available.
	Address netip.Addr
	// HasAddress reports whether Address is populated.
	HasAddress bool
}

// String returns a stable string representation for sorting/deduplication.
func (n NSItem) String() string {
	if n.HasAddress {
		return n.Name.String() + "/" + n.Address.String()
	}
	return n.Name.String()
}

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

// ClearCache clears cached results for GetParentNSNamesAndIPs.
func ClearCache() {
	parentCache.mu.Lock()
	parentCache.items = map[string]parentCacheEntry{}
	parentCache.mu.Unlock()
}

// GetParentNSNamesAndIPs returns nameservers for the parent zone.
func GetParentNSNamesAndIPs(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
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

// GetParentNSIPs returns parent nameservers filtered to unique IPs.
func GetParentNSIPs(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	parent, err := GetParentNSNamesAndIPs(ctx, z)
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

// GetDelNSNamesAndIPs returns delegation names and addresses for the zone.
func GetDelNSNamesAndIPs(ctx context.Context, z *zone.Zone) ([]NSItem, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}

	items, err := getDelegation(ctx, z)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return nil, nil
	}

	var nsNames []dnsname.Name
	for _, item := range items {
		if !item.HasAddress {
			nsNames = append(nsNames, item.Name)
		}
	}

	oob, err := getOOBIPs(ctx, z, nsNames)
	if err != nil {
		return nil, err
	}

	var out []NSItem
	for _, item := range items {
		if item.HasAddress || z.Name.IsInBailiwick(item.Name) {
			out = append(out, item)
		}
	}
	out = append(out, oob...)
	return uniqueSortedItems(out), nil
}

// GetDelNSNames returns delegation names for the zone.
func GetDelNSNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	items, err := GetDelNSNamesAndIPs(ctx, z)
	if err != nil || items == nil {
		return nil, err
	}

	seen := map[string]dnsname.Name{}
	for _, item := range items {
		seen[strings.ToLower(item.Name.String())] = item.Name
	}
	return sortedNames(seen), nil
}

// GetDelNSIPs returns delegation IPs for the zone.
func GetDelNSIPs(ctx context.Context, z *zone.Zone) ([]string, error) {
	items, err := GetDelNSNamesAndIPs(ctx, z)
	if err != nil || items == nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		if !item.HasAddress {
			continue
		}
		ip := item.Address.String()
		if !seen[ip] {
			seen[ip] = true
			out = append(out, ip)
		}
	}
	sort.Strings(out)
	return out, nil
}

// GetZoneNSNames returns authoritative nameserver names from the zone apex.
func GetZoneNSNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	if r := z.Recursor(); r != nil && z.Name.String() != "." && r.HasFakeAddresses(z.Name.String()) {
		return GetDelNSNames(ctx, z)
	}

	items, err := GetDelNSNamesAndIPs(ctx, z)
	if err != nil || items == nil {
		return nil, err
	}

	var nsNames []dnsname.Name
	for _, item := range items {
		if !item.HasAddress {
			continue
		}
		ns, ok := toNameserver(ctx, z, item)
		if !ok {
			continue
		}
		resp := queryPacket(ctx, ns, z.Name.String(), "NS")
		if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
			continue
		}
		for _, rr := range resp.GetRecordsForName("NS", z.Name, "answer") {
			if nsRR, ok := rr.(*dns.NS); ok {
				name := dnsname.New(strings.ToLower(nsRR.Ns))
				nsNames = append(nsNames, name)
			}
		}
	}

	seen := map[string]dnsname.Name{}
	for _, name := range nsNames {
		seen[strings.ToLower(name.String())] = name
	}
	return sortedNames(seen), nil
}

// GetZoneNSNamesAndIPs returns names and addresses from the zone apex.
func GetZoneNSNamesAndIPs(ctx context.Context, z *zone.Zone) ([]NSItem, error) {
	nsNames, err := GetZoneNSNames(ctx, z)
	if err != nil || nsNames == nil {
		return nil, err
	}
	if len(nsNames) == 0 {
		return []NSItem{}, nil
	}

	ibNS, err := getIBAddrInZone(ctx, z)
	if err != nil {
		return nil, err
	}
	oobNS, err := getOOBIPs(ctx, z, nsNames)
	if err != nil {
		return nil, err
	}

	var out []NSItem
	for _, nsName := range nsNames {
		if z.Name.IsInBailiwick(nsName) {
			if len(ibNS) > 0 {
				for _, ib := range ibNS {
					if strings.EqualFold(nsName.String(), ib.Name.String()) {
						out = append(out, NSItem{Name: nsName, Address: ib.Address, HasAddress: true})
					}
				}
			} else {
				out = append(out, NSItem{Name: nsName})
			}
		} else {
			if len(oobNS) > 0 {
				for _, oob := range oobNS {
					if strings.EqualFold(nsName.String(), oob.Name.String()) {
						if oob.HasAddress {
							out = append(out, NSItem{Name: nsName, Address: oob.Address, HasAddress: true})
						} else {
							out = append(out, NSItem{Name: nsName})
						}
					}
				}
			} else {
				out = append(out, NSItem{Name: nsName})
			}
		}
	}

	return uniqueSortedItems(out), nil
}

// GetZoneNSIPs returns authoritative nameserver IPs from the zone apex.
func GetZoneNSIPs(ctx context.Context, z *zone.Zone) ([]string, error) {
	items, err := GetZoneNSNamesAndIPs(ctx, z)
	if err != nil || items == nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		if !item.HasAddress {
			continue
		}
		ip := item.Address.String()
		if !seen[ip] {
			seen[ip] = true
			out = append(out, ip)
		}
	}
	sort.Strings(out)
	return out, nil
}

func getDelegation(ctx context.Context, z *zone.Zone) ([]NSItem, error) {
	r := z.Recursor()
	if r == nil {
		return nil, fmt.Errorf("missing recursor")
	}

	if r.HasFakeAddresses(z.Name.String()) {
		var out []NSItem
		for _, nsName := range r.GetFakeNames(z.Name.String()) {
			nameObj := dnsname.New(nsName)
			addrs := r.GetFakeAddresses(z.Name.String(), nsName)
			if len(addrs) == 0 {
				out = append(out, NSItem{Name: nameObj})
				continue
			}
			for _, addr := range addrs {
				out = append(out, NSItem{Name: nameObj, Address: addr, HasAddress: true})
			}
		}
		return uniqueSortedItems(out), nil
	}

	if z.Name.String() == "." {
		root, err := r.RootServers(ctx)
		if err != nil {
			return nil, err
		}
		var out []NSItem
		for _, ns := range root {
			out = append(out, NSItem{Name: ns.Name, Address: ns.Address, HasAddress: true})
		}
		return uniqueSortedItems(out), nil
	}

	parentNS, err := GetParentNSIPs(ctx, z)
	if err != nil {
		return nil, err
	}
	if parentNS == nil {
		return nil, nil
	}

	delegationNS := map[string][]netip.Addr{}
	aaNS := map[string][]netip.Addr{}
	zoneName := z.Name

	for _, ns := range parentNS {
		resp := queryPacket(ctx, ns, zoneName.String(), "NS")
		if resp.Msg == nil || resp.Rcode() != "NOERROR" {
			continue
		}

		if resp.IsRedirect() {
			for _, rr := range resp.GetRecordsForName("NS", zoneName, "authority") {
				if nsRR, ok := rr.(*dns.NS); ok {
					nsNameObj := dnsname.New(nsRR.Ns)
					key := strings.ToLower(nsNameObj.String())
					if _, exists := delegationNS[key]; !exists {
						delegationNS[key] = []netip.Addr{}
					}
				}
			}

			for _, rr := range resp.GetRecords("A", "additional") {
				owner := dnsname.New(rr.Header().Name)
				if !zoneName.IsInBailiwick(owner) {
					continue
				}
				key := strings.ToLower(owner.String())
				if _, ok := delegationNS[key]; ok {
					if addr, ok := addrFromRR(rr); ok {
						delegationNS[key] = append(delegationNS[key], addr)
					}
				}
			}
			for _, rr := range resp.GetRecords("AAAA", "additional") {
				owner := dnsname.New(rr.Header().Name)
				if !zoneName.IsInBailiwick(owner) {
					continue
				}
				key := strings.ToLower(owner.String())
				if _, ok := delegationNS[key]; ok {
					if addr, ok := addrFromRR(rr); ok {
						delegationNS[key] = append(delegationNS[key], addr)
					}
				}
			}
		} else if resp.AA() && len(resp.GetRecordsForName("NS", zoneName, "answer")) > 0 {
			for _, rr := range resp.GetRecordsForName("NS", zoneName, "answer") {
				if nsRR, ok := rr.(*dns.NS); ok {
					nsNameObj := dnsname.New(nsRR.Ns)
					key := strings.ToLower(nsNameObj.String())
					if _, exists := aaNS[key]; !exists {
						aaNS[key] = []netip.Addr{}
					}
				}
			}

			for _, rr := range resp.GetRecords("A", "additional") {
				owner := dnsname.New(rr.Header().Name)
				if !zoneName.IsInBailiwick(owner) {
					continue
				}
				key := strings.ToLower(owner.String())
				if _, ok := aaNS[key]; ok {
					if addr, ok := addrFromRR(rr); ok {
						aaNS[key] = append(aaNS[key], addr)
					}
				}
			}
			for _, rr := range resp.GetRecords("AAAA", "additional") {
				owner := dnsname.New(rr.Header().Name)
				if !zoneName.IsInBailiwick(owner) {
					continue
				}
				key := strings.ToLower(owner.String())
				if _, ok := aaNS[key]; ok {
					if addr, ok := addrFromRR(rr); ok {
						aaNS[key] = append(aaNS[key], addr)
					}
				}
			}

			for nsName := range aaNS {
				if len(aaNS[nsName]) > 0 {
					continue
				}
				for _, qtype := range []string{"A", "AAAA"} {
					resp, err := r.Recurse(ctx, nsName, qtype, "IN")
					if err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
						continue
					}
					addrList := collectResolvedAddrs(resp, qtype, dnsname.New(nsName))
					aaNS[nsName] = append(aaNS[nsName], addrList...)
				}
			}
		}
	}

	var hashRef map[string][]netip.Addr
	if len(delegationNS) > 0 {
		hashRef = delegationNS
	} else if len(aaNS) > 0 {
		hashRef = aaNS
	} else {
		return []NSItem{}, nil
	}

	var out []NSItem
	for nsName, addrs := range hashRef {
		if len(addrs) > 0 {
			for _, addr := range uniqueAddrs(addrs) {
				out = append(out, NSItem{Name: dnsname.New(nsName), Address: addr, HasAddress: true})
			}
		} else {
			out = append(out, NSItem{Name: dnsname.New(nsName)})
		}
	}

	return uniqueSortedItems(out), nil
}

func getOOBIPs(ctx context.Context, z *zone.Zone, nsNames []dnsname.Name) ([]NSItem, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	if len(nsNames) == 0 {
		return []NSItem{}, nil
	}

	r := z.Recursor()
	if r == nil {
		return nil, fmt.Errorf("missing recursor")
	}

	isUndelegated := r.HasFakeAddresses(z.Name.String())
	var out []NSItem

	for _, nsName := range nsNames {
		if z.Name.IsInBailiwick(nsName) {
			continue
		}
		found := false
		if isUndelegated {
			addrs := r.GetFakeAddresses(z.Name.String(), nsName.String())
			if len(addrs) > 0 {
				for _, addr := range addrs {
					out = append(out, NSItem{Name: nsName, Address: addr, HasAddress: true})
				}
				continue
			}
		}
		for _, qtype := range []string{"A", "AAAA"} {
			resp, err := r.Recurse(ctx, nsName.String(), qtype, "IN")
			if err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
				continue
			}
			addrs := collectResolvedAddrs(resp, qtype, nsName)
			if len(addrs) > 0 {
				for _, addr := range addrs {
					out = append(out, NSItem{Name: nsName, Address: addr, HasAddress: true})
					found = true
				}
			}
		}
		if !found {
			out = append(out, NSItem{Name: nsName})
		}
	}

	return uniqueSortedItems(out), nil
}

func getIBAddrInZone(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	delItems, err := GetDelNSNamesAndIPs(ctx, z)
	if err != nil {
		return nil, err
	}
	nsNames, err := GetZoneNSNames(ctx, z)
	if err != nil {
		return nil, err
	}
	if delItems == nil && nsNames == nil {
		return nil, nil
	}

	hasInBailiwick := false
	for _, name := range nsNames {
		if z.Name.IsInBailiwick(name) {
			hasInBailiwick = true
			break
		}
	}
	if !hasInBailiwick {
		return []nameserver.Nameserver{}, nil
	}

	r := z.Recursor()
	if r == nil {
		return nil, fmt.Errorf("missing recursor")
	}
	if z.Name.String() != "." && r.HasFakeAddresses(z.Name.String()) {
		seen := map[string]nameserver.Nameserver{}
		for _, item := range delItems {
			if !item.HasAddress || !z.Name.IsInBailiwick(item.Name) {
				continue
			}
			ns, ok := toNameserver(ctx, z, item)
			if !ok {
				continue
			}
			seen[strings.ToLower(ns.String())] = ns
		}

		keys := make([]string, 0, len(seen))
		for key := range seen {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]nameserver.Nameserver, 0, len(keys))
		for _, key := range keys {
			out = append(out, seen[key])
		}
		return out, nil
	}

	var delServers []nameserver.Nameserver
	for _, item := range delItems {
		if !item.HasAddress {
			continue
		}
		ns, ok := toNameserver(ctx, z, item)
		if ok {
			delServers = append(delServers, ns)
		}
	}

	ibNS := map[string][]netip.Addr{}
	deadDel := map[string]bool{}
	for _, nsName := range nsNames {
		if !z.Name.IsInBailiwick(nsName) {
			continue
		}
		for _, ns := range delServers {
			if deadDel[ns.Address.String()] {
				continue
			}
			for _, qtype := range []string{"A", "AAAA"} {
				resp, err := r.RecurseWithNameservers(ctx, nsName.String(), qtype, "IN", []nameserver.Nameserver{ns})
				if err != nil || resp.Msg == nil {
					deadDel[ns.Address.String()] = true
					break
				}
				if resp.Rcode() != "NOERROR" || !resp.AA() {
					continue
				}
				ibNS[nsName.String()] = append(ibNS[nsName.String()], collectResolvedAddrs(resp, qtype, nsName)...)
			}
			if len(ibNS[nsName.String()]) > 0 {
				break
			}
		}
	}

	seen := map[string]nameserver.Nameserver{}
	for nsName, addrs := range ibNS {
		for _, addr := range uniqueAddrs(addrs) {
			ns, err := nameserver.NewWithContext(ctx, nsName, addr.String(), r.Client())
			if err != nil {
				continue
			}
			seen[strings.ToLower(ns.String())] = ns
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]nameserver.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out, nil
}

func toNameserver(ctx context.Context, z *zone.Zone, item NSItem) (nameserver.Nameserver, bool) {
	if !item.HasAddress {
		return nameserver.Nameserver{}, false
	}
	r := z.Recursor()
	if r == nil {
		return nameserver.Nameserver{}, false
	}
	ns, err := nameserver.NewWithContext(ctx, item.Name.String(), item.Address.String(), r.Client())
	if err != nil {
		return nameserver.Nameserver{}, false
	}
	return ns, true
}

func validSOA(resp packet.Packet, name dnsname.Name) bool {
	return resp.Msg != nil && resp.Rcode() == "NOERROR" && resp.AA() &&
		len(resp.GetRecordsForName("SOA", name, "answer")) == 1
}

func validNS(resp packet.Packet, name dnsname.Name) bool {
	if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
		return false
	}
	answer := resp.GetRecords("NS", "answer")
	if len(answer) == 0 {
		return false
	}
	return len(answer) == len(resp.GetRecordsForName("NS", name, "answer"))
}

func nsMapFromResponse(resp packet.Packet, owner dnsname.Name, section string) map[string][]netip.Addr {
	out := map[string][]netip.Addr{}
	for _, rr := range resp.GetRecordsForName("NS", owner, section) {
		if nsRR, ok := rr.(*dns.NS); ok {
			nsNameObj := dnsname.New(nsRR.Ns)
			key := strings.ToLower(nsNameObj.String())
			out[key] = []netip.Addr{}
		}
	}
	for _, rr := range append(resp.GetRecords("A", "additional"), resp.GetRecords("AAAA", "additional")...) {
		ownerName := dnsname.New(rr.Header().Name)
		key := strings.ToLower(ownerName.String())
		if _, ok := out[key]; ok {
			if addr, ok := addrFromRR(rr); ok {
				out[key] = append(out[key], addr)
			}
		}
	}
	return out
}

func addrFromRR(rr dns.RR) (netip.Addr, bool) {
	switch v := rr.(type) {
	case *dns.A:
		return v.Addr, v.Addr.IsValid()
	case *dns.AAAA:
		return v.Addr, v.Addr.IsValid()
	default:
		return netip.Addr{}, false
	}
}

func collectAddrs(resp packet.Packet, qtype string, target dnsname.Name) []netip.Addr {
	var out []netip.Addr
	for _, rr := range resp.GetRecordsForName(qtype, target) {
		if addr, ok := addrFromRR(rr); ok {
			out = append(out, addr)
		}
	}
	return out
}

func collectResolvedAddrs(resp packet.Packet, qtype string, nsName dnsname.Name) []netip.Addr {
	if resp.HasRRsOfTypeForName("CNAME", nsName, "answer") {
		target := followCNAME(resp, nsName)
		return collectAddrs(resp, qtype, target)
	}

	if cnameFollowed(resp, nsName) {
		target := cnameTargetFromQuestion(resp)
		if resp.HasRRsOfTypeForName("CNAME", target, "answer") {
			target = followCNAME(resp, target)
		}
		return collectAddrs(resp, qtype, target)
	}

	if resp.HasRRsOfTypeForName(qtype, nsName, "answer") {
		return collectAddrs(resp, qtype, nsName)
	}
	return nil
}

func cnameFollowed(resp packet.Packet, nsName dnsname.Name) bool {
	questions := resp.Question()
	if len(questions) == 0 {
		return false
	}
	owner := dnsname.New(questions[0].Header().Name)
	return !strings.EqualFold(owner.String(), nsName.String())
}

func cnameTargetFromQuestion(resp packet.Packet) dnsname.Name {
	questions := resp.Question()
	if len(questions) == 0 {
		return dnsname.Name{}
	}
	return dnsname.New(questions[0].Header().Name)
}

func followCNAME(resp packet.Packet, start dnsname.Name) dnsname.Name {
	cnames := map[string]dnsname.Name{}
	for _, rr := range resp.GetRecords("CNAME", "answer") {
		if cname, ok := rr.(*dns.CNAME); ok {
			owner := dnsname.New(cname.Hdr.Name)
			target := dnsname.New(cname.Target)
			cnames[strings.ToLower(owner.String())] = target
		}
	}

	key := strings.ToLower(start.String())
	for {
		next, ok := cnames[key]
		if !ok {
			break
		}
		key = strings.ToLower(next.String())
	}
	return dnsname.New(key)
}

func queryPacket(ctx context.Context, ns nameserver.Nameserver, name string, qtype string) packet.Packet {
	resp, err := ns.Query(ctx, name, qtype)
	if err != nil {
		return packet.Packet{}
	}
	return resp
}

func uniqueAddrs(addrs []netip.Addr) []netip.Addr {
	seen := map[string]netip.Addr{}
	for _, addr := range addrs {
		seen[addr.String()] = addr
	}
	out := make([]netip.Addr, 0, len(seen))
	for _, addr := range seen {
		out = append(out, addr)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

func uniqueSortedItems(items []NSItem) []NSItem {
	seen := map[string]NSItem{}
	for _, item := range items {
		seen[item.String()] = item
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]NSItem, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func uniqueSortedNameservers(items []nameserver.Nameserver) []nameserver.Nameserver {
	seen := map[string]nameserver.Nameserver{}
	for _, item := range items {
		seen[strings.ToLower(item.String())] = item
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]nameserver.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func sortedNames(seen map[string]dnsname.Name) []dnsname.Name {
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]dnsname.Name, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func firstKey(m map[string][]nameserver.Nameserver) string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys[0]
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

func cloneNameservers(list []nameserver.Nameserver) []nameserver.Nameserver {
	if list == nil {
		return nil
	}
	out := make([]nameserver.Nameserver, len(list))
	copy(out, list)
	return out
}
