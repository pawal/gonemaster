package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	engineprofile "codeberg.org/pawal/gonemaster/engine/profile"
)

func addDomainsToTag(t *testing.T, srv *Server, tag string, domains []string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"domains": domains})
	if err != nil {
		t.Fatalf("marshal domains: %v", err)
	}
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/tags/"+tag+"/domains", body)
	wantStatus(t, resp, http.StatusNoContent)
}

func setTagProfile(t *testing.T, srv *Server, tag string, profileID int64) {
	t.Helper()
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/tags/"+tag+"/profile", fmt.Sprintf(`{"profile_id":%d}`, profileID))
	wantStatus(t, resp, http.StatusNoContent)
}

func getRunByAPI(t *testing.T, srv *Server, id string) Run {
	t.Helper()
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/runs/"+id, nil)
	run := mustJSON[Run](t, resp, http.StatusOK)
	return run
}

func TestProfileIntegrationBatchFromTagSnapshotsStoredProfile(t *testing.T) {
	srv := newTestServer(t)
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{{Module: "BASIC", Testcase: "basic01", Tag: "BASIC01", Level: "NOTICE"}}, nil
	}

	storedProfile := createProfile(t, srv, `{
		"name":"strict",
		"config":{"net":{"ipv4":false},"resolver":{"defaults":{"timeout":5}}}
	}`)
	createTag(t, srv, "ops", "")
	addDomainsToTag(t, srv, "ops", []string{"example.com"})
	setTagProfile(t, srv, "ops", storedProfile.ID)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs/batch", `{"from_tag":"ops"}`)
	wantStatus(t, resp, http.StatusAccepted)

	var batchResp JobBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(batchResp.JobIDs) != 1 {
		t.Fatalf("expected 1 batch job, got %d", len(batchResp.JobIDs))
	}

	if err := srv.runJob(batchResp.JobIDs[0]); err != nil {
		t.Fatalf("run job: %v", err)
	}

	run := getRunByAPI(t, srv, batchResp.JobIDs[0])
	if run.ProfileID == nil || *run.ProfileID != storedProfile.ID {
		t.Fatalf("ProfileID: got %v, want %d", run.ProfileID, storedProfile.ID)
	}
	if run.ProfileName != "strict" {
		t.Fatalf("ProfileName: got %q", run.ProfileName)
	}
	if run.EntryCount != 1 {
		t.Fatalf("EntryCount: got %d, want 1", run.EntryCount)
	}

	effective, err := engineprofile.FromJSON(run.EffectiveProfile)
	if err != nil {
		t.Fatalf("parse effective profile: %v", err)
	}
	timeout, err := effective.Get("resolver.defaults.timeout")
	if err != nil {
		t.Fatalf("get timeout: %v", err)
	}
	if value, ok := timeout.(int); !ok || value != 5 {
		t.Fatalf("expected timeout 5, got %v", timeout)
	}
}

func TestProfileIntegrationJobOverridesAppearInRunSnapshot(t *testing.T) {
	srv := newTestServer(t)
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	storedProfile := createProfile(t, srv, `{
		"name":"strict",
		"config":{"net":{"ipv4":false},"resolver":{"defaults":{"timeout":5}}}
	}`)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", fmt.Sprintf(`{
			"domain":"example.com",
			"profile_id":%d,
			"profile_overrides":{"resolver":{"defaults":{"timeout":7}}}
		}`, storedProfile.ID))
	wantStatus(t, resp, http.StatusCreated)

	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if created.ProfileID == nil || *created.ProfileID != storedProfile.ID {
		t.Fatalf("ProfileID: got %v, want %d", created.ProfileID, storedProfile.ID)
	}
	if created.ProfileName != "strict" {
		t.Fatalf("ProfileName: got %q", created.ProfileName)
	}

	if err := srv.runJob(created.ID); err != nil {
		t.Fatalf("run job: %v", err)
	}

	run := getRunByAPI(t, srv, created.ID)
	if run.ProfileID == nil || *run.ProfileID != storedProfile.ID {
		t.Fatalf("run ProfileID: got %v, want %d", run.ProfileID, storedProfile.ID)
	}
	if run.ProfileName != "strict" {
		t.Fatalf("run ProfileName: got %q", run.ProfileName)
	}
	if run.EffectiveProfile == "" {
		t.Fatal("expected effective_profile snapshot")
	}

	effective, err := engineprofile.FromJSON(run.EffectiveProfile)
	if err != nil {
		t.Fatalf("parse effective profile: %v", err)
	}
	ipv4, err := effective.Get("net.ipv4")
	if err != nil {
		t.Fatalf("get net.ipv4: %v", err)
	}
	if value, ok := ipv4.(bool); !ok || value != false {
		t.Fatalf("expected net.ipv4 false, got %v", ipv4)
	}
	timeout, err := effective.Get("resolver.defaults.timeout")
	if err != nil {
		t.Fatalf("get timeout: %v", err)
	}
	if value, ok := timeout.(int); !ok || value != 7 {
		t.Fatalf("expected timeout 7, got %v", timeout)
	}
}

// TestPublicAPIDisableIPv6IntegrationReachesEngineAndEffectiveProfile exercises
// the full pipeline the public UI uses: POST /pub/api/v1/jobs with
// ipv6_disabled=true, worker picks up the job, engine-runner boundary sees
// req.IPv6=false, and the snapshotted effective profile has net.ipv6=false.
//
// Regression test for the bug where public-UI "disable IPv6" runs returned
// "The test run failed. Please try again." because JobCreateRequest lacked
// the field and readJSON's DisallowUnknownFields rejected the payload.
func TestPublicAPIDisableIPv6IntegrationReachesEngineAndEffectiveProfile(t *testing.T) {
	srv := newTestServer(t)

	var capturedIPv4, capturedIPv6 *bool
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		capturedIPv4 = req.IPv4
		capturedIPv6 = req.IPv6
		return nil, nil
	}

	// POST the exact body shape the public UI sends.
	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com","ipv6_disabled":true}`)

	created := mustJSON[PublicJobView](t, resp, http.StatusCreated)

	// Resolve internal job ID via the store so we can drive the worker.
	stored, ok := srv.store.GetByPublicID(created.PublicID)
	if !ok {
		t.Fatal("expected stored job for public id")
	}
	if !stored.IPv6Disabled {
		t.Fatal("expected stored Job.IPv6Disabled=true")
	}

	if err := srv.runJob(stored.ID); err != nil {
		t.Fatalf("runJob: %v", err)
	}

	// Engine boundary: IPv6 pointer set to false, IPv4 untouched.
	if capturedIPv6 == nil || *capturedIPv6 {
		t.Fatalf("expected captured req.IPv6=false, got %#v", capturedIPv6)
	}
	if capturedIPv4 != nil {
		t.Fatalf("expected captured req.IPv4=nil, got %#v", capturedIPv4)
	}

	// Effective profile snapshot on the Run should reflect the disable.
	run := getRunByAPI(t, srv, stored.ID)
	if run.EffectiveProfile == "" {
		t.Fatal("expected effective_profile snapshot")
	}
	effective, err := engineprofile.FromJSON(run.EffectiveProfile)
	if err != nil {
		t.Fatalf("parse effective profile: %v", err)
	}
	ipv6, err := effective.Get("net.ipv6")
	if err != nil {
		t.Fatalf("get net.ipv6: %v", err)
	}
	if value, ok := ipv6.(bool); !ok || value {
		t.Fatalf("expected net.ipv6 false in effective profile, got %v", ipv6)
	}
}
