package server

import (
	"encoding/json"
	"net/http"

	"codeberg.org/pawal/gonemaster/scoring"
)

type scoringConfigResponse struct {
	Config   scoring.Config `json:"config"`
	Source   SettingSource  `json:"source"`
	Readonly bool           `json:"readonly,omitempty"`
}

// scoringConfigSource resolves where the effective scoring config came from.
func (s *Server) scoringConfigSource() SettingSource {
	if s.configSources != nil && s.configSources["scoring_config"] == SourceCLIFlag {
		return SourceCLIFlag
	}
	if _, ok := s.store.GetSetting("scoring_config"); ok {
		return SourceDatabase
	}
	if s.cfg.ScoringConfigPath != "" {
		return SourceConfigFile
	}
	return SourceDefault
}

// effectiveScoringConfig builds the scoring config following the precedence chain.
func (s *Server) effectiveScoringConfig() (scoring.Config, error) {
	if s.configSources != nil && s.configSources["scoring_config"] == SourceCLIFlag {
		return scoring.LoadConfig(s.cfg.ScoringConfigPath)
	}
	if raw, ok := s.store.GetSetting("scoring_config"); ok {
		var cfg scoring.Config
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return scoring.Config{}, err
		}
		return cfg, nil
	}
	if s.cfg.ScoringConfigPath != "" {
		return scoring.LoadConfig(s.cfg.ScoringConfigPath)
	}
	return scoring.DefaultConfig(), nil
}

// applyScoringConfigToStore calls SetScoringConfig on supported store types.
func applyScoringConfigToStore(store JobStore, cfg scoring.Config) {
	switch s := store.(type) {
	case *InMemoryJobStore:
		s.SetScoringConfig(cfg)
	case *SQLJobStore:
		s.SetScoringConfig(cfg)
	}
}

func (s *Server) handleScoringConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetScoringConfig(w, r)
	case http.MethodPut:
		if !enforceCSRF(w, r) {
			return
		}
		s.handlePutScoringConfig(w, r)
	case http.MethodDelete:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleDeleteScoringConfig(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleGetScoringConfig(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.effectiveScoringConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scoring_config_error", err.Error(), nil)
		return
	}
	src := s.scoringConfigSource()
	writeJSON(w, http.StatusOK, scoringConfigResponse{
		Config:   cfg,
		Source:   src,
		Readonly: src == SourceCLIFlag,
	})
}

func (s *Server) handlePutScoringConfig(w http.ResponseWriter, r *http.Request) {
	if s.configSources != nil && s.configSources["scoring_config"] == SourceCLIFlag {
		writeError(w, http.StatusBadRequest, "readonly_scoring_config",
			"scoring config is set via CLI flag and cannot be overridden at runtime", nil)
		return
	}

	var cfg scoring.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_scoring_config", "invalid JSON: "+err.Error(), nil)
		return
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scoring_config_error", err.Error(), nil)
		return
	}
	if err := s.store.SetSetting("scoring_config", string(raw)); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	applyScoringConfigToStore(s.store, cfg)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteScoringConfig(w http.ResponseWriter, _ *http.Request) {
	if s.configSources != nil && s.configSources["scoring_config"] == SourceCLIFlag {
		writeError(w, http.StatusBadRequest, "readonly_scoring_config",
			"scoring config is set via CLI flag and cannot be overridden at runtime", nil)
		return
	}

	// Ignore not-found — idempotent.
	_ = s.store.DeleteSetting("scoring_config")

	// Revert to file config or defaults.
	var revertCfg scoring.Config
	if s.cfg.ScoringConfigPath != "" {
		loaded, err := scoring.LoadConfig(s.cfg.ScoringConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scoring_config_error", err.Error(), nil)
			return
		}
		revertCfg = loaded
	} else {
		revertCfg = scoring.DefaultConfig()
	}
	applyScoringConfigToStore(s.store, revertCfg)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleScoringConfigDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, scoring.DefaultConfig())
}
