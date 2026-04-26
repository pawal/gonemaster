package server

import (
	"net/http"
	"net/url"
	"sort"
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
	SeverityDistribution  map[string]int                            `json:"severity_distribution,omitempty"`
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
		aggregates := readStore.ListSnapshotAggregates(snapshot.ID)
		snapshotView := publicAnalysisSnapshotView(snapshot)
		detail.Snapshot = &snapshotView
		_ = aggregates // reserved for future trend surface
	} else if snapshot.ID == 0 {
		detail.Status = PublicAnalysisStatusNoSnapshot
	}
	if _, canRead := s.store.(AnalysisReadStore); canRead {
		data := s.latestMaterializationForSnapshot(cohort, snapshot)

		domainSet := map[int64]struct{}{}
		nsSet := map[int64]struct{}{}
		endpointSet := map[[2]int64]struct{}{}
		asnSet := map[int64]struct{}{}
		prefixSet := map[int64]struct{}{}
		severity := map[string]int{}
		for _, pair := range data.latest {
			domainSet[pair.summary.DomainID] = struct{}{}
			severity[severityBucket(pair.summary.WorstLevel)]++
		}
		for _, ep := range data.endpoints {
			nsSet[ep.NameserverID] = struct{}{}
			endpointSet[[2]int64{ep.NameserverID, ep.AddressID}] = struct{}{}
		}
		for _, fact := range data.addressASNs {
			if fact.ASN != nil {
				asnSet[*fact.ASN] = struct{}{}
			}
			if fact.PrefixID != nil {
				prefixSet[*fact.PrefixID] = struct{}{}
			}
		}
		for _, da := range data.domainASNs {
			asnSet[da.ASN] = struct{}{}
		}
		detail.DomainCount = len(domainSet)
		detail.NameserverCount = len(nsSet)
		detail.EndpointCount = len(endpointSet)
		detail.ASNCount = len(asnSet)
		detail.PrefixCount = len(prefixSet)
		if len(severity) > 0 {
			detail.SeverityDistribution = severity
		}
		if dist := buildFactDistributions(data.domainFacts); len(dist) > 0 {
			detail.FactDistributions = dist
		}
	}
	writeJSON(w, http.StatusOK, detail)
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

	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, domainViewToDetail(view))
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
			out.Tags = append(out.Tags, PublicAnalysisDomainTag{
				Tag:      t.Tag,
				Module:   t.Module,
				Testcase: t.Testcase,
				Level:    t.Level,
			})
		}
	}
	return out
}

func domainAddressToPublic(a DomainViewAddress) PublicAnalysisDomainAddress {
	return PublicAnalysisDomainAddress{
		Address:  a.Address,
		Family:   a.Family,
		ASN:      a.ASN,
		ASNLabel: a.ASNLabel,
		Prefix:   a.Prefix,
		Status:   a.Status,
	}
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
	cohort, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	data := s.latestMaterializationForSnapshot(cohort, snapshot)

	domainSet := map[int64]struct{}{}
	addrSet := map[int64]struct{}{}
	prefixSet := map[int64]struct{}{}
	for _, fact := range data.addressASNs {
		if fact.ASN == nil || *fact.ASN != asn {
			continue
		}
		domainSet[fact.DomainID] = struct{}{}
		addrSet[fact.AddressID] = struct{}{}
		if fact.PrefixID != nil {
			prefixSet[*fact.PrefixID] = struct{}{}
		}
	}
	for _, da := range data.domainASNs {
		if da.ASN != asn {
			continue
		}
		domainSet[da.DomainID] = struct{}{}
	}
	if len(domainSet) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "asn not found in cohort", nil)
		return
	}

	nsSet := map[int64]struct{}{}
	for _, ep := range data.endpoints {
		if _, addrIn := addrSet[ep.AddressID]; addrIn {
			nsSet[ep.NameserverID] = struct{}{}
		}
	}

	domains := make([]string, 0, len(domainSet))
	for domainID := range domainSet {
		if d, ok := s.store.GetDomain(domainID); ok {
			domains = append(domains, d.Name)
		}
	}
	sort.Strings(domains)

	nsNames := make([]string, 0, len(nsSet))
	for nsID := range nsSet {
		if ns, ok := readStore.GetAnalysisNameserver(nsID); ok {
			nsNames = append(nsNames, ns.Name)
		}
	}
	sort.Strings(nsNames)

	prefixes := make([]string, 0, len(prefixSet))
	for prefixID := range prefixSet {
		if p, ok := readStore.GetAnalysisPrefix(prefixID); ok {
			prefixes = append(prefixes, p.Prefix)
		}
	}
	sort.Strings(prefixes)

	label := ""
	if meta, ok := readStore.GetAnalysisASN(asn); ok {
		label = meta.Label
	}

	writeJSON(w, http.StatusOK, PublicAnalysisASNDetail{
		ASN:             asn,
		Label:           label,
		DomainCount:     len(domainSet),
		AddressCount:    len(addrSet),
		NameserverCount: len(nsSet),
		PrefixCount:     len(prefixSet),
		Domains:         domains,
		Nameservers:     nsNames,
		Prefixes:        prefixes,
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
	cohort, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	prefix, found := readStore.GetAnalysisPrefix(0) // placeholder; overridden below
	_ = prefix
	_ = found
	// Walk address_asns to find the matching prefix_id by re-resolving via GetAnalysisPrefix.
	var prefixID int64
	var meta AnalysisPrefix
	data := s.latestMaterializationForSnapshot(cohort, snapshot)
	for _, fact := range data.addressASNs {
		if fact.PrefixID == nil {
			continue
		}
		if p, ok := readStore.GetAnalysisPrefix(*fact.PrefixID); ok && p.Prefix == raw {
			prefixID = p.ID
			meta = p
			break
		}
	}
	if prefixID == 0 {
		writeError(w, http.StatusNotFound, "not_found", "prefix not found in cohort", nil)
		return
	}

	domainSet := map[int64]struct{}{}
	addrSet := map[int64]struct{}{}
	asnSet := map[int64]struct{}{}
	for _, fact := range data.addressASNs {
		if fact.PrefixID == nil || *fact.PrefixID != prefixID {
			continue
		}
		domainSet[fact.DomainID] = struct{}{}
		addrSet[fact.AddressID] = struct{}{}
		if fact.ASN != nil {
			asnSet[*fact.ASN] = struct{}{}
		}
	}

	domains := make([]string, 0, len(domainSet))
	for domainID := range domainSet {
		if d, ok := s.store.GetDomain(domainID); ok {
			domains = append(domains, d.Name)
		}
	}
	sort.Strings(domains)

	addresses := make([]string, 0, len(addrSet))
	for addrID := range addrSet {
		if a, ok := readStore.GetAnalysisAddress(addrID); ok {
			addresses = append(addresses, a.Address)
		}
	}
	sort.Strings(addresses)

	asns := make([]int64, 0, len(asnSet))
	for asn := range asnSet {
		asns = append(asns, asn)
	}
	sort.Slice(asns, func(i, j int) bool { return asns[i] < asns[j] })

	writeJSON(w, http.StatusOK, PublicAnalysisPrefixDetail{
		Prefix:       meta.Prefix,
		Family:       meta.Family,
		DomainCount:  len(domainSet),
		AddressCount: len(addrSet),
		ASNs:         asns,
		Domains:      domains,
		Addresses:    addresses,
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
	cohort, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	if _, ok := s.analysisReadStore(w); !ok {
		return
	}
	latest := s.latestMaterializationForSnapshot(cohort, snapshot).latest

	domainSet := map[int64]struct{}{}
	tagSet := map[string]struct{}{}
	entryCount := 0
	worstLevel := ""
	for _, pair := range latest {
		for _, entry := range s.loadAllEntriesForRun(pair.summary.RunID) {
			if entry.Testcase != testcase {
				continue
			}
			if module != "" && entry.Module != module {
				continue
			}
			domainSet[pair.summary.DomainID] = struct{}{}
			tagSet[entry.Tag] = struct{}{}
			entryCount++
			if severityRank(entry.Level) > severityRank(worstLevel) {
				worstLevel = entry.Level
			}
		}
	}
	if entryCount == 0 {
		writeError(w, http.StatusNotFound, "not_found", "testcase not found in cohort", nil)
		return
	}

	domains := make([]string, 0, len(domainSet))
	for id := range domainSet {
		if d, ok := s.store.GetDomain(id); ok {
			domains = append(domains, d.Name)
		}
	}
	sort.Strings(domains)

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

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
