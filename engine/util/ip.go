package util

import "net/netip"

// IPVersion returns 4 or 6 depending on the IP address type, or 0 if unknown.
func IPVersion(addr netip.Addr) int {
	if addr.Is4() {
		return 4
	}
	if addr.Is6() {
		return 6
	}
	return 0
}
