package nameserver

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

var cacheKeyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 256)
		return &buf
	},
}

// String returns the nameserver name and IP address.
func (ns Nameserver) String() string {
	return fmt.Sprintf("%s/%s", ns.Name.String(), ns.Address.String())
}

// AddFakeDelegation injects a delegation response for a domain.
func (ns *Nameserver) AddFakeDelegation(domain string, data map[string][]string) error {
	if ns == nil {
		return fmt.Errorf("nameserver is nil")
	}
	ns.ensureState()
	if ns.state.fakeDelegations == nil {
		ns.state.fakeDelegations = map[string]delegation{}
	}
	if ns.state.fakeDS == nil {
		ns.state.fakeDS = map[string][]dns.RR{}
	}
	if ns.state.blacklisted == nil {
		ns.state.blacklisted = map[bool]bool{}
	}

	domainName := dnsname.New(domain)
	domainKey := strings.ToLower(domainName.String())
	if data == nil {
		data = map[string][]string{}
	}

	var authority []dns.RR
	var additional []dns.RR
	for nsName, ips := range data {
		nsObj := dnsname.New(nsName)
		nsRR := &dns.NS{Hdr: dns.Header{
			Name:  dnsutil.Fqdn(domainName.String()),
			Class: dns.ClassINET,
		}}
		nsRR.Ns = dnsutil.Fqdn(nsObj.String())
		authority = append(authority, nsRR)

		for _, ip := range ips {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				return fmt.Errorf("invalid delegation IP %q: %w", ip, err)
			}
			if addr == ns.Address {
				logArgs := map[string]any{
					"domain": domainKey,
					"data":   data,
				}
				logargs.SetNS(logArgs, ns.NameString(), ns.AddressString())
				logSystemWithLogger(ns.log, "FAKE_DELEGATION_TO_SELF", logArgs)
				return nil
			}

			if addr.Is6() {
				aaaa := &dns.AAAA{Hdr: dns.Header{
					Name:  dnsutil.Fqdn(nsObj.String()),
					Class: dns.ClassINET,
				}}
				aaaa.Addr = addr
				additional = append(additional, aaaa)
			} else {
				a := &dns.A{Hdr: dns.Header{
					Name:  dnsutil.Fqdn(nsObj.String()),
					Class: dns.ClassINET,
				}}
				a.Addr = addr
				additional = append(additional, a)
			}
		}
	}

	ns.state.fakeDelegations[domainKey] = delegation{
		authority:  authority,
		additional: additional,
	}
	logArgs := map[string]any{
		"domain": domainKey,
		"data":   data,
	}
	logargs.SetNS(logArgs, ns.NameString(), ns.AddressString())
	logSystemWithLogger(ns.log, "FAKE_DELEGATION_ADDED", logArgs)
	return nil
}

// AddFakeDS injects DS records for a domain.
func (ns *Nameserver) AddFakeDS(domain string, data []DSData) error {
	if ns == nil {
		return fmt.Errorf("nameserver is nil")
	}
	ns.ensureState()
	if ns.state.fakeDelegations == nil {
		ns.state.fakeDelegations = map[string]delegation{}
	}
	if ns.state.fakeDS == nil {
		ns.state.fakeDS = map[string][]dns.RR{}
	}
	if ns.state.blacklisted == nil {
		ns.state.blacklisted = map[bool]bool{}
	}

	domainName := dnsname.New(domain)
	domainKey := strings.ToLower(domainName.String())
	var records []dns.RR
	for _, ds := range data {
		dsRR := &dns.DS{Hdr: dns.Header{
			Name:  dnsutil.Fqdn(domainName.String()),
			Class: dns.ClassINET,
		}}
		dsRR.KeyTag = ds.KeyTag
		dsRR.Algorithm = ds.Algorithm
		dsRR.DigestType = ds.DigestType
		dsRR.Digest = ds.Digest
		records = append(records, dsRR)
	}
	ns.state.fakeDS[domainKey] = records
	logArgs := map[string]any{
		"domain": domainKey,
		"data":   data,
	}
	logargs.SetNS(logArgs, ns.NameString(), ns.AddressString())
	logSystemWithLogger(ns.log, "FAKE_DS_ADDED", logArgs)
	return nil
}

// FakeDSRecords returns any fake DS records for a domain.
func (ns Nameserver) FakeDSRecords(domain string) []dns.RR {
	if ns.state == nil {
		return nil
	}
	domainName := dnsname.New(domain)
	domainKey := strings.ToLower(domainName.String())
	records := ns.state.fakeDS[domainKey]
	if len(records) == 0 {
		return nil
	}
	out := make([]dns.RR, len(records))
	copy(out, records)
	return out
}

func (ns Nameserver) fakeDSResponse(name string, qtype string, qclass string, opts *QueryOptions, runLog *logger.Logger) (packet.Packet, bool) {
	if ns.state == nil || qtype != "DS" || qclass != "IN" {
		return packet.Packet{}, false
	}
	nameObj := dnsname.New(name)
	key := strings.ToLower(nameObj.String())
	records := ns.state.fakeDS[key]
	if len(records) == 0 {
		return packet.Packet{}, false
	}

	msg := new(dns.Msg)
	msg.Response = true
	msg.Authoritative = true
	msg.RecursionDesired = resolveRecurse(opts)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(name), dns.TypeDS)
	msg.Answer = append(msg.Answer, records...)

	dnssec := resolveDNSSEC(opts)
	ednsSize := resolveEDNSSize(opts, dnssec)
	setResponseEDNS(msg, dnssec, ednsSize, opts)

	resp := packet.Packet{Msg: msg, AnswerFrom: ns.Address.String(), Log: runLog}
	logArgs := map[string]any{
		"query_name":  nameObj.String(),
		"query_type":  qtype,
		"query_class": qclass,
		"from":        ns.String(),
	}
	logargs.SetNS(logArgs, ns.NameString(), ns.AddressString())
	logSystemWithLogger(runLog, "FAKE_DS_RETURNED", logArgs)
	if runLog.Wants(systemModuleName, "FAKE_PACKET_RETURNED") {
		logSystemWithLogger(runLog, "FAKE_PACKET_RETURNED", map[string]any{
			"packet": resp.String(),
		})
	}
	return resp, true
}

func (ns Nameserver) fakeDelegationResponse(name string, qtype string, qclass string, opts *QueryOptions, runLog *logger.Logger) (packet.Packet, bool) {
	if ns.state == nil {
		return packet.Packet{}, false
	}

	nameObj := dnsname.New(name)
	nameKey := strings.ToLower(nameObj.String())
	keys := make([]string, 0, len(ns.state.fakeDelegations))
	for key := range ns.state.fakeDelegations {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if !matchesZone(nameKey, key) {
			continue
		}
		del := ns.state.fakeDelegations[key]
		msg := new(dns.Msg)
		msg.Response = true
		msg.Authoritative = qtype == "DS"
		msg.RecursionDesired = resolveRecurse(opts)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(name), dns.StringToType[qtype])
		msg.Question[0].Header().Class = dns.StringToClass[qclass]

		if strings.EqualFold(nameKey, key) && qtype == "NS" {
			msg.Answer = append(msg.Answer, del.authority...)
			msg.Extra = append(msg.Extra, del.additional...)
		} else if qtype != "DS" {
			msg.Ns = append(msg.Ns, del.authority...)
			msg.Extra = append(msg.Extra, del.additional...)
		}

		dnssec := resolveDNSSEC(opts)
		ednsSize := resolveEDNSSize(opts, dnssec)
		setResponseEDNS(msg, dnssec, ednsSize, opts)

		resp := packet.Packet{Msg: msg, AnswerFrom: ns.Address.String(), Log: runLog}
		logArgs := map[string]any{
			"query_name":  nameObj.String(),
			"query_type":  qtype,
			"query_class": qclass,
			"from":        ns.String(),
		}
		logargs.SetNS(logArgs, ns.NameString(), ns.AddressString())
		logSystemWithLogger(runLog, "FAKE_DELEGATION_RETURNED", logArgs)
		if runLog.Wants(systemModuleName, "FAKE_PACKET_RETURNED") {
			logSystemWithLogger(runLog, "FAKE_PACKET_RETURNED", map[string]any{
				"packet": resp.String(),
			})
		}
		return resp, true
	}
	return packet.Packet{}, false
}

func matchesZone(name string, zone string) bool {
	if name == zone {
		return true
	}
	return strings.HasSuffix(name, "."+zone)
}

func resolveDNSSEC(opts *QueryOptions) bool {
	if opts != nil && opts.EDNSDetails != nil && opts.EDNSDetails.Do != nil {
		return *opts.EDNSDetails.Do
	}
	if opts != nil && opts.DNSSEC != nil {
		return *opts.DNSSEC
	}
	return false
}

func resolveUseVC(opts *QueryOptions) bool {
	if opts != nil && opts.UseVC != nil {
		return *opts.UseVC
	}
	return false
}

func resolveRecurse(opts *QueryOptions) bool {
	if opts != nil && opts.Recurse != nil {
		return *opts.Recurse
	}
	return false
}

func setResponseEDNS(msg *dns.Msg, dnssec bool, size uint16, opts *QueryOptions) {
	if msg == nil {
		return
	}
	if size == 0 && opts == nil {
		return
	}

	msg.UDPSize = size
	msg.Security = dnssec

	if opts == nil || opts.EDNSDetails == nil {
		return
	}
	if opts.EDNSDetails.Do != nil {
		msg.Security = *opts.EDNSDetails.Do
	}
	if opts.EDNSDetails.Version != nil {
		msg.Version = *opts.EDNSDetails.Version
	}
	if opts.EDNSDetails.Rcode != nil {
		msg.Rcode = uint16(*opts.EDNSDetails.Rcode)
	}
	for _, opt := range opts.EDNSDetails.Data {
		msg.Pseudo = append(msg.Pseudo, opt)
	}
	if opts.EDNSDetails.Z != nil {
		setMessageEDNSZ(msg, *opts.EDNSDetails.Z)
	}
}

func setMessageEDNSZ(msg *dns.Msg, z uint16) {
	if msg == nil {
		return
	}

	opt := &dns.OPT{Hdr: dns.Header{Name: "."}}
	for _, rr := range msg.Pseudo {
		edns, ok := rr.(dns.EDNS0)
		if !ok {
			return
		}
		opt.Options = append(opt.Options, edns)
	}

	udpSize := max(msg.UDPSize, dns.MinMsgSize)
	opt.SetUDPSize(udpSize)
	opt.SetVersion(msg.Version)
	opt.SetSecurity(msg.Security)
	opt.SetCompactAnswers(msg.CompactAnswers)
	opt.SetDelegation(msg.Delegation)
	opt.SetRcode(msg.Rcode)
	opt.SetZ(z)

	extra := make([]dns.RR, 0, len(msg.Extra)+1)
	for _, rr := range msg.Extra {
		if _, isOPT := rr.(*dns.OPT); isOPT {
			continue
		}
		extra = append(extra, rr)
	}
	extra = append(extra, opt)
	msg.Extra = extra

	msg.Pseudo = nil
	msg.UDPSize = 0
	msg.Security = false
	msg.CompactAnswers = false
	msg.Delegation = false
	msg.Version = 0
	msg.Rcode &= 0xF
}

func buildCacheKey(name string, qtype string, qclass string, opts *QueryOptions) (string, uint16, bool, error) {
	dnssec := resolveDNSSEC(opts)
	usevc := resolveUseVC(opts)
	recurse := resolveRecurse(opts)
	ednsSize := resolveEDNSSize(opts, dnssec)

	nameObj := dnsname.New(name)
	bufPtr := cacheKeyBufferPool.Get().(*[]byte)
	buf := (*bufPtr)[:0]
	defer putCacheKeyBuffer(bufPtr, buf)

	buf = appendCacheKeyString(buf, "NAME", nameObj.String())
	buf = appendCacheKeyString(buf, "TYPE", qtype)
	buf = appendCacheKeyString(buf, "CLASS", qclass)
	buf = appendCacheKeyBool(buf, "DNSSEC", dnssec)
	buf = appendCacheKeyBool(buf, "USEVC", usevc)
	buf = appendCacheKeyBool(buf, "RECURSE", recurse)

	// Transport overrides change what an answer means. Only explicit ones are
	// keyed, so default queries keep their existing key.
	if opts != nil {
		if opts.Fallback != nil {
			buf = appendCacheKeyBool(buf, "FALLBACK", *opts.Fallback)
		}
		if opts.Retry != nil {
			buf = appendCacheKeyInt(buf, "RETRY", int64(*opts.Retry))
		}
		if opts.Retrans != nil {
			buf = appendCacheKeyInt(buf, "RETRANS", int64(*opts.Retrans))
		}
		if opts.Timeout != nil {
			buf = appendCacheKeyInt(buf, "TIMEOUT", int64(*opts.Timeout))
		}
		if opts.CheckingDisabled != nil {
			buf = appendCacheKeyBool(buf, "CD", *opts.CheckingDisabled)
		}
	}

	if opts != nil && opts.EDNSDetails != nil {
		buf = appendCacheKeyUint8Ptr(buf, "EDNS_VERSION", opts.EDNSDetails.Version)
		buf = appendCacheKeyUint16Ptr(buf, "EDNS_Z", opts.EDNSDetails.Z)
		buf = appendCacheKeyUint8Ptr(buf, "EDNS_RCODE", opts.EDNSDetails.Rcode)
		buf = appendCacheKeyString(buf, "EDNS_DATA", formatEDNSData(opts.EDNSDetails.Data))
	}
	buf = appendCacheKeyUint(buf, "EDNS_SIZE", uint64(ednsSize))
	return string(buf), ednsSize, dnssec, nil
}

func putCacheKeyBuffer(bufPtr *[]byte, buf []byte) {
	if cap(buf) > 4096 {
		*bufPtr = make([]byte, 0, 256)
		cacheKeyBufferPool.Put(bufPtr)
		return
	}
	*bufPtr = buf[:0]
	cacheKeyBufferPool.Put(bufPtr)
}

func appendCacheKeyPrefix(buf []byte, key string) []byte {
	if len(buf) > 0 {
		buf = append(buf, '|')
	}
	buf = append(buf, key...)
	buf = append(buf, '=')
	return buf
}

func appendCacheKeyString(buf []byte, key string, value string) []byte {
	buf = appendCacheKeyPrefix(buf, key)
	buf = append(buf, value...)
	return buf
}

func appendCacheKeyBool(buf []byte, key string, value bool) []byte {
	buf = appendCacheKeyPrefix(buf, key)
	return strconv.AppendBool(buf, value)
}

func appendCacheKeyUint(buf []byte, key string, value uint64) []byte {
	buf = appendCacheKeyPrefix(buf, key)
	return strconv.AppendUint(buf, value, 10)
}

func appendCacheKeyInt(buf []byte, key string, value int64) []byte {
	buf = appendCacheKeyPrefix(buf, key)
	return strconv.AppendInt(buf, value, 10)
}

func appendCacheKeyUint8Ptr(buf []byte, key string, value *uint8) []byte {
	if value == nil {
		return appendCacheKeyUint(buf, key, 0)
	}
	return appendCacheKeyUint(buf, key, uint64(*value))
}

func appendCacheKeyUint16Ptr(buf []byte, key string, value *uint16) []byte {
	if value == nil {
		return appendCacheKeyUint(buf, key, 0)
	}
	return appendCacheKeyUint(buf, key, uint64(*value))
}

func formatEDNSData(data []dns.EDNS0) string {
	if len(data) == 0 {
		return ""
	}
	return fmt.Sprint(data)
}
