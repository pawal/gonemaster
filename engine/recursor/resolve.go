package recursor

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/parallel"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
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

	root, err := r.RootServers(ctx)
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
		if len(nameObj.Labels()) == 1 {
			if nextHigher, ok := nameObj.NextHigher(); ok {
				return nextHigher.String(), resp, nil
			}
		}
		return "", resp, nil
	}

	pname := state.trace[0].zoneName
	pnameObj := dnsname.New(pname)
	if strings.EqualFold(pnameObj.String(), nameObj.String()) {
		if len(state.trace) > 1 {
			pname = state.trace[1].zoneName
			pnameObj = dnsname.New(pname)
		} else if nextHigher, ok := nameObj.NextHigher(); ok {
			// Keep parity with Zonemaster behavior: when the trace only
			// contains the child zone itself, fall back to next higher.
			pname = nextHigher.String()
			pnameObj = nextHigher
		}
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
	state.ensureLock()
	state.lock()
	if state.inProgress == nil {
		state.inProgress = map[string]map[string]bool{}
	}
	if state.glue == nil {
		state.glue = map[string]map[netip.Addr]bool{}
	}
	state.unlock()

	root, err := r.RootServers(ctx)
	if err != nil {
		return nil, err
	}
	buildQueryers := func() []queryer {
		queryers := make([]queryer, 0, len(root))
		for _, server := range root {
			queryers = append(queryers, server)
		}
		return queryers
	}

	pa := packet.Packet{}
	paaaa := packet.Packet{}

	parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
	if parallelism < 1 {
		parallelism = 1
	}
	if profile.FromContext(ctx).Resolver.Defaults.Unordered || isUnorderedContext(ctx) {
		parallelism = 1
	}
	if parallelism == 1 {
		pa, _, err = r.recurse(ctx, name, "A", "IN", &recurseState{
			ns:         buildQueryers(),
			count:      state.count,
			common:     0,
			seen:       map[string]bool{},
			inProgress: state.inProgress,
			glue:       state.glue,
			mu:         state.mu,
		})
		if err != nil {
			return nil, err
		}
		if pa.NoSuchName() {
			return nil, nil
		}
		paaaa, _, err = r.recurse(ctx, name, "AAAA", "IN", &recurseState{
			ns:         buildQueryers(),
			count:      state.count,
			common:     0,
			seen:       map[string]bool{},
			inProgress: state.inProgress,
			glue:       state.glue,
			mu:         state.mu,
		})
		if err != nil {
			return nil, err
		}
	} else {
		type addrResult struct {
			resp  packet.Packet
			state *recurseState
		}

		cloneInProgress := func(src map[string]map[string]bool) map[string]map[string]bool {
			if src == nil {
				return map[string]map[string]bool{}
			}
			dst := make(map[string]map[string]bool, len(src))
			for key, inner := range src {
				copied := make(map[string]bool, len(inner))
				for innerKey, value := range inner {
					copied[innerKey] = value
				}
				dst[key] = copied
			}
			return dst
		}
		cloneGlue := func(src map[string]map[netip.Addr]bool) map[string]map[netip.Addr]bool {
			if src == nil {
				return map[string]map[netip.Addr]bool{}
			}
			dst := make(map[string]map[netip.Addr]bool, len(src))
			for key, inner := range src {
				copied := make(map[netip.Addr]bool, len(inner))
				for innerKey, value := range inner {
					copied[innerKey] = value
				}
				dst[key] = copied
			}
			return dst
		}
		mergeInProgress := func(dst map[string]map[string]bool, src map[string]map[string]bool) {
			if dst == nil || src == nil {
				return
			}
			for key, inner := range src {
				if dst[key] == nil {
					dst[key] = map[string]bool{}
				}
				for innerKey := range inner {
					dst[key][innerKey] = true
				}
			}
		}
		mergeGlue := func(dst map[string]map[netip.Addr]bool, src map[string]map[netip.Addr]bool) {
			if dst == nil || src == nil {
				return
			}
			for key, inner := range src {
				if dst[key] == nil {
					dst[key] = map[netip.Addr]bool{}
				}
				for innerKey := range inner {
					dst[key][innerKey] = true
				}
			}
		}

		state.lock()
		baseInProgress := cloneInProgress(state.inProgress)
		baseGlue := cloneGlue(state.glue)
		state.unlock()

		tasks := []parallel.Task[addrResult]{
			func(ctx context.Context) (addrResult, error) {
				resp, nextState, err := r.recurse(ctx, name, "A", "IN", &recurseState{
					ns:         buildQueryers(),
					count:      state.count,
					common:     0,
					seen:       map[string]bool{},
					inProgress: cloneInProgress(baseInProgress),
					glue:       cloneGlue(baseGlue),
				})
				return addrResult{resp: resp, state: nextState}, err
			},
			func(ctx context.Context) (addrResult, error) {
				resp, nextState, err := r.recurse(ctx, name, "AAAA", "IN", &recurseState{
					ns:         buildQueryers(),
					count:      state.count,
					common:     0,
					seen:       map[string]bool{},
					inProgress: cloneInProgress(baseInProgress),
					glue:       cloneGlue(baseGlue),
				})
				return addrResult{resp: resp, state: nextState}, err
			},
		}

		results := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: 2, CancelOnError: false})

		if results[0].Value.state != nil {
			state.lock()
			mergeInProgress(state.inProgress, results[0].Value.state.inProgress)
			mergeGlue(state.glue, results[0].Value.state.glue)
			state.unlock()
		}
		if results[0].Err != nil {
			return nil, results[0].Err
		}
		pa = results[0].Value.resp
		if pa.NoSuchName() {
			return nil, nil
		}

		if results[1].Value.state != nil {
			state.lock()
			mergeInProgress(state.inProgress, results[1].Value.state.inProgress)
			mergeGlue(state.glue, results[1].Value.state.glue)
			state.unlock()
		}
		if results[1].Err != nil {
			return nil, results[1].Err
		}
		paaaa = results[1].Value.resp
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
		root, err := r.RootServers(ctx)
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

func (r *Recursor) getNSFrom(ctx context.Context, resp packet.Packet, state *recurseState) ([]queryer, error) {
	nsRecords := resp.GetRecords("NS")
	if len(nsRecords) == 0 {
		return nil, nil
	}

	if state == nil {
		state = &recurseState{}
	}
	state.ensureLock()
	state.lock()
	if state.glue == nil {
		state.glue = map[string]map[netip.Addr]bool{}
	}
	state.unlock()

	var names []string
	for _, rr := range nsRecords {
		nsRR, ok := rr.(*dns.NS)
		if !ok {
			continue
		}
		nsName := dnsname.New(nsRR.Ns)
		names = append(names, nsName.String())
	}

	state.lock()
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
	state.unlock()

	var withGlue []nameserver.Nameserver
	var extra []string
	glueSnapshot := make(map[string][]netip.Addr, len(names))
	state.lock()
	for _, name := range names {
		nameObj := dnsname.New(name)
		key := strings.ToLower(nameObj.String())
		if addrs, ok := state.glue[key]; ok && len(addrs) > 0 {
			ordered := make([]netip.Addr, 0, len(addrs))
			for addr := range addrs {
				ordered = append(ordered, addr)
			}
			glueSnapshot[key] = ordered
		}
	}
	state.unlock()

	for _, name := range names {
		nameObj := dnsname.New(name)
		key := strings.ToLower(nameObj.String())
		if addrs, ok := glueSnapshot[key]; ok && len(addrs) > 0 {
			for _, addr := range addrs {
				ns, err := nameserver.NewWithContext(ctx, name, addr.String(), r.client)
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

	parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
	if parallelism < 1 {
		parallelism = 1
	}
	if profile.FromContext(ctx).Resolver.Defaults.Unordered || isUnorderedContext(ctx) {
		parallelism = 1
	}
	queryAddresses := func(addrs []netip.Addr) (packet.Packet, error) {
		if len(addrs) == 0 {
			return packet.Packet{}, nil
		}
		if parallelism <= 1 || len(addrs) == 1 {
			for _, addr := range addrs {
				ns, err := nameserver.NewWithContext(ctx, l.name, addr.String(), l.recursor.client)
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

		tasks := make([]parallel.Task[packet.Packet], len(addrs))
		for i, addr := range addrs {
			addr := addr
			tasks[i] = func(ctx context.Context) (packet.Packet, error) {
				ns, err := nameserver.NewWithContext(ctx, l.name, addr.String(), l.recursor.client)
				if err != nil {
					return packet.Packet{}, err
				}
				return ns.QueryWithClass(ctx, qname, qtype, qclass)
			}
		}

		results := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: parallelism, CancelOnError: false})
		for _, res := range results {
			if res.Err == nil && res.Value.Msg != nil {
				return res.Value, nil
			}
		}
		return packet.Packet{}, nil
	}

	var cachedAddrs []netip.Addr
	if l.state != nil {
		l.state.ensureLock()
		l.state.lock()
		if l.state.glue == nil {
			l.state.glue = map[string]map[netip.Addr]bool{}
		}
		if addrs, ok := l.state.glue[nameKey]; ok {
			if len(addrs) > 0 {
				cachedAddrs = make([]netip.Addr, 0, len(addrs))
				for addr := range addrs {
					cachedAddrs = append(cachedAddrs, addr)
				}
			}
		} else {
			l.state.glue[nameKey] = map[netip.Addr]bool{}
		}
		l.state.unlock()
	}
	if len(cachedAddrs) > 0 {
		return queryAddresses(cachedAddrs)
	}

	addrs, err := l.recursor.getAddressesFor(ctx, l.name, l.state)
	if err != nil {
		return packet.Packet{}, err
	}
	if l.state != nil {
		l.state.ensureLock()
		l.state.lock()
		if l.state.glue == nil {
			l.state.glue = map[string]map[netip.Addr]bool{}
		}
		if l.state.glue[nameKey] == nil {
			l.state.glue[nameKey] = map[netip.Addr]bool{}
		}
		for _, addr := range addrs {
			l.state.glue[nameKey][addr] = true
		}
		l.state.unlock()
	}
	return queryAddresses(addrs)
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
