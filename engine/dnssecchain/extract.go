package dnssecchain

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// Array caps bound the emitted document. Truncated is set when any cap is hit.
const (
	maxDS      = 16
	maxDNSKEY  = 16
	maxRRSIG   = 8
	maxLinks   = 32
	maxServers = 16
)

const revokeFlag = 1 << 7 // DNSKEY REVOKE bit (RFC 5011)

// Input carries the per-run identity and cached nameserver sets the extractor
// reads from. The tested zone's ChildNS and the parent's ParentNS are the
// exact nameserver objects the run memoized, so their caches hold the answers
// the testcases already collected.
type Input struct {
	Zone       dnsname.Name
	ParentZone dnsname.Name
	ChildNS    []nameserver.Nameserver
	ParentNS   []nameserver.Nameserver
	At         time.Time
}

// Extract builds the chain summary from cached responses, returning nil when
// no DNSSEC evidence was cached. NoNetwork plus a discard logger keep it
// strictly cache-only.
func Extract(ctx context.Context, in Input) *Summary {
	prof := profile.FromContext(ctx)
	if prof == nil {
		return nil
	}
	cctx := cacheOnlyContext(ctx, prof)

	at := in.At
	if at.IsZero() {
		at = time.Now().UTC()
	}

	e := &extractor{
		summary: &Summary{
			Version:    Version,
			Zone:       in.Zone.String(),
			ParentZone: in.ParentZone.String(),
			Delegation: DelegationNormal,
		},
		zone:       in.Zone,
		at:         at,
		dsIndex:    map[string]int{},
		keyIndex:   map[string]int{},
		childKeys:  map[uint16][]*dns.DNSKEY{},
		signed:     map[string][]RRSIG{},
		signedRefs: map[string]map[uint16]bool{},
	}

	e.extractParent(cctx, in)
	e.extractChild(cctx, in)

	if !e.hasEvidence() {
		return nil
	}

	e.buildLinks()
	e.markAnchoredKeys()
	e.finalize()
	e.summary.Status = e.rollup()
	return e.summary
}

type extractor struct {
	summary *Summary
	zone    dnsname.Name
	at      time.Time

	dsIndex    map[string]int             // DS identity -> index into summary.Parent.DS
	keyIndex   map[string]int             // DNSKEY identity -> index into summary.Child.DNSKEYs
	childKeys  map[uint16][]*dns.DNSKEY   // keytag -> key objects, for digest comparison
	signed     map[string][]RRSIG         // RRset type -> covering signatures
	signedRefs map[string]map[uint16]bool // RRset type -> referenced DNSKEY key tags
}

// cacheOnlyContext clones the profile with NoNetwork set and swaps in a fresh
// discard logger so reads cannot query or pollute the run.
func cacheOnlyContext(ctx context.Context, prof *profile.Profile) context.Context {
	clone := *prof
	clone.NoNetwork = true
	out := profile.WithContext(ctx, &clone)
	return logger.WithContext(out, logger.New())
}

func query(ctx context.Context, ns nameserver.Nameserver, name string, qtype string) (packet.Packet, bool) {
	dnssecOn := true
	resp, err := ns.QueryWithOptions(ctx, name, qtype, &nameserver.QueryOptions{DNSSEC: &dnssecOn})
	if err != nil || resp.Msg == nil {
		return packet.Packet{}, false
	}
	return resp, true
}

// validDNSSECAnswer applies the same acceptance gate the DNSSEC testcases use.
func validDNSSECAnswer(pkt packet.Packet) bool {
	return pkt.Rcode() == "NOERROR" && pkt.HasEdns() && pkt.DO() && pkt.AA()
}

func (e *extractor) extractParent(ctx context.Context, in Input) {
	zoneName := e.zone.String()

	if fake := firstFakeDS(zoneName, in.ParentNS, in.ChildNS); len(fake) > 0 {
		e.summary.Delegation = DelegationUndelegated
		e.summary.Parent.DSSource = DSSourceInput
		for _, rr := range fake {
			if ds, ok := rr.(*dns.DS); ok {
				e.addDS(ds, "-")
			}
		}
		e.summary.Parent.ServersQueried = []string{"-"}
		return
	}

	parentApex := in.ParentZone.String()
	sigByIP := map[string]string{}
	for _, ns := range dedupeByAddress(in.ParentNS) {
		ip := ns.AddressString()
		resp, ok := query(ctx, ns, zoneName, "DS")
		if !ok || !validDNSSECAnswer(resp) {
			continue
		}
		e.summary.Parent.ServersQueried = append(e.summary.Parent.ServersQueried, ip)

		dsRRs := dsRecords(resp, e.zone)
		if len(dsRRs) == 0 {
			e.summary.Parent.ServersWithoutDS = append(e.summary.Parent.ServersWithoutDS, ip)
			sigByIP[ip] = ""
			continue
		}
		sigByIP[ip] = dsSetSignature(dsRRs)
		for _, ds := range dsRRs {
			e.addDS(ds, ip)
		}

		// Verify DS-covering RRSIGs against this server's DS RRset using the
		// parent-apex DNSKEYs from the same server, matching DNSSEC21.
		parentKeys := parentApexKeys(ctx, ns, in.ParentZone, parentApex)
		dsRRset := dsRRsetToRR(dsRRs)
		for _, sig := range coveringRRSIG(resp, dns.TypeDS, parentApex) {
			state := sigState(sig, dsRRset, parentKeys, e.at)
			e.addRRSIG(&e.summary.Parent.DSRRSIG, sig, state, ip)
		}
	}

	if len(e.summary.Parent.DS) > 0 {
		e.summary.Parent.DSSource = DSSourceParent
	} else {
		e.summary.Parent.DSSource = DSSourceNone
	}
	e.summary.Parent.ServersDisagreeing = disagreeing(sigByIP)
}

func (e *extractor) extractChild(ctx context.Context, in Input) {
	zoneName := e.zone.String()
	sigByIP := map[string]string{}
	for _, ns := range dedupeByAddress(in.ChildNS) {
		ip := ns.AddressString()
		resp, ok := query(ctx, ns, zoneName, "DNSKEY")
		if !ok || !validDNSSECAnswer(resp) {
			continue
		}
		e.summary.Child.ServersQueried = append(e.summary.Child.ServersQueried, ip)

		keyRRs := dnskeyRecords(resp, e.zone)
		if len(keyRRs) == 0 {
			e.summary.Child.ServersWithoutDNSKEY = append(e.summary.Child.ServersWithoutDNSKEY, ip)
			sigByIP[ip] = ""
			continue
		}
		sigByIP[ip] = keySetSignature(keyRRs)
		for _, key := range keyRRs {
			e.addDNSKEY(key, ip)
		}

		keyRRset := keysToRR(keyRRs)
		for _, sig := range coveringRRSIG(resp, dns.TypeDNSKEY, zoneName) {
			state := sigState(sig, keyRRset, keyRRs, e.at)
			e.addRRSIG(&e.summary.Child.DNSKEYRRSIG, sig, state, ip)
		}

		// Apex zone-data signatures cached with the DO bit by a testcase.
		for _, zt := range zoneDataTypes {
			zresp, ok := query(ctx, ns, zoneName, zt.name)
			if !ok || !validDNSSECAnswer(zresp) {
				continue
			}
			rrset := recordsOfType(zresp, e.zone, zt.rrtype)
			for _, sig := range coveringRRSIG(zresp, zt.rrtype, zoneName) {
				state := sigState(sig, rrset, keyRRs, e.at)
				e.addSignedRRSIG(zt.name, sig, state, ip)
			}
			if zt.refs != nil {
				for _, kt := range zt.refs(zresp, e.zone) {
					e.addSignedRef(zt.name, kt)
				}
			}
		}
	}
	e.summary.Child.ServersDisagreeing = disagreeing(sigByIP)
	e.buildSigned()
}

// zoneDataTypes are apex RRsets, in display order, whose signatures the run
// already cached with the DO bit.
var zoneDataTypes = []struct {
	name   string
	rrtype uint16
	refs   func(packet.Packet, dnsname.Name) []uint16
}{
	{"SOA", dns.TypeSOA, nil},
	{"NSEC3PARAM", dns.TypeNSEC3PARAM, nil},
	{"CDS", dns.TypeCDS, cdsRefs},
	{"CDNSKEY", dns.TypeCDNSKEY, cdnskeyRefs},
}

func (e *extractor) addSignedRRSIG(rrtype string, sig *dns.RRSIG, state, server string) {
	dst := e.signed[rrtype]
	mergeRRSIG(&dst, sig, state, server)
	e.signed[rrtype] = dst
}

func (e *extractor) addSignedRef(rrtype string, keytag uint16) {
	if e.signedRefs[rrtype] == nil {
		e.signedRefs[rrtype] = map[uint16]bool{}
	}
	e.signedRefs[rrtype][keytag] = true
}

func (e *extractor) buildSigned() {
	for _, zt := range zoneDataTypes {
		sigs := e.signed[zt.name]
		refs := sortedKeytags(e.signedRefs[zt.name])
		if len(sigs) == 0 && len(refs) == 0 {
			continue
		}
		rr := SignedRRset{Type: zt.name, RRSIG: sigs, Refs: refs}
		if zt.name == "CDS" || zt.name == "CDNSKEY" {
			rr.DSMatch, rr.NewKeys = e.compareRefsToDS(refs)
		}
		e.summary.Child.Signed = append(e.summary.Child.Signed, rr)
	}
}

// compareRefsToDS returns "" when the parent has no DS to compare against.
func (e *extractor) compareRefsToDS(refs []uint16) (string, []uint16) {
	if len(e.summary.Parent.DS) == 0 || len(refs) == 0 {
		return "", nil
	}
	dsTags := map[uint16]bool{}
	for _, ds := range e.summary.Parent.DS {
		dsTags[ds.KeyTag] = true
	}
	refTags := map[uint16]bool{}
	var newKeys []uint16
	for _, kt := range refs {
		if refTags[kt] {
			continue
		}
		refTags[kt] = true
		if !dsTags[kt] {
			newKeys = append(newKeys, kt)
		}
	}
	if len(newKeys) == 0 && len(refTags) == len(dsTags) {
		return CDSMatchExact, nil
	}
	sort.Slice(newKeys, func(i, j int) bool { return newKeys[i] < newKeys[j] })
	return CDSMatchRollover, newKeys
}

func (e *extractor) hasEvidence() bool {
	return len(e.summary.Parent.DS) > 0 ||
		e.summary.Parent.DSSource == DSSourceInput ||
		len(e.summary.Child.DNSKEYs) > 0 ||
		len(e.summary.Parent.ServersQueried) > 0 ||
		len(e.summary.Child.ServersQueried) > 0
}

func (e *extractor) buildLinks() {
	for _, ds := range e.summary.Parent.DS {
		keys := e.childKeys[ds.KeyTag]
		if len(keys) == 0 {
			e.summary.Links = append(e.summary.Links, Link{
				DSKeyTag:     ds.KeyTag,
				DSDigestType: ds.DigestType,
				Status:       LinkNoDNSKEY,
				Servers:      ds.Servers,
			})
			continue
		}
		e.summary.Links = append(e.summary.Links, Link{
			DSKeyTag:     ds.KeyTag,
			DSDigestType: ds.DigestType,
			DNSKEYKeyTag: ds.KeyTag,
			Status:       dsLinkStatus(ds, keys),
			Servers:      e.serversForKey(ds.KeyTag),
		})
	}
}

// dsLinkStatus compares the DS digest against every key with its tag; a nil
// ToDS counts as unsupported digest, never as a mismatch.
func dsLinkStatus(ds DS, keys []*dns.DNSKEY) string {
	if !dnssecutil.DigestSupported(ds.DigestType) {
		return LinkUnsupportedDigest
	}
	sawDigest := false
	for _, key := range keys {
		tmp := key.ToDS(ds.DigestType)
		if tmp == nil {
			continue
		}
		sawDigest = true
		if strings.EqualFold(tmp.Digest, ds.Digest) {
			return LinkMatch
		}
	}
	if !sawDigest {
		return LinkUnsupportedDigest
	}
	return LinkDigestMismatch
}

func (e *extractor) rollup() string {
	hasDS := len(e.summary.Parent.DS) > 0 || e.summary.Parent.DSSource == DSSourceInput
	hasKeys := len(e.summary.Child.DNSKEYs) > 0

	switch {
	case !hasDS && !hasKeys:
		if len(e.summary.Child.ServersWithoutDNSKEY) > 0 {
			return StatusUnsigned
		}
		return StatusIndeterminate
	case !hasDS && hasKeys:
		return StatusIsland
	case hasDS && !hasKeys:
		// Broken needs positive child evidence; a cold child cache is not proof.
		if len(e.summary.Child.ServersQueried) == 0 {
			return StatusIndeterminate
		}
		return StatusBroken
	default:
		if !e.hasMatchingLink() {
			return StatusBroken
		}
		if e.hasValidDNSKEYSignature() {
			return StatusSecure
		}
		return StatusBroken
	}
}

// markAnchoredKeys flags each DNSKEY a matching DS names.
func (e *extractor) markAnchoredKeys() {
	matched := map[uint16]bool{}
	for _, l := range e.summary.Links {
		if l.Status == LinkMatch {
			matched[l.DNSKEYKeyTag] = true
		}
	}
	for i := range e.summary.Child.DNSKEYs {
		if matched[e.summary.Child.DNSKEYs[i].KeyTag] {
			e.summary.Child.DNSKEYs[i].Anchored = true
		}
	}
}

func (e *extractor) hasMatchingLink() bool {
	for _, l := range e.summary.Links {
		if l.Status == LinkMatch {
			return true
		}
	}
	return false
}

// hasValidDNSKEYSignature reports whether a key that matches a DS validly signs
// the DNSKEY RRset.
func (e *extractor) hasValidDNSKEYSignature() bool {
	matched := map[uint16]bool{}
	for _, l := range e.summary.Links {
		if l.Status == LinkMatch {
			matched[l.DNSKEYKeyTag] = true
		}
	}
	for _, sig := range e.summary.Child.DNSKEYRRSIG {
		if sig.State == SigValid && matched[sig.KeyTag] {
			return true
		}
	}
	return false
}

// --- accumulation helpers ---

func (e *extractor) addDS(ds *dns.DS, server string) {
	if ds == nil {
		return
	}
	id := fmt.Sprintf("%d|%d|%d|%s", ds.KeyTag, ds.Algorithm, ds.DigestType, strings.ToLower(ds.Digest))
	if idx, ok := e.dsIndex[id]; ok {
		e.summary.Parent.DS[idx].Servers = append(e.summary.Parent.DS[idx].Servers, server)
		return
	}
	e.dsIndex[id] = len(e.summary.Parent.DS)
	e.summary.Parent.DS = append(e.summary.Parent.DS, DS{
		KeyTag:     ds.KeyTag,
		Algorithm:  ds.Algorithm,
		DigestType: ds.DigestType,
		Digest:     strings.ToLower(ds.Digest),
		Servers:    []string{server},
	})
}

func (e *extractor) addDNSKEY(key *dns.DNSKEY, server string) {
	if key == nil {
		return
	}
	keytag := key.KeyTag()
	id := fmt.Sprintf("%d|%d|%d", keytag, key.Algorithm, key.Flags)
	if idx, ok := e.keyIndex[id]; ok {
		e.summary.Child.DNSKEYs[idx].Servers = append(e.summary.Child.DNSKEYs[idx].Servers, server)
		return
	}
	e.childKeys[keytag] = append(e.childKeys[keytag], key)
	e.keyIndex[id] = len(e.summary.Child.DNSKEYs)
	e.summary.Child.DNSKEYs = append(e.summary.Child.DNSKEYs, DNSKEY{
		KeyTag:    keytag,
		Algorithm: key.Algorithm,
		Flags:     key.Flags,
		SEP:       key.Flags&dns.FlagSEP != 0,
		ZoneKey:   key.Flags&dns.FlagZONE != 0,
		Revoked:   key.Flags&revokeFlag != 0,
		KeySize:   keySizeBits(key),
		Servers:   []string{server},
	})
}

func (e *extractor) addRRSIG(dst *[]RRSIG, sig *dns.RRSIG, state string, server string) {
	mergeRRSIG(dst, sig, state, server)
}

// mergeRRSIG appends sig to dst, merging servers when the same signature was
// already seen and preferring the valid state.
func mergeRRSIG(dst *[]RRSIG, sig *dns.RRSIG, state string, server string) {
	if sig == nil {
		return
	}
	signer := canonName(sig.SignerName)
	for i := range *dst {
		r := &(*dst)[i]
		if r.KeyTag == sig.KeyTag && r.Algorithm == sig.Algorithm &&
			r.Inception == int64(sig.Inception) && r.Expiration == int64(sig.Expiration) && r.Signer == signer {
			r.Servers = append(r.Servers, server)
			if state == SigValid {
				r.State = SigValid
			}
			return
		}
	}
	*dst = append(*dst, RRSIG{
		KeyTag:     sig.KeyTag,
		Algorithm:  sig.Algorithm,
		Inception:  int64(sig.Inception),
		Expiration: int64(sig.Expiration),
		Signer:     signer,
		State:      state,
		Servers:    []string{server},
	})
}

func (e *extractor) serversForKey(keytag uint16) []string {
	for _, k := range e.summary.Child.DNSKEYs {
		if k.KeyTag == keytag {
			return k.Servers
		}
	}
	return nil
}

// --- finalize: dedupe/sort/cap for deterministic, bounded output ---

func (e *extractor) finalize() {
	p := &e.summary.Parent
	p.ServersQueried = sortUnique(p.ServersQueried)
	p.ServersWithoutDS = sortUnique(p.ServersWithoutDS)
	p.ServersDisagreeing = sortUnique(p.ServersDisagreeing)
	c := &e.summary.Child
	c.ServersQueried = sortUnique(c.ServersQueried)
	c.ServersWithoutDNSKEY = sortUnique(c.ServersWithoutDNSKEY)
	c.ServersDisagreeing = sortUnique(c.ServersDisagreeing)

	sort.Slice(p.DS, func(i, j int) bool {
		if p.DS[i].KeyTag != p.DS[j].KeyTag {
			return p.DS[i].KeyTag < p.DS[j].KeyTag
		}
		return p.DS[i].DigestType < p.DS[j].DigestType
	})
	sort.Slice(c.DNSKEYs, func(i, j int) bool {
		if c.DNSKEYs[i].SEP != c.DNSKEYs[j].SEP {
			return c.DNSKEYs[i].SEP // SEP (KSK) first
		}
		return c.DNSKEYs[i].KeyTag < c.DNSKEYs[j].KeyTag
	})
	sortRRSIG(p.DSRRSIG)
	sortRRSIG(c.DNSKEYRRSIG)
	for i := range c.Signed {
		sortRRSIG(c.Signed[i].RRSIG)
	}
	sort.Slice(e.summary.Links, func(i, j int) bool {
		if e.summary.Links[i].DSKeyTag != e.summary.Links[j].DSKeyTag {
			return e.summary.Links[i].DSKeyTag < e.summary.Links[j].DSKeyTag
		}
		return e.summary.Links[i].DNSKEYKeyTag < e.summary.Links[j].DNSKEYKeyTag
	})

	for i := range p.DS {
		p.DS[i].Servers = sortUnique(p.DS[i].Servers)
	}
	for i := range c.DNSKEYs {
		c.DNSKEYs[i].Servers = sortUnique(c.DNSKEYs[i].Servers)
	}
	for i := range e.summary.Links {
		e.summary.Links[i].Servers = sortUnique(e.summary.Links[i].Servers)
	}
	finalizeRRSIGServers(p.DSRRSIG)
	finalizeRRSIGServers(c.DNSKEYRRSIG)
	for i := range c.Signed {
		finalizeRRSIGServers(c.Signed[i].RRSIG)
	}

	e.applyCaps()
	e.ensureNonNil()
}

// ensureNonNil replaces nil arrays with empty ones so the JSON matches the
// documented schema (empty arrays, never null).
func (e *extractor) ensureNonNil() {
	p := &e.summary.Parent
	c := &e.summary.Child
	if p.DS == nil {
		p.DS = []DS{}
	}
	if p.DSRRSIG == nil {
		p.DSRRSIG = []RRSIG{}
	}
	p.ServersQueried = orEmpty(p.ServersQueried)
	p.ServersWithoutDS = orEmpty(p.ServersWithoutDS)
	p.ServersDisagreeing = orEmpty(p.ServersDisagreeing)
	if c.DNSKEYs == nil {
		c.DNSKEYs = []DNSKEY{}
	}
	if c.DNSKEYRRSIG == nil {
		c.DNSKEYRRSIG = []RRSIG{}
	}
	if c.Signed == nil {
		c.Signed = []SignedRRset{}
	}
	c.ServersQueried = orEmpty(c.ServersQueried)
	c.ServersWithoutDNSKEY = orEmpty(c.ServersWithoutDNSKEY)
	c.ServersDisagreeing = orEmpty(c.ServersDisagreeing)
	if e.summary.Links == nil {
		e.summary.Links = []Link{}
	}
}

func orEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func (e *extractor) applyCaps() {
	p := &e.summary.Parent
	c := &e.summary.Child
	trunc := false
	p.DS, trunc = capSlice(p.DS, maxDS, trunc)
	c.DNSKEYs, trunc = capSlice(c.DNSKEYs, maxDNSKEY, trunc)
	p.DSRRSIG, trunc = capSlice(p.DSRRSIG, maxRRSIG, trunc)
	c.DNSKEYRRSIG, trunc = capSlice(c.DNSKEYRRSIG, maxRRSIG, trunc)
	for i := range c.Signed {
		c.Signed[i].RRSIG, trunc = capSlice(c.Signed[i].RRSIG, maxRRSIG, trunc)
	}
	e.summary.Links, trunc = capSlice(e.summary.Links, maxLinks, trunc)

	p.ServersQueried, trunc = capSlice(p.ServersQueried, maxServers, trunc)
	p.ServersWithoutDS, trunc = capSlice(p.ServersWithoutDS, maxServers, trunc)
	p.ServersDisagreeing, trunc = capSlice(p.ServersDisagreeing, maxServers, trunc)
	c.ServersQueried, trunc = capSlice(c.ServersQueried, maxServers, trunc)
	c.ServersWithoutDNSKEY, trunc = capSlice(c.ServersWithoutDNSKEY, maxServers, trunc)
	c.ServersDisagreeing, trunc = capSlice(c.ServersDisagreeing, maxServers, trunc)
	for i := range p.DS {
		p.DS[i].Servers, trunc = capSlice(p.DS[i].Servers, maxServers, trunc)
	}
	for i := range c.DNSKEYs {
		c.DNSKEYs[i].Servers, trunc = capSlice(c.DNSKEYs[i].Servers, maxServers, trunc)
	}
	e.summary.Truncated = trunc
}
