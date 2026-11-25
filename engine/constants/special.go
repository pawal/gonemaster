package constants

import (
	"net/netip"
	"strings"
)

// FindSpecialAddress returns the most specific special-purpose block containing ip.
func FindSpecialAddress(ip netip.Addr) *SpecialIPBlock {
	blocks := IPv6SpecialAddresses
	if ip.Is4() {
		blocks = IPv4SpecialAddresses
	}
	var found *SpecialIPBlock
	for i := range blocks {
		block := &blocks[i]
		if block.Prefix.Contains(ip) && (found == nil || block.Prefix.Bits() > found.Prefix.Bits()) {
			found = block
		}
	}
	return found
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
	for _, block := range deprecatedIPv6 {
		if block.Prefix.Contains(ip) {
			return block.Name
		}
	}
	if nat64WellKnown.Contains(ip) {
		b := ip.As16()
		if reason := NotQueryableReason(netip.AddrFrom4([4]byte(b[12:]))); reason != "" {
			return "embedded " + reason
		}
	}
	return ""
}

var (
	nat64WellKnown = netip.MustParsePrefix("64:ff9b::/96")
	nat64LocalUse  = netip.MustParsePrefix("64:ff9b:1::/48")
	ipv4Compatible = netip.MustParsePrefix("::/96")
)

// Deprecated IPv6 ranges absent from the special-purpose registry.
var deprecatedIPv6 = []SpecialIPBlock{
	{Prefix: netip.MustParsePrefix("fec0::/10"), Name: "Site-Local (deprecated)", Reference: "[RFC3879]", GloballyReachable: "False"},
	{Prefix: ipv4Compatible, Name: "IPv4-Compatible (deprecated)", Reference: "[RFC4291]", GloballyReachable: "False"},
}

// IsIPv4Embedded reports whether ip is an IPv4-mapped, IPv4-compatible or NAT64 address (RFC 10001 section 4.1).
func IsIPv4Embedded(ip netip.Addr) bool {
	ip = ip.WithZone("")
	switch {
	case ip.Is4In6():
		return true
	case ipv4Compatible.Contains(ip):
		return ip != netip.IPv6Unspecified() && ip != netip.IPv6Loopback()
	default:
		return nat64WellKnown.Contains(ip) || nat64LocalUse.Contains(ip)
	}
}
