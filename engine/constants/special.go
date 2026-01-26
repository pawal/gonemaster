package constants

import (
	"net/netip"
	"strings"
)

// FindSpecialAddress returns the special address metadata for a given IP.
func FindSpecialAddress(ip netip.Addr) *SpecialIPBlock {
	if ip.Is4() {
		for i := range IPv4SpecialAddresses {
			block := &IPv4SpecialAddresses[i]
			if block.Prefix.Contains(ip) {
				return block
			}
		}
		return nil
	}

	if ip.Is6() {
		for i := range IPv6SpecialAddresses {
			block := &IPv6SpecialAddresses[i]
			if block.Prefix.Contains(ip) {
				return block
			}
		}
	}

	return nil
}

// IsGloballyReachable reports whether the IANA metadata marks the block as globally reachable.
func IsGloballyReachable(block *SpecialIPBlock) bool {
	if block == nil {
		return true
	}
	return strings.Contains(block.GloballyReachable, "True")
}
