package recursor

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

type queryer interface {
	QueryWithClass(ctx context.Context, qname string, qtype string, qclass string) (packet.Packet, error)
}

type recurseState struct {
	muInit     sync.Mutex
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
	state.muInit.Lock()
	if state.mu == nil {
		state.mu = &sync.Mutex{}
	}
	state.muInit.Unlock()
}

func (state *recurseState) lock() {
	if state == nil {
		return
	}
	state.ensureLock()
	state.mu.Lock()
}

func (state *recurseState) unlock() {
	if state == nil {
		return
	}
	state.muInit.Lock()
	mu := state.mu
	state.muInit.Unlock()
	if mu == nil {
		return
	}
	mu.Unlock()
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
	logRecursorSystem(ctx, "RECURSE", map[string]any{
		"name":  nameObj.String(),
		"type":  qtype,
		"class": qclass,
	})

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
	parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
	if parallelism < 1 {
		parallelism = 1
	}
	parentLogger := logger.FromContext(ctx)
	for len(state.ns) > 0 {
		batchSize := 1
		if parallelism > 1 {
			batchSize = parallelism
			if batchSize > len(state.ns) {
				batchSize = len(state.ns)
			}
		}

		batch := make([]queryer, 0, batchSize)
		for i := 0; i < batchSize; i++ {
			idx := len(state.ns) - 1
			batch = append(batch, state.ns[idx])
			state.ns = state.ns[:idx]
		}

		if len(batch) == 1 {
			logRecursorSystem(ctx, "RECURSE_QUERY", recurseQueryArgs(batch[0], nameObj, qtype, qclass))
			resp, err := batch[0].QueryWithClass(ctx, name, qtype, qclass)
			out, nextState, action, actionErr := r.processOrderedResponse(ctx, nameObj, qtype, qclass, state, batch[0], resp, err)
			state = nextState
			if actionErr != nil {
				return packet.Packet{}, state, actionErr
			}
			if action == orderedActionReturn {
				return out, state, nil
			}
			if action == orderedActionRedirect {
				continue
			}
			continue
		}

		results := make([]orderedQueryResult, len(batch))
		done := make([]chan struct{}, len(batch))
		ctxBatch, cancelBatch := context.WithCancel(ctx)
		for i, ns := range batch {
			done[i] = make(chan struct{})
			go func(i int, ns queryer) {
				defer close(done[i])
				queryCtx := ctxBatch
				var taskLogger *logger.Logger
				if parentLogger != nil {
					taskLogger = logger.New()
					taskLogger.CopyConfigFrom(parentLogger)
					taskLogger.CopyStartTimeFrom(parentLogger)
					queryCtx = logger.WithContext(queryCtx, taskLogger)
				}
				logRecursorSystem(queryCtx, "RECURSE_QUERY", recurseQueryArgs(ns, nameObj, qtype, qclass))
				resp, err := ns.QueryWithClass(queryCtx, name, qtype, qclass)
				result := orderedQueryResult{
					ns:   ns,
					resp: resp,
					err:  err,
				}
				if taskLogger != nil {
					result.logs = taskLogger.Entries()
				}
				results[i] = result
			}(i, ns)
		}

		processed := 0
		redirected := false
		var returnResp packet.Packet
		var returnErr error
		returnNow := false

		for i := 0; i < len(batch); i++ {
			<-done[i]
			processed = i + 1

			if parentLogger != nil && len(results[i].logs) > 0 {
				_ = parentLogger.Append(results[i].logs...)
			}

			out, nextState, action, actionErr := r.processOrderedResponse(ctx, nameObj, qtype, qclass, state, results[i].ns, results[i].resp, results[i].err)
			state = nextState
			if actionErr != nil {
				returnErr = actionErr
				returnNow = true
				cancelBatch()
				break
			}
			if action == orderedActionReturn {
				returnResp = out
				returnNow = true
				cancelBatch()
				break
			}
			if action == orderedActionRedirect {
				redirected = true
				cancelBatch()
				break
			}
		}

		for i := processed; i < len(batch); i++ {
			<-done[i]
		}
		cancelBatch()

		if returnErr != nil {
			return packet.Packet{}, state, returnErr
		}
		if returnNow {
			return returnResp, state, nil
		}
		if redirected {
			continue
		}
	}

	if state.candidate.Msg != nil {
		return state.candidate, state, nil
	}
	return packet.Packet{}, state, nil
}

type orderedAction int

const (
	orderedActionContinue orderedAction = iota
	orderedActionReturn
	orderedActionRedirect
)

type orderedQueryResult struct {
	ns   queryer
	resp packet.Packet
	err  error
	logs []*logger.Entry
}

func (r *Recursor) processOrderedResponse(ctx context.Context, nameObj dnsname.Name, qtype string, qclass string, state *recurseState, ns queryer, resp packet.Packet, err error) (packet.Packet, *recurseState, orderedAction, error) {
	if err != nil || resp.Msg == nil {
		return packet.Packet{}, state, orderedActionContinue, nil
	}

	if resp.Rcode() == "REFUSED" || resp.Rcode() == "SERVFAIL" {
		state.candidate = resp
		return packet.Packet{}, state, orderedActionContinue, nil
	}

	if resp.NoSuchRecord() || resp.NoSuchName() {
		return resp, state, orderedActionReturn, nil
	}

	if resp.Type() == "answer" {
		if !resp.HasRRsOfTypeForName(qtype, nameObj, "answer") && len(resp.GetRecordsForName("CNAME", nameObj, "answer")) > 0 {
			cnameResp, nextState, err := r.resolveCNAME(ctx, nameObj, qtype, qclass, resp, state)
			return cnameResp, nextState, orderedActionReturn, err
		}
		return resp, state, orderedActionReturn, nil
	}

	if resp.IsRedirect() {
		zname, ok := redirectName(resp)
		if !ok {
			return packet.Packet{}, state, orderedActionContinue, nil
		}
		if zname == "." {
			return packet.Packet{}, state, orderedActionContinue, nil
		}
		zkey := strings.ToLower(zname)
		if state.seen[zkey] {
			return packet.Packet{}, state, orderedActionContinue, nil
		}
		state.seen[zkey] = true

		common := dnsname.New(zname).Common(state.qname)
		if common < state.common {
			return packet.Packet{}, state, orderedActionContinue, nil
		}
		state.common = common

		next, err := state.nsFrom(ctx, resp, state)
		if err != nil {
			return packet.Packet{}, state, orderedActionReturn, err
		}
		state.ns = next
		state.count++
		if state.count > 20 {
			logRecursorSystem(ctx, "LOOP_PROTECTION", map[string]any{
				"caller":          "gonemaster.recursor._recurse",
				"child_zone_name": nameObj.String(),
				"zone_name":       zname,
			})
			return packet.Packet{}, state, orderedActionReturn, nil
		}
		state.trace = append([]traceEntry{{
			zoneName:   zname,
			source:     ns,
			answerFrom: resp.AnswerFrom,
		}}, state.trace...)
		return packet.Packet{}, state, orderedActionRedirect, nil
	}

	return packet.Packet{}, state, orderedActionContinue, nil
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
					logRecursorSystem(ctxBatch, "RECURSE_QUERY", recurseQueryArgs(ns, nameObj, qtype, qclass))
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
				logRecursorSystem(ctx, "LOOP_PROTECTION", map[string]any{
					"caller":          "gonemaster.recursor._recurse",
					"child_zone_name": nameObj.String(),
					"zone_name":       redirectZName,
				})
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

func recurseQueryArgs(ns queryer, name dnsname.Name, qtype string, qclass string) map[string]any {
	source, nsName, nsAddress := describeQuerySource(ns)
	return map[string]any{
		"source":  source,
		"ns":      nsName,
		"address": nsAddress,
		"name":    name.String(),
		"type":    qtype,
		"class":   qclass,
	}
}

func describeQuerySource(ns queryer) (string, string, string) {
	switch value := ns.(type) {
	case nameserver.Nameserver:
		return value.String(), value.Name.String(), value.Address.String()
	case *nameserver.Nameserver:
		if value == nil {
			return "<nil>", "", ""
		}
		return value.String(), value.Name.String(), value.Address.String()
	case lazyNameserver:
		return value.name, value.name, ""
	case *lazyNameserver:
		if value == nil {
			return "<nil>", "", ""
		}
		return value.name, value.name, ""
	default:
		return fmt.Sprintf("%v", ns), "", ""
	}
}

func logRecursorSystem(ctx context.Context, tag string, args map[string]any) {
	log := logger.FromContext(ctx)
	if log == nil {
		return
	}
	_, _ = log.Add(tag, args, "System", "")
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

		// Use a fresh inProgress map for CNAME resolution. The parent's
		// inProgress blocks re-resolution of nameserver addresses (e.g.
		// ns1.example A) that were already resolved during the parent
		// delegation walk. The CNAME target may need the same nameservers
		// via a different delegation path, so it must be able to resolve
		// them independently. CNAME-specific loop detection is handled
		// separately by tseen/tcount.
		nextState := &recurseState{
			ns:         queryers,
			count:      0,
			common:     0,
			seen:       map[string]bool{},
			inProgress: map[string]map[string]bool{},
			tseen:      state.tseen,
			tcount:     tcount,
			mu:         state.mu,
		}
		return r.recurse(ctx, targetName.String(), qtype, qclass, nextState)
	}

	return resp, state, nil
}
