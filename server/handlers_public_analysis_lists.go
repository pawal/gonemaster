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
	Domain          string     `json:"domain"`
	Score           *int       `json:"score,omitempty"`
	Grade           *string    `json:"grade,omitempty"`
	WorstLevel      string     `json:"worst_level,omitempty"`
	NameserverCount int        `json:"nameserver_count"`
	EndpointCount   int        `json:"endpoint_count"`
	ASNCount        int        `json:"asn_count"`
	PrefixCount     int        `json:"prefix_count"`
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
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
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
	data := s.latestMaterializationForCohort(cohort)
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

// latestSummariesByDomain collapses multiple materialized summaries per domain
// into the latest one, using the run's finished_at timestamp as the tiebreaker.
func latestSummariesByDomain(summaries []AnalysisRunDomainSummary, runLookup interface {
	GetRun(id string) (Run, bool)
}) []domainSummaryPair {
	byDomain := map[int64]domainSummaryPair{}
	for _, sum := range summaries {
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

// cohortMaterializationLookup is the subset of the job store that
// computeLatestMaterializationForCohort needs beyond the analysis read
// surface: run metadata (finished_at) and bulk domain-name lookup, both
// kept narrow so test fakes don't have to implement the whole store.
type cohortMaterializationLookup interface {
	GetRun(id string) (Run, bool)
	GetDomainNamesByIDs(ids []int64) map[int64]string
}

// computeLatestMaterializationForCohort runs the uncached four-table scan for a
// cohort and collapses multiple runs per domain into the latest. Callers should
// prefer (*Server).latestMaterializationForCohort, which wraps this with a
// stamp-keyed cache.
func computeLatestMaterializationForCohort(readStore AnalysisReadStore, runLookup cohortMaterializationLookup, cohortID int64) latestCohortMaterialization {
	latest := latestSummariesByDomain(readStore.ListAnalysisRunDomainSummariesByCohort(cohortID), runLookup)
	runIDs := make(map[string]struct{}, len(latest))
	domainIDs := make([]int64, 0, len(latest))
	for _, pair := range latest {
		runIDs[pair.summary.RunID] = struct{}{}
		domainIDs = append(domainIDs, pair.summary.DomainID)
	}
	domainNames := runLookup.GetDomainNamesByIDs(domainIDs)
	// Drop parent-role endpoints (e.g. root servers recorded while traversing
	// the delegation chain for a TLD). They are not the cohort zones' own
	// authoritative servers and only pollute the nameserver/endpoint/ASN
	// views. Role tagging happens at projection time in projector.go.
	endpoints := filterAnalysisRunNSEndpointsByRunIDs(readStore.ListAnalysisRunNSEndpointsByCohort(cohortID), runIDs)
	authoritative := make([]AnalysisRunNameserverEndpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.Role == "parent" {
			continue
		}
		authoritative = append(authoritative, ep)
	}
	return latestCohortMaterialization{
		latest:       latest,
		latestRuns:   runIDs,
		endpoints:    authoritative,
		addressASNs:  filterAnalysisRunAddressASNsByRunIDs(readStore.ListAnalysisRunAddressASNsByCohort(cohortID), runIDs),
		domainASNs:   filterAnalysisRunDomainASNsByRunIDs(readStore.ListAnalysisRunDomainASNsByCohort(cohortID), runIDs),
		tagSummaries: filterAnalysisRunTagSummariesByRunIDs(readStore.ListAnalysisRunTagSummariesByCohort(cohortID), runIDs),
		domainFacts:  filterAnalysisRunDomainFactsByRunIDs(readStore.ListAnalysisRunDomainFactsByCohort(cohortID), runIDs),
		domainNames:  domainNames,
	}
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
	default:
		sort.Slice(items, func(i, j int) bool { return items[i].Domain < items[j].Domain })
	}
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
