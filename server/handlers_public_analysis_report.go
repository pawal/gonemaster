package server

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"

	"codeberg.org/pawal/gonemaster/scoring"
)

// analysisReportCacheSize caps the rendered reports kept in memory. A pair
// is a few hundred kilobytes and only the pairs a reader is flipping
// between are worth holding.
const analysisReportCacheSize = 16

// analysisReportCache memoizes rendered reports per snapshot pair and
// coalesces concurrent computes of the same pair.
type analysisReportCache struct {
	mu      sync.Mutex
	group   singleflight.Group
	entries map[string]PublicAnalysisReportResponse
	order   []string
}

func newAnalysisReportCache() *analysisReportCache {
	return &analysisReportCache{entries: map[string]PublicAnalysisReportResponse{}}
}

func (c *analysisReportCache) get(key string) (PublicAnalysisReportResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp, ok := c.entries[key]
	return resp, ok
}

func (c *analysisReportCache) put(key string, resp PublicAnalysisReportResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; !ok {
		c.order = append(c.order, key)
	}
	c.entries[key] = resp
	for len(c.order) > analysisReportCacheSize {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
}

// reset drops every cached report. Called when the scoring configuration
// changes, since attribution is computed under it.
func (c *analysisReportCache) reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]PublicAnalysisReportResponse{}
	c.order = nil
}

// compute returns the cached report for key, computing it once when several
// requests race on the same pair.
func (c *analysisReportCache) compute(key string, build func() PublicAnalysisReportResponse) PublicAnalysisReportResponse {
	if resp, ok := c.get(key); ok {
		return resp
	}
	v, _, _ := c.group.Do(key, func() (any, error) {
		if resp, ok := c.get(key); ok {
			return resp, nil
		}
		resp := build()
		c.put(key, resp)
		return resp, nil
	})
	return v.(PublicAnalysisReportResponse)
}

// reportCacheKey addresses one rendered report. Both snapshots' update
// times are part of it so a rematerialize does not serve a stale report.
func reportCacheKey(from, to AnalysisCohortSnapshot, minCluster, maxSpread int) string {
	return strings.Join([]string{
		strconv.FormatInt(from.ID, 10),
		strconv.FormatInt(from.UpdatedAt.UnixNano(), 10),
		strconv.FormatInt(to.ID, 10),
		strconv.FormatInt(to.UpdatedAt.UnixNano(), 10),
		strconv.Itoa(minCluster),
		strconv.Itoa(maxSpread),
	}, ":")
}

// parseReportBound reads one positive integer query parameter, or writes a
// 400 and returns false.
func parseReportBound(w http.ResponseWriter, r *http.Request, name string, def, limit int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 || v > limit {
		writeError(w, http.StatusBadRequest, "invalid_"+name,
			name+" must be between 1 and "+strconv.Itoa(limit), nil)
		return 0, false
	}
	return v, true
}

// handlePublicAnalysisReport handles
// GET /pub/api/v1/analysis/cohorts/{dataset_tag}/report?from=&to=.
// Returns the classified comparison of two snapshots: what the engine
// changed, what the cohort changed, and which movers moved together.
func (s *Server) handlePublicAnalysisReport(w http.ResponseWriter, r *http.Request) {
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
	fromSlug := strings.TrimSpace(r.URL.Query().Get("from"))
	toSlug := strings.TrimSpace(r.URL.Query().Get("to"))
	if fromSlug == "" || toSlug == "" {
		writeError(w, http.StatusBadRequest, "missing_snapshots",
			"report requires both ?from=<slug> and ?to=<slug>", nil)
		return
	}
	fromSnap, ok := readStore.GetAnalysisCohortSnapshotBySlug(cohort.ID, fromSlug)
	if !ok || !isPublicSnapshot(fromSnap) {
		writeError(w, http.StatusNotFound, "snapshot_not_found",
			"from snapshot is not available on the public path", nil)
		return
	}
	toSnap, ok := readStore.GetAnalysisCohortSnapshotBySlug(cohort.ID, toSlug)
	if !ok || !isPublicSnapshot(toSnap) {
		writeError(w, http.StatusNotFound, "snapshot_not_found",
			"to snapshot is not available on the public path", nil)
		return
	}
	minCluster, ok := parseReportBound(w, r, "min_cluster", defaultReportMinCluster, maxReportMinCluster)
	if !ok {
		return
	}
	maxSpread, ok := parseReportBound(w, r, "max_spread", defaultReportMaxSpread, maxReportMaxSpread)
	if !ok {
		return
	}

	cfg, err := s.effectiveScoringConfig()
	if err != nil {
		s.logger.Warn("report attribution falls back to the default scoring config", "err", err)
		cfg = scoring.DefaultConfig()
	}
	key := reportCacheKey(fromSnap, toSnap, minCluster, maxSpread)
	resp := s.reportCache.compute(key, func() PublicAnalysisReportResponse {
		return buildAnalysisReport(reportInput{
			DatasetTag:  cohort.SourceTag,
			From:        fromSnap,
			To:          toSnap,
			FromDomains: readStore.ListSnapshotDomainViews(fromSnap.ID),
			ToDomains:   readStore.ListSnapshotDomainViews(toSnap.ID),
			FromTags:    readStore.ListSnapshotTagViews(fromSnap.ID),
			ToTags:      readStore.ListSnapshotTagViews(toSnap.ID),
			Scoring:     cfg,
			MinCluster:  minCluster,
			MaxSpread:   maxSpread,
		})
	})
	writeSnapshotCacheHeaders(w, r, toSnap)
	writeJSON(w, http.StatusOK, resp)
}
