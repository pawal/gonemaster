package ednsopt

import dns "codeberg.org/miekg/dns"

// ApplyExplicit replaces any OPT in the additional section with one carrying the
// message's EDNS settings, and clears the message-level EDNS fields so Pack does
// not synthesize a second OPT. z is the caller's 16-bit Z value, or nil for none.
//
// Msg.Pack omits the OPT entirely when UDPSize is 512 or less and nothing else is
// set, so a query advertising exactly 512 needs the record built here.
func ApplyExplicit(msg *dns.Msg, z *uint16) {
	if msg == nil {
		return
	}

	// A non-EDNS pseudo record (TSIG, SIG0) must keep the default packing path,
	// which places it last.
	opt := &dns.OPT{Hdr: dns.Header{Name: "."}}
	for _, rr := range msg.Pseudo {
		edns, ok := rr.(dns.EDNS0)
		if !ok {
			return
		}
		opt.Options = append(opt.Options, edns)
	}

	// CO and DE sit above the 13-bit Z field, so SetZ would drop them.
	compactAnswers, delegation := msg.CompactAnswers, msg.Delegation
	zValue := msg.Z
	if z != nil {
		compactAnswers = compactAnswers || *z&FlagCO != 0
		delegation = delegation || *z&FlagDE != 0
		zValue = *z
	}

	SetUDPSize(opt, max(msg.UDPSize, dns.MinMsgSize))
	SetVersion(opt, msg.Version)
	SetSecurity(opt, msg.Security)
	SetCompactAnswers(opt, compactAnswers)
	SetDelegation(opt, delegation)
	SetRcode(opt, msg.Rcode)
	SetZ(opt, zValue)

	extra := make([]dns.RR, 0, len(msg.Extra)+1)
	for _, rr := range msg.Extra {
		if _, isOPT := rr.(*dns.OPT); isOPT {
			continue
		}
		extra = append(extra, rr)
	}
	msg.Extra = append(extra, opt)

	// Every field Pack reads as an OPT trigger, with the base rcode left in the header.
	msg.Pseudo = nil
	msg.UDPSize = 0
	msg.Security = false
	msg.CompactAnswers = false
	msg.Delegation = false
	msg.Version = 0
	msg.Z = 0
	msg.Rcode &= 0xF
}
