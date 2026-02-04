package engine

import (
	"net"
	"net/netip"
)

func shouldAutoDisableIPv6(req RunRequest, enabled bool) bool {
	if req.IPv6 != nil {
		return false
	}
	if !enabled {
		return false
	}
	return !hasGlobalIPv6()
}

func hasGlobalIPv6() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		// On failure, keep IPv6 enabled to avoid surprising behavior.
		return true
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := addrToIP(addr)
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				continue
			}
			ip16 := ip.To16()
			if ip16 == nil {
				continue
			}
			nip, ok := netip.AddrFromSlice(ip16)
			if !ok {
				continue
			}
			if isGlobalIPv6(nip) {
				return true
			}
		}
	}
	return false
}

func addrToIP(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		if addr == nil {
			return nil
		}
		if ip, _, err := net.ParseCIDR(addr.String()); err == nil {
			return ip
		}
		return net.ParseIP(addr.String())
	}
}

func isGlobalIPv6(addr netip.Addr) bool {
	if !addr.Is6() || addr.Is4In6() {
		return false
	}
	if addr.IsLoopback() || addr.IsUnspecified() || addr.IsMulticast() {
		return false
	}
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() {
		return false
	}
	if addr.IsPrivate() {
		return false
	}
	return addr.IsGlobalUnicast()
}
