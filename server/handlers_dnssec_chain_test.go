package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	srv := New(DefaultConfig())
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/dnssec-chain", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if cc := resp.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Errorf("Cache-Control = %q, want public, max-age=300", cc)
	}
	if body := resp.Body.String(); body != handlerChainJSON {
		t.Errorf("body = %q, want %q", body, handlerChainJSON)
	}
}

func TestPublicDNSSECChainMarkerInResult(t *testing.T) {
	srv := New(DefaultConfig())
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/result", nil)
	srv.Handler().ServeHTTP(resp, req)

	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.HasDNSSECChain {
		t.Error("expected has_dnssec_chain marker true")
	}
}

func TestPublicDNSSECChainUnknownIDReturnsNotFound(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/nosuchid1/dnssec-chain", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
	assertErrorCode(t, resp, "not_found")
	if cc := resp.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("no cache header expected on 404, got %q", cc)
	}
}

func TestPublicDNSSECChainRunWithoutBlobReturnsNoChainData(t *testing.T) {
	srv := New(DefaultConfig())
	// Graduate a public job without a chain blob.
	publicID := graduatePublicJobWithChain(t, srv, "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/dnssec-chain", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
	assertErrorCode(t, resp, "no_chain_data")
	if cc := resp.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("no cache header expected on no_chain_data, got %q", cc)
	}
}

func TestPublicDNSSECChainFlagOffReturnsNotFound(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowDNSSECChainPublic = false
	srv := New(cfg)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/dnssec-chain", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when flag off, got %d", resp.Code)
	}
	// Flag-off is indistinguishable from an unknown id: not_found, not no_chain_data.
	assertErrorCode(t, resp, "not_found")
}

func TestPublicDNSSECChainMarkerMaskedWhenFlagOff(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowDNSSECChainPublic = false
	srv := New(cfg)
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/result", nil)
	srv.Handler().ServeHTTP(resp, req)

	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.HasDNSSECChain {
		t.Error("expected marker masked to false when flag off")
	}
}

func TestAdminDNSSECChainTwin(t *testing.T) {
	srv := New(DefaultConfig())
	publicID := graduatePublicJobWithChain(t, srv, handlerChainJSON)
	job, ok := srv.store.GetByPublicID(publicID)
	if !ok {
		t.Fatal("expected job")
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+job.ID+"/dnssec-chain", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if body := resp.Body.String(); body != handlerChainJSON {
		t.Errorf("body = %q, want %q", body, handlerChainJSON)
	}
}

// assertErrorCode decodes a writeError body and checks its error code.
func assertErrorCode(t *testing.T, resp *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, resp.Body.String())
	}
	if body.Error.Code != want {
		t.Errorf("error code = %q, want %q", body.Error.Code, want)
	}
}
