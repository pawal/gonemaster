package server

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestPublicLookupEndpointAccessible(t *testing.T) {
	srv := newTestServer(t, withLookupResolvers(startLookupDNS(t)))

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/example.com", nil)

	got := mustJSON[DelegationInfo](t, resp, http.StatusOK)

	wantNS := []DelegationNS{{NS: strings.TrimSuffix(lookupFixtureNS, "."), IP: lookupFixtureIP}}
	if !slices.Equal(got.Nameservers, wantNS) {
		t.Errorf("nameservers = %+v, want %+v", got.Nameservers, wantNS)
	}
	wantDS := []DelegationDS{{
		KeyTag:    lookupFixtureKeyTag,
		Algorithm: 13,
		DigType:   2,
		Digest:    "be74359954660069d5c63d200c39f5603827d7dd02b56f120ee9f3a86764247c",
	}}
	if !slices.Equal(got.DSRecords, wantDS) {
		t.Errorf("ds records = %+v, want %+v", got.DSRecords, wantDS)
	}
}

// The zero delegation the handler returns when no resolver answers.
func TestPublicLookupUnreachableResolverReturnsEmpty(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/example.com", nil)

	got := mustJSON[DelegationInfo](t, resp, http.StatusOK)

	if len(got.Nameservers) != 0 || len(got.DSRecords) != 0 {
		t.Errorf("expected an empty delegation, got %+v", got)
	}
}

func TestPublicLookupMissingDomainReturns400(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/%20", nil)

	wantStatus(t, resp, http.StatusBadRequest)
}

func TestPublicLookupDropsNonGlobalAddresses(t *testing.T) {
	srv := newTestServer(t, withLookupResolvers(startLookupDNS(t)))

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/internal.example", nil)

	got := mustJSON[DelegationInfo](t, resp, http.StatusOK)
	wantNS := []DelegationNS{{NS: strings.TrimSuffix(lookupInternalNS, ".")}}
	if !slices.Equal(got.Nameservers, wantNS) {
		t.Errorf("nameservers = %+v, want %+v", got.Nameservers, wantNS)
	}
}

func TestPublicLookupKeepsNonGlobalAddressesWhenAllowed(t *testing.T) {
	srv := newTestServer(t, withLookupResolvers(startLookupDNS(t)), withPublicAPI(func(c *PublicAPIConfig) {
		c.AllowNonGlobalTargets = true
	}))

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/internal.example", nil)

	got := mustJSON[DelegationInfo](t, resp, http.StatusOK)
	ns := strings.TrimSuffix(lookupInternalNS, ".")
	wantNS := []DelegationNS{{NS: ns, IP: lookupInternalIPv4}, {NS: ns, IP: lookupInternalIPv6}}
	if !slices.Equal(got.Nameservers, wantNS) {
		t.Errorf("nameservers = %+v, want %+v", got.Nameservers, wantNS)
	}
}
