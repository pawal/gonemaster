package recursor

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"sort"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/parallel"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

var recurseCacheMaxEntries = 10000

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
			// When the trace only contains the child zone itself, fall
			// back to next higher.
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

func cloneInProgressMap(src map[string]map[string]bool) map[string]map[string]bool {
	if src == nil {
		return map[string]map[string]bool{}
	}
	dst := make(map[string]map[string]bool, len(src))
	for key, inner := range src {
		copied := make(map[string]bool, len(inner))
		maps.Copy(copied, inner)
		dst[key] = copied
	}
	return dst
}

func cloneGlueMap(src map[string]map[netip.Addr]bool) map[string]map[netip.Addr]bool {
	if src == nil {
		return map[string]map[netip.Addr]bool{}
	}
	dst := make(map[string]map[netip.Addr]bool, len(src))
	for key, inner := range src {
		copied := make(map[netip.Addr]bool, len(inner))
		maps.Copy(copied, inner)
		dst[key] = copied
	}
	return dst
}

func mergeInProgressMaps(dst map[string]map[string]bool, src map[string]map[string]bool) {
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

func mergeGlueMaps(dst map[string]map[netip.Addr]bool, src map[string]map[netip.Addr]bool) {
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

func snapshotStateMaps(state *recurseState) (map[string]map[string]bool, map[string]map[netip.Addr]bool) {
	if state == nil {
		return map[string]map[string]bool{}, map[string]map[netip.Addr]bool{}
	}
	state.ensureLock()
	state.lock()
	inProgress := cloneInProgressMap(state.inProgress)
	glue := cloneGlueMap(state.glue)
	state.unlock()
	return inProgress, glue
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

	var pa, paaaa packet.Packet

	parallelism := max(profile.FromContext(ctx).Resolver.Defaults.Parallel, 1)
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
		baseInProgress, baseGlue := snapshotStateMaps(state)

		tasks := []parallel.Task[addrResult]{
			func(ctx context.Context) (addrResult, error) {
				resp, nextState, err := r.recurse(ctx, name, "A", "IN", &recurseState{
					ns:         buildQueryers(),
					count:      state.count,
					common:     0,
					seen:       map[string]bool{},
					inProgress: cloneInProgressMap(baseInProgress),
					glue:       cloneGlueMap(baseGlue),
				})
				return addrResult{resp: resp, state: nextState}, err
			},
			func(ctx context.Context) (addrResult, error) {
				resp, nextState, err := r.recurse(ctx, name, "AAAA", "IN", &recurseState{
					ns:         buildQueryers(),
					count:      state.count,
					common:     0,
					seen:       map[string]bool{},
					inProgress: cloneInProgressMap(baseInProgress),
					glue:       cloneGlueMap(baseGlue),
				})
				return addrResult{resp: resp, state: nextState}, err
			},
		}

		results := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: 2, CancelOnError: false})

		if results[0].Value.state != nil {
			srcInProgress, srcGlue := snapshotStateMaps(results[0].Value.state)
			state.lock()
			mergeInProgressMaps(state.inProgress, srcInProgress)
			mergeGlueMaps(state.glue, srcGlue)
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
			srcInProgress, srcGlue := snapshotStateMaps(results[1].Value.state)
			state.lock()
			mergeInProgressMaps(state.inProgress, srcInProgress)
			mergeGlueMaps(state.glue, srcGlue)
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
	r.recurseCache = map[string]map[string]map[string]*recurseCacheEntry{}
	r.recurseCount = 0
	r.cacheMu.Unlock()
}

func (r *Recursor) recurseWithNameservers(ctx context.Context, name string, qtype string, qclass string, ns []nameserver.Nameserver) (resp packet.Packet, err error) {
	if qtype == "" {
		qtype = "A"
	}
	if qclass == "" {
		qclass = "IN"
	}
	qtype = strings.ToUpper(qtype)
	qclass = strings.ToUpper(qclass)

	nameObj := dnsname.New(name)
	key := cacheNameKey(nameObj, ns)
	runLog := logger.FromContext(ctx)
	if cached, ok := r.cacheLookup(key, qtype, qclass); ok {
		cached.Log = runLog
		return cached, nil
	}
	if cached, cachedOK, inflight, wait := r.cacheLookupOrWaitOrRegister(key, qtype, qclass); wait {
		if ctx == nil {
			<-inflight.done
			if inflight.resp == nil {
				return packet.Packet{}, inflight.err
			}
			copyResp := *inflight.resp
			copyResp.Log = runLog
			return copyResp, inflight.err
		}
		select {
		case <-inflight.done:
			if inflight.resp == nil {
				return packet.Packet{}, inflight.err
			}
			copyResp := *inflight.resp
			copyResp.Log = runLog
			return copyResp, inflight.err
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	} else if cachedOK {
		cached.Log = runLog
		return cached, nil
	}
	defer func() {
		var infResp *packet.Packet
		if err == nil {
			copyResp := resp
			infResp = &copyResp
		}
		r.finishInflightLookup(key, qtype, qclass, infResp, err)
	}()

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

	resp, _, err = r.recurse(ctx, name, qtype, qclass, state)
	if err != nil {
		if ctx == nil || ctx.Err() == nil {
			r.cacheStoreNegative(key, qtype, qclass)
		}
		return packet.Packet{}, err
	}
	if resp.Msg == nil {
		if ctx == nil || ctx.Err() == nil {
			r.cacheStoreNegative(key, qtype, qclass)
		}
		resp.Log = runLog
		return resp, nil
	}
	cached := resp
	cached.Log = nil
	r.cacheStore(key, qtype, qclass, cached)
	resp.Log = runLog
	return resp, nil
}

type inflightLookup struct {
	done chan struct{}
	resp *packet.Packet
	err  error
}

func cacheNameKey(name dnsname.Name, ns []nameserver.Nameserver) string {
	if len(ns) == 0 {
		return "root|" + name.String()
	}
	parts := make([]string, 0, len(ns))
	for _, server := range ns {
		parts = append(parts, strings.ToLower(server.Name.String())+"@"+server.Address.String())
	}
	sort.Strings(parts)
	return "ns|" + strings.Join(parts, ",") + "|" + name.String()
}

func recurseLookupKey(name string, qtype string, qclass string) string {
	return name + "|" + qtype + "|" + qclass
}

func (r *Recursor) cacheLookupLocked(name string, qtype string, qclass string) (packet.Packet, bool) {
	if r.recurseCache == nil {
		return packet.Packet{}, false
	}
	byType, ok := r.recurseCache[name]
	if !ok {
		return packet.Packet{}, false
	}
	byClass, ok := byType[qtype]
	if !ok {
		return packet.Packet{}, false
	}
	entry, ok := byClass[qclass]
	if !ok {
		return packet.Packet{}, false
	}
	if entry == nil {
		r.evictCacheEntryLocked(name, qtype, qclass)
		return packet.Packet{}, false
	}
	if !entry.expires.IsZero() && !time.Now().Before(entry.expires) {
		r.evictCacheEntryLocked(name, qtype, qclass)
		return packet.Packet{}, false
	}
	if entry.resp == nil {
		// Negative cache hit: return an empty packet but signal hit=true so
		// callers skip re-issuing the lookup.
		return packet.Packet{}, true
	}
	return *entry.resp, true
}

func (r *Recursor) evictCacheEntryLocked(name string, qtype string, qclass string) {
	byType, ok := r.recurseCache[name]
	if !ok {
		return
	}
	byClass, ok := byType[qtype]
	if !ok {
		return
	}
	if _, ok := byClass[qclass]; ok {
		delete(byClass, qclass)
		if r.recurseCount > 0 {
			r.recurseCount--
		}
	}
	if len(byClass) == 0 {
		delete(byType, qtype)
	}
	if len(byType) == 0 {
		delete(r.recurseCache, name)
	}
}

func (r *Recursor) cacheLookupOrWaitOrRegister(name string, qtype string, qclass string) (packet.Packet, bool, *inflightLookup, bool) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if cached, ok := r.cacheLookupLocked(name, qtype, qclass); ok {
		return cached, true, nil, false
	}
	if r.inflight == nil {
		r.inflight = map[string]*inflightLookup{}
	}
	key := recurseLookupKey(name, qtype, qclass)
	if inflight, ok := r.inflight[key]; ok {
		return packet.Packet{}, false, inflight, true
	}
	r.inflight[key] = &inflightLookup{done: make(chan struct{})}
	return packet.Packet{}, false, nil, false
}

func (r *Recursor) finishInflightLookup(name string, qtype string, qclass string, resp *packet.Packet, err error) {
	r.cacheMu.Lock()
	if r.inflight != nil {
		key := recurseLookupKey(name, qtype, qclass)
		if inflight := r.inflight[key]; inflight != nil {
			inflight.resp = resp
			inflight.err = err
			close(inflight.done)
			delete(r.inflight, key)
		}
	}
	r.cacheMu.Unlock()
}

func (r *Recursor) cacheLookup(name string, qtype string, qclass string) (packet.Packet, bool) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	return r.cacheLookupLocked(name, qtype, qclass)
}

func (r *Recursor) cacheStore(name string, qtype string, qclass string, resp packet.Packet) {
	if resp.Msg == nil {
		// Indeterminate lookups are stored separately via cacheStoreNegative.
		return
	}
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	copyResp := resp
	r.storeEntryLocked(name, qtype, qclass, &recurseCacheEntry{resp: &copyResp})
}

// cacheStoreNegative stores a "no answer" entry for the supplied lookup,
// expiring after r.negativeCacheTTL. When the TTL is zero (default) no
// entry is stored, preserving the historical "do not cache indeterminate
// lookups" behavior.
func (r *Recursor) cacheStoreNegative(name string, qtype string, qclass string) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if r.negativeCacheTTL <= 0 {
		return
	}
	r.storeEntryLocked(name, qtype, qclass, &recurseCacheEntry{
		expires: time.Now().Add(r.negativeCacheTTL),
	})
}

func (r *Recursor) storeEntryLocked(name string, qtype string, qclass string, entry *recurseCacheEntry) {
	if r.recurseCache == nil {
		r.recurseCache = map[string]map[string]map[string]*recurseCacheEntry{}
	}
	if r.recurseCache[name] == nil {
		r.recurseCache[name] = map[string]map[string]*recurseCacheEntry{}
	}
	if r.recurseCache[name][qtype] == nil {
		r.recurseCache[name][qtype] = map[string]*recurseCacheEntry{}
	}
	if _, exists := r.recurseCache[name][qtype][qclass]; !exists {
		r.recurseCount++
	}
	if recurseCacheMaxEntries > 0 && r.recurseCount > recurseCacheMaxEntries {
		// Keep cache bounded for long-running processes.
		r.recurseCache = map[string]map[string]map[string]*recurseCacheEntry{}
		r.recurseCount = 1
		r.recurseCache[name] = map[string]map[string]*recurseCacheEntry{
			qtype: {
				qclass: entry,
			},
		}
		return
	}
	r.recurseCache[name][qtype][qclass] = entry
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
	glueAllowed := map[string]bool{}
	for _, rr := range nsRecords {
		nsRR, ok := rr.(*dns.NS)
		if !ok {
			continue
		}
		nsName := dnsname.New(nsRR.Ns)
		names = append(names, nsName.String())
		zoneName := dnsname.New(nsRR.Hdr.Name)
		if zoneName.IsInBailiwick(nsName) {
			glueAllowed[strings.ToLower(nsName.String())] = true
		}
	}

	state.lock()
	for _, rr := range resp.GetRecords("A") {
		if a, ok := rr.(*dns.A); ok {
			ownerName := dnsname.New(rr.Header().Name)
			owner := strings.ToLower(ownerName.String())
			if !glueAllowed[owner] {
				continue
			}
			if a.Addr.IsValid() {
				if state.glue[owner] == nil {
					state.glue[owner] = map[netip.Addr]bool{}
				}
				state.glue[owner][a.Addr] = true
			}
		}
	}
	for _, rr := range resp.GetRecords("AAAA") {
		if aaaa, ok := rr.(*dns.AAAA); ok {
			ownerName := dnsname.New(rr.Header().Name)
			owner := strings.ToLower(ownerName.String())
			if !glueAllowed[owner] {
				continue
			}
			if aaaa.Addr.IsValid() {
				if state.glue[owner] == nil {
					state.glue[owner] = map[netip.Addr]bool{}
				}
				state.glue[owner][aaaa.Addr] = true
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

// QueryWithClass resolves addresses for the lazy nameserver and forwards the query.
func (l lazyNameserver) QueryWithClass(ctx context.Context, qname string, qtype string, qclass string) (packet.Packet, error) {
	if l.recursor == nil {
		return packet.Packet{}, fmt.Errorf("missing recursor for %s", l.name)
	}
	nameObj := dnsname.New(l.name)
	nameKey := strings.ToLower(nameObj.String())

	parallelism := max(profile.FromContext(ctx).Resolver.Defaults.Parallel, 1)
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
				if a.Addr.IsValid() {
					out = append(out, a.Addr)
				}
			}
		}
	}
	for _, rr := range resp.GetRecords("AAAA") {
		if aaaa, ok := rr.(*dns.AAAA); ok {
			ownerName := dnsname.New(rr.Header().Name)
			owner := strings.ToLower(ownerName.String())
			if owner == targetKey || cnames[owner] {
				if aaaa.Addr.IsValid() {
					out = append(out, aaaa.Addr)
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
