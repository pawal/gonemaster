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
	Entries         []PublicAnalysisDomainEntry      `json:"entries"`
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
	cohort, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}
	domain, found := s.store.GetDomainByName(domainName)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "domain not found", nil)
		return
	}

	data := s.latestMaterializationForSnapshot(cohort, snapshot)
	var pair domainSummaryPair
	haveSummary := false
	for _, p := range data.latest {
		if p.summary.DomainID == domain.ID {
			pair = p
			haveSummary = true
			break
		}
	}
	if !haveSummary {
		writeError(w, http.StatusNotFound, "not_found", "domain not materialized in cohort", nil)
		return
	}

	type nsEntry struct {
		name string
		v4   map[int64]struct{}
		v6   map[int64]struct{}
		// addrs preserves the ordered set of (addressID, family) this
		// nameserver serves, so we can later hang per-address facts off
		// each nameserver rather than duplicating them into a flat list.
		addrs []int64
		seen  map[int64]struct{}
	}
	nsByID := map[int64]*nsEntry{}
	addrIDs := map[int64]struct{}{}
	// addrStatus tracks reachability per address. "ok" wins over
	// "unreachable" when the same address appears in multiple endpoint
	// rows with different query counts.
	addrStatus := map[int64]string{}
	for _, ep := range data.endpoints {
		if ep.RunID != pair.summary.RunID || ep.DomainID != domain.ID {
			continue
		}
		addrIDs[ep.AddressID] = struct{}{}
		n, exists := nsByID[ep.NameserverID]
		if !exists {
			ns, found := readStore.GetAnalysisNameserver(ep.NameserverID)
			if !found {
				continue
			}
			n = &nsEntry{
				name: ns.Name,
				v4:   map[int64]struct{}{},
				v6:   map[int64]struct{}{},
				seen: map[int64]struct{}{},
			}
			nsByID[ep.NameserverID] = n
		}
		switch ep.Family {
		case "ipv4":
			n.v4[ep.AddressID] = struct{}{}
		case "ipv6":
			n.v6[ep.AddressID] = struct{}{}
		}
		if _, ok := n.seen[ep.AddressID]; !ok {
			n.seen[ep.AddressID] = struct{}{}
			n.addrs = append(n.addrs, ep.AddressID)
		}
		if ep.AddressID != 0 {
			if ep.QueryCount > 0 {
				addrStatus[ep.AddressID] = "ok"
			} else if _, set := addrStatus[ep.AddressID]; !set {
				addrStatus[ep.AddressID] = "unreachable"
			}
		}
	}

	// Build per-address facts once, then reuse per nameserver.
	addressView := make(map[int64]PublicAnalysisDomainAddress, len(addrIDs))
	for addrID := range addrIDs {
		addr, found := readStore.GetAnalysisAddress(addrID)
		if !found {
			continue
		}
		view := PublicAnalysisDomainAddress{Address: addr.Address, Family: addr.Family}
		if s := addrStatus[addrID]; s == "unreachable" {
			view.Status = "unreachable"
		}
		for _, fact := range data.addressASNs {
			if fact.RunID != pair.summary.RunID || fact.DomainID != domain.ID || fact.AddressID != addrID {
				continue
			}
			if fact.ASN != nil {
				asn := *fact.ASN
				view.ASN = &asn
				if meta, ok := readStore.GetAnalysisASN(asn); ok {
					view.ASNLabel = meta.Label
				}
			}
			if fact.PrefixID != nil {
				if prefix, ok := readStore.GetAnalysisPrefix(*fact.PrefixID); ok {
					view.Prefix = prefix.Prefix
				}
			}
			break
		}
		addressView[addrID] = view
	}

	nameservers := make([]PublicAnalysisDomainNameserver, 0, len(nsByID))
	for _, n := range nsByID {
		nsAddrs := make([]PublicAnalysisDomainAddress, 0, len(n.addrs))
		for _, addrID := range n.addrs {
			if view, ok := addressView[addrID]; ok {
				nsAddrs = append(nsAddrs, view)
			}
		}
		sort.Slice(nsAddrs, func(i, j int) bool {
			if nsAddrs[i].Family != nsAddrs[j].Family {
				// Keep IPv4 before IPv6 for a predictable reading order.
				return nsAddrs[i].Family < nsAddrs[j].Family
			}
			return nsAddrs[i].Address < nsAddrs[j].Address
		})
		nsView := PublicAnalysisDomainNameserver{
			Nameserver: n.name,
			IPv4Count:  len(n.v4),
			IPv6Count:  len(n.v6),
			Addresses:  nsAddrs,
		}
		if len(nsAddrs) == 0 {
			nsView.Status = "unresolved"
		}
		nameservers = append(nameservers, nsView)
	}
	sort.Slice(nameservers, func(i, j int) bool { return nameservers[i].Nameserver < nameservers[j].Nameserver })

	addresses := make([]PublicAnalysisDomainAddress, 0, len(addressView))
	for _, view := range addressView {
		addresses = append(addresses, view)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Address < addresses[j].Address })

	// Build one entry per observed log row so the UI can render a grouped
	// module/testcase results view with per-entry translated messages, the
	// same shape the public UI's Results view consumes. Entries with an
	// empty tag are logger metadata noise and are skipped here.
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	loaded := s.loadAllEntriesForRun(pair.summary.RunID)
	resultEntries := make([]JobResultEntry, 0, len(loaded))
	for _, e := range loaded {
		if strings.TrimSpace(e.Tag) == "" {
			continue
		}
		resultEntries = append(resultEntries, JobResultEntry{
			Timestamp: e.Timestamp,
			Module:    e.Module,
			Testcase:  e.Testcase,
			Tag:       e.Tag,
			Level:     e.Level,
			Args:      e.Args,
		})
	}
	localized := localizeResultEntries(resultEntries, locale)
	entries := make([]PublicAnalysisDomainEntry, 0, len(localized))
	for _, e := range localized {
		entries = append(entries, PublicAnalysisDomainEntry{
			Timestamp: e.Timestamp,
			Module:    e.Module,
			Testcase:  e.Testcase,
			Tag:       e.Tag,
			Level:     e.Level,
			Message:   e.Message,
			Raw:       e.Raw,
		})
	}

	detail := PublicAnalysisDomainDetail{
		Domain:          domain.Name,
		Score:           pair.summary.Score,
		Grade:           pair.summary.Grade,
		WorstLevel:      pair.summary.WorstLevel,
		NameserverCount: pair.summary.NameserverCount,
		EndpointCount:   pair.summary.EndpointCount,
		ASNCount:        pair.summary.ASNCount,
		PrefixCount:     pair.summary.PrefixCount,
		Nameservers:     nameservers,
		Addresses:       addresses,
		Entries:         entries,
	}
	if !pair.finishedAt.IsZero() {
		ft := pair.finishedAt
		detail.FinishedAt = &ft
	}
	writeJSON(w, http.StatusOK, detail)
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
