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

	data := s.latestMaterializationForCohort(cohort)
	endpoints := data.endpoints
	addressASNs := data.addressASNs

	addressASN := map[int64]int64{} // address_id → asn
	for _, fact := range addressASNs {
		if fact.ASN != nil {
			addressASN[fact.AddressID] = *fact.ASN
		}
	}

	type nsAgg struct {
		name       string
		domains    map[int64]struct{}
		addresses  map[int64]struct{}
		ipv4       map[int64]struct{}
		ipv6       map[int64]struct{}
		asns       map[int64]struct{}
		queryCount int
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
		// Synthetic delegation-only endpoints carry AddressID=0 (no
		// resolved address). They still tell us the NS serves this
		// domain, so keep the domain in the set above, but skip the
		// address / family / ASN / query-count tallies that only make
		// sense for real (ns, addr) pairs.
		if ep.AddressID == 0 {
			continue
		}
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
		view := PublicAnalysisNameserverView{
			Nameserver:    b.name,
			DomainCount:   len(b.domains),
			EndpointCount: len(b.addresses),
			IPv4Count:     len(b.ipv4),
			IPv6Count:     len(b.ipv6),
			ASNCount:      len(b.asns),
			QueryCount:    b.queryCount,
		}
		switch len(b.asns) {
		case 0:
			// No ASN linkage — leave Operator empty.
		case 1:
			for asn := range b.asns {
				asnCopy := asn
				view.OperatorASN = &asnCopy
				if meta, ok := readStore.GetAnalysisASN(asn); ok {
					view.Operator = meta.Label
				}
			}
		default:
			view.Operator = "Multiple"
		}
		items = append(items, view)
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

	data := s.latestMaterializationForCohort(cohort)
	endpoints := data.endpoints
	addressASNs := data.addressASNs

	// Index addressASNs by AddressID once so the per-bucket facts
	// lookup below is O(k) per bucket instead of O(M). Before this the
	// inner loop was O(N * M) and dominated wall-clock for large
	// cohorts. Same idea for prefix IDs: collect all distinct IDs and
	// resolve them in a single pass, not per-fact inside the bucket
	// loop. Replaces what used to be one GetAnalysisPrefix DB call per
	// matching fact.
	factsByAddr := map[int64][]AnalysisRunAddressASN{}
	prefixIDs := map[int64]struct{}{}
	for _, fact := range addressASNs {
		factsByAddr[fact.AddressID] = append(factsByAddr[fact.AddressID], fact)
		if fact.PrefixID != nil {
			prefixIDs[*fact.PrefixID] = struct{}{}
		}
	}
	prefixByID := make(map[int64]string, len(prefixIDs))
	for id := range prefixIDs {
		if prefix, ok := readStore.GetAnalysisPrefix(id); ok {
			prefixByID[id] = prefix.Prefix
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
		asnSet := map[int64]struct{}{}
		prefixSet := map[string]struct{}{}
		for _, fact := range factsByAddr[key.addressID] {
			if _, ok := b.domains[fact.DomainID]; !ok {
				continue
			}
			if fact.ASN != nil {
				asnSet[*fact.ASN] = struct{}{}
			}
			if fact.PrefixID != nil {
				if prefix, ok := prefixByID[*fact.PrefixID]; ok {
					prefixSet[prefix] = struct{}{}
				}
			}
		}
		if len(asnSet) == 1 {
			for asn := range asnSet {
				asnCopy := asn
				v.ASN = &asnCopy
				if meta, ok := readStore.GetAnalysisASN(asn); ok {
					v.ASNLabel = meta.Label
				}
			}
		}
		if len(prefixSet) == 1 {
			for prefix := range prefixSet {
				v.Prefix = prefix
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
	sortEndpointViews(items, filter.Sort)
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

	data := s.latestMaterializationForCohort(cohort)
	endpoints := data.endpoints
	addressASNs := data.addressASNs

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

	// Domain-level ASN aggregates (IPV4_*_ASN / IPV6_*_ASN tags without an
	// address pairing) surface here too. They only contribute domain_count
	// and the per-family presence flag — there is no address/nameserver/
	// prefix linkage to populate from these rows.
	for _, da := range data.domainASNs {
		b, exists := buckets[da.ASN]
		if !exists {
			asn, _ := readStore.GetAnalysisASN(da.ASN)
			b = &asnAgg{
				label:       asn.Label,
				domains:     map[int64]struct{}{},
				addresses:   map[int64]struct{}{},
				prefixes:    map[int64]struct{}{},
				nameservers: map[int64]struct{}{},
				ipv4:        map[int64]struct{}{},
				ipv6:        map[int64]struct{}{},
			}
			buckets[da.ASN] = b
		}
		b.domains[da.DomainID] = struct{}{}
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

	addressASNs := s.latestMaterializationForCohort(cohort).addressASNs

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
