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
	DatasetTag            string         `json:"dataset_tag"`
	Label                 string         `json:"label"`
	Description           string         `json:"description,omitempty"`
	IsDefault             bool           `json:"is_default"`
	MaterializationStatus string         `json:"materialization_status"`
	LastMaterializedAt    *time.Time     `json:"last_materialized_at,omitempty"`
	DomainCount           int            `json:"domain_count"`
	NameserverCount       int            `json:"nameserver_count"`
	EndpointCount         int            `json:"endpoint_count"`
	ASNCount              int            `json:"asn_count"`
	PrefixCount           int            `json:"prefix_count"`
	SeverityDistribution  map[string]int `json:"severity_distribution,omitempty"`
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
	Tags            []PublicAnalysisDomainTag        `json:"tags"`
}

type PublicAnalysisDomainNameserver struct {
	Nameserver string                        `json:"nameserver"`
	IPv4Count  int                           `json:"ipv4_count"`
	IPv6Count  int                           `json:"ipv6_count"`
	Addresses  []PublicAnalysisDomainAddress `json:"addresses"`
}

type PublicAnalysisDomainAddress struct {
	Address  string `json:"address"`
	Family   string `json:"family"`
	ASN      *int64 `json:"asn,omitempty"`
	ASNLabel string `json:"asn_label,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
}

type PublicAnalysisDomainTag struct {
	Tag      string `json:"tag"`
	Module   string `json:"module,omitempty"`
	Testcase string `json:"testcase,omitempty"`
	Level    string `json:"level,omitempty"`
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
	if _, canRead := s.store.(AnalysisReadStore); canRead {
		data := s.latestMaterializationForCohort(cohort)

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
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
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

	data := s.latestMaterializationForCohort(cohort)
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
	}

	// Build per-address facts once, then reuse per nameserver.
	addressView := make(map[int64]PublicAnalysisDomainAddress, len(addrIDs))
	for addrID := range addrIDs {
		addr, found := readStore.GetAnalysisAddress(addrID)
		if !found {
			continue
		}
		view := PublicAnalysisDomainAddress{Address: addr.Address, Family: addr.Family}
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
		nameservers = append(nameservers, PublicAnalysisDomainNameserver{
			Nameserver: n.name,
			IPv4Count:  len(n.v4),
			IPv6Count:  len(n.v6),
			Addresses:  nsAddrs,
		})
	}
	sort.Slice(nameservers, func(i, j int) bool { return nameservers[i].Nameserver < nameservers[j].Nameserver })

	addresses := make([]PublicAnalysisDomainAddress, 0, len(addressView))
	for _, view := range addressView {
		addresses = append(addresses, view)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Address < addresses[j].Address })

	tagsByKey := map[string]PublicAnalysisDomainTag{}
	for _, entry := range s.loadAllEntriesForRun(pair.summary.RunID) {
		if strings.TrimSpace(entry.Tag) == "" {
			continue
		}
		existing, seen := tagsByKey[entry.Tag]
		if !seen || severityRank(entry.Level) > severityRank(existing.Level) {
			tagsByKey[entry.Tag] = PublicAnalysisDomainTag{
				Tag:      entry.Tag,
				Module:   entry.Module,
				Testcase: entry.Testcase,
				Level:    entry.Level,
			}
		}
	}
	tags := make([]PublicAnalysisDomainTag, 0, len(tagsByKey))
	for _, v := range tagsByKey {
		tags = append(tags, v)
	}
	sort.Slice(tags, func(i, j int) bool {
		li, lj := severityRank(tags[i].Level), severityRank(tags[j].Level)
		if li != lj {
			return li > lj
		}
		return tags[i].Tag < tags[j].Tag
	})

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
		Tags:            tags,
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
	decoded, err := url.PathUnescape(name)
	if err == nil {
		name = decoded
	}
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	data := s.latestMaterializationForCohort(cohort)

	addressASN := map[int64]int64{}
	for _, fact := range data.addressASNs {
		if fact.ASN != nil {
			addressASN[fact.AddressID] = *fact.ASN
		}
	}

	target := strings.ToLower(name)
	var nsID int64
	foundName := ""
	addrSet := map[int64]struct{}{}
	domainSet := map[int64]struct{}{}
	v4Set := map[int64]struct{}{}
	v6Set := map[int64]struct{}{}
	asnSet := map[int64]struct{}{}
	for _, ep := range data.endpoints {
		ns, found := readStore.GetAnalysisNameserver(ep.NameserverID)
		if !found || strings.ToLower(ns.Name) != target {
			continue
		}
		nsID = ep.NameserverID
		foundName = ns.Name
		addrSet[ep.AddressID] = struct{}{}
		domainSet[ep.DomainID] = struct{}{}
		switch ep.Family {
		case "ipv4":
			v4Set[ep.AddressID] = struct{}{}
		case "ipv6":
			v6Set[ep.AddressID] = struct{}{}
		}
		if asn, has := addressASN[ep.AddressID]; has {
			asnSet[asn] = struct{}{}
		}
	}
	if nsID == 0 {
		writeError(w, http.StatusNotFound, "not_found", "nameserver not found in cohort", nil)
		return
	}

	addresses := make([]string, 0, len(addrSet))
	for addrID := range addrSet {
		if addr, ok := readStore.GetAnalysisAddress(addrID); ok {
			addresses = append(addresses, addr.Address)
		}
	}
	sort.Strings(addresses)

	domains := make([]string, 0, len(domainSet))
	for domainID := range domainSet {
		if d, ok := s.store.GetDomain(domainID); ok {
			domains = append(domains, d.Name)
		}
	}
	sort.Strings(domains)

	asns := make([]int64, 0, len(asnSet))
	for asn := range asnSet {
		asns = append(asns, asn)
	}
	sort.Slice(asns, func(i, j int) bool { return asns[i] < asns[j] })

	writeJSON(w, http.StatusOK, PublicAnalysisNameserverDetail{
		Nameserver:    foundName,
		DomainCount:   len(domainSet),
		EndpointCount: len(addrSet),
		IPv4Count:     len(v4Set),
		IPv6Count:     len(v6Set),
		Addresses:     addresses,
		Domains:       domains,
		ASNs:          asns,
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
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	data := s.latestMaterializationForCohort(cohort)

	target := strings.ToLower(rawAddr)
	selectedNameserver := strings.TrimSpace(r.URL.Query().Get("nameserver"))
	selectedNameserverLower := strings.ToLower(selectedNameserver)
	var addr AnalysisAddress
	found := false
	domainSet := map[int64]struct{}{}
	nameservers := map[int64]struct{}{}
	for _, ep := range data.endpoints {
		a, ok := readStore.GetAnalysisAddress(ep.AddressID)
		if !ok || strings.ToLower(a.Address) != target {
			continue
		}
		ns, ok := readStore.GetAnalysisNameserver(ep.NameserverID)
		if !ok {
			continue
		}
		if selectedNameserverLower != "" && strings.ToLower(ns.Name) != selectedNameserverLower {
			continue
		}
		addr = a
		found = true
		domainSet[ep.DomainID] = struct{}{}
		nameservers[ep.NameserverID] = struct{}{}
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found in cohort", nil)
		return
	}
	if selectedNameserverLower == "" && len(nameservers) > 1 {
		writeError(w, http.StatusBadRequest, "ambiguous_endpoint", "multiple nameservers use that address; provide nameserver query parameter", nil)
		return
	}

	asnSet := map[int64]struct{}{}
	prefixSet := map[string]struct{}{}
	for _, fact := range data.addressASNs {
		if fact.AddressID != addr.ID {
			continue
		}
		if _, ok := domainSet[fact.DomainID]; !ok {
			continue
		}
		if fact.ASN != nil {
			asnSet[*fact.ASN] = struct{}{}
		}
		if fact.PrefixID != nil {
			if p, ok := readStore.GetAnalysisPrefix(*fact.PrefixID); ok {
				prefixSet[p.Prefix] = struct{}{}
			}
		}
	}
	var asn *int64
	if len(asnSet) == 1 {
		for value := range asnSet {
			asnCopy := value
			asn = &asnCopy
		}
	}
	prefix := ""
	if len(prefixSet) == 1 {
		for value := range prefixSet {
			prefix = value
		}
	}

	domains := make([]string, 0, len(domainSet))
	for domainID := range domainSet {
		if d, ok := s.store.GetDomain(domainID); ok {
			domains = append(domains, d.Name)
		}
	}
	sort.Strings(domains)

	nsNames := make([]string, 0, len(nameservers))
	for nsID := range nameservers {
		if ns, ok := readStore.GetAnalysisNameserver(nsID); ok {
			nsNames = append(nsNames, ns.Name)
		}
	}
	sort.Strings(nsNames)
	primaryNS := ""
	if len(nsNames) > 0 {
		primaryNS = nsNames[0]
	}

	asnLabel := ""
	if asn != nil {
		if meta, ok := readStore.GetAnalysisASN(*asn); ok {
			asnLabel = meta.Label
		}
	}
	writeJSON(w, http.StatusOK, PublicAnalysisEndpointDetail{
		Nameserver:  primaryNS,
		Address:     addr.Address,
		Family:      addr.Family,
		ASN:         asn,
		ASNLabel:    asnLabel,
		Prefix:      prefix,
		DomainCount: len(domainSet),
		Domains:     domains,
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
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
	if !ok {
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}

	data := s.latestMaterializationForCohort(cohort)

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
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
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
	data := s.latestMaterializationForCohort(cohort)
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
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
	if !ok {
		return
	}
	if _, ok := s.analysisReadStore(w); !ok {
		return
	}
	latest := s.latestMaterializationForCohort(cohort).latest

	domainNames := map[int64]string{}
	for _, pair := range latest {
		if d, ok := s.store.GetDomain(pair.summary.DomainID); ok {
			domainNames[pair.summary.DomainID] = d.Name
		}
	}

	domainSet := map[int64]struct{}{}
	var occurrences int
	var module, testcase, level string
	for _, pair := range latest {
		for _, entry := range s.loadAllEntriesForRun(pair.summary.RunID) {
			if entry.Tag != tag {
				continue
			}
			domainSet[pair.summary.DomainID] = struct{}{}
			occurrences++
			module = entry.Module
			testcase = entry.Testcase
			if severityRank(entry.Level) > severityRank(level) {
				level = entry.Level
			}
		}
	}
	if occurrences == 0 {
		writeError(w, http.StatusNotFound, "not_found", "tag not found in cohort", nil)
		return
	}

	domains := make([]string, 0, len(domainSet))
	for id := range domainSet {
		if name, ok := domainNames[id]; ok {
			domains = append(domains, name)
		}
	}
	sort.Strings(domains)

	writeJSON(w, http.StatusOK, PublicAnalysisTagDetail{
		Tag:             tag,
		Module:          module,
		Testcase:        testcase,
		Level:           level,
		DomainCount:     len(domainSet),
		OccurrenceCount: occurrences,
		Domains:         domains,
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
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
	if !ok {
		return
	}
	if _, ok := s.analysisReadStore(w); !ok {
		return
	}
	latest := s.latestMaterializationForCohort(cohort).latest

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
