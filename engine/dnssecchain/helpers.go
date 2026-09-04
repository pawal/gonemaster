package dnssecchain

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// firstFakeDS returns the first non-empty fake DS set for the zone across the
// given nameserver groups. Undelegated tests attach synthesized DS records to
// the parent's servers, bypassing the response cache.
func firstFakeDS(zoneName string, groups ...[]nameserver.Nameserver) []dns.RR {
	for _, group := range groups {
		for _, ns := range group {
			if recs := ns.FakeDSRecords(zoneName); len(recs) > 0 {
				return recs
			}
		}
	}
	return nil
}

// dedupeByAddress keeps one nameserver per IP; same-IP servers share a cache.
func dedupeByAddress(nss []nameserver.Nameserver) []nameserver.Nameserver {
	seen := map[string]bool{}
	out := make([]nameserver.Nameserver, 0, len(nss))
	for _, ns := range nss {
		ip := ns.AddressString()
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		out = append(out, ns)
	}
	return out
}

// cdsRefs returns the DNSKEY key tags named by the CDS records in resp,
// skipping the RFC 8078 DELETE sentinel (algorithm 0).
func cdsRefs(resp packet.Packet, zone dnsname.Name) []uint16 {
	var out []uint16
	for _, rr := range resp.GetRecordsForName("CDS", zone, "answer") {
		if cds, ok := rr.(*dns.CDS); ok && cds.Algorithm != 0 {
			out = append(out, cds.KeyTag)
		}
	}
	return out
}

// cdnskeyRefs returns the DNSKEY key tags named by the CDNSKEY records in resp,
// skipping the RFC 8078 DELETE sentinel (algorithm 0).
func cdnskeyRefs(resp packet.Packet, zone dnsname.Name) []uint16 {
	var out []uint16
	for _, rr := range resp.GetRecordsForName("CDNSKEY", zone, "answer") {
		if ck, ok := rr.(*dns.CDNSKEY); ok && ck.Algorithm != 0 {
			out = append(out, dnssecutil.KeyTag(&ck.DNSKEY))
		}
	}
	return out
}

func sortedKeytags(set map[uint16]bool) []uint16 {
	if len(set) == 0 {
		return nil
	}
	out := make([]uint16, 0, len(set))
	for kt := range set {
		out = append(out, kt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func dsRecords(resp packet.Packet, zone dnsname.Name) []*dns.DS {
	var out []*dns.DS
	for _, rr := range resp.GetRecordsForName("DS", zone, "answer") {
		if ds, ok := rr.(*dns.DS); ok {
			out = append(out, ds)
		}
	}
	return out
}

func dnskeyRecords(resp packet.Packet, zone dnsname.Name) []*dns.DNSKEY {
	var out []*dns.DNSKEY
	for _, rr := range resp.GetRecordsForName("DNSKEY", zone, "answer") {
		if key, ok := rr.(*dns.DNSKEY); ok {
			out = append(out, key)
		}
	}
	return out
}

func recordsOfType(resp packet.Packet, name dnsname.Name, rrtype uint16) []dns.RR {
	return resp.GetRecordsForName(dns.TypeToString[rrtype], name, "answer")
}

// sectionRecords returns the records of rrtype in section. Answer-section
// RRsets are the ones owned by the zone apex; authority-section NSEC3 records
// are owned by a hashed name, so those are taken as served.
func sectionRecords(resp packet.Packet, zone dnsname.Name, rrtype uint16, section string) []dns.RR {
	if section == "authority" {
		return resp.GetRecords(dns.TypeToString[rrtype], section)
	}
	return recordsOfType(resp, zone, rrtype)
}

// rrsetForOwner narrows rrs to the RRset a signature covers.
func rrsetForOwner(rrs []dns.RR, owner string) []dns.RR {
	want := canonName(owner)
	out := make([]dns.RR, 0, len(rrs))
	for _, rr := range rrs {
		if canonName(rr.Header().Name) == want {
			out = append(out, rr)
		}
	}
	return out
}

// coveringRRSIG returns RRSIGs in section covering rrtype, filtered by signer
// when signer is non-empty. Authenticated-denial signatures (NSEC, NSEC3) sit
// in the authority section of a negative answer.
func coveringRRSIG(resp packet.Packet, rrtype uint16, signer, section string) []*dns.RRSIG {
	want := canonName(signer)
	var out []*dns.RRSIG
	for _, rr := range resp.GetRecords("RRSIG", section) {
		sig, ok := rr.(*dns.RRSIG)
		if !ok || sig.TypeCovered != rrtype {
			continue
		}
		if want != "" && canonName(sig.SignerName) != want {
			continue
		}
		out = append(out, sig)
	}
	return out
}

// parentApexKeys reads the parent-apex DNSKEYs from this server's cache. Empty
// when DNSSEC21 did not run or the answer was not cached.
func parentApexKeys(ctx context.Context, ns nameserver.Nameserver, parentZone dnsname.Name, parentApex string) []*dns.DNSKEY {
	if parentApex == "" {
		return nil
	}
	resp, ok := query(ctx, ns, parentApex, "DNSKEY")
	if !ok || !validDNSSECAnswer(resp) {
		return nil
	}
	return dnskeyRecords(resp, parentZone)
}

func dsRRsetToRR(dsRRs []*dns.DS) []dns.RR {
	out := make([]dns.RR, 0, len(dsRRs))
	for _, ds := range dsRRs {
		out = append(out, ds)
	}
	return out
}

func keysToRR(keys []*dns.DNSKEY) []dns.RR {
	out := make([]dns.RR, 0, len(keys))
	for _, k := range keys {
		out = append(out, k)
	}
	return out
}

// sigState evaluates an RRSIG with the DNSSEC21 precedence: window checks by
// wire time, then algorithm support, key presence, and finally verification.
// With no keys available the signature is unverified (window still applies).
func sigState(sig *dns.RRSIG, rrset []dns.RR, keys []*dns.DNSKEY, at time.Time) string {
	if sig == nil {
		return SigUnverified
	}
	if int64(sig.Inception) > at.Unix() {
		return SigNotYetValid
	}
	if int64(sig.Expiration) < at.Unix() {
		return SigExpired
	}
	if !dnssecutil.AlgorithmSupported(sig.Algorithm) {
		return SigUnsupported
	}
	if len(keys) == 0 {
		return SigUnverified
	}
	var matching []*dns.DNSKEY
	for _, k := range keys {
		if dnssecutil.KeyTag(k) == sig.KeyTag {
			matching = append(matching, k)
		}
	}
	if len(matching) == 0 {
		return SigNoKey
	}
	algoUnsupported := false
	unsupportedKey := false
	for _, k := range matching {
		if err := dnssecutil.VerifyRRSIG(sig, rrset, k, at); err == nil {
			return SigValid
		} else if errors.Is(err, dns.ErrAlg) {
			algoUnsupported = true
		} else if errors.Is(err, dnssecutil.ErrRSAExponentUnsupported) {
			unsupportedKey = true // RSA exponent beyond the local verifier
		}
	}
	if algoUnsupported {
		return SigUnsupported
	}
	if unsupportedKey {
		return SigUnsupportedKey
	}
	return SigBogus
}

func dsSetSignature(dsRRs []*dns.DS) string {
	parts := make([]string, 0, len(dsRRs))
	for _, ds := range dsRRs {
		parts = append(parts, strings.ToLower(ds.Digest))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func keySetSignature(keys []*dns.DNSKEY) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, strings.ToLower(k.PublicKey))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// sigProfile accumulates one server's signature evidence: the (type, key tag,
// state) tuples it served and whether any of them was outside its validity
// window. Two servers serving identical records with differently aged
// signatures must still count as disagreeing.
type sigProfile struct {
	states []string
	stale  bool
}

func (p *sigProfile) add(rrtype string, sig *dns.RRSIG, state string) {
	if sig == nil {
		return
	}
	p.states = append(p.states, fmt.Sprintf("%s|%d|%s", rrtype, sig.KeyTag, state))
	if state == SigExpired || state == SigNotYetValid {
		p.stale = true
	}
}

// fingerprint folds the signature states into the record-set signature.
func (p *sigProfile) fingerprint(recordSet string) string {
	sort.Strings(p.states)
	return recordSet + "#" + strings.Join(p.states, ",")
}

// disagreeing returns servers whose record-set signature differs from the most
// common one. With fewer than two distinct signatures it returns nothing.
func disagreeing(sigByIP map[string]string) []string {
	if len(sigByIP) < 2 {
		return nil
	}
	counts := map[string]int{}
	for _, sig := range sigByIP {
		counts[sig]++
	}
	if len(counts) < 2 {
		return nil
	}
	sigs := make([]string, 0, len(counts))
	for sig := range counts {
		sigs = append(sigs, sig)
	}
	sort.Strings(sigs)
	majority := sigs[0]
	for _, sig := range sigs {
		if counts[sig] > counts[majority] {
			majority = sig
		}
	}
	var out []string
	for ip, sig := range sigByIP {
		if sig != majority {
			out = append(out, ip)
		}
	}
	return out
}

func canonName(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

func sortUnique(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func sortRRSIG(sigs []RRSIG) {
	sort.Slice(sigs, func(i, j int) bool {
		if sigs[i].KeyTag != sigs[j].KeyTag {
			return sigs[i].KeyTag < sigs[j].KeyTag
		}
		return sigs[i].Inception < sigs[j].Inception
	})
}

func finalizeRRSIGServers(sigs []RRSIG) {
	for i := range sigs {
		sigs[i].Servers = sortUnique(sigs[i].Servers)
	}
}

func capSlice[T any](s []T, max int, trunc bool) ([]T, bool) {
	if len(s) > max {
		return s[:max], true
	}
	return s, trunc
}
