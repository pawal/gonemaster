package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	engineprofile "codeberg.org/pawal/gonemaster/engine/profile"
)

func postProfileDiff(t *testing.T, srv *Server, body string) (int, *httptest.ResponseRecorder) {
	t.Helper()
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/profiles/diff", body)
	return resp.Code, resp
}

func decodeDiff(t *testing.T, resp *httptest.ResponseRecorder) engineprofile.DiffResult {
	t.Helper()
	result := mustJSON[engineprofile.DiffResult](t, resp, http.StatusOK)
	return result
}

func diffProperty(result engineprofile.DiffResult, path string) *engineprofile.DiffProperty {
	for i := range result.Properties {
		if result.Properties[i].Path == path {
			return &result.Properties[i]
		}
	}
	return nil
}

func TestProfileDiffEndpoint(t *testing.T) {
	srv := newTestServer(t)

	code, resp := postProfileDiff(t, srv, `{"config":{
		"resolver":{"defaults":{"timeout":5,"nameserver_max_total_ms":60000}}
	}}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, resp.Body)
	}
	result := decodeDiff(t, resp)

	timeout := diffProperty(result, "resolver.defaults.timeout")
	if timeout == nil || !timeout.Redundant {
		t.Fatalf("expected the restated timeout to be redundant, got %+v", timeout)
	}
	totalMS := diffProperty(result, "resolver.defaults.nameserver_max_total_ms")
	if totalMS == nil || totalMS.Kind != engineprofile.DiffKindChanged {
		t.Fatalf("expected the total-ms cap to be a deviation, got %+v", totalMS)
	}
	if result.Summary.Deviations != 1 || result.Summary.Redundant != 1 {
		t.Fatalf("expected 1 deviation and 1 redundant, got %+v", result.Summary)
	}
}

func TestProfileDiffEndpointEmptyConfig(t *testing.T) {
	srv := newTestServer(t)

	code, resp := postProfileDiff(t, srv, `{}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, resp.Body)
	}
	result := decodeDiff(t, resp)
	if len(result.Properties) != 0 {
		t.Fatalf("expected no properties for an absent config, got %+v", result.Properties)
	}
}

func TestProfileDiffEndpointInvalidConfig(t *testing.T) {
	srv := newTestServer(t)

	code, resp := postProfileDiff(t, srv, `{"config":{"not_a_property":1}}`)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown property, got %d: %s", code, resp.Body)
	}
}

func TestProfileDiffEndpointInvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	code, _ := postProfileDiff(t, srv, `not json`)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d", code)
	}
}

func TestProfileDiffEndpointStoresNothing(t *testing.T) {
	srv := newTestServer(t)

	// The draft config travels in the body and must never reach the store.
	code, _ := postProfileDiff(t, srv, `{"config":{"net":{"ipv6":false}}}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if profiles := srv.store.ListProfiles(); len(profiles) != 0 {
		t.Fatalf("expected an empty profile store, got %+v", profiles)
	}
}

func TestProfileDiffEndpointTestLevelsMissingTag(t *testing.T) {
	srv := newTestServer(t)

	code, resp := postProfileDiff(t, srv, `{"config":{
		"test_levels":{"BASIC":{"B01_CHILD_FOUND":"WARNING"}}
	}}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, resp.Body)
	}
	result := decodeDiff(t, resp)

	levels := diffProperty(result, "test_levels")
	if levels == nil || levels.Kind != engineprofile.DiffKindMap || !levels.Wholesale {
		t.Fatalf("expected a wholesale map entry for test_levels, got %+v", levels)
	}
	// Overriding one tag of one module leaves every other tag of every module
	// unset, which resolves to DEBUG at run time.
	if result.Summary.Missing == 0 {
		t.Fatalf("expected missing tags to be reported, got %+v", result.Summary)
	}
}
