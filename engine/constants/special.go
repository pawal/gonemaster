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

// IsQueryable reports whether a DNS query may be sent to ip.
func IsQueryable(ip netip.Addr) bool {
	return NotQueryableReason(ip) == ""
}

// NotQueryableReason names the range that makes ip not queryable, or returns
// "" for a queryable address. An embedded IPv4 address must be queryable too.
func NotQueryableReason(ip netip.Addr) string {
	ip = ip.Unmap().WithZone("")
	if block := FindSpecialAddress(ip); !IsGloballyReachable(block) {
		return block.Name
	}
	if ip.IsMulticast() {
		return "Multicast"
	}
	if v4, ok := embeddedIPv4(ip); ok {
		if reason := NotQueryableReason(v4); reason != "" {
			return "embedded " + reason
		}
	}
	return ""
}

var (
	nat64WellKnown = netip.MustParsePrefix("64:ff9b::/96")
	ipv4Compatible = netip.MustParsePrefix("::/96")
)

// embeddedIPv4 returns the IPv4 address in the low 32 bits of a NAT64 well-known
// prefix address (RFC 6052) or an IPv4-compatible address (RFC 4291).
func embeddedIPv4(ip netip.Addr) (netip.Addr, bool) {
	if !nat64WellKnown.Contains(ip) && !ipv4Compatible.Contains(ip) {
		return netip.Addr{}, false
	}
	b := ip.As16()
	return netip.AddrFrom4([4]byte(b[12:])), true
}
