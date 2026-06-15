package server

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// PublicAnalysisCohortDetail describes one cohort plus its top-level counts.
type PublicAnalysisCohortDetail struct {
	DatasetTag            string                                    `json:"dataset_tag"`
	Label                 string                                    `json:"label"`
	Description           string                                    `json:"description,omitempty"`
	IsDefault             bool                                      `json:"is_default"`
	MaterializationStatus string                                    `json:"materialization_status"`
	LastMaterializedAt    *time.Time                                `json:"last_materialized_at,omitempty"`
	DomainCount           int                                       `json:"domain_count"`
	NameserverCount       int                                       `json:"nameserver_count"`
	EndpointCount         int                                       `json:"endpoint_count"`
	ASNCount              int                                       `json:"asn_count"`
	PrefixCount           int                                       `json:"prefix_count"`
	FactDistributions     map[string]PublicAnalysisFactDistribution `json:"fact_distributions,omitempty"`
	// Snapshot is the specific materialization being described when the
	// cohort has a captured snapshot; nil for cohorts awaiting their first
	// snapshot.
	Snapshot *PublicAnalysisSnapshotView `json:"snapshot,omitempty"`
	// Status is "no_snapshot" when the cohort has no captured public
	// snapshot and the UI should render an empty-state panel.
	Status string `json:"status,omitempty"`
}

// severityBucket normalizes a run worst_level into one of the five buckets
// used by the cohort health chart. Anything below NOTICE (including the
// empty string the projector stores for runs with no findings above INFO)
// collapses into OK.
func severityBucket(level string) string {
	switch strings.ToUpper(level) {
	case "CRITICAL":
		return "CRITICAL"
	case "ERROR":
		return "ERROR"
	case "WARNING":
		return "WARNING"
	case "NOTICE":
		return "NOTICE"
	default:
		return "OK"
	}
}

// PublicAnalysisDomainDetail is the per-domain detail view.
type PublicAnalysisDomainDetail struct {
	Domain          string                           `json:"domain"`
	Score           *int                             `json:"score,omitempty"`
	Grade           *string                          `json:"grade,omitempty"`
	WorstLevel      string                           `json:"worst_level,omitempty"`
	FinishedAt      *time.Time                       `json:"finished_at,omitempty"`
	NameserverCount int                              `json:"nameserver_count"`
	EndpointCount   int                              `json:"endpoint_count"`
	ASNCount        int                              `json:"asn_count"`
	PrefixCount     int                              `json:"prefix_count"`
	Nameservers     []PublicAnalysisDomainNameserver `json:"nameservers"`
	Addresses       []PublicAnalysisDomainAddress    `json:"addresses"`
	Tags            []PublicAnalysisDomainTag        `json:"tags,omitempty"`
	// Localized log entries from the run; empty when the run was purged.
	Entries []PublicAnalysisDomainEntry `json:"entries,omitempty"`
}

// PublicAnalysisDomainTag is one tag observed at the capture-time floor.
type PublicAnalysisDomainTag struct {
	Tag      string `json:"tag"`
	Module   string `json:"module,omitempty"`
	Testcase string `json:"testcase,omitempty"`
	Level    string `json:"level,omitempty"`
}

type PublicAnalysisDomainNameserver struct {
	Nameserver string                        `json:"nameserver"`
	IPv4Count  int                           `json:"ipv4_count"`
	IPv6Count  int                           `json:"ipv6_count"`
	Addresses  []PublicAnalysisDomainAddress `json:"addresses"`
	// "unresolved" when no real address is materialized for this NS.
	Status string `json:"status,omitempty"`
}

type PublicAnalysisDomainAddress struct {
	Address  string `json:"address"`
	Family   string `json:"family"`
	ASN      *int64 `json:"asn,omitempty"`
	ASNLabel string `json:"asn_label,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	// "unreachable" when the engine got no samples for this endpoint.
	Status string `json:"status,omitempty"`
}

// PublicAnalysisDomainEntry is one log entry emitted by a Zonemaster testcase
// run, carried through with a translated human-readable message so the UI can
// render a results view instead of just a bag of tags.
type PublicAnalysisDomainEntry struct {
	Timestamp float64 `json:"timestamp"`
	Module    string  `json:"module,omitempty"`
	Testcase  string  `json:"testcase,omitempty"`
	Tag       string  `json:"tag"`
	Level     string  `json:"level,omitempty"`
	Message   string  `json:"message,omitempty"`
	Raw       string  `json:"raw,omitempty"`
}

// PublicAnalysisNameserverDetail is the per-nameserver detail view.
type PublicAnalysisNameserverDetail struct {
	Nameserver    string   `json:"nameserver"`
	DomainCount   int      `json:"domain_count"`
	EndpointCount int      `json:"endpoint_count"`
	IPv4Count     int      `json:"ipv4_count"`
	IPv6Count     int      `json:"ipv6_count"`
	Addresses     []string `json:"addresses"`
	Domains       []string `json:"domains"`
	ASNs          []int64  `json:"asns"`
}

// PublicAnalysisEndpointDetail is the per-(nameserver,address) detail view.
type PublicAnalysisEndpointDetail struct {
	Nameserver  string   `json:"nameserver"`
	Address     string   `json:"address"`
	Family      string   `json:"family"`
	ASN         *int64   `json:"asn,omitempty"`
	ASNLabel    string   `json:"asn_label,omitempty"`
	Prefix      string   `json:"prefix,omitempty"`
	DomainCount int      `json:"domain_count"`
	Domains     []string `json:"domains"`
}

// PublicAnalysisASNDetail is the per-ASN detail view.
type PublicAnalysisASNDetail struct {
	ASN             int64    `json:"asn"`
	Label           string   `json:"label,omitempty"`
	DomainCount     int      `json:"domain_count"`
	AddressCount    int      `json:"address_count"`
	NameserverCount int      `json:"nameserver_count"`
	PrefixCount     int      `json:"prefix_count"`
	Domains         []string `json:"domains"`
	Nameservers     []string `json:"nameservers"`
	Prefixes        []string `json:"prefixes"`
}

// PublicAnalysisPrefixDetail is the per-prefix detail view.
type PublicAnalysisPrefixDetail struct {
	Prefix       string   `json:"prefix"`
	Family       string   `json:"family"`
	DomainCount  int      `json:"domain_count"`
	AddressCount int      `json:"address_count"`
	ASNs         []int64  `json:"asns"`
	Domains      []string `json:"domains"`
	Addresses    []string `json:"addresses"`
}

// PublicAnalysisTagDetail is the per-finding-tag detail view.
type PublicAnalysisTagDetail struct {
	Tag             string   `json:"tag"`
	Module          string   `json:"module,omitempty"`
	Testcase        string   `json:"testcase,omitempty"`
	Level           string   `json:"level,omitempty"`
	DomainCount     int      `json:"domain_count"`
	OccurrenceCount int      `json:"occurrence_count"`
	Domains         []string `json:"domains"`
}

// PublicAnalysisTestcaseDetail is the per-(module,testcase) detail view.
type PublicAnalysisTestcaseDetail struct {
	Module      string   `json:"module"`
	Testcase    string   `json:"testcase"`
	DomainCount int      `json:"domain_count"`
	EntryCount  int      `json:"entry_count"`
	WorstLevel  string   `json:"worst_level,omitempty"`
	Tags        []string `json:"tags"`
	Domains     []string `json:"domains"`
}

// ── cohort detail ──────────────────────────────────────────────────────────────

// handlePublicAnalysisCohortDetail handles GET /pub/api/v1/analysis/cohorts/{dataset_tag}.
func (s *Server) handlePublicAnalysisCohortDetail(w http.ResponseWriter, r *http.Request) {
	datasetTag, ok := pathValueNonEmpty(w, r, "dataset_tag", "cohort")
	if !ok {
		return
	}
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		writePublicAnalysisResolutionError(w, err)
		return
	}
	detail := PublicAnalysisCohortDetail{
		DatasetTag:            cohort.SourceTag,
		Label:                 cohort.Label,
		Description:           cohort.Description,
		IsDefault:             cohort.IsDefault,
		MaterializationStatus: cohort.MaterializationStatus,
	}
	if !cohort.LastMaterializedAt.IsZero() {
		t := cohort.LastMaterializedAt
		detail.LastMaterializedAt = &t
	}
	// Snapshot resolution: use the ?snapshot= slug when present, else the
	// cohort's auto-latest default. Detail counts are scoped to the
	// resolved snapshot so the page always describes a specific
	// materialization rather than averaging across batches.
	snapshot := s.resolvePublicSnapshotOrNone(w, r, cohort)
	if w.Header().Get("Content-Type") != "" {
		// resolvePublicSnapshotOrNone already wrote a 404 for a bad slug.
		return
	}
	if readStore, canRead := s.store.(AnalysisReadStore); canRead && snapshot.ID != 0 {
		snapshotView := publicAnalysisSnapshotView(snapshot)
		detail.Snapshot = &snapshotView
		populateCohortDetailFromViews(&detail, readStore, snapshot.ID)
	} else if snapshot.ID == 0 {
		detail.Status = PublicAnalysisStatusNoSnapshot
	}
	writeJSON(w, http.StatusOK, detail)
}

// populateCohortDetailFromViews fills totals + distributions from the
// overview_v2 aggregate and the per-snapshot view tables.
func populateCohortDetailFromViews(detail *PublicAnalysisCohortDetail, readStore AnalysisReadStore, snapshotID int64) {
	if overview, ok := loadSnapshotOverviewV2(readStore, snapshotID); ok {
		detail.DomainCount = overview.Totals.DomainCount
		detail.NameserverCount = overview.Totals.NameserverCount
		detail.EndpointCount = overview.Totals.EndpointCount
		detail.ASNCount = overview.Totals.ASNCount
		detail.PrefixCount = overview.Totals.PrefixCount
		if len(overview.FactDistributions) > 0 {
			detail.FactDistributions = overview.FactDistributions
		}
		return
	}
	detail.DomainCount = len(readStore.ListSnapshotDomainViews(snapshotID))
	detail.NameserverCount = len(readStore.ListSnapshotNameserverViews(snapshotID))
	detail.EndpointCount = len(readStore.ListSnapshotEndpointViews(snapshotID))
	detail.ASNCount = len(readStore.ListSnapshotASNViews(snapshotID))
	detail.PrefixCount = len(readStore.ListSnapshotPrefixViews(snapshotID))
}

// ── domain detail ──────────────────────────────────────────────────────────────

// handlePublicAnalysisDomainDetail handles GET /pub/api/v1/analysis/domains/{domain}.
func (s *Server) handlePublicAnalysisDomainDetail(w http.ResponseWriter, r *http.Request) {
	domainName, ok := pathValueNonEmpty(w, r, "domain", "domain")
	if !ok {
		return
	}
	if decoded, err := url.PathUnescape(domainName); err == nil {
		domainName = decoded
	}
	_, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	view, found := readStore.GetSnapshotDomainViewByName(snapshot.ID, domainName)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "domain not found in cohort", nil)
		return
	}

	detail := domainViewToDetail(view)
	if entries, ok := s.lookupSnapshotDomainEntries(snapshot.BatchID, view.DomainID); ok {
		detail.Entries = entries
	}

	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, detail)
}

// lookupSnapshotDomainEntries returns English log entries for the run that
// materialized this snapshot's (batch, domain), or ok=false when the run has
// been purged.
func (s *Server) lookupSnapshotDomainEntries(batchID string, domainID int64) ([]PublicAnalysisDomainEntry, bool) {
	if batchID == "" || domainID == 0 {
		return nil, false
	}
	list := s.store.ListRuns(RunFilter{BatchID: batchID, DomainID: domainID, Limit: 1})
	if len(list.Items) == 0 {
		return nil, false
	}
	result, ok := s.store.GetResult(list.Items[0].ID)
	if !ok || result.Raw == nil || len(result.Raw.Entries) == 0 {
		return nil, false
	}
	localized := localizeResultEntries(result.Raw.Entries, "en")
	out := make([]PublicAnalysisDomainEntry, 0, len(localized))
	for _, e := range localized {
		out = append(out, PublicAnalysisDomainEntry{
			Timestamp: e.Timestamp,
			Module:    e.Module,
			Testcase:  e.Testcase,
			Tag:       e.Tag,
			Level:     e.Level,
			Message:   e.Message,
			Raw:       e.Raw,
		})
	}
	return out, true
}

// domainViewToDetail rehydrates the per-snapshot domain view into the
// public response shape: rebuilds nameserver/address structs and the
// at-floor tag list from the JSON columns.
func domainViewToDetail(v AnalysisSnapshotDomainView) PublicAnalysisDomainDetail {
	out := PublicAnalysisDomainDetail{
		Domain:          v.DomainName,
		Score:           v.Score,
		WorstLevel:      v.WorstLevel,
		FinishedAt:      v.FinishedAt,
		NameserverCount: v.NameserverCount,
		EndpointCount:   v.EndpointCount,
		ASNCount:        v.ASNCount,
		PrefixCount:     v.PrefixCount,
		Nameservers:     make([]PublicAnalysisDomainNameserver, 0, len(v.Nameservers)),
		Addresses:       make([]PublicAnalysisDomainAddress, 0, len(v.Addresses)),
	}
	if v.Grade != "" {
		g := v.Grade
		out.Grade = &g
	}
	for _, ns := range v.Nameservers {
		nsAddrs := make([]PublicAnalysisDomainAddress, 0, len(ns.Addresses))
		for _, a := range ns.Addresses {
			nsAddrs = append(nsAddrs, domainAddressToPublic(a))
		}
		out.Nameservers = append(out.Nameservers, PublicAnalysisDomainNameserver{
			Nameserver: ns.Name,
			IPv4Count:  ns.IPv4Count,
			IPv6Count:  ns.IPv6Count,
			Addresses:  nsAddrs,
			Status:     ns.Status,
		})
	}
	for _, a := range v.Addresses {
		out.Addresses = append(out.Addresses, domainAddressToPublic(a))
	}
	if len(v.Tags) > 0 {
		out.Tags = make([]PublicAnalysisDomainTag, 0, len(v.Tags))
		for _, t := range v.Tags {
			out.Tags = append(out.Tags, PublicAnalysisDomainTag(t))
		}
	}
	return out
}

func domainAddressToPublic(a DomainViewAddress) PublicAnalysisDomainAddress {
	return PublicAnalysisDomainAddress(a)
}

// ── nameserver detail ──────────────────────────────────────────────────────────

// handlePublicAnalysisNameserverDetail handles GET /pub/api/v1/analysis/nameservers/{name}.
func (s *Server) handlePublicAnalysisNameserverDetail(w http.ResponseWriter, r *http.Request) {
	name, ok := pathValueNonEmpty(w, r, "name", "nameserver")
	if !ok {
		return
	}
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	_, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	view, found := readStore.GetSnapshotNameserverViewByName(snapshot.ID, name)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "nameserver not found in cohort", nil)
		return
	}

	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisNameserverDetail{
		Nameserver:    view.NameserverName,
		DomainCount:   view.DomainCount,
		EndpointCount: view.EndpointCount,
		IPv4Count:     view.IPv4Count,
		IPv6Count:     view.IPv6Count,
		Addresses:     view.Addresses,
		Domains:       view.Domains,
		ASNs:          view.ASNs,
	})
}

// ── endpoint detail ────────────────────────────────────────────────────────────

// handlePublicAnalysisEndpointDetail handles GET /pub/api/v1/analysis/endpoints/{address}.
func (s *Server) handlePublicAnalysisEndpointDetail(w http.ResponseWriter, r *http.Request) {
	rawAddr, ok := pathValueNonEmpty(w, r, "address", "address")
	if !ok {
		return
	}
	if decoded, err := url.PathUnescape(rawAddr); err == nil {
		rawAddr = decoded
	}
	_, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	selectedNameserver := strings.TrimSpace(r.URL.Query().Get("nameserver"))
	rows := readStore.ListSnapshotEndpointViewsByAddress(snapshot.ID, rawAddr, selectedNameserver)
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found in cohort", nil)
		return
	}
	if selectedNameserver == "" && len(rows) > 1 {
		writeError(w, http.StatusBadRequest, "ambiguous_endpoint", "multiple nameservers use that address; provide nameserver query parameter", nil)
		return
	}
	view := rows[0]

	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisEndpointDetail{
		Nameserver:  view.NameserverName,
		Address:     view.Address,
		Family:      view.Family,
		ASN:         view.ASN,
		ASNLabel:    view.ASNLabel,
		Prefix:      view.Prefix,
		DomainCount: view.DomainCount,
		Domains:     view.Domains,
	})
}

// ── ASN detail ─────────────────────────────────────────────────────────────────

// handlePublicAnalysisASNDetail handles GET /pub/api/v1/analysis/asns/{asn}.
func (s *Server) handlePublicAnalysisASNDetail(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("asn")
	asn, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(raw), "as"), 10, 64)
	if err != nil || asn <= 0 {
		writeError(w, http.StatusNotFound, "not_found", "asn not found", nil)
		return
	}
	_, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	view, found := readStore.GetSnapshotASNView(snapshot.ID, asn)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "asn not found in cohort", nil)
		return
	}

	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisASNDetail{
		ASN:             view.ASN,
		Label:           view.Label,
		DomainCount:     view.DomainCount,
		AddressCount:    view.AddressCount,
		NameserverCount: view.NameserverCount,
		PrefixCount:     view.PrefixCount,
		Domains:         view.Domains,
		Nameservers:     view.Nameservers,
		Prefixes:        view.Prefixes,
	})
}

// ── prefix detail ──────────────────────────────────────────────────────────────

// handlePublicAnalysisPrefixDetail handles GET /pub/api/v1/analysis/prefixes.
// It expects the prefix as a "prefix" query parameter to avoid URL-encoding
// the "/" inside IPv4/IPv6 CIDRs.
func (s *Server) handlePublicAnalysisPrefixDetail(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("prefix"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "missing_prefix", "prefix query parameter is required", nil)
		return
	}
	_, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	view, found := readStore.GetSnapshotPrefixView(snapshot.ID, raw)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "prefix not found in cohort", nil)
		return
	}

	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisPrefixDetail{
		Prefix:       view.Prefix,
		Family:       view.Family,
		DomainCount:  view.DomainCount,
		AddressCount: view.AddressCount,
		ASNs:         view.ASNs,
		Domains:      view.Domains,
		Addresses:    view.Addresses,
	})
}

// ── tag detail ─────────────────────────────────────────────────────────────────

// handlePublicAnalysisTagDetail handles GET /pub/api/v1/analysis/tags/{tag}.
func (s *Server) handlePublicAnalysisTagDetail(w http.ResponseWriter, r *http.Request) {
	tag, ok := pathValueNonEmpty(w, r, "tag", "tag")
	if !ok {
		return
	}
	_, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	view, found := readStore.GetSnapshotTagView(snapshot.ID, tag)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "tag not found in cohort", nil)
		return
	}

	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisTagDetail{
		Tag:             view.Tag,
		Module:          view.Module,
		Testcase:        view.Testcase,
		Level:           view.Level,
		DomainCount:     view.DomainCount,
		OccurrenceCount: view.OccurrenceCount,
		Domains:         view.Domains,
	})
}

// ── testcase detail ────────────────────────────────────────────────────────────

// handlePublicAnalysisTestcaseDetail handles GET /pub/api/v1/analysis/testcases/by-name.
// Expects "module" and "testcase" query parameters to avoid URL-encoding issues.
func (s *Server) handlePublicAnalysisTestcaseDetail(w http.ResponseWriter, r *http.Request) {
	module := strings.TrimSpace(r.URL.Query().Get("module"))
	testcase := strings.TrimSpace(r.URL.Query().Get("testcase"))
	if testcase == "" {
		writeError(w, http.StatusBadRequest, "missing_testcase", "testcase query parameter is required", nil)
		return
	}
	_, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	domainSet := map[string]struct{}{}
	tagSet := map[string]struct{}{}
	entryCount := 0
	worstLevel := ""
	for _, row := range readStore.ListSnapshotTagViews(snapshot.ID) {
		if row.Testcase != testcase {
			continue
		}
		if module != "" && row.Module != module {
			continue
		}
		tagSet[row.Tag] = struct{}{}
		entryCount += row.OccurrenceCount
		for _, d := range row.Domains {
			domainSet[d] = struct{}{}
		}
		if severityRank(row.Level) > severityRank(worstLevel) {
			worstLevel = row.Level
		}
	}
	if len(tagSet) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "testcase not found in cohort", nil)
		return
	}

	domains := slices.Sorted(maps.Keys(domainSet))

	tags := slices.Sorted(maps.Keys(tagSet))

	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisTestcaseDetail{
		Module:      module,
		Testcase:    testcase,
		DomainCount: len(domainSet),
		EntryCount:  entryCount,
		WorstLevel:  worstLevel,
		Tags:        tags,
		Domains:     domains,
	})
}

func pathValueNonEmpty(w http.ResponseWriter, r *http.Request, key, entity string) (string, bool) {
	raw := strings.TrimSpace(r.PathValue(key))
	if raw == "" {
		writeError(w, http.StatusNotFound, "not_found", entity+" not found", nil)
		return "", false
	}
	return raw, true
}
