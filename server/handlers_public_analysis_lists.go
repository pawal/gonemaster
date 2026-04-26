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
	// Snapshot-side read surface used by the public snapshot selector and
	// the snapshot-scoped materialization cache.
	ListAnalysisCohortSnapshots(cohortID int64) []AnalysisCohortSnapshot
	GetAnalysisCohortSnapshotBySlug(cohortID int64, slug string) (AnalysisCohortSnapshot, bool)
	GetDefaultSnapshotForCohort(cohortID int64) (AnalysisCohortSnapshot, bool)
	ListSnapshotAggregates(snapshotID int64) []AnalysisCohortSnapshotAggregate
	ListSnapshotNameserverViews(snapshotID int64) []AnalysisSnapshotNameserverView
	ListSnapshotEndpointViews(snapshotID int64) []AnalysisSnapshotEndpointView
	ListSnapshotASNViews(snapshotID int64) []AnalysisSnapshotASNView
	GetSnapshotNameserverViewByName(snapshotID int64, name string) (AnalysisSnapshotNameserverView, bool)
	ListSnapshotEndpointViewsByAddress(snapshotID int64, address, nameserver string) []AnalysisSnapshotEndpointView
	ListSnapshotTagViews(snapshotID int64) []AnalysisSnapshotTagView
	GetSnapshotTagView(snapshotID int64, tag string) (AnalysisSnapshotTagView, bool)
}

// analysisListFilter captures the shared query parameters used by public list
// endpoints.
type analysisListFilter struct {
	Search string
	Limit  int
	Offset int
	Sort   string
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
	return filter, true
}

// resolvePublicAnalysisCohort resolves the public cohort from the dataset_tag
// query parameter or the admin-selected default public cohort.
func (s *Server) resolvePublicAnalysisCohort(w http.ResponseWriter, r *http.Request) (AnalysisCohort, bool) {
	datasetTag := strings.TrimSpace(r.URL.Query().Get("dataset_tag"))
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		writePublicAnalysisResolutionError(w, err)
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

// writeSnapshotCacheHeaders sets Cache-Control and ETag based on the
// resolution mode. Explicit ?snapshot= → immutable for a day; auto-latest
// or no-snapshot → must revalidate so a new default takes effect promptly.
func writeSnapshotCacheHeaders(w http.ResponseWriter, r *http.Request, snap AnalysisCohortSnapshot) {
	if snap.ID == 0 {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		return
	}
	explicit := strings.TrimSpace(r.URL.Query().Get("snapshot")) != ""
	if explicit && snap.Status == AnalysisSnapshotStatusCaptured {
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	}
	w.Header().Set("ETag", snapshotETag(snap))
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
	Slug        string    `json:"slug"`
	Label       string    `json:"label,omitempty"`
	CapturedAt  time.Time `json:"captured_at,omitempty"`
	FirstRunAt  time.Time `json:"first_run_at,omitempty"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	RunCount    int       `json:"run_count"`
	DomainCount int       `json:"domain_count"`
	ProfileName string    `json:"profile_name,omitempty"`
}

func publicAnalysisSnapshotView(snap AnalysisCohortSnapshot) PublicAnalysisSnapshotView {
	return PublicAnalysisSnapshotView{
		Slug:        snap.Slug,
		Label:       snap.Label,
		CapturedAt:  snap.CapturedAt,
		FirstRunAt:  snap.FirstRunAt,
		LastRunAt:   snap.LastRunAt,
		RunCount:    snap.RunCount,
		DomainCount: snap.DomainCount,
		ProfileName: snap.ProfileName,
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

// validWorstLevelBuckets pins the set of accepted worst_level filter values
// to the same buckets rendered by the health bar on the overview. Kept in
// one place so the filter, the docs, and the UI stay in sync.
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
	cohort, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
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

	// Exact-bucket filter so overview health-bar segments can deep-link to
	// the matching subset of domains. Buckets mirror severityBucket's output
	// (empty / INFO / unknown collapse into OK), which is how the bar itself
	// counts them.
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

	// Grade filter — exact string match on the stored grade. Not
	// enum-validated because scoring is configurable and custom profiles
	// may emit labels outside the default A+/A/B/C/D/F set. Unknown
	// grades simply return zero results.
	gradeFilter := strings.TrimSpace(r.URL.Query().Get("grade"))

	// Go through the cohort materialization cache so /domains
	// inherits the same bulk preload the landing page gets: one IN
	// query for all domain names instead of a per-row GetDomain.
	data := s.latestMaterializationForSnapshot(cohort, snapshot)
	latest := data.latest

	// Pre-compute the ASNs each domain's authoritative addresses resolve
	// to, so we can show an Operator column without paying the ASN lookup
	// cost per row.
	domainASNs := map[int64]map[int64]struct{}{}
	for _, fact := range data.addressASNs {
		if fact.ASN == nil {
			continue
		}
		set, ok := domainASNs[fact.DomainID]
		if !ok {
			set = map[int64]struct{}{}
			domainASNs[fact.DomainID] = set
		}
		set[*fact.ASN] = struct{}{}
	}

	items := make([]PublicAnalysisDomainView, 0, len(latest))
	for _, pair := range latest {
		sum := pair.summary
		name, found := data.domainNames[sum.DomainID]
		if !found {
			continue
		}
		if worstLevelFilter != "" && severityBucket(sum.WorstLevel) != worstLevelFilter {
			continue
		}
		if gradeFilter != "" {
			if sum.Grade == nil || *sum.Grade != gradeFilter {
				continue
			}
		}
		v := PublicAnalysisDomainView{
			Domain:          name,
			Score:           sum.Score,
			Grade:           sum.Grade,
			WorstLevel:      sum.WorstLevel,
			NameserverCount: sum.NameserverCount,
			EndpointCount:   sum.EndpointCount,
			ASNCount:        sum.ASNCount,
			PrefixCount:     sum.PrefixCount,
		}
		if asns := domainASNs[sum.DomainID]; len(asns) == 1 {
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
		if !pair.finishedAt.IsZero() {
			fa := pair.finishedAt
			v.FinishedAt = &fa
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

type domainSummaryPair struct {
	summary    AnalysisRunDomainSummary
	finishedAt time.Time
}

type latestCohortMaterialization struct {
	latest       []domainSummaryPair
	latestRuns   map[string]struct{}
	endpoints    []AnalysisRunNameserverEndpoint
	addressASNs  []AnalysisRunAddressASN
	domainASNs   []AnalysisRunDomainASN
	tagSummaries []AnalysisRunTagSummary
	domainFacts  []AnalysisRunDomainFact
	// domainNames is the id -> name map for every domain in `latest`,
	// preloaded once during cache compute so list handlers don't have
	// to issue one GetDomain DB round-trip per row.
	domainNames map[int64]string
}

// cohortMaterializationLookup is the subset of the job store that
// computeSnapshotMaterialization needs beyond the analysis read surface:
// run metadata (finished_at / batch_id) and bulk domain-name lookup, both
// kept narrow so test fakes don't have to implement the whole store.
type cohortMaterializationLookup interface {
	GetRun(id string) (Run, bool)
	GetDomainNamesByIDs(ids []int64) map[int64]string
	ListRuns(filter RunFilter) RunList
}

// computeSnapshotMaterialization is the snapshot-scoped counterpart of
// the old cohort-wide compute: instead of collapsing every run into
// "latest per domain", it starts from the exact set of runs attached to
// the snapshot's batch. The resulting set has at most one run per
// (cohort, domain) pair because the batch catalog enforces one run per
// domain per batch.
func computeSnapshotMaterialization(readStore AnalysisReadStore, runLookup cohortMaterializationLookup, cohortID int64, batchID string) latestCohortMaterialization {
	runSet := runIDsForBatch(runLookup, batchID)
	latest := collapseSummariesForRuns(readStore.ListAnalysisRunDomainSummariesByCohort(cohortID), runSet, runLookup)
	domainIDs := make([]int64, 0, len(latest))
	for _, pair := range latest {
		domainIDs = append(domainIDs, pair.summary.DomainID)
	}
	domainNames := runLookup.GetDomainNamesByIDs(domainIDs)
	// Drop parent-role endpoints (e.g. root servers recorded while traversing
	// the delegation chain for a TLD). They are not the cohort zones' own
	// authoritative servers and only pollute the nameserver/endpoint/ASN
	// views. Role tagging happens at projection time in projector.go.
	endpoints := filterAnalysisRunNSEndpointsByRunIDs(readStore.ListAnalysisRunNSEndpointsByCohort(cohortID), runSet)
	authoritative := make([]AnalysisRunNameserverEndpoint, 0, len(endpoints))
	authoritativeAddrIDs := map[int64]struct{}{}
	for _, ep := range endpoints {
		if ep.Role == "parent" {
			continue
		}
		authoritative = append(authoritative, ep)
		if ep.AddressID != 0 {
			authoritativeAddrIDs[ep.AddressID] = struct{}{}
		}
	}
	addressASNs := filterAnalysisRunAddressASNsByRunIDs(readStore.ListAnalysisRunAddressASNsByCohort(cohortID), runSet)
	addressASNs = filterAnalysisRunAddressASNsToAuthoritative(addressASNs, authoritativeAddrIDs)
	return latestCohortMaterialization{
		latest:       latest,
		latestRuns:   runSet,
		endpoints:    authoritative,
		addressASNs:  addressASNs,
		domainASNs:   filterAnalysisRunDomainASNsByRunIDs(readStore.ListAnalysisRunDomainASNsByCohort(cohortID), runSet),
		tagSummaries: filterAnalysisRunTagSummariesByRunIDs(readStore.ListAnalysisRunTagSummariesByCohort(cohortID), runSet),
		domainFacts:  filterAnalysisRunDomainFactsByRunIDs(readStore.ListAnalysisRunDomainFactsByCohort(cohortID), runSet),
		domainNames:  domainNames,
	}
}

// runIDsForBatch returns the set of graduated run ids that belong to the
// given batch, loaded in pages so a large batch doesn't blow the 100-row
// default limit.
func runIDsForBatch(runLookup cohortMaterializationLookup, batchID string) map[string]struct{} {
	out := map[string]struct{}{}
	if batchID == "" {
		return out
	}
	offset := 0
	for {
		list := runLookup.ListRuns(RunFilter{BatchID: batchID, Limit: 500, Offset: offset})
		for _, run := range list.Items {
			out[run.ID] = struct{}{}
		}
		if len(list.Items) == 0 || offset+len(list.Items) >= list.Total {
			break
		}
		offset += len(list.Items)
	}
	return out
}

// collapseSummariesForRuns filters cohort summaries down to the
// snapshot's batch run set, then collapses any accidental
// multiple-summary-per-domain rows to the freshest one. A batch has at
// most one run per domain by construction, so this is typically
// identity-on-filter; the collapse guards against historical data with
// reprojected runs.
func collapseSummariesForRuns(summaries []AnalysisRunDomainSummary, runSet map[string]struct{}, runLookup interface {
	GetRun(id string) (Run, bool)
}) []domainSummaryPair {
	byDomain := map[int64]domainSummaryPair{}
	for _, sum := range summaries {
		if _, ok := runSet[sum.RunID]; !ok {
			continue
		}
		var finishedAt time.Time
		if run, ok := runLookup.GetRun(sum.RunID); ok {
			finishedAt = run.FinishedAt
		}
		existing, seen := byDomain[sum.DomainID]
		if !seen || finishedAt.After(existing.finishedAt) {
			byDomain[sum.DomainID] = domainSummaryPair{summary: sum, finishedAt: finishedAt}
		}
	}
	out := make([]domainSummaryPair, 0, len(byDomain))
	for _, pair := range byDomain {
		out = append(out, pair)
	}
	return out
}

// filterAnalysisRunAddressASNsToAuthoritative drops any fact whose address
// has no authoritative endpoint in the cohort. authoritativeAddrIDs is the
// AddressID set assembled from the post-role-filter endpoint slice.
func filterAnalysisRunAddressASNsToAuthoritative(items []AnalysisRunAddressASN, authoritativeAddrIDs map[int64]struct{}) []AnalysisRunAddressASN {
	if len(items) == 0 {
		return items
	}
	if len(authoritativeAddrIDs) == 0 {
		return nil
	}
	out := make([]AnalysisRunAddressASN, 0, len(items))
	for _, fact := range items {
		if _, ok := authoritativeAddrIDs[fact.AddressID]; !ok {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterAnalysisRunDomainFactsByRunIDs(items []AnalysisRunDomainFact, runIDs map[string]struct{}) []AnalysisRunDomainFact {
	if len(runIDs) == 0 {
		return nil
	}
	out := make([]AnalysisRunDomainFact, 0, len(items))
	for _, item := range items {
		if _, ok := runIDs[item.RunID]; !ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterAnalysisRunTagSummariesByRunIDs(items []AnalysisRunTagSummary, runIDs map[string]struct{}) []AnalysisRunTagSummary {
	if len(runIDs) == 0 {
		return nil
	}
	out := make([]AnalysisRunTagSummary, 0, len(items))
	for _, item := range items {
		if _, ok := runIDs[item.RunID]; !ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterAnalysisRunDomainASNsByRunIDs(items []AnalysisRunDomainASN, runIDs map[string]struct{}) []AnalysisRunDomainASN {
	if len(runIDs) == 0 {
		return nil
	}
	out := make([]AnalysisRunDomainASN, 0, len(items))
	for _, item := range items {
		if _, ok := runIDs[item.RunID]; !ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterAnalysisRunNSEndpointsByRunIDs(items []AnalysisRunNameserverEndpoint, runIDs map[string]struct{}) []AnalysisRunNameserverEndpoint {
	if len(runIDs) == 0 {
		return nil
	}
	out := make([]AnalysisRunNameserverEndpoint, 0, len(items))
	for _, item := range items {
		if _, ok := runIDs[item.RunID]; !ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterAnalysisRunAddressASNsByRunIDs(items []AnalysisRunAddressASN, runIDs map[string]struct{}) []AnalysisRunAddressASN {
	if len(runIDs) == 0 {
		return nil
	}
	out := make([]AnalysisRunAddressASN, 0, len(items))
	for _, item := range items {
		if _, ok := runIDs[item.RunID]; !ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

const analysisEntryPageSize = 1000

func (s *Server) loadAllEntriesForRun(runID string) []Entry {
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	offset := 0
	entries := make([]Entry, 0, analysisEntryPageSize)
	for {
		page := s.store.QueryEntries(EntryFilter{
			RunID:  runID,
			Limit:  analysisEntryPageSize,
			Offset: offset,
		})
		if len(page.Items) == 0 {
			break
		}
		entries = append(entries, page.Items...)
		offset += len(page.Items)
		if offset >= page.Total {
			break
		}
	}
	return entries
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
	end := offset + limit
	if end > total {
		end = total
	}
	return offset, end
}
