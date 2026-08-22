package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

const handlerChainJSON = `{"version":1,"zone":"example.com","status":"secure"}`

// graduatePublicJobWithChain creates and graduates a public-origin job that
// carries a chain blob, returning its public ID.
func graduatePublicJobWithChain(t *testing.T, srv *Server, chain string) string {
	t.Helper()
	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
		Origin:    JobOriginPublic,
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	created.DNSSECChainJSON = chain
	if err := srv.store.GraduateJob(created, nil); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}
	return created.PublicID
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

	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	cfg := DefaultConfig()
	cfg.ShowDNSSECChainPublic = false
	srv := New(cfg)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/dnssec-chain", nil)

	// Flag-off is indistinguishable from an unknown id: not_found, not no_chain_data.
	wantErrorCode(t, resp, http.StatusNotFound, "not_found")
}

func TestPublicDNSSECChainMarkerMaskedWhenFlagOff(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowDNSSECChainPublic = false
	srv := New(cfg)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/result", nil)

	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
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

// errChainStore wraps a real store but fails every chain lookup, standing in
// for a transient database error. Everything else delegates to the embedded
// store, so the handler still resolves the job before the failing lookup.
type errChainStore struct {
	JobStore
	err error
}

func (e errChainStore) GetRunDNSSECChain(string) (string, bool, error) {
	return "", false, e.err
}

func TestPublicDNSSECChainLookupErrorReturns500(t *testing.T) {
	srv := newTestServer(t)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)
	srv.store = errChainStore{JobStore: srv.store, err: errors.New("db down")}

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
	srv.store = errChainStore{JobStore: srv.store, err: errors.New("db down")}

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs/"+job.ID+"/dnssec-chain", nil)

	wantErrorCode(t, resp, http.StatusInternalServerError, "lookup_failed")
}
