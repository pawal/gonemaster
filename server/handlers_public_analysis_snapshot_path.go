package server

import (
	"net/http"
	"net/url"
	"strings"
)

// pubAnalysisSnapshotPathPrefix is the URL prefix for path-segmented
// snapshot reads.
const pubAnalysisSnapshotPathPrefix = "/pub/api/v1/analysis/cohorts/"

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

// maybeRedirectToSnapshotPath 307s legacy query-param reads to the
// path-segmented URL when no explicit ?snapshot= is set and the cohort
// has a captured public snapshot. Falls through otherwise.
func (s *Server) maybeRedirectToSnapshotPath(child http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.URL.Query().Get("snapshot")) != "" {
			child(w, r)
			return
		}
		cohort, err := ResolveAnalysisCohort(
			s.store.ListAnalysisCohorts(),
			strings.TrimSpace(r.URL.Query().Get("dataset_tag")),
			"",
		)
		if err != nil {
			child(w, r)
			return
		}
		readStore, ok := s.store.(AnalysisReadStore)
		if !ok {
			child(w, r)
			return
		}
		snap, found := readStore.GetDefaultSnapshotForCohort(cohort.ID)
		if !found || !isPublicSnapshot(snap) {
			child(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		http.Redirect(w, r, snapshotPathTargetURL(r, cohort.SourceTag, snap.Slug), http.StatusTemporaryRedirect)
	}
}

// snapshotPathTargetURL builds the path-segmented URL mirroring the
// caller's legacy URL, with dataset_tag + snapshot lifted into the path.
func snapshotPathTargetURL(r *http.Request, datasetTag, slug string) string {
	sub := strings.TrimPrefix(r.URL.Path, "/analysis/")
	target := pubAnalysisSnapshotPathPrefix + url.PathEscape(datasetTag) +
		"/snapshots/" + url.PathEscape(slug)
	if sub != "" {
		target += "/" + sub
	}
	q := r.URL.Query()
	q.Del("dataset_tag")
	q.Del("snapshot")
	if encoded := q.Encode(); encoded != "" {
		target += "?" + encoded
	}
	return target
}
