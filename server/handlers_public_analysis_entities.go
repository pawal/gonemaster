package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// PublicAnalysisNameserverView aggregates one nameserver's footprint in the cohort.
type PublicAnalysisNameserverView struct {
	Nameserver      string `json:"nameserver"`
	DomainCount     int    `json:"domain_count"`
	EndpointCount   int    `json:"endpoint_count"`
	IPv4Count       int    `json:"ipv4_count"`
	IPv6Count       int    `json:"ipv6_count"`
	ASNCount        int    `json:"asn_count"`
	QueryCount      int    `json:"query_count,omitempty"`
}

// PublicAnalysisEndpointView is one (nameserver, address) pair in the cohort.
type PublicAnalysisEndpointView struct {
	Nameserver  string `json:"nameserver"`
	Address     string `json:"address"`
	Family      string `json:"family"`
	DomainCount int    `json:"domain_count"`
	ASN         *int64 `json:"asn,omitempty"`
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
}

// handlePublicAnalysisNameservers handles GET /pub/api/v1/analysis/nameservers.
func (s *Server) handlePublicAnalysisNameservers(w http.ResponseWriter, r *http.Request) {
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

	endpoints := readStore.ListAnalysisRunNSEndpointsByCohort(cohort.ID)
	addressASNs := readStore.ListAnalysisRunAddressASNsByCohort(cohort.ID)

	addressASN := map[int64]int64{} // address_id → asn
	for _, fact := range addressASNs {
		if fact.ASN != nil {
			addressASN[fact.AddressID] = *fact.ASN
		}
	}

	type nsAgg struct {
		name        string
		domains     map[int64]struct{}
		addresses   map[int64]struct{}
		ipv4        map[int64]struct{}
		ipv6        map[int64]struct{}
		asns        map[int64]struct{}
		queryCount  int
	}
	buckets := map[int64]*nsAgg{}
	for _, ep := range endpoints {
		b, ok := buckets[ep.NameserverID]
		if !ok {
			ns, found := readStore.GetAnalysisNameserver(ep.NameserverID)
			if !found {
				continue
			}
			b = &nsAgg{
				name:      ns.Name,
				domains:   map[int64]struct{}{},
				addresses: map[int64]struct{}{},
				ipv4:      map[int64]struct{}{},
				ipv6:      map[int64]struct{}{},
				asns:      map[int64]struct{}{},
			}
			buckets[ep.NameserverID] = b
		}
		b.domains[ep.DomainID] = struct{}{}
		b.addresses[ep.AddressID] = struct{}{}
		switch ep.Family {
		case "ipv4":
			b.ipv4[ep.AddressID] = struct{}{}
		case "ipv6":
			b.ipv6[ep.AddressID] = struct{}{}
		}
		if asn, has := addressASN[ep.AddressID]; has {
			b.asns[asn] = struct{}{}
		}
		b.queryCount += ep.QueryCount
	}

	items := make([]PublicAnalysisNameserverView, 0, len(buckets))
	for _, b := range buckets {
		items = append(items, PublicAnalysisNameserverView{
			Nameserver:    b.name,
			DomainCount:   len(b.domains),
			EndpointCount: len(b.addresses),
			IPv4Count:     len(b.ipv4),
			IPv6Count:     len(b.ipv6),
			ASNCount:      len(b.asns),
			QueryCount:    b.queryCount,
		})
	}

	items = applyNameserverFilter(items, filter)
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
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
	case "endpoint_count_desc":
		sort.Slice(items, func(i, j int) bool {
			if items[i].EndpointCount != items[j].EndpointCount {
				return items[i].EndpointCount > items[j].EndpointCount
			}
			return items[i].Nameserver < items[j].Nameserver
		})
	case "name_desc":
		sort.Slice(items, func(i, j int) bool { return items[i].Nameserver > items[j].Nameserver })
	default:
		sort.Slice(items, func(i, j int) bool { return items[i].Nameserver < items[j].Nameserver })
	}
}

// handlePublicAnalysisEndpoints handles GET /pub/api/v1/analysis/endpoints.
func (s *Server) handlePublicAnalysisEndpoints(w http.ResponseWriter, r *http.Request) {
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

	endpoints := readStore.ListAnalysisRunNSEndpointsByCohort(cohort.ID)
	addressASNs := readStore.ListAnalysisRunAddressASNsByCohort(cohort.ID)

	addressASN := map[int64]int64{}
	addressPrefix := map[int64]int64{}
	for _, fact := range addressASNs {
		if fact.ASN != nil {
			addressASN[fact.AddressID] = *fact.ASN
		}
		if fact.PrefixID != nil {
			addressPrefix[fact.AddressID] = *fact.PrefixID
		}
	}

	type endpointKey struct {
		nameserverID int64
		addressID    int64
	}
	type endpointAgg struct {
		nameserver string
		address    string
		family     string
		domains    map[int64]struct{}
	}
	buckets := map[endpointKey]*endpointAgg{}
	for _, ep := range endpoints {
		key := endpointKey{nameserverID: ep.NameserverID, addressID: ep.AddressID}
		b, exists := buckets[key]
		if !exists {
			ns, nsOk := readStore.GetAnalysisNameserver(ep.NameserverID)
			addr, addrOk := readStore.GetAnalysisAddress(ep.AddressID)
			if !nsOk || !addrOk {
				continue
			}
			b = &endpointAgg{
				nameserver: ns.Name,
				address:    addr.Address,
				family:     addr.Family,
				domains:    map[int64]struct{}{},
			}
			buckets[key] = b
		}
		b.domains[ep.DomainID] = struct{}{}
	}

	items := make([]PublicAnalysisEndpointView, 0, len(buckets))
	for key, b := range buckets {
		v := PublicAnalysisEndpointView{
			Nameserver:  b.nameserver,
			Address:     b.address,
			Family:      b.family,
			DomainCount: len(b.domains),
		}
		if asn, has := addressASN[key.addressID]; has {
			v.ASN = &asn
		}
		if prefixID, has := addressPrefix[key.addressID]; has {
			if prefix, ok := readStore.GetAnalysisPrefix(prefixID); ok {
				v.Prefix = prefix.Prefix
			}
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
	sort.Slice(items, func(i, j int) bool {
		if items[i].Nameserver != items[j].Nameserver {
			return items[i].Nameserver < items[j].Nameserver
		}
		return items[i].Address < items[j].Address
	})
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisEndpointView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

// handlePublicAnalysisASNs handles GET /pub/api/v1/analysis/asns.
func (s *Server) handlePublicAnalysisASNs(w http.ResponseWriter, r *http.Request) {
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

	endpoints := readStore.ListAnalysisRunNSEndpointsByCohort(cohort.ID)
	addressASNs := readStore.ListAnalysisRunAddressASNsByCohort(cohort.ID)

	addressInfo := map[int64]AnalysisAddress{}
	for _, fact := range addressASNs {
		if _, ok := addressInfo[fact.AddressID]; ok {
			continue
		}
		if addr, found := readStore.GetAnalysisAddress(fact.AddressID); found {
			addressInfo[fact.AddressID] = addr
		}
	}

	type asnAgg struct {
		label       string
		domains     map[int64]struct{}
		addresses   map[int64]struct{}
		prefixes    map[int64]struct{}
		nameservers map[int64]struct{}
		ipv4        map[int64]struct{}
		ipv6        map[int64]struct{}
	}
	buckets := map[int64]*asnAgg{}
	for _, fact := range addressASNs {
		if fact.ASN == nil {
			continue
		}
		b, exists := buckets[*fact.ASN]
		if !exists {
			asn, _ := readStore.GetAnalysisASN(*fact.ASN)
			b = &asnAgg{
				label:       asn.Label,
				domains:     map[int64]struct{}{},
				addresses:   map[int64]struct{}{},
				prefixes:    map[int64]struct{}{},
				nameservers: map[int64]struct{}{},
				ipv4:        map[int64]struct{}{},
				ipv6:        map[int64]struct{}{},
			}
			buckets[*fact.ASN] = b
		}
		b.domains[fact.DomainID] = struct{}{}
		b.addresses[fact.AddressID] = struct{}{}
		if fact.PrefixID != nil {
			b.prefixes[*fact.PrefixID] = struct{}{}
		}
		if addr, ok := addressInfo[fact.AddressID]; ok {
			switch addr.Family {
			case "ipv4":
				b.ipv4[fact.AddressID] = struct{}{}
			case "ipv6":
				b.ipv6[fact.AddressID] = struct{}{}
			}
		}
	}
	addressToNameservers := map[int64]map[int64]struct{}{}
	for _, ep := range endpoints {
		set, ok := addressToNameservers[ep.AddressID]
		if !ok {
			set = map[int64]struct{}{}
			addressToNameservers[ep.AddressID] = set
		}
		set[ep.NameserverID] = struct{}{}
	}
	for asn, b := range buckets {
		for addrID := range b.addresses {
			for nsID := range addressToNameservers[addrID] {
				b.nameservers[nsID] = struct{}{}
			}
		}
		_ = asn
	}

	items := make([]PublicAnalysisASNView, 0, len(buckets))
	for asn, b := range buckets {
		items = append(items, PublicAnalysisASNView{
			ASN:             asn,
			Label:           b.label,
			DomainCount:     len(b.domains),
			AddressCount:    len(b.addresses),
			NameserverCount: len(b.nameservers),
			PrefixCount:     len(b.prefixes),
			IPv4Count:       len(b.ipv4),
			IPv6Count:       len(b.ipv6),
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
		case "address_count_desc":
			if items[i].AddressCount != items[j].AddressCount {
				return items[i].AddressCount > items[j].AddressCount
			}
		}
		return items[i].ASN < items[j].ASN
	})
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisASNView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

// handlePublicAnalysisPrefixes handles GET /pub/api/v1/analysis/prefixes.
func (s *Server) handlePublicAnalysisPrefixes(w http.ResponseWriter, r *http.Request) {
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

	addressASNs := readStore.ListAnalysisRunAddressASNsByCohort(cohort.ID)

	type prefixAgg struct {
		prefix    string
		family    string
		domains   map[int64]struct{}
		addresses map[int64]struct{}
		asns      map[int64]struct{}
	}
	buckets := map[int64]*prefixAgg{}
	for _, fact := range addressASNs {
		if fact.PrefixID == nil {
			continue
		}
		b, exists := buckets[*fact.PrefixID]
		if !exists {
			prefix, found := readStore.GetAnalysisPrefix(*fact.PrefixID)
			if !found {
				continue
			}
			b = &prefixAgg{
				prefix:    prefix.Prefix,
				family:    prefix.Family,
				domains:   map[int64]struct{}{},
				addresses: map[int64]struct{}{},
				asns:      map[int64]struct{}{},
			}
			buckets[*fact.PrefixID] = b
		}
		b.domains[fact.DomainID] = struct{}{}
		b.addresses[fact.AddressID] = struct{}{}
		if fact.ASN != nil {
			b.asns[*fact.ASN] = struct{}{}
		}
	}

	items := make([]PublicAnalysisPrefixView, 0, len(buckets))
	for _, b := range buckets {
		v := PublicAnalysisPrefixView{
			Prefix:       b.prefix,
			Family:       b.family,
			DomainCount:  len(b.domains),
			AddressCount: len(b.addresses),
		}
		if len(b.asns) == 1 {
			for asn := range b.asns {
				copy := asn
				v.ASN = &copy
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
		if filter.Sort == "domain_count_desc" && items[i].DomainCount != items[j].DomainCount {
			return items[i].DomainCount > items[j].DomainCount
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

