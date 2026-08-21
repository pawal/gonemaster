package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	engineprofile "codeberg.org/pawal/gonemaster/engine/profile"
)

type profileUpsertRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
	Public      bool            `json:"public"`
}

type tagProfileRequest struct {
	ProfileID int64 `json:"profile_id"`
}

type resolvedProfileRef struct {
	ID   *int64
	Name string
}

func (s *Server) handleDefaultProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	apiProfile, err := s.defaultAPIProfile()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, apiProfile)
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !s.enforceCSRF(w, r) {
			return
		}
		s.handleCreateProfile(w, r)
	case http.MethodGet:
		s.handleListProfiles(w)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleProfileByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseProfileID(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetProfile(w, id)
	case http.MethodPut:
		if !s.enforceCSRF(w, r) {
			return
		}
		s.handleUpdateProfile(w, r, id)
	case http.MethodDelete:
		if !s.enforceCSRF(w, r) {
			return
		}
		s.handleDeleteProfile(w, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func parseProfileID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := strings.TrimSpace(r.PathValue("id"))
	if idStr == "" {
		writeError(w, http.StatusNotFound, "not_found", "profile not found", nil)
		return 0, false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid profile id", nil)
		return 0, false
	}
	return id, true
}

func (s *Server) defaultAPIProfile() (Profile, error) {
	base, err := engineprofile.Default()
	if err != nil {
		return Profile{}, err
	}
	if strings.TrimSpace(s.cfg.ProfilePath) != "" {
		data, err := os.ReadFile(s.cfg.ProfilePath)
		if err != nil {
			return Profile{}, err
		}
		override, err := engineprofile.FromYAML(string(data))
		if err != nil {
			return Profile{}, err
		}
		if err := base.Merge(override); err != nil {
			return Profile{}, err
		}
	}
	configJSON, err := base.ToJSON()
	if err != nil {
		return Profile{}, err
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return Profile{}, err
	}
	description := "Server base profile"
	if strings.TrimSpace(s.cfg.ProfilePath) != "" {
		description = "Server base profile with config-file overrides"
	}
	return Profile{
		ID:          0,
		Name:        "default",
		Description: description,
		Config:      config,
		Public:      false,
	}, nil
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req profileUpsertRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	stored, apiProfile, err := storedAndAPIProfileFromRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error(), nil)
		return
	}
	stored.SchemaVersion = engine.VersionFull()
	apiProfile.SchemaVersion = stored.SchemaVersion
	if _, ok := s.store.GetProfileByName(stored.Name); ok {
		writeError(w, http.StatusConflict, "profile_exists", "profile name already exists", nil)
		return
	}
	created, err := s.store.CreateProfile(stored)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	apiProfile.ID = created.ID
	apiProfile.CreatedAt = created.CreatedAt
	apiProfile.UpdatedAt = created.UpdatedAt
	writeJSON(w, http.StatusCreated, apiProfile)
}

func (s *Server) handleListProfiles(w http.ResponseWriter) {
	storedProfiles := s.store.ListProfiles()
	profiles := make([]Profile, 0, len(storedProfiles))
	for _, stored := range storedProfiles {
		apiProfile, err := apiProfileFromStored(stored)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
		profiles = append(profiles, apiProfile)
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (s *Server) handleGetProfile(w http.ResponseWriter, id int64) {
	stored, ok := s.store.GetProfile(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "profile not found", nil)
		return
	}
	apiProfile, err := apiProfileFromStored(stored)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, apiProfile)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request, id int64) {
	existing, ok := s.store.GetProfile(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "profile not found", nil)
		return
	}

	var req profileUpsertRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	stored, apiProfile, err := storedAndAPIProfileFromRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error(), nil)
		return
	}
	if conflict, ok := s.store.GetProfileByName(stored.Name); ok && conflict.ID != id {
		writeError(w, http.StatusConflict, "profile_exists", "profile name already exists", nil)
		return
	}

	stored.ID = id
	stored.CreatedAt = existing.CreatedAt
	stored.SchemaVersion = engine.VersionFull()
	apiProfile.SchemaVersion = stored.SchemaVersion
	if err := s.store.UpdateProfile(stored); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	updated, ok := s.store.GetProfile(id)
	if !ok {
		writeError(w, http.StatusInternalServerError, "store_error", "updated profile not found", nil)
		return
	}
	apiProfile.ID = updated.ID
	apiProfile.CreatedAt = updated.CreatedAt
	apiProfile.UpdatedAt = updated.UpdatedAt
	writeJSON(w, http.StatusOK, apiProfile)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, id int64) {
	if _, ok := s.store.GetProfile(id); !ok {
		writeError(w, http.StatusNotFound, "not_found", "profile not found", nil)
		return
	}
	if err := s.store.DeleteProfile(id); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTagProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	switch r.Method {
	case http.MethodPut:
		if !s.enforceCSRF(w, r) {
			return
		}
		s.handleSetTagProfile(w, r, name)
	case http.MethodDelete:
		if !s.enforceCSRF(w, r) {
			return
		}
		s.handleDeleteTagProfile(w, name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleSetTagProfile(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.store.GetTag(name); !ok {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	var req tagProfileRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if req.ProfileID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_profile_id", "profile_id is required", nil)
		return
	}
	if _, ok := s.store.GetProfile(req.ProfileID); !ok {
		writeError(w, http.StatusNotFound, "profile_not_found", "profile not found", nil)
		return
	}
	if err := s.store.SetTagDefaultProfile(name, &req.ProfileID); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteTagProfile(w http.ResponseWriter, name string) {
	if _, ok := s.store.GetTag(name); !ok {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	if err := s.store.SetTagDefaultProfile(name, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func storedAndAPIProfileFromRequest(req profileUpsertRequest) (StoredProfile, Profile, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return StoredProfile{}, Profile{}, fmt.Errorf("name is required")
	}
	configJSON, configMap, err := normalizeAndValidateProfileConfig(req.Config)
	if err != nil {
		return StoredProfile{}, Profile{}, err
	}
	stored := StoredProfile{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Config:      configJSON,
		Public:      req.Public,
	}
	api := Profile{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Config:      configMap,
		Public:      req.Public,
	}
	return stored, api, nil
}

func apiProfileFromStored(stored StoredProfile) (Profile, error) {
	configMap := map[string]any{}
	if strings.TrimSpace(stored.Config) != "" {
		if err := json.Unmarshal([]byte(stored.Config), &configMap); err != nil {
			return Profile{}, fmt.Errorf("decode stored profile %d config: %w", stored.ID, err)
		}
	}
	if configMap == nil {
		configMap = map[string]any{}
	}
	return Profile{
		ID:            stored.ID,
		Name:          stored.Name,
		Description:   stored.Description,
		Config:        configMap,
		Public:        stored.Public,
		SchemaVersion: stored.SchemaVersion,
		CreatedAt:     stored.CreatedAt,
		UpdatedAt:     stored.UpdatedAt,
	}, nil
}

func normalizeAndValidateProfileConfig(raw json.RawMessage) (string, map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		trimmed = []byte(`{}`)
	}
	validated, err := engineprofile.FromJSON(string(trimmed))
	if err != nil {
		return "", nil, fmt.Errorf("invalid profile config: %w", err)
	}
	canonical, err := validated.ToJSON()
	if err != nil {
		return "", nil, fmt.Errorf("serialize profile config: %w", err)
	}
	configMap := map[string]any{}
	if err := json.Unmarshal([]byte(canonical), &configMap); err != nil {
		return "", nil, fmt.Errorf("decode normalized profile config: %w", err)
	}
	if configMap == nil {
		configMap = map[string]any{}
	}
	return canonical, configMap, nil
}

func (s *Server) resolveStoredProfile(profileID *int64, publicOnly bool) (resolvedProfileRef, string, string) {
	if profileID == nil {
		return resolvedProfileRef{}, "", ""
	}
	if *profileID <= 0 {
		return resolvedProfileRef{}, "invalid_profile_id", "profile_id must be a positive integer"
	}
	stored, ok := s.store.GetProfile(*profileID)
	if !ok {
		return resolvedProfileRef{}, "profile_not_found", "profile not found"
	}
	if publicOnly && !stored.Public {
		return resolvedProfileRef{}, "profile_not_public", "profile is not available in the public API"
	}
	return resolvedProfileRef{
		ID:   &stored.ID,
		Name: stored.Name,
	}, "", ""
}

func (s *Server) resolveDefaultProfileFromTags(tagNames []string) (resolvedProfileRef, string, string) {
	var selectedID int64
	found := false
	for _, tagName := range tagNames {
		tag, ok := s.store.GetTag(tagName)
		if !ok || tag.DefaultProfileID == nil {
			continue
		}
		if !found {
			selectedID = *tag.DefaultProfileID
			found = true
			continue
		}
		if selectedID != *tag.DefaultProfileID {
			return resolvedProfileRef{}, "ambiguous_profile", "multiple tags define different default profiles; choose profile_id explicitly"
		}
	}
	if !found {
		return resolvedProfileRef{}, "", ""
	}
	return s.resolveStoredProfile(&selectedID, false)
}
