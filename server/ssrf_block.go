package server

import (
	"net/netip"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/constants"
)

// isBlockedPublicNameserverIP reports whether the engine's non-global guard
// refuses ip, and names the range. Empty is allowed (the engine resolves the
// NS name).
func isBlockedPublicNameserverIP(ip string) (bool, string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false, ""
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false, ""
	}
	reason := constants.NotQueryableReason(addr)
	return reason != "", reason
}
