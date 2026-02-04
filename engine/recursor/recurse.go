package recursor

import (
	"context"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

type queryer interface {
	QueryWithClass(ctx context.Context, qname string, qtype string, qclass string) (packet.Packet, error)
}

type recurseState struct {
	mu         *sync.Mutex
	ns         []queryer
	count      int
	common     int
	seen       map[string]bool
	inProgress map[string]map[string]bool
	tseen      map[string]bool
	tcount     int
	qname      dnsname.Name
	qnameSet   bool
	candidate  packet.Packet
	nsFrom     func(context.Context, packet.Packet, *recurseState) ([]queryer, error)
	trace      []traceEntry
	glue       map[string]map[netip.Addr]bool
}

type traceEntry struct {
	zoneName   string
	source     queryer
	answerFrom string
}

func (state *recurseState) ensureLock() {
	if state == nil {
		return
	}
	if state.mu == nil {
		state.mu = &sync.Mutex{}
	}
}

func (state *recurseState) lock() {
	if state == nil || state.mu == nil {
		return
	}
	state.mu.Lock()
}

func (state *recurseState) unlock() {
	if state == nil || state.mu == nil {
		return
	}
	state.mu.Unlock()
}

func (r *Recursor) recurse(ctx context.Context, name string, qtype string, qclass string, state *recurseState) (packet.Packet, *recurseState, error) {
	if state == nil {
		state = &recurseState{}
	}
	state.ensureLock()
	if !state.qnameSet {
		state.qname = dnsname.New(name)
		state.qnameSet = true
	}
	if state.nsFrom == nil {
		state.nsFrom = r.getNSFrom
	}
	if state.seen == nil {
		state.seen = map[string]bool{}
	}
	if state.trace == nil {
		state.trace = []traceEntry{}
	}
	state.lock()
	if state.inProgress == nil {
		state.inProgress = map[string]map[string]bool{}
	}
	if state.tseen == nil {
		state.tseen = map[string]bool{}
	}
	if state.glue == nil {
		state.glue = map[string]map[netip.Addr]bool{}
	}
	state.unlock()

	if qtype == "" {
		qtype = "A"
	}
	if qclass == "" {
		qclass = "IN"
	}
	qtype = strings.ToUpper(qtype)
	qclass = strings.ToUpper(qclass)

	nameObj := dnsname.New(name)
	nameKey := strings.ToLower(nameObj.String())
	state.lock()
	if state.inProgress[nameKey] == nil {
		state.inProgress[nameKey] = map[string]bool{}
	}
	if state.inProgress[nameKey][qtype] {
		state.unlock()
		return packet.Packet{}, state, nil
	}
	state.inProgress[nameKey][qtype] = true
	state.unlock()

	if profile.FromContext(ctx).Resolver.Defaults.Unordered {
		return r.recurseUnordered(ctx, name, qtype, qclass, state)
	}
	return r.recurseOrdered(ctx, name, qtype, qclass, state)
}

func (r *Recursor) recurseOrdered(ctx context.Context, name string, qtype string, qclass string, state *recurseState) (packet.Packet, *recurseState, error) {
	nameObj := dnsname.New(name)
	for len(state.ns) > 0 {
		idx := len(state.ns) - 1
		ns := state.ns[idx]
		state.ns = state.ns[:idx]

		resp, err := ns.QueryWithClass(ctx, name, qtype, qclass)
		if err != nil || resp.Msg == nil {
			continue
		}

		if resp.Rcode() == "REFUSED" || resp.Rcode() == "SERVFAIL" {
			state.candidate = resp
			continue
		}

		if resp.NoSuchRecord() || resp.NoSuchName() {
			return resp, state, nil
		}

		if resp.Type() == "answer" {
			if !resp.HasRRsOfTypeForName(qtype, nameObj, "answer") && len(resp.GetRecordsForName("CNAME", nameObj, "answer")) > 0 {
				cnameResp, state, err := r.resolveCNAME(ctx, nameObj, qtype, qclass, resp, state)
				return cnameResp, state, err
			}
			return resp, state, nil
		}

		if resp.IsRedirect() {
			zname, ok := redirectName(resp)
			if !ok {
				continue
			}
			if zname == "." {
				continue
			}
			zkey := strings.ToLower(zname)
			if state.seen[zkey] {
				continue
			}
			state.seen[zkey] = true

			common := dnsname.New(zname).Common(state.qname)
			if common < state.common {
				continue
			}
			state.common = common

			next, err := state.nsFrom(ctx, resp, state)
			if err != nil {
				return packet.Packet{}, state, err
			}
			state.ns = next
			state.count++
			if state.count > 20 {
				return packet.Packet{}, state, nil
			}
			state.trace = append([]traceEntry{{
				zoneName:   zname,
				source:     ns,
				answerFrom: resp.AnswerFrom,
			}}, state.trace...)
		}
	}

	if state.candidate.Msg != nil {
		return state.candidate, state, nil
	}
	return packet.Packet{}, state, nil
}

type unorderedResult struct {
	ns   queryer
	resp packet.Packet
	err  error
}

func (r *Recursor) recurseUnordered(ctx context.Context, name string, qtype string, qclass string, state *recurseState) (packet.Packet, *recurseState, error) {
	depth := unorderedDepth(ctx)
	nameObj := dnsname.New(name)
	for len(state.ns) > 0 {
		nss := state.ns
		state.ns = nil

		if len(nss) == 0 {
			break
		}
		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		if parallelism < 1 {
			parallelism = 1
		}
		if depth > 0 {
			parallelism = 1
		}
		workers := parallelism
		if workers > len(nss) {
			workers = len(nss)
		}
		if workers < 1 {
			workers = 1
		}
		jobs := make(chan queryer)
		results := make(chan unorderedResult, len(nss))

		defaults := profile.FromContext(ctx).Resolver.Defaults
		batchTimeout := time.Duration(defaults.Timeout) * time.Second
		if defaults.Retry > 0 {
			batchTimeout = batchTimeout * time.Duration(defaults.Retry+1)
		}
		var ctxBatch context.Context
		var cancel context.CancelFunc
		if batchTimeout > 0 {
			ctxBatch, cancel = context.WithTimeout(ctx, batchTimeout)
		} else {
			ctxBatch, cancel = context.WithCancel(ctx)
		}
		ctxBatch = withUnorderedContext(ctxBatch)
		ctxBatch = withUnorderedDepth(ctxBatch, depth+1)
		ctxBatch = withUnorderedContext(ctxBatch)
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				for ns := range jobs {
					if ctxBatch.Err() != nil {
						return
					}
					resp, err := ns.QueryWithClass(ctxBatch, name, qtype, qclass)
					select {
					case results <- unorderedResult{ns: ns, resp: resp, err: err}:
					case <-ctxBatch.Done():
						return
					}
				}
			}()
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(results)
			close(done)
		}()
		go func() {
			defer close(jobs)
			for _, ns := range nss {
				select {
				case <-ctxBatch.Done():
					return
				case jobs <- ns:
				}
			}
		}()

		redirected := false
		var redirectResp packet.Packet
		var redirectNS queryer
		var redirectZName string
		var redirectCommon int
		decided := false
		needsCNAME := false
		var decidedResp packet.Packet
	loop:
		for {
			select {
			case <-ctxBatch.Done():
				break loop
			case res, ok := <-results:
				if !ok {
					break loop
				}
				if res.err != nil || res.resp.Msg == nil {
					continue
				}

				resp := res.resp
				if resp.Rcode() == "REFUSED" || resp.Rcode() == "SERVFAIL" {
					if state.candidate.Msg == nil {
						state.candidate = resp
					}
					continue
				}

				if resp.NoSuchRecord() || resp.NoSuchName() {
					decided = true
					decidedResp = resp
					cancel()
					break loop
				}

				if resp.Type() == "answer" {
					if !resp.HasRRsOfTypeForName(qtype, nameObj, "answer") && len(resp.GetRecordsForName("CNAME", nameObj, "answer")) > 0 {
						decided = true
						needsCNAME = true
						decidedResp = resp
						cancel()
						break loop
					}
					decided = true
					decidedResp = resp
					cancel()
					break loop
				}

				if resp.IsRedirect() {
					zname, ok := redirectName(resp)
					if !ok || zname == "." {
						continue
					}
					zkey := strings.ToLower(zname)
					if state.seen[zkey] {
						continue
					}

					common := dnsname.New(zname).Common(state.qname)
					if common < state.common {
						continue
					}

					redirected = true
					redirectResp = resp
					redirectNS = res.ns
					redirectZName = zname
					redirectCommon = common
					cancel()
					break loop
				}
			}
		}

		cancel()
		<-done

		if decided {
			if needsCNAME {
				cnameCtx := ctx
				if profile.FromContext(ctx).Resolver.Defaults.Unordered {
					cnameCtx = withUnorderedContext(cnameCtx)
					cnameCtx = withUnorderedDepth(cnameCtx, depth+1)
				}
				cnameResp, nextState, err := r.resolveCNAME(cnameCtx, nameObj, qtype, qclass, decidedResp, state)
				return cnameResp, nextState, err
			}
			return decidedResp, state, nil
		}
		if redirected {
			zkey := strings.ToLower(redirectZName)
			state.seen[zkey] = true
			state.common = redirectCommon

			next, err := state.nsFrom(ctx, redirectResp, state)
			if err != nil {
				return packet.Packet{}, state, err
			}
			state.ns = next
			state.count++
			if state.count > 20 {
				return packet.Packet{}, state, nil
			}
			state.trace = append([]traceEntry{{
				zoneName:   redirectZName,
				source:     redirectNS,
				answerFrom: redirectResp.AnswerFrom,
			}}, state.trace...)
			continue
		}
	}

	if state.candidate.Msg != nil {
		return state.candidate, state, nil
	}
	return packet.Packet{}, state, nil
}

func redirectName(resp packet.Packet) (string, bool) {
	records := resp.GetRecords("NS")
	if len(records) == 0 {
		return "", false
	}
	owner := records[0].Header().Name
	ownerName := dnsname.New(owner)
	return ownerName.String(), true
}

func (r *Recursor) resolveCNAME(ctx context.Context, name dnsname.Name, qtype string, qclass string, resp packet.Packet, state *recurseState) (packet.Packet, *recurseState, error) {
	cnameRRs := resp.GetRecords("CNAME", "answer")
	if len(cnameRRs) == 0 {
		return resp, state, nil
	}

	unique := make([]*dns.CNAME, 0, len(cnameRRs))
	seen := map[string]bool{}
	for _, rr := range cnameRRs {
		cname, ok := rr.(*dns.CNAME)
		if !ok {
			continue
		}
		key := strconv.Itoa(int(cname.Hdr.Class)) + "/CNAME/" + strings.ToLower(cname.Hdr.Name) + "/" + strings.ToLower(cname.Target)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, cname)
	}

	if len(unique) > constants.CNAMEMaxRecords {
		return packet.Packet{}, state, nil
	}

	cnames := map[string]string{}
	seenTargets := map[string]bool{}
	forbiddenTargets := map[string]bool{}
	for _, rr := range unique {
		ownerName := dnsname.New(rr.Hdr.Name)
		targetName := dnsname.New(rr.Target)
		ownerKey := strings.ToLower(ownerName.String())
		targetKey := strings.ToLower(targetName.String())

		if forbiddenTargets[ownerKey] {
			return packet.Packet{}, state, nil
		}
		if ownerKey == targetKey || seenTargets[targetKey] || forbiddenTargets[targetKey] {
			return packet.Packet{}, state, nil
		}

		seenTargets[targetKey] = true
		forbiddenTargets[ownerKey] = true
		cnames[ownerKey] = targetKey
	}

	targetKey := strings.ToLower(name.String())
	counter := 0
	for {
		next, ok := cnames[targetKey]
		if !ok {
			break
		}
		if counter > constants.CNAMEMaxRecords {
			return packet.Packet{}, state, nil
		}
		targetKey = next
		counter++
	}

	if counter != len(unique) {
		return packet.Packet{}, state, nil
	}

	if len(resp.GetRecords(qtype, "answer")) > 0 {
		targetName := dnsname.New(targetKey)
		if resp.HasRRsOfTypeForName(qtype, targetName, "answer") {
			return resp, state, nil
		}
		return packet.Packet{}, state, nil
	}

	if state == nil {
		state = &recurseState{}
	}
	state.ensureLock()
	state.lock()
	if state.inProgress == nil {
		state.inProgress = map[string]map[string]bool{}
	}
	if state.tseen == nil {
		state.tseen = map[string]bool{}
	}
	if state.inProgress[targetKey] != nil && state.inProgress[targetKey][qtype] {
		state.unlock()
		return packet.Packet{}, state, nil
	}
	state.tseen[targetKey] = true
	state.tcount++
	tcount := state.tcount
	state.unlock()
	if tcount > constants.CNAMEMaxChainLength {
		return packet.Packet{}, state, nil
	}

	targetName := dnsname.New(targetKey)
	if !name.IsInBailiwick(targetName) {
		root, err := r.RootServers(ctx)
		if err != nil {
			return packet.Packet{}, state, err
		}
		queryers := make([]queryer, 0, len(root))
		for _, server := range root {
			queryers = append(queryers, server)
		}

		nextState := &recurseState{
			ns:         queryers,
			count:      0,
			common:     0,
			seen:       map[string]bool{},
			inProgress: state.inProgress,
			tseen:      state.tseen,
			tcount:     tcount,
			mu:         state.mu,
		}
		return r.recurse(ctx, targetName.String(), qtype, qclass, nextState)
	}

	return resp, state, nil
}
