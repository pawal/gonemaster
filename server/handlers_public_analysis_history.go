package server

import (
	"net/http"
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

	points, err := readStore.AnalysisEntityHistory(cohort.ID, entity, key)
	if err != nil {
		writeError(w, http.StatusBadRequest, "history_error", err.Error(), nil)
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
