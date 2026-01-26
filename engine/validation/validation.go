package validation

import (
	"net/netip"
	"regexp"
)

var (
	ipv4Re = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,3}){3}$`)
	ipv6Re = regexp.MustCompile(`(?i)^[0-9a-f:]*:[0-9a-f:]+(:[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})?$`)
)

// ValidateIPv4 checks if the given string is a valid IPv4 address.
func ValidateIPv4(ip string) bool {
	if ip == "" {
		return false
	}
	if !ipv4Re.MatchString(ip) {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.Is4()
}

// ValidateIPv6 checks if the given string is a valid IPv6 address.
func ValidateIPv6(ip string) bool {
	if ip == "" {
		return false
	}
	if !ipv6Re.MatchString(ip) {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.Is6()
}
