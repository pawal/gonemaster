package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func createProfile(t *testing.T, srv *Server, body string) Profile {
	t.Helper()
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("createProfile: expected 201, got %d: %s", resp.Code, resp.Body)
	}
	var profile Profile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		t.Fatalf("createProfile decode: %v", err)
	}
	return profile
}

func TestCreateProfile(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{
		"name":"strict-dnssec",
		"description":"Strict DNSSEC validation",
		"config":{"resolver":{"defaults":{"timeout":10}}},
		"public":true
	}`)

	if profile.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if profile.Name != "strict-dnssec" {
		t.Fatalf("Name: got %q", profile.Name)
	}
	if profile.Description != "Strict DNSSEC validation" {
		t.Fatalf("Description: got %q", profile.Description)
	}
	if !profile.Public {
		t.Fatal("expected Public=true")
	}
	resolver, ok := profile.Config["resolver"].(map[string]any)
	if !ok {
		t.Fatalf("expected resolver config, got %#v", profile.Config)
	}
	defaults, ok := resolver["defaults"].(map[string]any)
	if !ok || defaults["timeout"] != float64(10) {
		t.Fatalf("expected timeout=10, got %#v", profile.Config)
	}

	stored, ok := srv.store.GetProfile(profile.ID)
	if !ok {
		t.Fatal("expected stored profile")
	}
	if stored.Config != `{"resolver":{"defaults":{"timeout":10}}}` {
		t.Fatalf("stored Config: got %q", stored.Config)
	}
}

func TestCreateProfileRejectsInvalidConfig(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles",
		bytes.NewBufferString(`{"name":"broken","config":{"net":1}}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body)
	}
	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "invalid_profile" {
		t.Fatalf("expected invalid_profile, got %q", out.Error.Code)
	}
}

func TestListProfiles(t *testing.T) {
	srv := New(DefaultConfig())
	createProfile(t, srv, `{"name":"alpha","config":{"net":{"ipv4":true}}}`)
	createProfile(t, srv, `{"name":"beta","config":{"net":{"ipv6":false}}}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var profiles []Profile
	if err := json.NewDecoder(resp.Body).Decode(&profiles); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0].Name != "alpha" || profiles[1].Name != "beta" {
		t.Fatalf("unexpected order: %#v", profiles)
	}
}

func TestGetProfile(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/profiles/%d", profile.ID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got Profile
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != profile.ID {
		t.Fatalf("ID: got %d, want %d", got.ID, profile.ID)
	}
	if got.Name != "default" {
		t.Fatalf("Name: got %q", got.Name)
	}
}

func TestUpdateProfile(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/profiles/%d", profile.ID),
		bytes.NewBufferString(`{
			"name":"strict",
			"description":"Updated profile",
			"config":{"resolver":{"defaults":{"retry":3}}},
			"public":true
		}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var updated Profile
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.Name != "strict" {
		t.Fatalf("Name: got %q", updated.Name)
	}
	if updated.Description != "Updated profile" {
		t.Fatalf("Description: got %q", updated.Description)
	}
	if !updated.Public {
		t.Fatal("expected Public=true")
	}

	stored, ok := srv.store.GetProfile(profile.ID)
	if !ok {
		t.Fatal("expected updated stored profile")
	}
	if stored.Name != "strict" {
		t.Fatalf("stored Name: got %q", stored.Name)
	}
	if stored.Config != `{"resolver":{"defaults":{"retry":3}}}` {
		t.Fatalf("stored Config: got %q", stored.Config)
	}
}

func TestDeleteProfile(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/profiles/%d", profile.ID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body)
	}
	if _, ok := srv.store.GetProfile(profile.ID); ok {
		t.Fatal("expected profile to be deleted")
	}
}

func TestSetTagProfile(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "ops", "")
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/ops/profile",
		bytes.NewBufferString(fmt.Sprintf(`{"profile_id":%d}`, profile.ID)))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body)
	}

	tag, ok := srv.store.GetTag("ops")
	if !ok {
		t.Fatal("expected tag")
	}
	if tag.DefaultProfileID == nil || *tag.DefaultProfileID != profile.ID {
		t.Fatalf("DefaultProfileID: got %v, want %d", tag.DefaultProfileID, profile.ID)
	}
}

func TestDeleteTagProfile(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "ops", "")
	profile := createProfile(t, srv, `{"name":"default","config":{"net":{"ipv4":true}}}`)
	if err := srv.store.SetTagDefaultProfile("ops", &profile.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/ops/profile", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body)
	}

	tag, ok := srv.store.GetTag("ops")
	if !ok {
		t.Fatal("expected tag")
	}
	if tag.DefaultProfileID != nil {
		t.Fatalf("expected cleared DefaultProfileID, got %v", *tag.DefaultProfileID)
	}
}
