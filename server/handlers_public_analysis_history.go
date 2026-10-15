package server

import (
	"net/http"
	"strconv"
	"strings"
)

// PublicAnalysisEntityHistoryResponse is the /history response: one entity's
// metric points across the cohort's captured snapshots, oldest first.
type PublicAnalysisEntityHistoryResponse struct {
	DatasetTag string                       `json:"dataset_tag"`
	Entity     string                       `json:"entity"`
	Key        string                       `json:"key"`
	Points     []AnalysisEntityHistoryPoint `json:"points"`
}

// handlePublicAnalysisEntityHistory handles
// GET /pub/api/v1/analysis/cohorts/{dataset_tag}/history?entity=&key=.
func (s *Server) handlePublicAnalysisEntityHistory(w http.ResponseWriter, r *http.Request) {
	datasetTag, ok := pathValueNonEmpty(w, r, "dataset_tag", "cohort")
	if !ok {
		return
	}
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		s.writePublicAnalysisResolutionError(w, r, err)
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	entity := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("entity")))
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if entity == "" || key == "" {
		writeError(w, http.StatusBadRequest, "missing_params",
			"history requires ?entity= and ?key=", nil)
		return
	}
	switch entity {
	case "nameserver", "asn", "tag", "domain":
	default:
		writeError(w, http.StatusBadRequest, "invalid_entity",
			"entity must be one of nameserver, asn, tag, domain", nil)
		return
	}
	if !validHistoryKey(entity, key) {
		writeError(w, http.StatusBadRequest, "invalid_key", "key is not a valid "+entity, nil)
		return
	}

	points, err := readStore.AnalysisEntityHistory(cohort.ID, entity, key)
	if err != nil {
		s.logger.Warn("analysis history read failed", "entity", entity, "err", err)
		writeError(w, http.StatusInternalServerError, "history_error", "history is unavailable", nil)
		return
	}
	if points == nil {
		points = []AnalysisEntityHistoryPoint{}
	}
	writeJSON(w, http.StatusOK, PublicAnalysisEntityHistoryResponse{
		DatasetTag: cohort.SourceTag,
		Entity:     entity,
		Key:        key,
		Points:     points,
	})
}

// validHistoryKey accepts a key of at most 255 octets; an asn key is a number with an optional "AS" prefix.
func validHistoryKey(entity, key string) bool {
	if len(key) > 255 {
		return false
	}
	if entity == "asn" {
		_, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(key), "as"), 10, 32)
		return err == nil
	}
	return true
}
