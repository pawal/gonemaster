package server

import (
	"net/http"
	"strings"
)

// pubAnalysisSnapshotPath wraps a query-param analysis handler so it
// can be reached at /cohorts/{tag}/snapshots/{slug}/<sub>. The cohort
// + slug are taken from path values and forwarded as query params.
func pubAnalysisSnapshotPath(child http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		datasetTag := strings.TrimSpace(r.PathValue("dataset_tag"))
		slug := strings.TrimSpace(r.PathValue("slug"))
		if datasetTag == "" || slug == "" {
			writeError(w, http.StatusNotFound, "not_found", "cohort or snapshot missing", nil)
			return
		}
		r2 := r.Clone(r.Context())
		q := r2.URL.Query()
		q.Set("dataset_tag", datasetTag)
		q.Set("snapshot", slug)
		r2.URL.RawQuery = q.Encode()
		child(w, r2)
	}
}
