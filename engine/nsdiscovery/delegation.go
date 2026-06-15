package nsdiscovery

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// NSItem is a nameserver as reported by parent delegation or zone apex.
// It captures the parent-queried distinction that a plain nameserver type
// cannot: a name may be known without yet having a resolved address (for
// example, an out-of-bailiwick NS before recursion completes).
type NSItem struct {
	// Name is the canonical nameserver host name (lowercased).
	Name dnsname.Name
	// Address is the nameserver IP address when available.
	Address netip.Addr
	// HasAddress reports whether Address has been resolved.
	HasAddress bool
	// Err is the typed resolution failure (e.g. *recursor.CNAMEError), if any.
	Err error
}

// String returns a stable "name" or "name/address" representation suitable
// for sorting and deduplication.
func (n NSItem) String() string {
	if n.HasAddress {
		return n.Name.String() + "/" + n.Address.String()
	}
	return n.Name.String()
}

// DelegationNameservers returns the parent-queried delegation view of the
// zone's nameservers: NS records from the parent, paired with addresses
// from in-bailiwick glue (if present) and out-of-bailiwick recursive
// resolution (for non-bailiwick names). Result is sorted and deduplicated.
//
// Errors are returned if the zone is nil or if the underlying parent
// chain walk / OOB resolution fails. An unreachable delegation chain
// returns an empty slice with a nil error - callers must check len().
func DelegationNameservers(ctx context.Context, z *zone.Zone) ([]NSItem, error) {
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

// delNSNames returns delegation names for the zone.
func delNSNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	items, err := DelegationNameservers(ctx, z)
	if err != nil || items == nil {
		return nil, err
	}

	seen := map[string]dnsname.Name{}
	for _, item := range items {
		seen[strings.ToLower(item.Name.String())] = item.Name
	}
	return sortedNames(seen), nil
}

// zoneNSNames returns authoritative nameserver names from the zone apex.
func zoneNSNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	if r := z.Recursor(); r != nil && z.Name.String() != "." && r.HasFakeAddresses(z.Name.String()) {
		return delNSNames(ctx, z)
	}

	items, err := DelegationNameservers(ctx, z)
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

// ZoneNameservers returns the authoritative apex view of the zone's
// nameservers. It queries the delegation servers for the apex NS RRset,
// keeps only responses with AA set, and pairs the resulting names with
// addresses via in-bailiwick (apex query) and out-of-bailiwick
// (recursive resolution) paths. Result is sorted and deduplicated.
//
// Differs from [DelegationNameservers] in that the names come from the
// child zone's own apex NS RRset, not the parent's delegation - useful
// for detecting lame delegations where parent and child disagree.
func ZoneNameservers(ctx context.Context, z *zone.Zone) ([]NSItem, error) {
	nsNames, err := zoneNSNames(ctx, z)
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

	parentNS, err := parentNSIPs(ctx, z)
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
					if err := recursor.IgnoreCNAMEError(err); err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
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
		var cnameErr error
		for _, qtype := range []string{"A", "AAAA"} {
			resp, err := r.Recurse(ctx, nsName.String(), qtype, "IN")
			// Keep the typed CNAME failure for the address-less item below.
			if err != nil && recursor.IgnoreCNAMEError(err) == nil {
				cnameErr = err
			}
			if err := recursor.IgnoreCNAMEError(err); err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
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
			out = append(out, NSItem{Name: nsName, Err: cnameErr})
		}
	}

	return uniqueSortedItems(out), nil
}

func getIBAddrInZone(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	delItems, err := DelegationNameservers(ctx, z)
	if err != nil {
		return nil, err
	}
	nsNames, err := zoneNSNames(ctx, z)
	if err != nil {
		return nil, err
	}
	if delItems == nil && nsNames == nil {
		return nil, nil
	}

	hasInBailiwick := slices.ContainsFunc(nsNames, z.Name.IsInBailiwick)
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

		keys := slices.Sorted(maps.Keys(seen))
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
				if err := recursor.IgnoreCNAMEError(err); err != nil || resp.Msg == nil {
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

	keys := slices.Sorted(maps.Keys(seen))

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
