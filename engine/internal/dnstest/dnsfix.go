package dnstest

import (
	"net/netip"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

// defaultTTL is the record TTL every fixture uses unless TTL overrides it.
const defaultTTL = 60

// MsgOpt shapes the packet Response builds. The option set mirrors tctest so
// the engine-core and testcase suites read the same.
type MsgOpt func(*packet.Packet)

// Response builds an authoritative NOERROR response and applies the options.
func Response(opts ...MsgOpt) packet.Packet {
	p := packet.Packet{Msg: new(dns.Msg)}
	p.Msg.Rcode = dns.RcodeSuccess
	p.Msg.Authoritative = true
	return From(p, opts...)
}

// From applies the options to an existing packet.
func From(p packet.Packet, opts ...MsgOpt) packet.Packet {
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

// Question sets the question section.
func Question(name string, qtype uint16) MsgOpt {
	return func(p *packet.Packet) { dnsutil.SetQuestion(p.Msg, dnsutil.Fqdn(name), qtype) }
}

// Answers appends records to the answer section.
func Answers(rrs ...dns.RR) MsgOpt {
	return func(p *packet.Packet) { p.Msg.Answer = append(p.Msg.Answer, rrs...) }
}

// Authority appends records to the authority section.
func Authority(rrs ...dns.RR) MsgOpt {
	return func(p *packet.Packet) { p.Msg.Ns = append(p.Msg.Ns, rrs...) }
}

// Additional appends records to the additional section.
func Additional(rrs ...dns.RR) MsgOpt {
	return func(p *packet.Packet) { p.Msg.Extra = append(p.Msg.Extra, rrs...) }
}

// Rcode sets the response code.
func Rcode(rcode uint16) MsgOpt {
	return func(p *packet.Packet) { p.Msg.Rcode = rcode }
}

// NXDOMAIN sets the NXDOMAIN response code.
func NXDOMAIN() MsgOpt {
	return Rcode(dns.RcodeNameError)
}

// NotAuthoritative clears the AA bit, as a recursor's answer has it clear.
func NotAuthoritative() MsgOpt {
	return func(p *packet.Packet) { p.Msg.Authoritative = false }
}

// Reply marks the message as a response.
func Reply() MsgOpt {
	return func(p *packet.Packet) { p.Msg.Response = true }
}

// Secure marks the message DNSSEC-checked and advertises an EDNS buffer.
func Secure() MsgOpt {
	return func(p *packet.Packet) {
		p.Msg.Security = true
		p.Msg.UDPSize = 1232
	}
}

// AnswerFrom records which address answered.
func AnswerFrom(address string) MsgOpt {
	return func(p *packet.Packet) { p.AnswerFrom = address }
}

// Referral builds a non-authoritative referral delegating zone to nsNames.
func Referral(zone string, nsNames ...string) packet.Packet {
	return Response(NotAuthoritative(), Authority(NSRRs(zone, nsNames...)...))
}

// NoData builds a NOERROR answer with the zone SOA in the authority section.
func NoData(owner string) packet.Packet {
	return Response(NotAuthoritative(), Authority(SOARR(owner)))
}

// MixedApexRecords builds an apex answer mixing SOA, NS and a decoy A record.
func MixedApexRecords(zoneName string, nsNames []string) packet.Packet {
	soa := SOARR(zoneName, MName("ns1."+zoneName))
	soa.Hdr.TTL = 3600
	answer := append([]dns.RR{soa}, NSRRs(zoneName, nsNames...)...)
	answer = append(answer, ARR("decoy."+zoneName, "192.0.2.99"))
	return Response(NotAuthoritative(), Answers(answer...))
}

// header returns the record header every RR builder starts from.
func header(owner string) dns.Header {
	return dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: defaultTTL}
}

// TTL overrides the TTL on the given records.
func TTL(ttl uint32, rrs ...dns.RR) []dns.RR {
	for _, rr := range rrs {
		rr.Header().TTL = ttl
	}
	return rrs
}

// ARR builds an A record.
func ARR(owner string, addr string) *dns.A {
	rr := &dns.A{Hdr: header(owner)}
	rr.Addr = netip.MustParseAddr(addr)
	return rr
}

// AAAARR builds an AAAA record.
func AAAARR(owner string, addr string) *dns.AAAA {
	rr := &dns.AAAA{Hdr: header(owner)}
	rr.Addr = netip.MustParseAddr(addr)
	return rr
}

// AddrRR builds an A or AAAA record, whichever the address is.
func AddrRR(owner string, addr string) dns.RR {
	parsed := netip.MustParseAddr(addr)
	if parsed.Is4() {
		return ARR(owner, addr)
	}
	return AAAARR(owner, addr)
}

// NSRR builds an NS record.
func NSRR(owner string, target string) *dns.NS {
	rr := &dns.NS{Hdr: header(owner)}
	rr.Ns = dnsutil.Fqdn(target)
	return rr
}

// NSRRs builds one NS record per target.
func NSRRs(owner string, targets ...string) []dns.RR {
	out := make([]dns.RR, 0, len(targets))
	for _, target := range targets {
		out = append(out, NSRR(owner, target))
	}
	return out
}

// SOAOpt overrides a field of the SOA record SOARR builds.
type SOAOpt func(*dns.SOA)

// MName sets the SOA primary nameserver.
func MName(name string) SOAOpt {
	return func(soa *dns.SOA) { soa.Ns = dnsutil.Fqdn(name) }
}

// RName sets the SOA responsible mailbox.
func RName(name string) SOAOpt {
	return func(soa *dns.SOA) { soa.Mbox = dnsutil.Fqdn(name) }
}

// Serial sets the SOA serial.
func Serial(v uint32) SOAOpt {
	return func(soa *dns.SOA) { soa.Serial = v }
}

// SOATimers sets the SOA refresh, retry, expire and minimum fields.
func SOATimers(refresh uint32, retry uint32, expire uint32, minttl uint32) SOAOpt {
	return func(soa *dns.SOA) {
		soa.Refresh = refresh
		soa.Retry = retry
		soa.Expire = expire
		soa.Minttl = minttl
	}
}

// SOARR builds an SOA record defaulting to ns/hostmaster under owner.
func SOARR(owner string, opts ...SOAOpt) *dns.SOA {
	soa := &dns.SOA{Hdr: header(owner)}
	soa.Ns = dnsutil.Fqdn("ns." + trimRoot(owner))
	soa.Mbox = dnsutil.Fqdn("hostmaster." + trimRoot(owner))
	soa.Serial = 1
	soa.Refresh = 3600
	soa.Retry = 600
	soa.Expire = 604800
	soa.Minttl = defaultTTL
	for _, opt := range opts {
		opt(soa)
	}
	return soa
}

// trimRoot maps the root zone to a label a name can be built on.
func trimRoot(owner string) string {
	if owner == "." || owner == "" {
		return "root"
	}
	return owner
}

// TXTRR builds a TXT record.
func TXTRR(owner string, values ...string) *dns.TXT {
	rr := &dns.TXT{Hdr: header(owner)}
	rr.Txt = values
	return rr
}

// PTRRR builds a PTR record.
func PTRRR(owner string, target string) *dns.PTR {
	rr := &dns.PTR{Hdr: header(owner)}
	rr.Ptr = dnsutil.Fqdn(target)
	return rr
}

// CNAMERR builds a CNAME record.
func CNAMERR(owner string, target string) *dns.CNAME {
	rr := &dns.CNAME{Hdr: header(owner)}
	rr.Target = dnsutil.Fqdn(target)
	return rr
}
