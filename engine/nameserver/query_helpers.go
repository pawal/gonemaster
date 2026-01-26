package nameserver

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/dnsname"
	"github.com/pawal/gonemaster/engine/packet"
)

// String returns the nameserver name and IP address.
func (ns Nameserver) String() string {
	return fmt.Sprintf("%s/%s", ns.Name.String(), ns.Address.String())
}

// AddFakeDelegation injects a delegation response for a domain.
func (ns *Nameserver) AddFakeDelegation(domain string, data map[string][]string) error {
	if ns == nil {
		return fmt.Errorf("nameserver is nil")
	}
	if ns.state == nil {
		ns.state = &nsState{
			cache:           cacheForAddress(ns.Address.String()),
			fakeDelegations: map[string]delegation{},
			fakeDS:          map[string][]dns.RR{},
			blacklisted:     map[bool]bool{},
		}
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
		authority = append(authority, &dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(domainName.String()),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			Ns: dns.Fqdn(nsObj.String()),
		})

		for _, ip := range ips {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				return fmt.Errorf("invalid delegation IP %q: %w", ip, err)
			}
			if addr == ns.Address {
				return nil
			}

			if addr.Is6() {
				additional = append(additional, &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   dns.Fqdn(nsObj.String()),
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    0,
					},
					AAAA: addr.AsSlice(),
				})
			} else {
				additional = append(additional, &dns.A{
					Hdr: dns.RR_Header{
						Name:   dns.Fqdn(nsObj.String()),
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    0,
					},
					A: addr.AsSlice(),
				})
			}
		}
	}

	ns.state.fakeDelegations[domainKey] = delegation{
		authority:  authority,
		additional: additional,
	}
	return nil
}

// AddFakeDS injects DS records for a domain.
func (ns *Nameserver) AddFakeDS(domain string, data []DSData) error {
	if ns == nil {
		return fmt.Errorf("nameserver is nil")
	}
	if ns.state == nil {
		ns.state = &nsState{
			cache:           cacheForAddress(ns.Address.String()),
			fakeDelegations: map[string]delegation{},
			fakeDS:          map[string][]dns.RR{},
			blacklisted:     map[bool]bool{},
		}
	}

	domainName := dnsname.New(domain)
	domainKey := strings.ToLower(domainName.String())
	var records []dns.RR
	for _, ds := range data {
		records = append(records, &dns.DS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(domainName.String()),
				Rrtype: dns.TypeDS,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			KeyTag:     ds.KeyTag,
			Algorithm:  ds.Algorithm,
			DigestType: ds.DigestType,
			Digest:     ds.Digest,
		})
	}
	ns.state.fakeDS[domainKey] = records
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

func (ns Nameserver) fakeDSResponse(name string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, bool) {
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
	msg.Question = []dns.Question{{
		Name:   dns.Fqdn(name),
		Qtype:  dns.TypeDS,
		Qclass: dns.ClassINET,
	}}
	msg.Answer = append(msg.Answer, records...)

	dnssec := resolveDNSSEC(opts)
	ednsSize := resolveEDNSSize(opts, dnssec)
	setResponseEDNS(msg, dnssec, ednsSize, opts)

	return packet.Packet{Msg: msg, AnswerFrom: ns.Address.String()}, true
}

func (ns Nameserver) fakeDelegationResponse(name string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, bool) {
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
		msg.Question = []dns.Question{{
			Name:   dns.Fqdn(name),
			Qtype:  dns.StringToType[qtype],
			Qclass: dns.StringToClass[qclass],
		}}

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

		return packet.Packet{Msg: msg, AnswerFrom: ns.Address.String()}, true
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

	msg.SetEdns0(size, dnssec)
	opt := msg.IsEdns0()
	if opt == nil || opts == nil || opts.EDNSDetails == nil {
		return
	}
	if opts.EDNSDetails.Do != nil {
		opt.SetDo(*opts.EDNSDetails.Do)
	}
	if opts.EDNSDetails.Version != nil {
		opt.SetVersion(*opts.EDNSDetails.Version)
	}
	if opts.EDNSDetails.Z != nil {
		opt.SetZ(*opts.EDNSDetails.Z)
	}
	if opts.EDNSDetails.Rcode != nil {
		opt.SetExtendedRcode(uint16(*opts.EDNSDetails.Rcode))
	}
	if len(opts.EDNSDetails.Data) > 0 {
		opt.Option = append(opt.Option, opts.EDNSDetails.Data...)
	}
}

func buildCacheKey(name string, qtype string, qclass string, opts *QueryOptions) (string, uint16, bool, error) {
	dnssec := resolveDNSSEC(opts)
	usevc := resolveUseVC(opts)
	recurse := resolveRecurse(opts)
	ednsSize := resolveEDNSSize(opts, dnssec)

	if ednsSize > 65535 {
		return "", 0, false, fmt.Errorf("edns_size must be between 0 and 65535")
	}

	nameObj := dnsname.New(name)
	parts := []string{
		"NAME=" + strings.ToLower(nameObj.String()),
		"TYPE=" + qtype,
		"CLASS=" + qclass,
		"DNSSEC=" + strconv.FormatBool(dnssec),
		"USEVC=" + strconv.FormatBool(usevc),
		"RECURSE=" + strconv.FormatBool(recurse),
	}

	if opts != nil && opts.EDNSDetails != nil {
		parts = append(parts,
			"EDNS_VERSION="+formatUint8(opts.EDNSDetails.Version),
			"EDNS_Z="+formatUint16(opts.EDNSDetails.Z),
			"EDNS_RCODE="+formatUint8(opts.EDNSDetails.Rcode),
			"EDNS_DATA="+formatEDNSData(opts.EDNSDetails.Data),
		)
	}
	parts = append(parts, "EDNS_SIZE="+strconv.FormatUint(uint64(ednsSize), 10))
	return strings.Join(parts, "|"), ednsSize, dnssec, nil
}

func formatUint8(value *uint8) string {
	if value == nil {
		return "0"
	}
	return strconv.FormatUint(uint64(*value), 10)
}

func formatUint16(value *uint16) string {
	if value == nil {
		return "0"
	}
	return strconv.FormatUint(uint64(*value), 10)
}

func formatEDNSData(data []dns.EDNS0) string {
	if len(data) == 0 {
		return ""
	}
	return fmt.Sprint(data)
}
