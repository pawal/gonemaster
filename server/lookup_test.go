package server

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestPublicLookupEndpointAccessible(t *testing.T) {
	srv := newTestServer(t, withLookupResolvers(startLookupDNS(t)))

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/example.com", nil)

	wantStatus(t, resp, http.StatusOK)

	var got DelegationInfo
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wantNS := []DelegationNS{{NS: strings.TrimSuffix(lookupFixtureNS, "."), IP: lookupFixtureIP}}
	if !reflect.DeepEqual(got.Nameservers, wantNS) {
		t.Errorf("nameservers = %+v, want %+v", got.Nameservers, wantNS)
	}
	wantDS := []DelegationDS{{
		KeyTag:    lookupFixtureKeyTag,
		Algorithm: 13,
		DigType:   2,
		Digest:    "be74359954660069d5c63d200c39f5603827d7dd02b56f120ee9f3a86764247c",
	}}
	if !reflect.DeepEqual(got.DSRecords, wantDS) {
		t.Errorf("ds records = %+v, want %+v", got.DSRecords, wantDS)
	}
}

// The zero delegation the handler returns when no resolver answers.
func TestPublicLookupUnreachableResolverReturnsEmpty(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/example.com", nil)

	wantStatus(t, resp, http.StatusOK)

	var got DelegationInfo
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(got.Nameservers) != 0 || len(got.DSRecords) != 0 {
		t.Errorf("expected an empty delegation, got %+v", got)
	}
}

func TestPublicLookupMissingDomainReturns400(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/%20", nil)

	wantStatus(t, resp, http.StatusBadRequest)
}
