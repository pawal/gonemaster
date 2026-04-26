package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// PublicAnalysisNameserverView aggregates one nameserver's footprint in the cohort.
type PublicAnalysisNameserverView struct {
	Nameserver    string `json:"nameserver"`
	DomainCount   int    `json:"domain_count"`
	EndpointCount int    `json:"endpoint_count"`
	IPv4Count     int    `json:"ipv4_count"`
	IPv6Count     int    `json:"ipv6_count"`
	ASNCount      int    `json:"asn_count"`
	// Operator is the ASN label when every endpoint on this nameserver
	// resolves to a single ASN. Empty string when unknown; set to
	// "Multiple" when endpoints span more than one ASN.
	Operator    string `json:"operator,omitempty"`
	OperatorASN *int64 `json:"operator_asn,omitempty"`
	QueryCount  int    `json:"query_count,omitempty"`
}

// PublicAnalysisEndpointView is one (nameserver, address) pair in the cohort.
type PublicAnalysisEndpointView struct {
	Nameserver  string `json:"nameserver"`
	Address     string `json:"address"`
	Family      string `json:"family"`
	DomainCount int    `json:"domain_count"`
	ASN         *int64 `json:"asn,omitempty"`
	ASNLabel    string `json:"asn_label,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
}

// PublicAnalysisASNView aggregates one ASN's footprint in the cohort.
type PublicAnalysisASNView struct {
	ASN             int64  `json:"asn"`
	Label           string `json:"label,omitempty"`
	DomainCount     int    `json:"domain_count"`
	AddressCount    int    `json:"address_count"`
	NameserverCount int    `json:"nameserver_count"`
	PrefixCount     int    `json:"prefix_count"`
	IPv4Count       int    `json:"ipv4_count"`
	IPv6Count       int    `json:"ipv6_count"`
}

// PublicAnalysisPrefixView aggregates one announced prefix in the cohort.
type PublicAnalysisPrefixView struct {
	Prefix       string `json:"prefix"`
	Family       string `json:"family"`
	DomainCount  int    `json:"domain_count"`
	AddressCount int    `json:"address_count"`
	ASN          *int64 `json:"asn,omitempty"`
	ASNLabel     string `json:"asn_label,omitempty"`
}

// handlePublicAnalysisNameservers handles GET /pub/api/v1/analysis/nameservers.
func (s *Server) handlePublicAnalysisNameservers(w http.ResponseWriter, r *http.Request) {
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

	rows := readStore.ListSnapshotNameserverViews(snapshot.ID)
	items := make([]PublicAnalysisNameserverView, 0, len(rows))
	for _, row := range rows {
		view := PublicAnalysisNameserverView{
			Nameserver:    row.NameserverName,
			DomainCount:   row.DomainCount,
			EndpointCount: row.EndpointCount,
			IPv4Count:     row.IPv4Count,
			IPv6Count:     row.IPv6Count,
			ASNCount:      row.ASNCount,
			Operator:      row.Operator,
			QueryCount:    row.QueryCount,
		}
		if row.OperatorASN != nil {
			asnCopy := *row.OperatorASN
			view.OperatorASN = &asnCopy
		}
		items = append(items, view)
	}

	items = applyNameserverFilter(items, filter)
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisNameserverView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

func applyNameserverFilter(items []PublicAnalysisNameserverView, filter analysisListFilter) []PublicAnalysisNameserverView {
	if filter.Search != "" {
		needle := strings.ToLower(filter.Search)
		kept := items[:0]
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Nameserver), needle) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	sortNameserverViews(items, filter.Sort)
	return items
}

func sortNameserverViews(items []PublicAnalysisNameserverView, mode string) {
	switch mode {
	case "domain_count_desc":
		sort.Slice(items, func(i, j int) bool {
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount > items[j].DomainCount
			}
			return items[i].Nameserver < items[j].Nameserver
		})
	case "domain_count_asc":
		sort.Slice(items, func(i, j int) bool {
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount < items[j].DomainCount
			}
			return items[i].Nameserver < items[j].Nameserver
		})
	case "endpoint_count_desc":
		sort.Slice(items, func(i, j int) bool {
			if items[i].EndpointCount != items[j].EndpointCount {
				return items[i].EndpointCount > items[j].EndpointCount
			}
			return items[i].Nameserver < items[j].Nameserver
		})
	case "endpoint_count_asc":
		sort.Slice(items, func(i, j int) bool {
			if items[i].EndpointCount != items[j].EndpointCount {
				return items[i].EndpointCount < items[j].EndpointCount
			}
			return items[i].Nameserver < items[j].Nameserver
		})
	case "name_desc":
		sort.Slice(items, func(i, j int) bool { return items[i].Nameserver > items[j].Nameserver })
	default:
		sort.Slice(items, func(i, j int) bool { return items[i].Nameserver < items[j].Nameserver })
	}
}

// sortEndpointViews orders the (nameserver, address) endpoint rows by the
// caller's sort token. The default (empty or unknown token) keeps the
// historical nameserver-then-address ordering. All comparators fall back to
// the same lexical pair so results are stable across repeated calls.
func sortEndpointViews(items []PublicAnalysisEndpointView, mode string) {
	less := func(keys [][2]string) func(i, j int) bool {
		return func(i, j int) bool {
			for _, k := range keys {
				switch k[0] {
				case "nameserver":
					if items[i].Nameserver != items[j].Nameserver {
						if k[1] == "desc" {
							return items[i].Nameserver > items[j].Nameserver
						}
						return items[i].Nameserver < items[j].Nameserver
					}
				case "address":
					if items[i].Address != items[j].Address {
						if k[1] == "desc" {
							return items[i].Address > items[j].Address
						}
						return items[i].Address < items[j].Address
					}
				case "domain_count":
					if items[i].DomainCount != items[j].DomainCount {
						if k[1] == "desc" {
							return items[i].DomainCount > items[j].DomainCount
						}
						return items[i].DomainCount < items[j].DomainCount
					}
				case "prefix":
					if items[i].Prefix != items[j].Prefix {
						if k[1] == "desc" {
							return items[i].Prefix > items[j].Prefix
						}
						return items[i].Prefix < items[j].Prefix
					}
				case "operator":
					if items[i].ASNLabel != items[j].ASNLabel {
						if k[1] == "desc" {
							return items[i].ASNLabel > items[j].ASNLabel
						}
						return items[i].ASNLabel < items[j].ASNLabel
					}
				}
			}
			return false
		}
	}
	var keys [][2]string
	switch mode {
	case "nameserver_desc":
		keys = [][2]string{{"nameserver", "desc"}, {"address", "asc"}}
	case "address_asc":
		keys = [][2]string{{"address", "asc"}, {"nameserver", "asc"}}
	case "address_desc":
		keys = [][2]string{{"address", "desc"}, {"nameserver", "asc"}}
	case "domain_count_asc":
		keys = [][2]string{{"domain_count", "asc"}, {"nameserver", "asc"}, {"address", "asc"}}
	case "domain_count_desc":
		keys = [][2]string{{"domain_count", "desc"}, {"nameserver", "asc"}, {"address", "asc"}}
	case "prefix_asc":
		keys = [][2]string{{"prefix", "asc"}, {"nameserver", "asc"}, {"address", "asc"}}
	case "prefix_desc":
		keys = [][2]string{{"prefix", "desc"}, {"nameserver", "asc"}, {"address", "asc"}}
	case "operator_asc":
		keys = [][2]string{{"operator", "asc"}, {"nameserver", "asc"}, {"address", "asc"}}
	case "operator_desc":
		keys = [][2]string{{"operator", "desc"}, {"nameserver", "asc"}, {"address", "asc"}}
	default:
		keys = [][2]string{{"nameserver", "asc"}, {"address", "asc"}}
	}
	sort.Slice(items, less(keys))
}

// handlePublicAnalysisEndpoints handles GET /pub/api/v1/analysis/endpoints.
func (s *Server) handlePublicAnalysisEndpoints(w http.ResponseWriter, r *http.Request) {
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

	rows := readStore.ListSnapshotEndpointViews(snapshot.ID)
	items := make([]PublicAnalysisEndpointView, 0, len(rows))
	for _, row := range rows {
		v := PublicAnalysisEndpointView{
			Nameserver:  row.NameserverName,
			Address:     row.Address,
			Family:      row.Family,
			DomainCount: row.DomainCount,
			ASNLabel:    row.ASNLabel,
			Prefix:      row.Prefix,
		}
		if row.ASN != nil {
			asnCopy := *row.ASN
			v.ASN = &asnCopy
		}
		items = append(items, v)
	}

	if filter.Search != "" {
		needle := strings.ToLower(filter.Search)
		kept := items[:0]
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Nameserver), needle) ||
				strings.Contains(strings.ToLower(it.Address), needle) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	sortEndpointViews(items, filter.Sort)
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisEndpointView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

// handlePublicAnalysisASNs handles GET /pub/api/v1/analysis/asns.
func (s *Server) handlePublicAnalysisASNs(w http.ResponseWriter, r *http.Request) {
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

	rows := readStore.ListSnapshotASNViews(snapshot.ID)
	items := make([]PublicAnalysisASNView, 0, len(rows))
	for _, row := range rows {
		items = append(items, PublicAnalysisASNView{
			ASN:             row.ASN,
			Label:           row.Label,
			DomainCount:     row.DomainCount,
			AddressCount:    row.AddressCount,
			NameserverCount: row.NameserverCount,
			PrefixCount:     row.PrefixCount,
			IPv4Count:       row.IPv4Count,
			IPv6Count:       row.IPv6Count,
		})
	}

	if filter.Search != "" {
		needle := strings.ToLower(filter.Search)
		kept := items[:0]
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Label), needle) {
				kept = append(kept, it)
				continue
			}
			// Numeric search on ASN value.
			asnStr := strings.TrimPrefix(strings.ToLower(needle), "as")
			if asnStr != "" && strings.Contains(strconv.FormatInt(it.ASN, 10), asnStr) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	sort.Slice(items, func(i, j int) bool {
		switch filter.Sort {
		case "domain_count_desc":
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount > items[j].DomainCount
			}
		case "domain_count_asc":
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount < items[j].DomainCount
			}
		case "address_count_desc":
			if items[i].AddressCount != items[j].AddressCount {
				return items[i].AddressCount > items[j].AddressCount
			}
		case "address_count_asc":
			if items[i].AddressCount != items[j].AddressCount {
				return items[i].AddressCount < items[j].AddressCount
			}
		case "nameserver_count_desc":
			if items[i].NameserverCount != items[j].NameserverCount {
				return items[i].NameserverCount > items[j].NameserverCount
			}
		case "nameserver_count_asc":
			if items[i].NameserverCount != items[j].NameserverCount {
				return items[i].NameserverCount < items[j].NameserverCount
			}
		case "prefix_count_desc":
			if items[i].PrefixCount != items[j].PrefixCount {
				return items[i].PrefixCount > items[j].PrefixCount
			}
		case "prefix_count_asc":
			if items[i].PrefixCount != items[j].PrefixCount {
				return items[i].PrefixCount < items[j].PrefixCount
			}
		}
		return items[i].ASN < items[j].ASN
	})
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisASNView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

// handlePublicAnalysisPrefixes handles GET /pub/api/v1/analysis/prefixes.
func (s *Server) handlePublicAnalysisPrefixes(w http.ResponseWriter, r *http.Request) {
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

	rows := readStore.ListSnapshotPrefixViews(snapshot.ID)

	items := make([]PublicAnalysisPrefixView, 0, len(rows))
	for _, b := range rows {
		v := PublicAnalysisPrefixView{
			Prefix:       b.Prefix,
			Family:       b.Family,
			DomainCount:  b.DomainCount,
			AddressCount: b.AddressCount,
		}
		if len(b.ASNs) == 1 {
			for _, asn := range b.ASNs {
				copy := asn
				v.ASN = &copy
				if meta, ok := readStore.GetAnalysisASN(asn); ok {
					v.ASNLabel = meta.Label
				}
			}
		}
		items = append(items, v)
	}

	if filter.Search != "" {
		needle := strings.ToLower(filter.Search)
		kept := items[:0]
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Prefix), needle) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	sort.Slice(items, func(i, j int) bool {
		switch filter.Sort {
		case "domain_count_desc":
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount > items[j].DomainCount
			}
		case "domain_count_asc":
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount < items[j].DomainCount
			}
		}
		return items[i].Prefix < items[j].Prefix
	})
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisPrefixView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}
