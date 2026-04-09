package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/"+tag+"/domains", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("addDomainsToTag: expected 204, got %d: %s", resp.Code, resp.Body.String())
	}
}

func setTagProfile(t *testing.T, srv *Server, tag string, profileID int64) {
	t.Helper()
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/"+tag+"/profile",
		bytes.NewBufferString(fmt.Sprintf(`{"profile_id":%d}`, profileID)))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("setTagProfile: expected 204, got %d: %s", resp.Code, resp.Body.String())
	}
}

func getRunByAPI(t *testing.T, srv *Server, id string) Run {
	t.Helper()
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+id, nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("getRunByAPI: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var run Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	return run
}

func TestProfileIntegrationBatchFromTagSnapshotsStoredProfile(t *testing.T) {
	srv := New(DefaultConfig())
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

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"from_tag":"ops"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

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
	srv := New(DefaultConfig())
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	storedProfile := createProfile(t, srv, `{
		"name":"strict",
		"config":{"net":{"ipv4":false},"resolver":{"defaults":{"timeout":5}}}
	}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(fmt.Sprintf(`{
			"domain":"example.com",
			"profile_id":%d,
			"profile_overrides":{"resolver":{"defaults":{"timeout":7}}}
		}`, storedProfile.ID)))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

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
