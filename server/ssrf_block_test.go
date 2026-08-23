package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestIsBlockedPublicNameserverIP(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"empty", "", false},
		{"public v4", "8.8.8.8", false},
		{"public v6", "2001:4860:4860::8888", false},
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},
		{"link-local v4 (metadata)", "169.254.169.254", true},
		{"link-local v6", "fe80::1", true},
		{"private 10/8", "10.0.0.5", true},
		{"private 172.16/12", "172.16.0.1", true},
		{"private 192.168/16", "192.168.1.1", true},
		{"IPv6 ULA", "fd00::1", true},
		{"CGNAT 100.64/10", "100.64.0.1", true},
		{"multicast v4", "224.0.0.1", true},
		{"multicast v6", "ff02::1", true},
		{"broadcast", "255.255.255.255", true},
		{"v4-mapped loopback", "::ffff:127.0.0.1", true},
		{"v4-mapped private", "::ffff:10.0.0.1", true},
		{"invalid ip is not blocked", "not-an-ip", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := isBlockedPublicNameserverIP(tc.ip)
			if got != tc.blocked {
				t.Fatalf("ip=%q: got blocked=%v, want %v", tc.ip, got, tc.blocked)
			}
		})
	}
}

func TestPublicCreateJobRejectsPrivateUndelegatedIP(t *testing.T) {
	srv := newTestServer(t)

	body := `{"domain":"example.com","nameservers":[{"ns":"ns1.attacker.example","ip":"169.254.169.254"}]}`
	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", body)

	wantErrorCode(t, resp, http.StatusBadRequest, "private_undelegated_ip")
}

func TestPublicCreateJobAllowsPrivateUndelegatedIPWhenEnabled(t *testing.T) {
	srv := newTestServer(t, withPublicAPI(func(c *PublicAPIConfig) { c.AllowPrivateUndelegatedIP = true }))

	body := `{"domain":"example.com","nameservers":[{"ns":"ns1.internal.example","ip":"10.0.0.1"}]}`
	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", body)

	wantStatus(t, resp, http.StatusCreated)
}

func TestPublicCreateJobAllowsPublicUndelegatedIP(t *testing.T) {
	srv := newTestServer(t)

	body := `{"domain":"example.com","nameservers":[{"ns":"ns1.example.","ip":"198.51.100.1"}]}`
	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", body)

	wantStatus(t, resp, http.StatusCreated)
}

func TestPublicCreateJobRejectsAnyPrivateIPInList(t *testing.T) {
	srv := newTestServer(t)

	body := `{"domain":"example.com","nameservers":[` +
		`{"ns":"ns1.example.","ip":"198.51.100.1"},` +
		`{"ns":"ns2.attacker.example","ip":"127.0.0.1"}` +
		`]}`
	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", body)

	out := wantErrorCode(t, resp, http.StatusBadRequest, "private_undelegated_ip")
	// Mention the offending index for operator clarity.
	if !strings.Contains(out.Error.Message, "[1]") {
		t.Logf("note: error message should reference the offending index: %q", out.Error.Message)
	}
}
