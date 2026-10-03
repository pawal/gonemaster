package engine

import (
	"net"
	"net/netip"
)

// ipv6RouteProbe is a.root-servers.net; a UDP dial selects a route and sends nothing.
const ipv6RouteProbe = "[2001:503:ba3e::2:30]:53"

// dialFunc opens a connection, as net.Dial does.
type dialFunc func(network, address string) (net.Conn, error)

// shouldAutoDisableIPv6 reports whether a run with net.ipv6 enabled must turn it off.
func shouldAutoDisableIPv6(req RunRequest, enabled bool, dial dialFunc) bool {
	if req.IPv6 != nil || req.SkipIPv6Detect || !enabled {
		return false
	}
	return !hasIPv6Route(dial)
}

// hasIPv6Route reports whether the host routes to a global IPv6 address from a usable source.
func hasIPv6Route(dial dialFunc) bool {
	conn, err := dial("udp6", ipv6RouteProbe)
	if err != nil {
		return false
	}
	defer conn.Close()
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return false
	}
	addr, ok := netip.AddrFromSlice(local.IP)
	return ok && usableIPv6Source(addr)
}

// usableIPv6Source reports whether addr can source a query to a global IPv6 address.
func usableIPv6Source(addr netip.Addr) bool {
	if !addr.Is6() || addr.Is4In6() {
		return false
	}
	return !addr.IsLoopback() && !addr.IsUnspecified() && !addr.IsMulticast() && !addr.IsLinkLocalUnicast()
}
