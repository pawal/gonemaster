package zone

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/parallel"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

// Zone represents a DNS zone under test.
type Zone struct {
	Name     dnsname.Name
	recursor *recursor.Recursor

	parent    *Zone
	parentSet bool

	glueNames    []dnsname.Name
	glueNamesSet bool

	glue    []nameserver.Nameserver
	glueSet bool

	nsNames    []dnsname.Name
	nsNamesSet bool

	ns    []nameserver.Nameserver
	nsSet bool

	glueAddresses    []dns.RR
	glueAddressesSet bool
}

// New creates a Zone from a domain name.
func New(name string) (Zone, error) {
	return NewWithRecursor(name, nil)
}

// NewWithRecursor creates a Zone with an explicit recursor.
func NewWithRecursor(name string, r *recursor.Recursor) (Zone, error) {
	if name == "" {
		return Zone{}, fmt.Errorf("zone name must not be empty")
	}
	if r == nil {
		var err error
		r, err = recursor.New()
		if err != nil {
			return Zone{}, err
		}
	}
	return Zone{Name: dnsname.New(name), recursor: r}, nil
}

// Recursor returns the recursor associated with the zone.
func (z *Zone) Recursor() *recursor.Recursor {
	if z == nil {
		return nil
	}
	return z.recursor
}

// Parent returns the parent zone for this zone.
func (z *Zone) Parent(ctx context.Context) (*Zone, error) {
	if z.parentSet {
		return z.parent, nil
	}
	if z.Name.String() == "." {
		z.parent = z
		z.parentSet = true
		return z.parent, nil
	}
	if z.recursor == nil {
		return nil, fmt.Errorf("missing recursor")
	}

	pname, _, err := z.recursor.Parent(ctx, z.Name.String())
	if err != nil {
		return nil, err
	}
	if pname == "" {
		z.parentSet = true
		return nil, nil
	}
	parentZone, err := NewWithRecursor(pname, z.recursor)
	if err != nil {
		return nil, err
	}
	z.parent = &parentZone
	z.parentSet = true
	return z.parent, nil
}

// GlueNames returns glue NS names from the parent zone.
func (z *Zone) GlueNames(ctx context.Context) ([]dnsname.Name, error) {
	if z.glueNamesSet {
		return append([]dnsname.Name{}, z.glueNames...), nil
	}

	parent, err := z.Parent(ctx)
	if err != nil || parent == nil {
		z.glueNamesSet = true
		return nil, err
	}

	resp, err := parent.QueryPersistent(ctx, z.Name.String(), "NS", nil)
	if err != nil {
		return nil, err
	}
	if resp.Msg == nil {
		z.glueNamesSet = true
		return nil, nil
	}

	records := resp.GetRecordsForName("NS", z.Name)
	seen := map[string]dnsname.Name{}
	for _, rr := range records {
		nsRR, ok := rr.(*dns.NS)
		if !ok {
			continue
		}
		nsName := dnsname.New(strings.ToLower(nsRR.Ns))
		seen[nsName.String()] = nsName
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var names []dnsname.Name
	for _, key := range keys {
		names = append(names, seen[key])
	}

	z.glueNames = names
	z.glueNamesSet = true
	return append([]dnsname.Name{}, z.glueNames...), nil
}

// Glue returns glue nameserver objects for the zone.
func (z *Zone) Glue(ctx context.Context) ([]nameserver.Nameserver, error) {
	if z.glueSet {
		return append([]nameserver.Nameserver{}, z.glue...), nil
	}
	if z.recursor == nil {
		return nil, fmt.Errorf("missing recursor")
	}

	glueNames, err := z.GlueNames(ctx)
	if err != nil {
		return nil, err
	}

	var servers []nameserver.Nameserver
	if z.recursor.HasFakeAddresses(z.Name.String()) {
		for _, nsName := range glueNames {
			for _, addr := range z.recursor.GetFakeAddresses(z.Name.String(), nsName.String()) {
				ns, err := nameserver.NewWithContext(ctx, nsName.String(), addr.String(), z.recursor.Client())
				if err != nil {
					return nil, err
				}
				servers = append(servers, ns)
			}
		}
	} else {
		for _, nsName := range glueNames {
			addrs, err := z.recursor.GetAddressesFor(ctx, nsName.String())
			if err != nil {
				return nil, err
			}
			for _, addr := range addrs {
				ns, err := nameserver.NewWithContext(ctx, nsName.String(), addr.String(), z.recursor.Client())
				if err != nil {
					return nil, err
				}
				servers = append(servers, ns)
			}
		}
	}

	z.glue = servers
	z.glueSet = true
	return append([]nameserver.Nameserver{}, z.glue...), nil
}

// NSNames returns nameserver names for the zone.
func (z *Zone) NSNames(ctx context.Context) ([]dnsname.Name, error) {
	if z.nsNamesSet {
		return append([]dnsname.Name{}, z.nsNames...), nil
	}
	if z.recursor == nil {
		return nil, fmt.Errorf("missing recursor")
	}

	if z.Name.String() == "." {
		servers, err := z.NS(ctx)
		if err != nil {
			return nil, err
		}
		seen := map[string]dnsname.Name{}
		for _, ns := range servers {
			name := dnsname.New(strings.ToLower(ns.Name.String()))
			seen[name.String()] = name
		}
		keys := make([]string, 0, len(seen))
		for key := range seen {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var names []dnsname.Name
		for _, key := range keys {
			names = append(names, seen[key])
		}
		z.nsNames = names
		z.nsNamesSet = true
		return append([]dnsname.Name{}, z.nsNames...), nil
	}

	glue, err := z.Glue(ctx)
	if err != nil {
		return nil, err
	}
	var resp packet.Packet
	for _, ns := range glue {
		r, err := ns.QueryWithOptions(ctx, z.Name.String(), "NS", nil)
		if err != nil {
			continue
		}
		if r.Msg != nil && r.Type() == "answer" && r.Rcode() == "NOERROR" {
			resp = r
			break
		}
	}
	if resp.Msg == nil {
		z.nsNamesSet = true
		return nil, nil
	}

	records := resp.GetRecordsForName("NS", z.Name)
	seen := map[string]dnsname.Name{}
	for _, rr := range records {
		nsRR, ok := rr.(*dns.NS)
		if !ok {
			continue
		}
		nsName := dnsname.New(strings.ToLower(nsRR.Ns))
		seen[nsName.String()] = nsName
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var names []dnsname.Name
	for _, key := range keys {
		names = append(names, seen[key])
	}

	z.nsNames = names
	z.nsNamesSet = true
	return append([]dnsname.Name{}, z.nsNames...), nil
}

// NS returns nameserver objects for the zone.
func (z *Zone) NS(ctx context.Context) ([]nameserver.Nameserver, error) {
	if z.nsSet {
		return append([]nameserver.Nameserver{}, z.ns...), nil
	}
	if z.recursor == nil {
		return nil, fmt.Errorf("missing recursor")
	}

	if z.Name.String() == "." {
		root, err := z.recursor.RootServers(ctx)
		if err != nil {
			return nil, err
		}
		z.ns = root
		z.nsSet = true
		return append([]nameserver.Nameserver{}, z.ns...), nil
	}

	names, err := z.NSNames(ctx)
	if err != nil {
		return nil, err
	}
	var servers []nameserver.Nameserver
	for _, nsName := range names {
		addrs, err := z.recursor.GetAddressesFor(ctx, nsName.String())
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			ns, err := nameserver.NewWithContext(ctx, nsName.String(), addr.String(), z.recursor.Client())
			if err != nil {
				return nil, err
			}
			servers = append(servers, ns)
		}
	}
	z.ns = servers
	z.nsSet = true
	return append([]nameserver.Nameserver{}, z.ns...), nil
}

// GlueAddresses returns glue A/AAAA records from the parent.
func (z *Zone) GlueAddresses(ctx context.Context) ([]dns.RR, error) {
	if z.glueAddressesSet {
		return append([]dns.RR{}, z.glueAddresses...), nil
	}
	parent, err := z.Parent(ctx)
	if err != nil || parent == nil {
		z.glueAddressesSet = true
		return nil, err
	}

	resp, err := parent.QueryOne(ctx, z.Name.String(), "NS", nil)
	if err != nil {
		return nil, err
	}
	if resp.Msg == nil {
		return nil, fmt.Errorf("failed to get glue addresses")
	}

	addresses := append([]dns.RR{}, resp.GetRecords("A")...)
	addresses = append(addresses, resp.GetRecords("AAAA")...)
	z.glueAddresses = addresses
	z.glueAddressesSet = true
	return append([]dns.RR{}, z.glueAddresses...), nil
}

// QueryOne returns the first response from the zone's nameservers.
func (z *Zone) QueryOne(ctx context.Context, name string, qtype string, opts *nameserver.QueryOptions) (packet.Packet, error) {
	servers, err := z.NS(ctx)
	if err != nil {
		return packet.Packet{}, err
	}
	prof := profile.FromContext(ctx)
	for _, ns := range servers {
		if ipDisabled(prof, ns.Address) {
			continue
		}
		resp, err := ns.QueryWithOptions(ctx, name, qtype, opts)
		if err != nil {
			continue
		}
		if resp.Msg != nil {
			return resp, nil
		}
	}
	return packet.Packet{}, nil
}

// QueryAll returns responses from all nameservers in the zone.
func (z *Zone) QueryAll(ctx context.Context, name string, qtype string, opts *nameserver.QueryOptions) ([]packet.Packet, error) {
	servers, err := z.NS(ctx)
	if err != nil {
		return nil, err
	}

	prof := profile.FromContext(ctx)
	parallelism := prof.Resolver.Defaults.Parallel
	if parallelism <= 1 {
		var res []packet.Packet
		for _, ns := range servers {
			if ipDisabled(prof, ns.Address) {
				continue
			}
			resp, _ := ns.QueryWithOptions(ctx, name, qtype, opts)
			res = append(res, resp)
		}
		return res, nil
	}

	targets := make([]nameserver.Nameserver, 0, len(servers))
	for _, ns := range servers {
		if ipDisabled(prof, ns.Address) {
			continue
		}
		targets = append(targets, ns)
	}
	if len(targets) == 0 {
		return nil, nil
	}

	tasks := make([]parallel.Task[packet.Packet], len(targets))
	for i, ns := range targets {
		ns := ns
		tasks[i] = func(ctx context.Context) (packet.Packet, error) {
			resp, _ := ns.QueryWithOptions(ctx, name, qtype, opts)
			return resp, nil
		}
	}

	results := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: parallelism, CancelOnError: false})
	res := make([]packet.Packet, 0, len(results))
	for _, result := range results {
		res = append(res, result.Value)
	}
	return res, nil
}

// QueryAuth returns the first response with AA set.
func (z *Zone) QueryAuth(ctx context.Context, name string, qtype string, opts *nameserver.QueryOptions) (packet.Packet, error) {
	servers, err := z.NS(ctx)
	if err != nil {
		return packet.Packet{}, err
	}
	prof := profile.FromContext(ctx)
	for _, ns := range servers {
		if ipDisabled(prof, ns.Address) {
			continue
		}
		resp, err := ns.QueryWithOptions(ctx, name, qtype, opts)
		if err != nil {
			continue
		}
		if resp.Msg != nil && resp.AA() {
			return resp, nil
		}
	}
	return packet.Packet{}, nil
}

// QueryPersistent returns the first response containing the requested RR type.
func (z *Zone) QueryPersistent(ctx context.Context, name string, qtype string, opts *nameserver.QueryOptions) (packet.Packet, error) {
	servers, err := z.NS(ctx)
	if err != nil {
		return packet.Packet{}, err
	}
	target := dnsname.New(name)
	prof := profile.FromContext(ctx)
	for _, ns := range servers {
		if ipDisabled(prof, ns.Address) {
			continue
		}
		resp, err := ns.QueryWithOptions(ctx, name, qtype, opts)
		if err != nil {
			continue
		}
		if resp.Msg != nil && len(resp.GetRecordsForName(qtype, target)) > 0 {
			return resp, nil
		}
	}
	return packet.Packet{}, nil
}

// IsInZone reports whether a name is in this zone.
func (z *Zone) IsInZone(ctx context.Context, name string) (bool, error) {
	target := dnsname.New(name)
	if len(z.Name.Labels()) != z.Name.Common(target) {
		return false, nil
	}

	resp, err := z.QueryAuth(ctx, target.String(), "SOA", nil)
	if err != nil || resp.Msg == nil {
		return false, err
	}
	if resp.IsRedirect() {
		return false, nil
	}

	records := resp.GetRecords("SOA")
	if len(records) == 0 {
		return false, nil
	}
	owner := dnsname.New(records[0].Header().Name)
	return strings.EqualFold(owner.String(), z.Name.String()), nil
}

func ipDisabled(prof *profile.Profile, addr netip.Addr) bool {
	if prof == nil {
		return false
	}
	if addr.Is4() && !prof.Net.IPv4 {
		return true
	}
	if addr.Is6() && !prof.Net.IPv6 {
		return true
	}
	return false
}
