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
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

// PublicAnalysisListResponse is the shared envelope for paginated public list
// endpoints.
type PublicAnalysisListResponse[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
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

	summaries := readStore.ListAnalysisRunDomainSummariesByCohort(cohort.ID)
	latest := latestSummariesByDomain(summaries, s.store)

	items := make([]PublicAnalysisDomainView, 0, len(latest))
	for _, pair := range latest {
		sum := pair.summary
		domain, found := s.store.GetDomain(sum.DomainID)
		if !found {
			continue
		}
		v := PublicAnalysisDomainView{
			Domain:          domain.Name,
			Score:           sum.Score,
			Grade:           sum.Grade,
			WorstLevel:      sum.WorstLevel,
			NameserverCount: sum.NameserverCount,
			EndpointCount:   sum.EndpointCount,
			ASNCount:        sum.ASNCount,
			PrefixCount:     sum.PrefixCount,
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
	latest      []domainSummaryPair
	latestRuns  map[string]struct{}
	endpoints   []AnalysisRunNameserverEndpoint
	addressASNs []AnalysisRunAddressASN
	domainASNs  []AnalysisRunDomainASN
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

func latestMaterializationForCohort(readStore AnalysisReadStore, runLookup interface {
	GetRun(id string) (Run, bool)
}, cohortID int64) latestCohortMaterialization {
	latest := latestSummariesByDomain(readStore.ListAnalysisRunDomainSummariesByCohort(cohortID), runLookup)
	runIDs := make(map[string]struct{}, len(latest))
	for _, pair := range latest {
		runIDs[pair.summary.RunID] = struct{}{}
	}
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
		latest:      latest,
		latestRuns:  runIDs,
		endpoints:   authoritative,
		addressASNs: filterAnalysisRunAddressASNsByRunIDs(readStore.ListAnalysisRunAddressASNsByCohort(cohortID), runIDs),
		domainASNs:  filterAnalysisRunDomainASNsByRunIDs(readStore.ListAnalysisRunDomainASNsByCohort(cohortID), runIDs),
	}
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
