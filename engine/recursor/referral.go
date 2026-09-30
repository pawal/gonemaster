package recursor

import (
	"context"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// referralEntry is a root referral cached for one zone cut.
type referralEntry struct {
	resp    packet.Packet
	expires time.Time
}

// referralClaim lets one walk ask the root for a TLD while others wait for its referral.
type referralClaim struct {
	tld  string
	done chan struct{}
}

// storeReferral caches a root referral to a cut above qname until its shortest NS TTL runs out.
func (r *Recursor) storeReferral(zone string, resp packet.Packet, qname dnsname.Name) {
	cut := dnsname.New(zone)
	nsRRs := resp.GetRecords("NS")
	if len(nsRRs) == 0 || cut.Common(qname) != len(cut.Labels()) {
		return
	}
	ttl := nsRRs[0].Header().TTL
	for _, rr := range nsRRs[1:] {
		ttl = min(ttl, rr.Header().TTL)
	}
	if ttl == 0 {
		return
	}
	resp.Log = nil
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if r.referrals == nil {
		r.referrals = map[string]referralEntry{}
	}
	r.referrals[strings.ToLower(cut.String())] = referralEntry{resp: resp, expires: time.Now().Add(time.Duration(ttl) * time.Second)}
}

// cachedReferralLocked returns the deepest cached cut strictly above name.
// A cut equal to name is skipped: data such as its DS lives above the cut.
func (r *Recursor) cachedReferralLocked(name dnsname.Name) (string, packet.Packet, bool) {
	labels := name.Labels()
	for i := 1; i < len(labels); i++ {
		cut := strings.ToLower(strings.Join(labels[i:], "."))
		entry, ok := r.referrals[cut]
		if !ok {
			continue
		}
		if !time.Now().Before(entry.expires) {
			delete(r.referrals, cut)
			continue
		}
		return cut, entry.resp, true
	}
	return "", packet.Packet{}, false
}

// referralWaitLimit caps how long a walk waits for another walk's root step.
const referralWaitLimit = time.Second

// awaitReferral returns a cached referral for name, or a claim on the root step for its TLD.
// If another walk holds the claim, it waits once, up to referralWaitLimit, for that walk's referral.
func (r *Recursor) awaitReferral(ctx context.Context, name dnsname.Name) (string, packet.Packet, *referralClaim, error) {
	labels := name.Labels()
	if len(labels) < 2 {
		return "", packet.Packet{}, nil, nil
	}
	tld := strings.ToLower(labels[len(labels)-1])
	r.cacheMu.Lock()
	if cut, resp, ok := r.cachedReferralLocked(name); ok {
		r.cacheMu.Unlock()
		return cut, resp, nil, nil
	}
	busy, ok := r.referralClaims[tld]
	if !ok {
		if r.referralClaims == nil {
			r.referralClaims = map[string]*referralClaim{}
		}
		claim := &referralClaim{tld: tld, done: make(chan struct{})}
		r.referralClaims[tld] = claim
		r.cacheMu.Unlock()
		return "", packet.Packet{}, claim, nil
	}
	r.cacheMu.Unlock()

	var cancelled <-chan struct{}
	if ctx != nil {
		cancelled = ctx.Done()
	}
	timer := time.NewTimer(referralWaitLimit)
	defer timer.Stop()
	select {
	case <-busy.done:
	case <-timer.C:
	case <-cancelled:
		return "", packet.Packet{}, nil, ctx.Err()
	}
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	cut, resp, _ := r.cachedReferralLocked(name)
	return cut, resp, nil, nil
}

// releaseClaim ends the walk's root step, waking walks waiting for its referral.
func (r *Recursor) releaseClaim(state *recurseState) {
	if state == nil || state.claim == nil {
		return
	}
	claim := state.claim
	state.claim = nil
	r.cacheMu.Lock()
	if r.referralClaims[claim.tld] == claim {
		delete(r.referralClaims, claim.tld)
	}
	r.cacheMu.Unlock()
	close(claim.done)
}

func (r *Recursor) dropReferral(cut string) {
	r.cacheMu.Lock()
	delete(r.referrals, cut)
	r.cacheMu.Unlock()
}

func (r *Recursor) clearReferrals() {
	r.cacheMu.Lock()
	r.referrals = nil
	r.cacheMu.Unlock()
}

// seedReferral points state at the servers of a cached cut above name.
func (r *Recursor) seedReferral(ctx context.Context, name dnsname.Name, cut string, resp packet.Packet, state *recurseState) (bool, error) {
	ns, err := r.getNSFrom(ctx, resp, state)
	if err != nil || len(ns) == 0 {
		return false, err
	}
	state.ns = ns
	state.seen = map[string]bool{cut: true}
	state.common = dnsname.New(cut).Common(name)
	state.count++
	return true, nil
}

// startAtRoot points state at the root servers.
func (r *Recursor) startAtRoot(ctx context.Context, state *recurseState) error {
	root, err := r.RootServers(ctx)
	if err != nil {
		return err
	}
	state.ns = make([]queryer, 0, len(root))
	for _, server := range root {
		state.ns = append(state.ns, server)
	}
	state.atRoot = true
	return nil
}

// recurseFromRoot walks from the root, or from a cached root referral with a root retry when that yields nothing.
func (r *Recursor) recurseFromRoot(ctx context.Context, name string, qtype string, qclass string, newState func() *recurseState) (packet.Packet, *recurseState, error) {
	state := newState()
	if !state.skipReferrals && !state.isInProgress(name, qtype) {
		nameObj := dnsname.New(name)
		cut, resp, claim, err := r.awaitReferral(ctx, nameObj)
		if err != nil {
			return packet.Packet{}, state, err
		}
		if cut != "" {
			seeded, err := r.seedReferral(ctx, nameObj, cut, resp, state)
			if err != nil {
				return packet.Packet{}, state, err
			}
			if seeded {
				resp, next, err := r.recurse(ctx, name, qtype, qclass, state)
				if err != nil || resp.Msg != nil || (ctx != nil && ctx.Err() != nil) {
					return resp, next, err
				}
				r.dropReferral(cut)
				next.clearInProgress(name, qtype)
			}
			state = newState()
		}
		state.claim = claim
		defer r.releaseClaim(state)
	}
	if err := r.startAtRoot(ctx, state); err != nil {
		return packet.Packet{}, state, err
	}
	return r.recurse(ctx, name, qtype, qclass, state)
}
