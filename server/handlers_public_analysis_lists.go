package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AnalysisReadStore is the read-only analysis store surface used by the public
// list/detail endpoints. It is implemented by *SQLJobStore.
type AnalysisReadStore interface {
	ListAnalysisRunDomainSummariesByCohort(cohortID int64) []AnalysisRunDomainSummary
	ListAnalysisRunNSEndpointsByCohort(cohortID int64) []AnalysisRunNameserverEndpoint
	ListAnalysisRunAddressASNsByCohort(cohortID int64) []AnalysisRunAddressASN
	ListAnalysisRunDomainASNsByCohort(cohortID int64) []AnalysisRunDomainASN
	ListAnalysisRunTagSummariesByCohort(cohortID int64) []AnalysisRunTagSummary
	ListAnalysisRunDomainFactsByCohort(cohortID int64) []AnalysisRunDomainFact
	GetAnalysisNameserver(id int64) (AnalysisNameserver, bool)
	GetAnalysisAddress(id int64) (AnalysisAddress, bool)
	GetAnalysisPrefix(id int64) (AnalysisPrefix, bool)
	GetAnalysisASN(asn int64) (AnalysisASN, bool)
	ListAnalysisCohortSnapshots(cohortID int64) []AnalysisCohortSnapshot
	GetAnalysisCohortSnapshotBySlug(cohortID int64, slug string) (AnalysisCohortSnapshot, bool)
	GetDefaultSnapshotForCohort(cohortID int64) (AnalysisCohortSnapshot, bool)
	GetSnapshotOverview(snapshotID int64) (SnapshotOverviewV2, bool)
	ListSnapshotOverviewsByIDs(snapshotIDs []int64) map[int64]SnapshotOverviewV2
	ListSnapshotNameserverViews(snapshotID int64) []AnalysisSnapshotNameserverView
	ListSnapshotEndpointViews(snapshotID int64) []AnalysisSnapshotEndpointView
	ListSnapshotASNViews(snapshotID int64) []AnalysisSnapshotASNView
	GetSnapshotNameserverViewByName(snapshotID int64, name string) (AnalysisSnapshotNameserverView, bool)
	ListSnapshotEndpointViewsByAddress(snapshotID int64, address, nameserver string) []AnalysisSnapshotEndpointView
	ListSnapshotTagViews(snapshotID int64) []AnalysisSnapshotTagView
	GetSnapshotTagView(snapshotID int64, tag string) (AnalysisSnapshotTagView, bool)
	ListSnapshotDomainViews(snapshotID int64) []AnalysisSnapshotDomainView
	GetSnapshotDomainViewByName(snapshotID int64, name string) (AnalysisSnapshotDomainView, bool)
	GetSnapshotASNView(snapshotID, asn int64) (AnalysisSnapshotASNView, bool)
	ListSnapshotPrefixViews(snapshotID int64) []AnalysisSnapshotPrefixView
	GetSnapshotPrefixView(snapshotID int64, prefix string) (AnalysisSnapshotPrefixView, bool)
	AnalysisEntityHistory(cohortID int64, entity, key string) ([]AnalysisEntityHistoryPoint, error)
}

// analysisListFilter captures the shared query parameters used by public list
// endpoints.
type analysisListFilter struct {
	Search            string
	Limit             int
	Offset            int
	Sort              string
	MinLatencySamples int
}

// defaultAnalysisListLimit and maxAnalysisListLimit bound the page size.
const (
	defaultAnalysisListLimit = 100
	maxAnalysisListLimit     = 500
)

// parseAnalysisListFilter returns a validated list filter or writes an error
// response. The caller must check ok before proceeding.
func parseAnalysisListFilter(w http.ResponseWriter, r *http.Request) (analysisListFilter, bool) {
	q := r.URL.Query()
	filter := analysisListFilter{
		Search: strings.TrimSpace(q.Get("search")),
		Limit:  defaultAnalysisListLimit,
		Sort:   strings.TrimSpace(q.Get("sort")),
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 || v > maxAnalysisListLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500", nil)
			return filter, false
		}
		filter.Limit = v
	}
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative integer", nil)
			return filter, false
		}
		filter.Offset = v
	}
	if raw := strings.TrimSpace(q.Get("min_latency_samples")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "invalid_min_latency_samples", "min_latency_samples must be a non-negative integer", nil)
			return filter, false
		}
		filter.MinLatencySamples = v
	}
	return filter, true
}

// resolvePublicAnalysisCohort resolves the public cohort from the dataset_tag
// query parameter or the admin-selected default public cohort.
func (s *Server) resolvePublicAnalysisCohort(w http.ResponseWriter, r *http.Request) (AnalysisCohort, bool) {
	datasetTag := strings.TrimSpace(r.URL.Query().Get("dataset_tag"))
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		s.writePublicAnalysisResolutionError(w, r, err)
		return AnalysisCohort{}, false
	}
	return cohort, true
}

// PublicAnalysisStatusNoSnapshot is the status token handlers emit when a
// cohort has no captured public snapshot yet. The UI keys on this to
// render a helpful empty state instead of treating the empty payload as
// a transient failure.
const PublicAnalysisStatusNoSnapshot = "no_snapshot"

// resolvePublicAnalysisCohortAndSnapshot resolves (cohort, snapshot) for a
// public request. The resolution rules:
//
//  1. dataset_tag selects the cohort (or the admin default when omitted).
//  2. An explicit ?snapshot=<slug> pins that snapshot. Retired, non-public,
//     or failed_mixed_profiles snapshots are hidden: looking them up by slug
//     returns 404 so an operator sharing a link to a broken snapshot does
//     not leak it through the public path.
//  3. Without an explicit slug, the cohort's auto-latest default snapshot
//     is used; it is always the most recent captured public snapshot.
//  4. When no captured public snapshot exists for the cohort, snapshot.ID
//     is zero and the caller should render the no_snapshot state.
func (s *Server) resolvePublicAnalysisCohortAndSnapshot(w http.ResponseWriter, r *http.Request) (AnalysisCohort, AnalysisCohortSnapshot, bool) {
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
	if !ok {
		return AnalysisCohort{}, AnalysisCohortSnapshot{}, false
	}
	readStore, ok := s.store.(AnalysisReadStore)
	if !ok {
		// Non-SQL stores do not carry the snapshot tables; fall back to
		// the empty-snapshot state so the page still renders.
		return cohort, AnalysisCohortSnapshot{}, true
	}
	slug := strings.TrimSpace(r.URL.Query().Get("snapshot"))
	if slug != "" {
		snap, found := readStore.GetAnalysisCohortSnapshotBySlug(cohort.ID, slug)
		if !found || !isPublicSnapshot(snap) {
			writeError(w, http.StatusNotFound, "snapshot_not_found",
				"requested snapshot is not available on the public path", nil)
			return AnalysisCohort{}, AnalysisCohortSnapshot{}, false
		}
		return cohort, snap, true
	}
	snap, found := readStore.GetDefaultSnapshotForCohort(cohort.ID)
	if !found {
		return cohort, AnalysisCohortSnapshot{}, true
	}
	return cohort, snap, true
}

// writeSnapshotCacheHeaders sets Cache-Control and ETag. Snapshots can be
// rebuilt in place (same slug, new content), so responses are never marked
// immutable; clients revalidate via the ETag, which changes on rebuild -
// a cheap 304 when unchanged, fresh 200 after a rebuild.
func writeSnapshotCacheHeaders(w http.ResponseWriter, r *http.Request, snap AnalysisCohortSnapshot) {
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	if snap.ID != 0 {
		w.Header().Set("ETag", snapshotETag(snap))
	}
}

func snapshotETag(snap AnalysisCohortSnapshot) string {
	return `"` + strconv.FormatInt(snap.ID, 10) + "-" + strconv.FormatInt(snap.CapturedAt.Unix(), 10) + `"`
}

// isPublicSnapshot returns true when a snapshot is eligible for the public
// read path: captured (aggregates written), public (admin-visible flag),
// and not in the failed_mixed_profiles state. Retired snapshots are
// hidden by the status gate.
func isPublicSnapshot(snap AnalysisCohortSnapshot) bool {
	if snap.ID == 0 {
		return false
	}
	if !snap.IsPublic {
		return false
	}
	return snap.Status == AnalysisSnapshotStatusCaptured
}

// resolvePublicSnapshotOrNone is a cohort-scoped variant of
// resolvePublicAnalysisCohortAndSnapshot used by handlers that have
// already resolved the cohort themselves (e.g. the cohort detail
// endpoint with dataset_tag in the URL path). Writes a 404 when the
// slug is invalid; returns a zero snapshot when the cohort has no
// captured public snapshot so callers can render the empty state.
func (s *Server) resolvePublicSnapshotOrNone(w http.ResponseWriter, r *http.Request, cohort AnalysisCohort) AnalysisCohortSnapshot {
	readStore, ok := s.store.(AnalysisReadStore)
	if !ok {
		return AnalysisCohortSnapshot{}
	}
	slug := strings.TrimSpace(r.URL.Query().Get("snapshot"))
	if slug != "" {
		snap, found := readStore.GetAnalysisCohortSnapshotBySlug(cohort.ID, slug)
		if !found || !isPublicSnapshot(snap) {
			writeError(w, http.StatusNotFound, "snapshot_not_found",
				"requested snapshot is not available on the public path", nil)
			return AnalysisCohortSnapshot{}
		}
		return snap
	}
	snap, found := readStore.GetDefaultSnapshotForCohort(cohort.ID)
	if !found {
		return AnalysisCohortSnapshot{}
	}
	return snap
}

// PublicAnalysisSnapshotView is the redacted shape handlers embed to
// tell the reader which snapshot is being served. Keyed on slug, which
// is the stable URL-safe identifier.
type PublicAnalysisSnapshotView struct {
	Slug            string    `json:"slug"`
	Label           string    `json:"label,omitempty"`
	CapturedAt      time.Time `json:"captured_at"`
	FirstRunAt      time.Time `json:"first_run_at"`
	LastRunAt       time.Time `json:"last_run_at"`
	RunCount        int       `json:"run_count"`
	DomainCount     int       `json:"domain_count"`
	ProfileName     string    `json:"profile_name,omitempty"`
	TagViewMinLevel string    `json:"tag_view_min_level,omitempty"`
}

func publicAnalysisSnapshotView(snap AnalysisCohortSnapshot) PublicAnalysisSnapshotView {
	return PublicAnalysisSnapshotView{
		Slug:            snap.Slug,
		Label:           snap.Label,
		CapturedAt:      snap.CapturedAt,
		FirstRunAt:      snap.FirstRunAt,
		LastRunAt:       snap.LastRunAt,
		RunCount:        snap.RunCount,
		DomainCount:     snap.DomainCount,
		ProfileName:     snap.ProfileName,
		TagViewMinLevel: snap.TagViewMinLevel,
	}
}

func analysisSnapshotSourceTime(snap AnalysisCohortSnapshot) time.Time {
	if !snap.LastRunAt.IsZero() {
		return snap.LastRunAt
	}
	if !snap.FirstRunAt.IsZero() {
		return snap.FirstRunAt
	}
	return snap.CapturedAt
}

// analysisReadStore returns the analysis read-store when the configured store
// supports it; otherwise it writes a 503 and returns false.
func (s *Server) analysisReadStore(w http.ResponseWriter) (AnalysisReadStore, bool) {
	readStore, ok := s.store.(AnalysisReadStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "analysis_unavailable", "analysis store not configured", nil)
		return nil, false
	}
	return readStore, true
}

// analysisBackendSupported reports whether the configured store implements the
// analysis fact-query surface. The in-memory store does not, so analysis
// materialization has nowhere to persist in that configuration.
func (s *Server) analysisBackendSupported() bool {
	_, ok := s.store.(AnalysisReadStore)
	return ok
}

// PublicAnalysisDomainView is the redacted public shape for a domain row in
// the cohort's member list.
type PublicAnalysisDomainView struct {
	Domain          string  `json:"domain"`
	Score           *int    `json:"score,omitempty"`
	Grade           *string `json:"grade,omitempty"`
	WorstLevel      string  `json:"worst_level,omitempty"`
	NameserverCount int     `json:"nameserver_count"`
	EndpointCount   int     `json:"endpoint_count"`
	ASNCount        int     `json:"asn_count"`
	PrefixCount     int     `json:"prefix_count"`
	// Operator is the ASN label when every authoritative address for the
	// domain shares a single ASN. Empty when unknown; "Multiple" when
	// spread across multiple ASNs. Gives operators at-a-glance without
	// clicking into the detail view.
	Operator    string     `json:"operator,omitempty"`
	OperatorASN *int64     `json:"operator_asn,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// PublicAnalysisListResponse is the shared envelope for paginated public list
// endpoints.
type PublicAnalysisListResponse[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// domainViewASNSet returns the distinct ASN set across the row's
// per-NS and flat address rosters.
func domainViewASNSet(row AnalysisSnapshotDomainView) map[int64]struct{} {
	out := map[int64]struct{}{}
	for _, ns := range row.Nameservers {
		for _, a := range ns.Addresses {
			if a.ASN != nil {
				out[*a.ASN] = struct{}{}
			}
		}
	}
	for _, a := range row.Addresses {
		if a.ASN != nil {
			out[*a.ASN] = struct{}{}
		}
	}
	return out
}

var validWorstLevelBuckets = map[string]struct{}{
	"OK":       {},
	"NOTICE":   {},
	"WARNING":  {},
	"ERROR":    {},
	"CRITICAL": {},
}

// handlePublicAnalysisDomains handles GET /pub/api/v1/analysis/domains. It
// returns the latest run per domain in the resolved cohort.
func (s *Server) handlePublicAnalysisDomains(w http.ResponseWriter, r *http.Request) {
	_, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}
	filter, ok := parseAnalysisListFilter(w, r)
	if !ok {
		return
	}

	worstLevelFilter := ""
	if raw := strings.TrimSpace(r.URL.Query().Get("worst_level")); raw != "" {
		normalized := strings.ToUpper(raw)
		if _, ok := validWorstLevelBuckets[normalized]; !ok {
			writeError(w, http.StatusBadRequest, "invalid_worst_level",
				"worst_level must be one of OK, NOTICE, WARNING, ERROR, CRITICAL", nil)
			return
		}
		worstLevelFilter = normalized
	}
	gradeFilter := strings.TrimSpace(r.URL.Query().Get("grade"))

	rows := readStore.ListSnapshotDomainViews(snapshot.ID)
	items := make([]PublicAnalysisDomainView, 0, len(rows))
	for _, row := range rows {
		if worstLevelFilter != "" && severityBucket(row.WorstLevel) != worstLevelFilter {
			continue
		}
		if gradeFilter != "" && row.Grade != gradeFilter {
			continue
		}
		v := PublicAnalysisDomainView{
			Domain:          row.DomainName,
			Score:           row.Score,
			WorstLevel:      row.WorstLevel,
			NameserverCount: row.NameserverCount,
			EndpointCount:   row.EndpointCount,
			ASNCount:        row.ASNCount,
			PrefixCount:     row.PrefixCount,
		}
		if row.Grade != "" {
			g := row.Grade
			v.Grade = &g
		}
		if row.FinishedAt != nil && !row.FinishedAt.IsZero() {
			fa := *row.FinishedAt
			v.FinishedAt = &fa
		}
		asns := domainViewASNSet(row)
		if len(asns) == 1 {
			for asn := range asns {
				asnCopy := asn
				v.OperatorASN = &asnCopy
				if meta, ok := readStore.GetAnalysisASN(asn); ok {
					v.Operator = meta.Label
				}
			}
		} else if len(asns) > 1 {
			v.Operator = "Multiple"
		}
		items = append(items, v)
	}

	if filter.Search != "" {
		needle := strings.ToLower(filter.Search)
		kept := items[:0]
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Domain), needle) {
				kept = append(kept, it)
			}
		}
		items = kept
	}

	sortAnalysisDomainViews(items, filter.Sort)

	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisDomainView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

// sortAnalysisDomainViews sorts by the requested order. "domain_asc" is the
// default when no sort is supplied.
func sortAnalysisDomainViews(items []PublicAnalysisDomainView, mode string) {
	switch mode {
	case "", "domain_asc":
		sort.Slice(items, func(i, j int) bool { return items[i].Domain < items[j].Domain })
	case "domain_desc":
		sort.Slice(items, func(i, j int) bool { return items[i].Domain > items[j].Domain })
	case "score_asc":
		sort.Slice(items, func(i, j int) bool {
			si := intFromPtr(items[i].Score, 1000)
			sj := intFromPtr(items[j].Score, 1000)
			if si != sj {
				return si < sj
			}
			return items[i].Domain < items[j].Domain
		})
	case "score_desc":
		sort.Slice(items, func(i, j int) bool {
			si := intFromPtr(items[i].Score, -1)
			sj := intFromPtr(items[j].Score, -1)
			if si != sj {
				return si > sj
			}
			return items[i].Domain < items[j].Domain
		})
	case "worst_level_desc":
		sort.Slice(items, func(i, j int) bool {
			ri, rj := severityRank(items[i].WorstLevel), severityRank(items[j].WorstLevel)
			if ri != rj {
				return ri > rj
			}
			return items[i].Domain < items[j].Domain
		})
	case "nameserver_count_asc":
		sortByDomainCount(items, func(v PublicAnalysisDomainView) int { return v.NameserverCount }, true)
	case "nameserver_count_desc":
		sortByDomainCount(items, func(v PublicAnalysisDomainView) int { return v.NameserverCount }, false)
	case "endpoint_count_asc":
		sortByDomainCount(items, func(v PublicAnalysisDomainView) int { return v.EndpointCount }, true)
	case "endpoint_count_desc":
		sortByDomainCount(items, func(v PublicAnalysisDomainView) int { return v.EndpointCount }, false)
	case "asn_count_asc":
		sortByDomainCount(items, func(v PublicAnalysisDomainView) int { return v.ASNCount }, true)
	case "asn_count_desc":
		sortByDomainCount(items, func(v PublicAnalysisDomainView) int { return v.ASNCount }, false)
	case "prefix_count_asc":
		sortByDomainCount(items, func(v PublicAnalysisDomainView) int { return v.PrefixCount }, true)
	case "prefix_count_desc":
		sortByDomainCount(items, func(v PublicAnalysisDomainView) int { return v.PrefixCount }, false)
	default:
		sort.Slice(items, func(i, j int) bool { return items[i].Domain < items[j].Domain })
	}
}

// sortByDomainCount sorts items by an integer extractor with the domain
// name as a stable tiebreaker. Shared by the count-column sort modes.
func sortByDomainCount(items []PublicAnalysisDomainView, get func(PublicAnalysisDomainView) int, asc bool) {
	sort.Slice(items, func(i, j int) bool {
		a, b := get(items[i]), get(items[j])
		if a != b {
			if asc {
				return a < b
			}
			return a > b
		}
		return items[i].Domain < items[j].Domain
	})
}

func intFromPtr(p *int, fallback int) int {
	if p == nil {
		return fallback
	}
	return *p
}

// clampPage returns the [start, end) slice bounds after applying the offset
// and limit against total.
func clampPage(limit, offset, total int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	return offset, end
}
