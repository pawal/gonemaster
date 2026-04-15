package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestPublicCreateJobReturnsPublicIDNotUUID(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var view map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view["public_id"] == "" || view["public_id"] == nil {
		t.Fatal("expected public_id in response")
	}
	if _, hasID := view["id"]; hasID {
		t.Fatal("internal UUID must not appear in public API response")
	}
}

func TestPublicCreateJobReturnsDomainStatusProgress(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	var view PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Domain != "example.com" {
		t.Fatalf("domain: got %q, want %q", view.Domain, "example.com")
	}
	if view.Status != JobQueued {
		t.Fatalf("status: got %q, want %q", view.Status, JobQueued)
	}
	if view.Progress != 0 {
		t.Fatalf("progress: got %d, want 0", view.Progress)
	}
}

func TestPublicCreateJobMissingDomainReturns400(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestPublicCreateJobWithPublicProfileID(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"public-profile","config":{"net":{"ipv4":true}},"public":true}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(fmt.Sprintf(`{"domain":"example.com","profile_id":%d}`, profile.ID)))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var created PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	job, ok := srv.store.GetByPublicID(created.PublicID)
	if !ok {
		t.Fatal("expected stored public job")
	}
	if job.ProfileID == nil || *job.ProfileID != profile.ID {
		t.Fatalf("ProfileID: got %v, want %d", job.ProfileID, profile.ID)
	}
	if job.ProfileName != "public-profile" {
		t.Fatalf("ProfileName: got %q", job.ProfileName)
	}
}

func TestPublicCreateJobRejectsPrivateProfile(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"private-profile","config":{"net":{"ipv4":true}},"public":false}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(fmt.Sprintf(`{"domain":"example.com","profile_id":%d}`, profile.ID)))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "profile_not_public" {
		t.Fatalf("expected profile_not_public, got %q", out.Error.Code)
	}
}

func TestPublicCreateJobRejectsProfileOverrides(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com","profile_overrides":{"net":{"ipv6":false}}}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "profile_overrides_not_allowed" {
		t.Fatalf("expected profile_overrides_not_allowed, got %q", out.Error.Code)
	}
}

func TestPublicProfilesReturnsOnlyPublicProfilesWithoutConfig(t *testing.T) {
	srv := New(DefaultConfig())
	createProfile(t, srv, `{"name":"public-profile","description":"Shown","config":{"net":{"ipv4":true}},"public":true}`)
	createProfile(t, srv, `{"name":"private-profile","description":"Hidden","config":{"net":{"ipv6":false}},"public":false}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/profiles", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var views []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 public profile, got %d", len(views))
	}
	if views[0]["name"] != "public-profile" {
		t.Fatalf("expected public-profile, got %#v", views[0])
	}
	if _, ok := views[0]["config"]; ok {
		t.Fatalf("public profile payload must not expose config: %#v", views[0])
	}
}

func TestPublicGetJobReturnsPublicIDNotUUID(t *testing.T) {
	srv := New(DefaultConfig())

	// Create via public API.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	var created PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	// Get via public ID.
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID, nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got["public_id"] != created.PublicID {
		t.Fatalf("public_id: got %v, want %q", got["public_id"], created.PublicID)
	}
	if _, hasID := got["id"]; hasID {
		t.Fatal("internal UUID must not appear in public get response")
	}
}

func TestPublicGetJobUnknownPublicIDReturns404(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/notexist1", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestPublicGetResultReturnsResultByPublicID(t *testing.T) {
	srv := New(DefaultConfig())

	// Create a job and manually store a result for it.
	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
		NameserverTimings: []NameserverTiming{
			{
				Nameserver: "ns1.example.com",
				Address:    "192.0.2.10",
				AvgMS:      24,
				MinMS:      20,
				MaxMS:      30,
				MedianMS:   22,
				StddevMS:   4,
				Count:      3,
			},
		},
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := srv.store.GraduateJob(created, nil); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Status != JobSucceeded {
		t.Fatalf("status: got %q, want %q", result.Status, JobSucceeded)
	}
	if len(result.NameserverTimings) != 1 {
		t.Fatalf("nameserver timings len = %d, want 1", len(result.NameserverTimings))
	}
}

func TestPublicGetResultUnknownPublicIDReturns404(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/notexist1/result", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestPublicLocalesEndpointAccessible(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/locales", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from /pub/api/v1/locales, got %d", resp.Code)
	}
}

func TestPublicGetResultNoResultYetReturns404(t *testing.T) {
	srv := New(DefaultConfig())

	// Create a job but do not store a result for it.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	var created PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when result not yet available, got %d", resp.Code)
	}
}

func TestPublicGetResultRespectsLocaleParam(t *testing.T) {
	srv := New(DefaultConfig())

	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
	}
	created, _ := srv.store.Create(job)
	if err := srv.store.GraduateJob(created, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "BASIC01", Level: "NOTICE"},
	}); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result?locale=sv", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Raw == nil || result.Raw.Locale != "sv" {
		t.Fatalf("expected locale=sv in result, got %v", result.Raw)
	}
}

func TestPublicAPIJobCreatedViaInternalAPIFetchableByPublicID(t *testing.T) {
	srv := New(DefaultConfig())

	// Create via internal API.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("internal create: expected 201, got %d", resp.Code)
	}
	var internalJob Job
	if err := json.NewDecoder(resp.Body).Decode(&internalJob); err != nil {
		t.Fatalf("decode internal job: %v", err)
	}
	if internalJob.PublicID == "" {
		t.Fatal("internal API response must include public_id")
	}

	// Fetch via public API using the public_id from the internal response.
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+internalJob.PublicID, nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("public get: expected 200, got %d", resp.Code)
	}
	var view PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode public view: %v", err)
	}
	if view.PublicID != internalJob.PublicID {
		t.Fatalf("public_id mismatch: got %q, want %q", view.PublicID, internalJob.PublicID)
	}
}

func TestPublicAPIEndpointsUnreachableViaInternalPrefix(t *testing.T) {
	srv := New(DefaultConfig())

	// Admin-only endpoints must not be reachable via /pub/
	for _, path := range []string{
		"/pub/api/v1/metrics",
		"/pub/api/v1/queue/pause",
		"/pub/api/v1/batches",
	} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Handler().ServeHTTP(resp, req)
		if resp.Code == http.StatusOK {
			t.Errorf("path %q should not return 200 via public prefix", path)
		}
	}
}

func TestPublicResultOmitsScoreWhenPublicScoringDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowScorePublic = false
	cfg.ShowNameserverTimingsPublic = false
	srv := New(cfg)

	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
		NameserverTimings: []NameserverTiming{
			{Nameserver: "ns1.example.com", Address: "192.0.2.10", AvgMS: 24, MinMS: 20, MaxMS: 30, Count: 3},
		},
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := srv.store.GraduateJob(created, nil); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Score != nil {
		t.Fatal("expected Score to be nil when ShowScorePublic=false")
	}
	if result.NameserverTimings != nil {
		t.Fatal("expected NameserverTimings to be nil when ShowNameserverTimingsPublic=false")
	}
}

func TestPublicCreateJobAcceptsIPv6Disabled(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com","ipv6_disabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var view PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	job, ok := srv.store.GetByPublicID(view.PublicID)
	if !ok {
		t.Fatal("expected stored public job")
	}
	if !job.IPv6Disabled {
		t.Fatal("expected job.IPv6Disabled=true")
	}
	if job.IPv4Disabled {
		t.Fatal("expected job.IPv4Disabled=false")
	}
}

func TestPublicCreateJobAcceptsIPv4Disabled(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com","ipv4_disabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var view PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	job, ok := srv.store.GetByPublicID(view.PublicID)
	if !ok {
		t.Fatal("expected stored public job")
	}
	if !job.IPv4Disabled {
		t.Fatal("expected job.IPv4Disabled=true")
	}
}
