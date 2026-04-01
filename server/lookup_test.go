package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicLookupEndpointAccessible(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/lookup/example.com", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from /pub/api/v1/lookup/example.com, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestPublicLookupMissingDomainReturns400(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/lookup/%20", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}
