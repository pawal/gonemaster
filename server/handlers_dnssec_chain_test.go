package server

import (
	"errors"
	"net/http"
	"testing"
)

const handlerChainJSON = `{"version":1,"zone":"example.com","status":"secure"}`

// graduatePublicJobWithChain creates and graduates a public-origin job that
// carries a chain blob, returning its public ID.
func graduatePublicJobWithChain(t *testing.T, srv *Server, chain string) string {
	t.Helper()
	return seedGraduatedRun(t, srv.store, runSpec{
		Progress:  100,
		Origin:    JobOriginPublic,
		ChainJSON: chain,
	}).PublicID
}

func TestPublicDNSSECChainReturnsBlobWithCacheHeader(t *testing.T) {
	srv := newTestServer(t)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/dnssec-chain", nil)

	wantStatus(t, resp, http.StatusOK)
	if cc := resp.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Errorf("Cache-Control = %q, want public, max-age=300", cc)
	}
	if body := resp.Body.String(); body != handlerChainJSON {
		t.Errorf("body = %q, want %q", body, handlerChainJSON)
	}
}

func TestPublicDNSSECChainMarkerInResult(t *testing.T) {
	srv := newTestServer(t)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/result", nil)

	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if !result.HasDNSSECChain {
		t.Error("expected has_dnssec_chain marker true")
	}
}

func TestPublicDNSSECChainUnknownIDReturnsNotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/nosuchid1/dnssec-chain", nil)

	wantErrorCode(t, resp, http.StatusNotFound, "not_found")
	if cc := resp.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("no cache header expected on 404, got %q", cc)
	}
}

func TestPublicDNSSECChainRunWithoutBlobReturnsNoChainData(t *testing.T) {
	srv := newTestServer(t)
	// Graduate a public job without a chain blob.
	publicID := graduatePublicJobWithChain(t, srv, "")

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/dnssec-chain", nil)

	wantErrorCode(t, resp, http.StatusNotFound, "no_chain_data")
	if cc := resp.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("no cache header expected on no_chain_data, got %q", cc)
	}
}

func TestPublicDNSSECChainFlagOffReturnsNotFound(t *testing.T) {
	srv := newTestServer(t, withConfig(func(c *Config) { c.ShowDNSSECChainPublic = false }))
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/dnssec-chain", nil)

	// Flag-off is indistinguishable from an unknown id: not_found, not no_chain_data.
	wantErrorCode(t, resp, http.StatusNotFound, "not_found")
}

func TestPublicDNSSECChainMarkerMaskedWhenFlagOff(t *testing.T) {
	srv := newTestServer(t, withConfig(func(c *Config) { c.ShowDNSSECChainPublic = false }))
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/result", nil)

	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if result.HasDNSSECChain {
		t.Error("expected marker masked to false when flag off")
	}
}

func TestAdminDNSSECChainTwin(t *testing.T) {
	srv := newTestServer(t)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)
	job, ok := srv.store.GetByPublicID(publicID)
	if !ok {
		t.Fatal("expected job")
	}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs/"+job.ID+"/dnssec-chain", nil)

	wantStatus(t, resp, http.StatusOK)
	if body := resp.Body.String(); body != handlerChainJSON {
		t.Errorf("body = %q, want %q", body, handlerChainJSON)
	}
}

func TestPublicDNSSECChainLookupErrorReturns500(t *testing.T) {
	srv := newTestServer(t)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)
	wrapStore(t, srv).chainErr = errors.New("db down")

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/dnssec-chain", nil)

	// A lookup failure must not masquerade as absent data (404), or the UI
	// would latch a permanent "no chain data" note for data that exists.
	wantErrorCode(t, resp, http.StatusInternalServerError, "lookup_failed")
	if cc := resp.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("no cache header expected on 500, got %q", cc)
	}
}

func TestAdminDNSSECChainLookupErrorReturns500(t *testing.T) {
	srv := newTestServer(t)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)
	job, ok := srv.store.GetByPublicID(publicID)
	if !ok {
		t.Fatal("expected job")
	}
	wrapStore(t, srv).chainErr = errors.New("db down")

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs/"+job.ID+"/dnssec-chain", nil)

	wantErrorCode(t, resp, http.StatusInternalServerError, "lookup_failed")
}
