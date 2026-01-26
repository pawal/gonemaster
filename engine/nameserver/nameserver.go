package nameserver

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/constants"
	"github.com/pawal/gonemaster/engine/dnsname"
	"github.com/pawal/gonemaster/engine/packet"
	"github.com/pawal/gonemaster/engine/profile"
	"github.com/pawal/gonemaster/engine/transport"
)

// Nameserver represents a DNS server endpoint.
type Nameserver struct {
	Name    dnsname.Name
	Address netip.Addr
	Client  *transport.Client
	state   *nsState
}

// QueryOptions configures per-query settings that mirror Perl flags.
type QueryOptions struct {
	Class                string
	UseVC                *bool
	Recurse              *bool
	Fallback             *bool
	Retry                *int
	Retrans              *time.Duration
	Timeout              *time.Duration
	DNSSEC               *bool
	EDNSSize             *uint16
	EDNSDetails          *transport.EDNSDetails
	BlacklistingDisabled bool
}

// New creates a Nameserver from a name and IP address.
func New(name string, address string, client *transport.Client) (Nameserver, error) {
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return Nameserver{}, fmt.Errorf("invalid nameserver address %q: %w", address, err)
	}

	if client == nil {
		client = &transport.Client{}
	}

	nameKey := strings.ToLower(name)
	if name == "" {
		nameKey = "$$$noname"
	}
	nameObj := dnsname.New(nameKey)
	nameKey = strings.ToLower(nameObj.String())

	addrKey := addr.String()
	if cached := cachedNameserver(nameKey, addrKey); cached != nil {
		return *cached, nil
	}

	state := &nsState{
		cache:           cacheForAddress(addrKey),
		fakeDelegations: map[string]delegation{},
		fakeDS:          map[string][]dns.RR{},
		blacklisted:     map[bool]bool{},
	}

	ns := &Nameserver{
		Name:    nameObj,
		Address: addr,
		Client:  client,
		state:   state,
	}
	storeNameserver(nameKey, addrKey, ns)
	return *ns, nil
}

// Query sends a DNS query using IN class.
func (ns Nameserver) Query(ctx context.Context, qname string, qtype string) (packet.Packet, error) {
	return ns.QueryWithOptions(ctx, qname, qtype, nil)
}

// QueryWithClass sends a DNS query using the given class.
func (ns Nameserver) QueryWithClass(ctx context.Context, qname string, qtype string, qclass string) (packet.Packet, error) {
	opts := &QueryOptions{Class: qclass}
	return ns.QueryWithOptions(ctx, qname, qtype, opts)
}

// QueryWithOptions sends a DNS query using per-query options.
func (ns Nameserver) QueryWithOptions(ctx context.Context, qname string, qtype string, opts *QueryOptions) (packet.Packet, error) {
	if ns.Client == nil {
		return packet.Packet{}, fmt.Errorf("missing transport client")
	}

	if qtype == "" {
		qtype = "A"
	}
	qtype = strings.ToUpper(qtype)

	qclass := "IN"
	if opts != nil && opts.Class != "" {
		qclass = opts.Class
	}
	qclass = strings.ToUpper(qclass)

	prof := profile.Effective()
	if ns.Address.Is4() && !prof.Net.IPv4 {
		return packet.Packet{}, nil
	}
	if ns.Address.Is6() && !prof.Net.IPv6 {
		return packet.Packet{}, nil
	}

	if resp, ok := ns.fakeDSResponse(qname, qtype, qclass, opts); ok {
		return resp, nil
	}
	if resp, ok := ns.fakeDelegationResponse(qname, qtype, qclass, opts); ok {
		return resp, nil
	}

	cacheKey, ednsSize, _, err := buildCacheKey(qname, qtype, qclass, opts)
	if err != nil {
		return packet.Packet{}, err
	}
	if ns.state != nil {
		if cached, ok := ns.state.cache.get(cacheKey); ok {
			if cached == nil {
				return packet.Packet{}, nil
			}
			return *cached, nil
		}
	}

	if prof.NoNetwork {
		return packet.Packet{}, fmt.Errorf("external query for %s %s attempted to %s while running with no_network", qname, qtype, ns.String())
	}

	usevc := resolveUseVC(opts)
	if constants.BlacklistingEnabled && ns.state != nil && ns.state.blacklisted[usevc] {
		return packet.Packet{}, nil
	}

	resp, err := ns.queryNetwork(ctx, qname, qtype, qclass, opts)

	blacklistingDisabled := opts != nil && opts.BlacklistingDisabled
	if err != nil && qtype == "SOA" && ednsSize == 0 && !blacklistingDisabled {
		if ns.state != nil {
			ns.state.blacklisted[usevc] = true
		}
	}

	if ns.state != nil {
		if resp.Msg == nil {
			ns.state.cache.set(cacheKey, nil)
		} else {
			copyResp := resp
			ns.state.cache.set(cacheKey, &copyResp)
		}
	}
	return resp, err
}

func (ns Nameserver) queryNetwork(ctx context.Context, qname string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error) {
	if ns.state != nil && ns.state.queryFunc != nil {
		return ns.state.queryFunc(ctx, qname, qtype, qclass, opts)
	}

	client, err := ns.clientForOptions(opts)
	if err != nil {
		return packet.Packet{}, err
	}

	qtypeID, ok := dns.StringToType[qtype]
	if !ok {
		return packet.Packet{}, fmt.Errorf("unknown query type %q", qtype)
	}

	qclassID, ok := dns.StringToClass[qclass]
	if !ok {
		return packet.Packet{}, fmt.Errorf("unknown query class %q", qclass)
	}

	msg := transport.BuildQueryWithClass(qname, qtypeID, qclassID)
	server := ns.Address.String()
	return client.Exchange(ctx, server, msg)
}

func (ns Nameserver) clientForOptions(opts *QueryOptions) (*transport.Client, error) {
	base := transport.Client{}
	if ns.Client != nil {
		base = *ns.Client
	}

	base.SetUseTCP(resolveUseVC(opts))
	base.SetRecursionDesired(resolveRecurse(opts))

	if opts != nil {
		if opts.Fallback != nil {
			base.SetFallback(*opts.Fallback)
		}
		if opts.Retry != nil {
			base.SetRetries(*opts.Retry)
		}
		if opts.Retrans != nil {
			base.SetRetrans(*opts.Retrans)
		}
		if opts.Timeout != nil {
			base.SetTimeout(*opts.Timeout)
		}
		if opts.DNSSEC != nil {
			base.DNSSEC = *opts.DNSSEC
		}
	}

	dnssec := base.DNSSEC
	if opts != nil && opts.EDNSDetails != nil && opts.EDNSDetails.Do != nil {
		dnssec = *opts.EDNSDetails.Do
		base.DNSSEC = dnssec
	}

	if opts != nil && opts.EDNSDetails != nil {
		ednsSize := resolveEDNSSize(opts, dnssec)
		base.SetEDNSSize(ednsSize)
		base.EDNSDetails = opts.EDNSDetails
	} else if opts != nil && opts.EDNSSize != nil {
		base.SetEDNSSize(*opts.EDNSSize)
	} else if dnssec {
		base.SetEDNSSize(constants.EDNSUDPPayloadDNSSECDefault)
	}

	base.ApplyProfileDefaults(nil)
	return &base, nil
}

func resolveEDNSSize(opts *QueryOptions, dnssec bool) uint16 {
	if opts != nil && opts.EDNSDetails != nil {
		if opts.EDNSDetails.Size != nil {
			return *opts.EDNSDetails.Size
		}
		if opts.EDNSSize != nil {
			return *opts.EDNSSize
		}
		if dnssec {
			return constants.EDNSUDPPayloadDNSSECDefault
		}
		return constants.EDNSUDPPayloadDefault
	}
	if opts != nil && opts.EDNSSize != nil {
		return *opts.EDNSSize
	}
	if dnssec {
		return constants.EDNSUDPPayloadDNSSECDefault
	}
	return 0
}
