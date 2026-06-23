package constants

import (
	"net/netip"
	"testing"
)

// TestIsQueryable covers one representative address for every IANA
// special-purpose category that is NOT marked "Globally Reachable: True", so
// the test documents and pins the complete set of addresses the query guard
// refuses to dial. It also covers IPv4-mapped IPv6 literals (to lock the
// Unmap step, since FindSpecialAddress branches on Is4/Is6 and does not unmap)
// and ordinary public unicast addresses that must remain queryable - including
// the real Telia addresses from the case that motivated the guard.
func TestIsQueryable(t *testing.T) {
	cases := []struct {
		addr string
		want bool
		note string
	}{
		// IPv4 - not globally reachable, must be blocked.
		{"0.0.0.0", false, "this host / this network"},
		{"0.1.2.3", false, "0.0.0.0/8 this network"},
		{"10.0.0.1", false, "RFC1918 private-use"},
		{"172.16.5.1", false, "RFC1918 private-use"},
		{"192.168.1.1", false, "RFC1918 private-use"},
		{"100.64.0.1", false, "shared address space / CGNAT"},
		{"127.0.0.1", false, "loopback"},
		{"169.254.1.1", false, "link local"},
		{"192.0.0.1", false, "IETF protocol assignments"},
		{"192.0.2.1", false, "documentation TEST-NET-1"},
		{"192.88.99.1", false, "deprecated 6to4 relay anycast"},
		{"198.18.0.1", false, "benchmarking"},
		{"198.51.100.1", false, "documentation TEST-NET-2"},
		{"203.0.113.1", false, "documentation TEST-NET-3"},
		{"240.0.0.1", false, "reserved"},
		{"255.255.255.255", false, "limited broadcast"},

		// IPv6 - not globally reachable, must be blocked.
		{"::1", false, "loopback"},
		{"::", false, "unspecified"},
		{"64:ff9b:1::1", false, "IPv4-IPv6 translation"},
		{"100::1", false, "discard-only"},
		{"2001:2::1", false, "benchmarking"},
		{"2001:db8::1", false, "documentation"},
		{"2002::1", false, "6to4"},
		{"3fff::1", false, "documentation"},
		{"5f00::1", false, "segment routing SRv6 SIDs"},
		{"fc00::1", false, "unique-local"},
		{"fd00::1", false, "unique-local"},
		{"fe80::1", false, "link-local unicast"},

		// IPv4-mapped IPv6 - must classify as the underlying IPv4 form.
		{"::ffff:127.0.0.1", false, "v4-mapped loopback"},
		{"::ffff:10.0.0.1", false, "v4-mapped RFC1918"},

		// Ordinary public unicast - must remain queryable.
		{"8.8.8.8", true, "public resolver"},
		{"1.1.1.1", true, "public resolver"},
		{"9.9.9.9", true, "public resolver"},
		{"81.228.11.67", true, "telia authoritative (motivating case)"},
		{"2001:4860:4860::8888", true, "public resolver"},
		{"2606:4700:4700::1111", true, "public resolver"},
		{"2001:2040:c001:101::5", true, "telia authoritative (motivating case)"},
	}

	for _, c := range cases {
		addr, err := netip.ParseAddr(c.addr)
		if err != nil {
			t.Fatalf("ParseAddr(%q): %v", c.addr, err)
		}
		if got := IsQueryable(addr); got != c.want {
			t.Errorf("IsQueryable(%s) = %v, want %v (%s)", c.addr, got, c.want, c.note)
		}
	}
}
