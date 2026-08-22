package server

import (
	"net/http"
	"testing"
)

func TestPublicLookupEndpointAccessible(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/example.com", nil)

	wantStatus(t, resp, http.StatusOK)
}

func TestPublicLookupMissingDomainReturns400(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/lookup/%20", nil)

	wantStatus(t, resp, http.StatusBadRequest)
}
