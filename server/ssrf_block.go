package server

import (
	"net/netip"
	"strings"
)

// cgnatPrefix is RFC 6598 shared address space; netip's IsPrivate omits it.
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// isBlockedPublicNameserverIP returns true and a reason when ip is a
// non-public address an internet-facing deployment must refuse, so the
// engine's outbound DNS queries cannot be aimed at internal hosts. Empty
// is allowed (engine resolves the NS name).
func isBlockedPublicNameserverIP(ip string) (bool, string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false, ""
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false, ""
	}
	addr = addr.Unmap()
	switch {
	case addr.IsLoopback():
		return true, "loopback"
	case addr.IsUnspecified():
		return true, "unspecified"
	case addr.IsLinkLocalUnicast():
		return true, "link-local"
	case addr.IsLinkLocalMulticast():
		return true, "link-local multicast"
	case addr.IsInterfaceLocalMulticast():
		return true, "interface-local multicast"
	case addr.IsMulticast():
		return true, "multicast"
	case addr.IsPrivate():
		return true, "private"
	case cgnatPrefix.Contains(addr):
		return true, "CGNAT"
	case addr.Is4() && addr.As4() == [4]byte{255, 255, 255, 255}:
		return true, "broadcast"
	}
	return false, ""
}
