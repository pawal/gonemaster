package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	engineprofile "codeberg.org/pawal/gonemaster/engine/profile"
)

type profileDiffRequest struct {
	Config json.RawMessage `json:"config"`
}

// handleProfileDiff handles POST /profiles/diff.
// The config travels in the request body so an unsaved editor draft can be
// compared without being stored first. The endpoint changes no state.
func (s *Server) handleProfileDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !s.enforceCSRF(w, r) {
		return
	}

	var req profileDiffRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	configJSON := "{}"
	if len(req.Config) > 0 {
		configJSON = string(req.Config)
	}

	override, err := engineprofile.FromJSON(configJSON)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error(), nil)
		return
	}
	defaultP, err := engineprofile.Default()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "engine_error", fmt.Sprintf("load default profile: %s", err), nil)
		return
	}

	writeJSON(w, http.StatusOK, engineprofile.Diff(override, defaultP))
}
