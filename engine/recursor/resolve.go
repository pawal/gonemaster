package recursor

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/dnsname"
	"github.com/pawal/gonemaster/engine/nameserver"
	"github.com/pawal/gonemaster/engine/packet"
)

// Recurse performs a recursive lookup using root servers.
func (r *Recursor) Recurse(ctx context.Context, name string, qtype string, qclass string) (packet.Packet, error) {
	return r.recurseWithNameservers(ctx, name, qtype, qclass, nil)
}

// RecurseWithNameservers performs a recursive lookup using a specific nameserver set.
func (r *Recursor) RecurseWithNameservers(ctx context.Context, name string, qtype string, qclass string, ns []nameserver.Nameserver) (packet.Packet, error) {
	return r.recurseWithNameservers(ctx, name, qtype, qclass, ns)
}

// Parent resolves the parent zone name for a domain.
func (r *Recursor) Parent(ctx context.Context, name string) (string, packet.Packet, error) {
	nameObj := dnsname.New(name)
	if nameObj.String() == "." {
		return ".", packet.Packet{}, nil
	}

	root, err := r.RootServers()
	if err != nil {
		return "", packet.Packet{}, err
	}

	queryers := make([]queryer, 0, len(root))
	for _, server := range root {
		queryers = append(queryers, server)
	}

	state := &recurseState{
		ns: queryers,
	}

	resp, state, err := r.recurse(ctx, nameObj.String(), "SOA", "IN", state)
	if err != nil {
		return "", resp, err
	}
	if len(state.trace) == 0 {
		return "", resp, nil
	}

	pname := state.trace[0].zoneName
	pnameObj := dnsname.New(pname)
	if strings.EqualFold(pnameObj.String(), nameObj.String()) && len(state.trace) > 1 {
		pname = state.trace[1].zoneName
		pnameObj = dnsname.New(pname)
	}

	if nextHigher, ok := nameObj.NextHigher(); ok {
		if !strings.EqualFold(nextHigher.String(), pnameObj.String()) {
			entry := state.trace[0]
			if entry.source != nil {
				pp, err := entry.source.QueryWithClass(ctx, nextHigher.String(), "SOA", "IN")
				if err == nil && pp.Msg != nil {
					if soa := firstSOAOwner(pp); soa != "" {
						pname = soa
					}
				}
			}
		}
	}

	return pname, resp, nil
}

// GetAddressesFor resolves A and AAAA addresses for a nameserver name.
func (r *Recursor) GetAddressesFor(ctx context.Context, name string) ([]netip.Addr, error) {
	return r.getAddressesFor(ctx, name, nil)
}

func (r *Recursor) getAddressesFor(ctx context.Context, name string, state *recurseState) ([]netip.Addr, error) {
	if state == nil {
		state = &recurseState{}
	}
	if state.inProgress == nil {
		state.inProgress = map[string]map[string]bool{}
	}
	if state.glue == nil {
		state.glue = map[string]map[netip.Addr]bool{}
	}

	root, err := r.RootServers()
	if err != nil {
		return nil, err
	}
	queryers := make([]queryer, 0, len(root))
	for _, server := range root {
		queryers = append(queryers, server)
	}

	pa, _, err := r.recurse(ctx, name, "A", "IN", &recurseState{
		ns:         queryers,
		count:      state.count,
		common:     0,
		seen:       map[string]bool{},
		inProgress: state.inProgress,
		glue:       state.glue,
	})
	if err != nil {
		return nil, err
	}
	if pa.NoSuchName() {
		return nil, nil
	}

	queryers = make([]queryer, 0, len(root))
	for _, server := range root {
		queryers = append(queryers, server)
	}
	paaaa, _, err := r.recurse(ctx, name, "AAAA", "IN", &recurseState{
		ns:         queryers,
		count:      state.count,
		common:     0,
		seen:       map[string]bool{},
		inProgress: state.inProgress,
		glue:       state.glue,
	})
	if err != nil {
		return nil, err
	}

	var res []netip.Addr
	target := dnsname.New(name)
	cnames := map[string]bool{}
	collectCNAMEs(pa, target, cnames)
	collectCNAMEs(paaaa, target, cnames)

	res = append(res, collectAddresses(pa, target, cnames)...)
	res = append(res, collectAddresses(paaaa, target, cnames)...)

	sort.Slice(res, func(i, j int) bool {
		return res[i].String() < res[j].String()
	})
	return res, nil
}

// ClearCache clears the recursive cache.
func (r *Recursor) ClearCache() {
	r.cacheMu.Lock()
	r.recurseCache = map[string]map[string]map[string]*packet.Packet{}
	r.cacheMu.Unlock()
}

func (r *Recursor) recurseWithNameservers(ctx context.Context, name string, qtype string, qclass string, ns []nameserver.Nameserver) (packet.Packet, error) {
	if qtype == "" {
		qtype = "A"
	}
	if qclass == "" {
		qclass = "IN"
	}
	qtype = strings.ToUpper(qtype)
	qclass = strings.ToUpper(qclass)

	nameObj := dnsname.New(name)
	key := strings.ToLower(nameObj.String())
	if cached, ok := r.cacheLookup(key, qtype, qclass); ok {
		return cached, nil
	}

	if ns == nil {
		root, err := r.RootServers()
		if err != nil {
			return packet.Packet{}, err
		}
		ns = root
	}

	queryers := make([]queryer, 0, len(ns))
	for _, server := range ns {
		queryers = append(queryers, server)
	}

	state := &recurseState{
		ns: queryers,
	}

	resp, _, err := r.recurse(ctx, name, qtype, qclass, state)
	if err != nil {
		return packet.Packet{}, err
	}
	r.cacheStore(key, qtype, qclass, resp)
	return resp, nil
}

func (r *Recursor) cacheLookup(name string, qtype string, qclass string) (packet.Packet, bool) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.recurseCache == nil {
		return packet.Packet{}, false
	}
	if byType, ok := r.recurseCache[name]; ok {
		if byClass, ok := byType[qtype]; ok {
			if cached, ok := byClass[qclass]; ok {
				if cached == nil {
					return packet.Packet{}, true
				}
				return *cached, true
			}
		}
	}
	return packet.Packet{}, false
}

func (r *Recursor) cacheStore(name string, qtype string, qclass string, resp packet.Packet) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.recurseCache == nil {
		r.recurseCache = map[string]map[string]map[string]*packet.Packet{}
	}
	if r.recurseCache[name] == nil {
		r.recurseCache[name] = map[string]map[string]*packet.Packet{}
	}
	if r.recurseCache[name][qtype] == nil {
		r.recurseCache[name][qtype] = map[string]*packet.Packet{}
	}

	if resp.Msg == nil {
		r.recurseCache[name][qtype][qclass] = nil
		return
	}

	copyResp := resp
	r.recurseCache[name][qtype][qclass] = &copyResp
}

func (r *Recursor) getNSFrom(resp packet.Packet, state *recurseState) ([]queryer, error) {
	nsRecords := resp.GetRecords("NS")
	if len(nsRecords) == 0 {
		return nil, nil
	}

	if state == nil {
		state = &recurseState{}
	}
	if state.glue == nil {
		state.glue = map[string]map[netip.Addr]bool{}
	}

	var names []string
	for _, rr := range nsRecords {
		nsRR, ok := rr.(*dns.NS)
		if !ok {
			continue
		}
		nsName := dnsname.New(nsRR.Ns)
		names = append(names, nsName.String())
	}

	for _, rr := range resp.GetRecords("A") {
		if a, ok := rr.(*dns.A); ok {
			ownerName := dnsname.New(rr.Header().Name)
			owner := strings.ToLower(ownerName.String())
			if addr, err := netip.ParseAddr(a.A.String()); err == nil {
				if state.glue[owner] == nil {
					state.glue[owner] = map[netip.Addr]bool{}
				}
				state.glue[owner][addr] = true
			}
		}
	}
	for _, rr := range resp.GetRecords("AAAA") {
		if aaaa, ok := rr.(*dns.AAAA); ok {
			ownerName := dnsname.New(rr.Header().Name)
			owner := strings.ToLower(ownerName.String())
			if addr, err := netip.ParseAddr(aaaa.AAAA.String()); err == nil {
				if state.glue[owner] == nil {
					state.glue[owner] = map[netip.Addr]bool{}
				}
				state.glue[owner][addr] = true
			}
		}
	}

	var withGlue []nameserver.Nameserver
	var extra []string
	for _, name := range names {
		nameObj := dnsname.New(name)
		key := strings.ToLower(nameObj.String())
		if addrs, ok := state.glue[key]; ok {
			for addr := range addrs {
				ns, err := nameserver.New(name, addr.String(), r.client)
				if err != nil {
					return nil, fmt.Errorf("create nameserver for %s: %w", name, err)
				}
				withGlue = append(withGlue, ns)
			}
		} else {
			extra = append(extra, name)
		}
	}

	sort.Slice(withGlue, func(i, j int) bool {
		left := strings.ToLower(withGlue[i].Name.String())
		right := strings.ToLower(withGlue[j].Name.String())
		if left == right {
			return withGlue[i].Address.String() < withGlue[j].Address.String()
		}
		return left < right
	})
	sort.Strings(extra)

	out := make([]queryer, 0, len(withGlue)+len(extra))
	for _, ns := range withGlue {
		out = append(out, ns)
	}
	for _, name := range extra {
		out = append(out, lazyNameserver{name: name, recursor: r, state: state})
	}
	return out, nil
}

type lazyNameserver struct {
	name     string
	recursor *Recursor
	state    *recurseState
}

func (l lazyNameserver) QueryWithClass(ctx context.Context, qname string, qtype string, qclass string) (packet.Packet, error) {
	if l.recursor == nil {
		return packet.Packet{}, fmt.Errorf("missing recursor for %s", l.name)
	}
	nameObj := dnsname.New(l.name)
	nameKey := strings.ToLower(nameObj.String())
	if l.state != nil && l.state.glue != nil {
		if addrs, ok := l.state.glue[nameKey]; ok {
			if len(addrs) > 0 {
				for addr := range addrs {
					ns, err := nameserver.New(l.name, addr.String(), l.recursor.client)
					if err != nil {
						continue
					}
					resp, err := ns.QueryWithClass(ctx, qname, qtype, qclass)
					if err == nil && resp.Msg != nil {
						return resp, nil
					}
				}
				return packet.Packet{}, nil
			}
		} else {
			l.state.glue[nameKey] = map[netip.Addr]bool{}
		}
	}

	addrs, err := l.recursor.getAddressesFor(ctx, l.name, l.state)
	if err != nil {
		return packet.Packet{}, err
	}
	if l.state != nil && l.state.glue != nil {
		if l.state.glue[nameKey] == nil {
			l.state.glue[nameKey] = map[netip.Addr]bool{}
		}
		for _, addr := range addrs {
			l.state.glue[nameKey][addr] = true
		}
	}
	for _, addr := range addrs {
		ns, err := nameserver.New(l.name, addr.String(), l.recursor.client)
		if err != nil {
			continue
		}
		resp, err := ns.QueryWithClass(ctx, qname, qtype, qclass)
		if err == nil && resp.Msg != nil {
			return resp, nil
		}
	}
	return packet.Packet{}, nil
}

func collectCNAMEs(resp packet.Packet, target dnsname.Name, out map[string]bool) {
	if resp.Msg == nil {
		return
	}
	for _, rr := range resp.GetRecordsForName("CNAME", target) {
		if cname, ok := rr.(*dns.CNAME); ok {
			targetName := dnsname.New(cname.Target)
			key := strings.ToLower(targetName.String())
			out[key] = true
		}
	}
}

func collectAddresses(resp packet.Packet, target dnsname.Name, cnames map[string]bool) []netip.Addr {
	if resp.Msg == nil {
		return nil
	}

	var out []netip.Addr
	targetKey := strings.ToLower(target.String())
	for _, rr := range resp.GetRecords("A") {
		if a, ok := rr.(*dns.A); ok {
			ownerName := dnsname.New(rr.Header().Name)
			owner := strings.ToLower(ownerName.String())
			if owner == targetKey || cnames[owner] {
				if addr, err := netip.ParseAddr(a.A.String()); err == nil {
					out = append(out, addr)
				}
			}
		}
	}
	for _, rr := range resp.GetRecords("AAAA") {
		if aaaa, ok := rr.(*dns.AAAA); ok {
			ownerName := dnsname.New(rr.Header().Name)
			owner := strings.ToLower(ownerName.String())
			if owner == targetKey || cnames[owner] {
				if addr, err := netip.ParseAddr(aaaa.AAAA.String()); err == nil {
					out = append(out, addr)
				}
			}
		}
	}
	return out
}

func firstSOAOwner(resp packet.Packet) string {
	records := resp.GetRecords("SOA", "answer")
	if len(records) == 0 {
		return ""
	}
	owner := records[0].Header().Name
	ownerName := dnsname.New(owner)
	return ownerName.String()
}
