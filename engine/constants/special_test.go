package constants

import (
	"net/netip"
	"slices"
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

func TestIsQueryableEmbeddedIPv4(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"64:ff9b::808:808", true},
		{"64:ff9b::a00:1", false},
		{"64:ff9b::7f00:1", false},
		{"64:ff9b::c0a8:101", false},
		{"64:ff9b::c612:1", false},
		{"64:ff9b:1::a00:1", false},
		{"::8.8.8.8", false},
		{"::10.0.0.1", false},
		{"::192.168.1.1", false},
		{"::198.18.0.1", false},
		{"2002:808:808::1", false},
		{"2002:a00:1::1", false},
	}
	for _, c := range cases {
		if got := IsQueryable(netip.MustParseAddr(c.addr)); got != c.want {
			t.Errorf("IsQueryable(%s) = %v, want %v", c.addr, got, c.want)
		}
	}
}

func TestIsQueryableRefusesMulticastAndZonedForms(t *testing.T) {
	for _, addr := range []string{"224.0.0.1", "239.255.255.250", "ff02::1", "ff0e::1", "fe80::1%eth0", "fd00::1%eth0"} {
		if IsQueryable(netip.MustParseAddr(addr)) {
			t.Errorf("IsQueryable(%s) = true, want false", addr)
		}
	}
}

func TestNotQueryableReason(t *testing.T) {
	for addr, want := range map[string]string{
		"8.8.8.8":        "",
		"10.0.0.1":       "Private-Use",
		"224.0.0.1":      "Multicast",
		"64:ff9b::a00:1": "embedded Private-Use",
		"fec0::1":        "Site-Local (deprecated)",
		"::8.8.8.8":      "IPv4-Compatible (deprecated)",
	} {
		if got := NotQueryableReason(netip.MustParseAddr(addr)); got != want {
			t.Errorf("NotQueryableReason(%s) = %q, want %q", addr, got, want)
		}
	}
}

type registryEntry struct {
	addr   string
	reason string
}

// One address per registry entry; "" marks a globally reachable entry.
var registryEntries = []registryEntry{
	{"0.1.2.3", `"This network"`},
	{"0.0.0.0", `"This host on this network"`},
	{"10.0.0.1", "Private-Use"},
	{"100.64.0.1", "Shared Address Space"},
	{"127.0.0.1", "Loopback"},
	{"169.254.1.1", "Link Local"},
	{"172.16.0.1", "Private-Use"},
	{"192.0.0.100", "IETF Protocol Assignments"},
	{"192.0.0.1", "IPv4 Service Continuity Prefix"},
	{"192.0.0.8", "IPv4 dummy address"},
	{"192.0.0.9", ""},
	{"192.0.0.10", ""},
	{"192.0.0.170", "NAT64/DNS64 Discovery"},
	{"192.0.0.171", "NAT64/DNS64 Discovery"},
	{"192.0.2.1", "Documentation (TEST-NET-1)"},
	{"192.31.196.1", ""},
	{"192.52.193.1", ""},
	{"192.88.99.1", "Deprecated (6to4 Relay Anycast)"},
	{"192.168.1.1", "Private-Use"},
	{"192.175.48.6", ""},
	{"198.18.0.1", "Benchmarking"},
	{"198.51.100.1", "Documentation (TEST-NET-2)"},
	{"203.0.113.1", "Documentation (TEST-NET-3)"},
	{"240.0.0.1", "Reserved"},
	{"255.255.255.255", "Limited Broadcast"},
	{"::1", "Loopback Address"},
	{"::", "Unspecified Address"},
	{"::ffff:10.0.0.1", "Private-Use"},
	{"64:ff9b::808:808", ""},
	{"64:ff9b:1::1", "IPv4-IPv6 Translat."},
	{"100::1", "Discard-Only Address Block"},
	{"100:0:0:1::1", "Dummy IPv6 Prefix"},
	{"2001:100::1", "IETF Protocol Assignments"},
	{"2001:0:4136:e378::1", "TEREDO"},
	{"2001:1::1", ""},
	{"2001:1::2", ""},
	{"2001:1::3", ""},
	{"2001:2::1", "Benchmarking"},
	{"2001:3::1", ""},
	{"2001:4:112::1", ""},
	{"2001:10::1", "Deprecated (previously ORCHID)"},
	{"2001:20::1", ""},
	{"2001:30::1", ""},
	{"2001:db8::1", "Documentation"},
	{"2002:808:808::1", "6to4"},
	{"2620:4f:8000::6", ""},
	{"3fff::1", "Documentation"},
	{"5f00::1", "Segment Routing (SRv6) SIDs"},
	{"fd00::1", "Unique-Local"},
	{"fe80::1", "Link-Local Unicast"},
}

func TestNotQueryableReasonPerRegistryEntry(t *testing.T) {
	for _, e := range registryEntries {
		addr := netip.MustParseAddr(e.addr)
		if got := NotQueryableReason(addr); got != e.reason {
			t.Errorf("NotQueryableReason(%s) = %q, want %q", e.addr, got, e.reason)
		}
		if addr.Is4() {
			mapped := netip.AddrFrom16(addr.As16())
			if got := NotQueryableReason(mapped); got != e.reason {
				t.Errorf("NotQueryableReason(%s) = %q, want %q", mapped, got, e.reason)
			}
		}
	}
}

func TestRegistryEntriesCoverEveryBlock(t *testing.T) {
	blocks := slices.Concat(IPv4SpecialAddresses, IPv6SpecialAddresses)
	if len(blocks) != 50 {
		t.Fatalf("registry has %d blocks, want 50", len(blocks))
	}
	seen := map[netip.Prefix]bool{}
	for _, e := range registryEntries {
		block := FindSpecialAddress(netip.MustParseAddr(e.addr))
		if block == nil {
			t.Fatalf("%s is in no block", e.addr)
		}
		seen[block.Prefix] = true
	}
	for _, block := range blocks {
		if !seen[block.Prefix] {
			t.Errorf("no entry for %s (%s)", block.Prefix, block.Name)
		}
	}
}
