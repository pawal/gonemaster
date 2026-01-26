package recursor

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"

	"github.com/pawal/gonemaster/engine/hints"
	"github.com/pawal/gonemaster/engine/nameserver"
	"github.com/pawal/gonemaster/engine/packet"
	"github.com/pawal/gonemaster/engine/transport"
	"github.com/pawal/gonemaster/share"
)

var namedRoot = share.NamedRoot

// Recursor holds root hints and fake delegations.
type Recursor struct {
	fakeAddresses map[string]map[string][]netip.Addr
	client        *transport.Client
	recurseCache  map[string]map[string]map[string]*packet.Packet
	cacheMu       sync.Mutex
}

// New creates a Recursor and initializes it with root hints.
func New() (*Recursor, error) {
	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{},
		client:        &transport.Client{},
		recurseCache:  map[string]map[string]map[string]*packet.Packet{},
	}

	rootHints, err := hints.ParseHints(namedRoot)
	if err != nil {
		return nil, err
	}

	if err := r.AddFakeAddresses(".", rootHints); err != nil {
		return nil, err
	}

	return r, nil
}

// AddFakeAddresses stores fake addresses for a domain.
func (r *Recursor) AddFakeAddresses(domain string, data map[string][]string) error {
	if r.fakeAddresses == nil {
		r.fakeAddresses = map[string]map[string][]netip.Addr{}
	}

	domain = strings.ToLower(domain)
	if r.fakeAddresses[domain] == nil {
		r.fakeAddresses[domain] = map[string][]netip.Addr{}
	}

	for name, ips := range data {
		name = strings.ToLower(name)
		if _, ok := r.fakeAddresses[domain][name]; !ok {
			r.fakeAddresses[domain][name] = []netip.Addr{}
		}
		unique := map[netip.Addr]bool{}
		for _, ip := range ips {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				return fmt.Errorf("invalid IP %q for %s: %w", ip, name, err)
			}
			unique[addr] = true
		}

		for addr := range unique {
			r.fakeAddresses[domain][name] = append(r.fakeAddresses[domain][name], addr)
		}
	}

	return nil
}

// HasFakeAddresses reports whether a domain has fake address data.
func (r *Recursor) HasFakeAddresses(domain string) bool {
	domain = strings.ToLower(domain)
	_, ok := r.fakeAddresses[domain]
	return ok
}

// GetFakeAddresses returns fake addresses for a domain and nameserver name.
func (r *Recursor) GetFakeAddresses(domain string, nsname string) []netip.Addr {
	domain = strings.ToLower(domain)
	if nsname == "" {
		nsname = ""
	}

	nsname = strings.ToLower(nsname)
	if r.fakeAddresses[domain] == nil {
		return nil
	}

	addrs := r.fakeAddresses[domain][nsname]
	out := make([]netip.Addr, len(addrs))
	copy(out, addrs)
	return out
}

// GetFakeNames returns fake nameserver names for a domain.
func (r *Recursor) GetFakeNames(domain string) []string {
	domain = strings.ToLower(domain)
	entries := r.fakeAddresses[domain]
	if entries == nil {
		return nil
	}

	out := make([]string, 0, len(entries))
	for name := range entries {
		out = append(out, name)
	}
	return out
}

// RemoveFakeAddresses deletes fake address data for a domain.
func (r *Recursor) RemoveFakeAddresses(domain string) {
	domain = strings.ToLower(domain)
	delete(r.fakeAddresses, domain)
}

// RootServers returns nameservers initialized from root hints.
func (r *Recursor) RootServers() ([]nameserver.Nameserver, error) {
	var servers []nameserver.Nameserver
	names := r.GetFakeNames(".")
	sort.Strings(names)
	for _, name := range names {
		addrs := r.GetFakeAddresses(".", name)
		for _, addr := range addrs {
			ns, err := nameserver.New(name, addr.String(), r.client)
			if err != nil {
				return nil, err
			}
			servers = append(servers, ns)
		}
	}
	return servers, nil
}

// Client returns the transport client used by the recursor.
func (r *Recursor) Client() *transport.Client {
	if r == nil || r.client == nil {
		return &transport.Client{}
	}
	return r.client
}
