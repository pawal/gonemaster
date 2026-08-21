package tctest

import (
	"crypto"
	"net/netip"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// defaultTTL is the record TTL every fixture uses unless TTL overrides it.
const defaultTTL = 60

// MsgOpt shapes the response Response builds.
type MsgOpt func(*dns.Msg)

// Response builds an authoritative NOERROR response and applies the options.
func Response(opts ...MsgOpt) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	for _, opt := range opts {
		opt(msg)
	}
	return packet.Packet{Msg: msg}
}

// Question sets the question section.
func Question(name string, qtype uint16) MsgOpt {
	return func(msg *dns.Msg) { dnsutil.SetQuestion(msg, dnsutil.Fqdn(name), qtype) }
}

// Answers appends records to the answer section.
func Answers(rrs ...dns.RR) MsgOpt {
	return func(msg *dns.Msg) { msg.Answer = append(msg.Answer, rrs...) }
}

// Authority appends records to the authority section.
func Authority(rrs ...dns.RR) MsgOpt {
	return func(msg *dns.Msg) { msg.Ns = append(msg.Ns, rrs...) }
}

// Additional appends records to the additional section.
func Additional(rrs ...dns.RR) MsgOpt {
	return func(msg *dns.Msg) { msg.Extra = append(msg.Extra, rrs...) }
}

// Rcode sets the response code.
func Rcode(rcode uint16) MsgOpt {
	return func(msg *dns.Msg) { msg.Rcode = rcode }
}

// NXDOMAIN sets the NXDOMAIN response code.
func NXDOMAIN() MsgOpt {
	return Rcode(dns.RcodeNameError)
}

// NotAuthoritative clears the AA bit, as a referral has it clear.
func NotAuthoritative() MsgOpt {
	return func(msg *dns.Msg) { msg.Authoritative = false }
}

// Secure marks the response as a DNSSEC-aware reply.
func Secure() MsgOpt {
	return func(msg *dns.Msg) {
		msg.Response = true
		msg.UDPSize = 1232
		msg.Security = true
	}
}

// TTL returns the records with their TTL set.
func TTL(ttl uint32, rrs ...dns.RR) []dns.RR {
	for _, rr := range rrs {
		rr.Header().TTL = ttl
	}
	return rrs
}

func header(owner string) dns.Header {
	return dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: defaultTTL}
}

// SOAOpt overrides one SOA field.
type SOAOpt func(*dns.SOA)

// MName sets the SOA primary server.
func MName(name string) SOAOpt {
	return func(soa *dns.SOA) { soa.Ns = dnsutil.Fqdn(name) }
}

// RName sets the SOA mailbox.
func RName(name string) SOAOpt {
	return func(soa *dns.SOA) { soa.Mbox = dnsutil.Fqdn(name) }
}

// Serial sets the SOA serial.
func Serial(v uint32) SOAOpt {
	return func(soa *dns.SOA) { soa.Serial = v }
}

// Refresh sets the SOA refresh timer.
func Refresh(v uint32) SOAOpt {
	return func(soa *dns.SOA) { soa.Refresh = v }
}

// Retry sets the SOA retry timer.
func Retry(v uint32) SOAOpt {
	return func(soa *dns.SOA) { soa.Retry = v }
}

// Expire sets the SOA expire timer.
func Expire(v uint32) SOAOpt {
	return func(soa *dns.SOA) { soa.Expire = v }
}

// Minttl sets the SOA negative-caching TTL.
func Minttl(v uint32) SOAOpt {
	return func(soa *dns.SOA) { soa.Minttl = v }
}

// SOARR builds a SOA record with the standard test timers.
func SOARR(owner string, opts ...SOAOpt) *dns.SOA {
	soa := &dns.SOA{Hdr: header(owner)}
	soa.Ns = dnsutil.Fqdn("ns1." + trimRoot(owner))
	soa.Mbox = dnsutil.Fqdn("hostmaster." + trimRoot(owner))
	soa.Serial = 1
	soa.Refresh = 3600
	soa.Retry = 600
	soa.Expire = 86400
	soa.Minttl = defaultTTL
	for _, opt := range opts {
		opt(soa)
	}
	return soa
}

// trimRoot makes owner usable as a label prefix, root included.
func trimRoot(owner string) string {
	if owner == "." || owner == "" {
		return "root"
	}
	return owner
}

// NSRR builds an NS record.
func NSRR(owner string, target string) *dns.NS {
	ns := &dns.NS{Hdr: header(owner)}
	ns.Ns = dnsutil.Fqdn(target)
	return ns
}

// NSRRs builds one NS record per target.
func NSRRs(owner string, targets ...string) []dns.RR {
	rrs := make([]dns.RR, 0, len(targets))
	for _, target := range targets {
		rrs = append(rrs, NSRR(owner, target))
	}
	return rrs
}

// ARR builds an A record.
func ARR(owner string, addr string) *dns.A {
	rr := &dns.A{Hdr: header(owner)}
	rr.Addr = mustAddr(addr)
	return rr
}

// AAAARR builds an AAAA record.
func AAAARR(owner string, addr string) *dns.AAAA {
	rr := &dns.AAAA{Hdr: header(owner)}
	rr.Addr = mustAddr(addr)
	return rr
}

// AddrRR builds an A or AAAA record, matching the address family.
func AddrRR(owner string, addr string) dns.RR {
	if mustAddr(addr).Is4() {
		return ARR(owner, addr)
	}
	return AAAARR(owner, addr)
}

// TXTRR builds a TXT record.
func TXTRR(owner string, values ...string) *dns.TXT {
	rr := &dns.TXT{Hdr: header(owner)}
	rr.Txt = values
	return rr
}

// MXRR builds an MX record.
func MXRR(owner string, preference uint16, exchange string) *dns.MX {
	rr := &dns.MX{Hdr: header(owner)}
	rr.Preference = preference
	rr.Mx = dnsutil.Fqdn(exchange)
	return rr
}

// CNAMERR builds a CNAME record.
func CNAMERR(owner string, target string) *dns.CNAME {
	rr := &dns.CNAME{Hdr: header(owner)}
	rr.Target = dnsutil.Fqdn(target)
	return rr
}

// DNAMERR builds a DNAME record.
func DNAMERR(owner string, target string) *dns.DNAME {
	rr := &dns.DNAME{Hdr: header(owner)}
	rr.Target = dnsutil.Fqdn(target)
	return rr
}

// PTRRR builds a PTR record.
func PTRRR(owner string, target string) *dns.PTR {
	rr := &dns.PTR{Hdr: header(owner)}
	rr.Ptr = dnsutil.Fqdn(target)
	return rr
}

// KeyOpt overrides one DNSKEY field.
type KeyOpt func(*dns.DNSKEY)

// SEP sets the secure-entry-point flag alongside the zone flag.
func SEP() KeyOpt {
	return func(key *dns.DNSKEY) { key.Flags = dns.FlagZONE | dns.FlagSEP }
}

// Flags replaces the DNSKEY flags.
func Flags(flags uint16) KeyOpt {
	return func(key *dns.DNSKEY) { key.Flags = flags }
}

// KeyTTL sets the DNSKEY TTL.
func KeyTTL(ttl uint32) KeyOpt {
	return func(key *dns.DNSKEY) { key.Hdr.TTL = ttl }
}

// PublicKey sets the base64 DNSKEY public key.
func PublicKey(value string) KeyOpt {
	return func(key *dns.DNSKEY) { key.PublicKey = value }
}

// DNSKEYRR builds a zone DNSKEY for the algorithm.
func DNSKEYRR(owner string, algo uint8, opts ...KeyOpt) *dns.DNSKEY {
	key := &dns.DNSKEY{Hdr: header(owner)}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = algo
	for _, opt := range opts {
		opt(key)
	}
	return key
}

// DSRR builds a DS record.
func DSRR(owner string, keytag uint16, algo uint8, digestType uint8, digest string) *dns.DS {
	ds := &dns.DS{Hdr: header(owner)}
	ds.KeyTag = keytag
	ds.Algorithm = algo
	ds.DigestType = digestType
	ds.Digest = digest
	return ds
}

// SigOpt overrides one RRSIG field.
type SigOpt func(*dns.RRSIG)

// Inception sets the signature inception time.
func Inception(at time.Time) SigOpt {
	return func(sig *dns.RRSIG) { sig.Inception = uint32(at.Unix()) }
}

// Expiration sets the signature expiration time.
func Expiration(at time.Time) SigOpt {
	return func(sig *dns.RRSIG) { sig.Expiration = uint32(at.Unix()) }
}

// KeyTag sets the signature key tag.
func KeyTag(keytag uint16) SigOpt {
	return func(sig *dns.RRSIG) { sig.KeyTag = keytag }
}

// SigTTL sets the RRSIG TTL.
func SigTTL(ttl uint32) SigOpt {
	return func(sig *dns.RRSIG) { sig.Hdr.TTL = ttl }
}

// SigAlgo sets the signature algorithm.
func SigAlgo(algo uint8) SigOpt {
	return func(sig *dns.RRSIG) { sig.Algorithm = algo }
}

// Signer sets the RRSIG signer name.
func Signer(name string) SigOpt {
	return func(sig *dns.RRSIG) { sig.SignerName = dnsutil.Fqdn(name) }
}

// RRSIGRR builds an unsigned RRSIG covering qtype, valid around now.
func RRSIGRR(owner string, typeCovered uint16, opts ...SigOpt) *dns.RRSIG {
	now := time.Now().UTC()
	sig := &dns.RRSIG{Hdr: header(owner)}
	sig.TypeCovered = typeCovered
	sig.Algorithm = dns.RSASHA256
	sig.Labels = uint8(dnsutil.Labels(dnsutil.Fqdn(owner)))
	sig.OrigTTL = defaultTTL
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(24 * time.Hour).Unix())
	sig.SignerName = dnsutil.Fqdn(owner)
	for _, opt := range opts {
		opt(sig)
	}
	return sig
}

// SignedKey generates a real key pair for the algorithm.
func SignedKey(t TB, owner string, algo uint8, opts ...KeyOpt) (*dns.DNSKEY, crypto.Signer) {
	t.Helper()
	key := DNSKEYRR(owner, algo, opts...)
	priv, err := key.Generate(256)
	if err != nil {
		t.Fatalf("generate %d key: %v", algo, err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("private key for algorithm %d is not a crypto.Signer", algo)
	}
	return key, signer
}

// Sign returns an RRSIG over rrset that really verifies against the key.
func Sign(t TB, key *dns.DNSKEY, signer crypto.Signer, typeCovered uint16, rrset []dns.RR, opts ...SigOpt) *dns.RRSIG {
	t.Helper()
	if len(rrset) == 0 {
		t.Fatalf("cannot sign an empty rrset")
	}
	sig := RRSIGRR(rrset[0].Header().Name, typeCovered,
		SigAlgo(key.Algorithm), KeyTag(key.KeyTag()), Signer(key.Hdr.Name),
		SigTTL(key.Hdr.TTL))
	sig.OrigTTL = rrset[0].Header().TTL
	for _, opt := range opts {
		opt(sig)
	}
	if err := sig.Sign(signer, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign rrset: %v", err)
	}
	return sig
}

// NSItem builds a discovery item; an empty address means unresolved.
func NSItem(name string, address string) nsdiscovery.NSItem {
	item := nsdiscovery.NSItem{Name: dnsname.New(name)}
	if address != "" {
		item.Address = netip.MustParseAddr(address)
		item.HasAddress = true
	}
	return item
}

// NSItems builds discovery items from "name" or "name/address" specs.
func NSItems(specs ...string) []nsdiscovery.NSItem {
	items := make([]nsdiscovery.NSItem, 0, len(specs))
	for _, spec := range specs {
		name, address, _ := strings.Cut(spec, "/")
		items = append(items, NSItem(name, address))
	}
	return items
}

func mustAddr(addr string) netip.Addr {
	return netip.MustParseAddr(addr)
}
