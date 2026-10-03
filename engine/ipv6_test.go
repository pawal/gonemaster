package engine

import (
	"errors"
	"net"
	"slices"
	"testing"
)

// routeConn is a dialed UDP socket that only reports its source address.
type routeConn struct {
	net.Conn
	local net.Addr
}

func (c routeConn) LocalAddr() net.Addr { return c.local }
func (c routeConn) Close() error        { return nil }

// udpSource is a source address the kernel could pick for the probe.
func udpSource(ip string) net.Addr { return &net.UDPAddr{IP: net.ParseIP(ip), Port: 40000} }

// dialFrom returns a dialer whose socket has local as its source address.
func dialFrom(local net.Addr) dialFunc {
	return func(string, string) (net.Conn, error) { return routeConn{local: local}, nil }
}

// noRoute fails as connect(2) does without an IPv6 route.
func noRoute(string, string) (net.Conn, error) { return nil, errors.New("network is unreachable") }

func TestHasIPv6Route(t *testing.T) {
	for _, tc := range []struct {
		name string
		dial dialFunc
		want bool
	}{
		{"global source", dialFrom(udpSource("2001:db8::4")), true},
		{"ULA source behind NAT66", dialFrom(udpSource("fd00:a605:ec08::4")), true},
		{"no route", noRoute, false},
		{"link-local source", dialFrom(udpSource("fe80::7e1e:52ff:fe3f:368c")), false},
		{"loopback source", dialFrom(udpSource("::1")), false},
		{"unspecified source", dialFrom(udpSource("::")), false},
		{"IPv4-mapped source", dialFrom(udpSource("::ffff:192.0.2.1")), false},
		{"IPv4 source", dialFrom(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 1).To4(), Port: 40000}), false},
		{"non-UDP source", dialFrom(&net.TCPAddr{IP: net.ParseIP("2001:db8::4"), Port: 40000}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasIPv6Route(tc.dial); got != tc.want {
				t.Fatalf("hasIPv6Route = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestShouldAutoDisableIPv6(t *testing.T) {
	off := false
	probe := []string{"udp6 [2001:503:ba3e::2:30]:53"}
	for _, tc := range []struct {
		name      string
		req       RunRequest
		enabled   bool
		routed    bool
		want      bool
		wantDials []string
	}{
		{name: "routed", enabled: true, routed: true, want: false, wantDials: probe},
		{name: "not routed", enabled: true, want: true, wantDials: probe},
		{name: "profile has IPv6 off", enabled: false},
		{name: "request sets IPv6", req: RunRequest{IPv6: &off}, enabled: true},
		{name: "detection skipped", req: RunRequest{SkipIPv6Detect: true}, enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dials []string
			dial := func(network, address string) (net.Conn, error) {
				dials = append(dials, network+" "+address)
				if !tc.routed {
					return noRoute(network, address)
				}
				return routeConn{local: udpSource("fd00::4")}, nil
			}
			if got := shouldAutoDisableIPv6(tc.req, tc.enabled, dial); got != tc.want {
				t.Fatalf("shouldAutoDisableIPv6 = %t, want %t", got, tc.want)
			}
			if !slices.Equal(dials, tc.wantDials) {
				t.Fatalf("dials = %v, want %v", dials, tc.wantDials)
			}
		})
	}
}

func TestEffectiveProfileSkipIPv6DetectKeepsProfileValue(t *testing.T) {
	p, err := EffectiveProfile(RunRequest{SkipIPv6Detect: true})
	if err != nil {
		t.Fatalf("EffectiveProfile: %v", err)
	}
	if !p.Net.IPv6 {
		t.Fatalf("net.ipv6 = %t, want true", p.Net.IPv6)
	}
}
