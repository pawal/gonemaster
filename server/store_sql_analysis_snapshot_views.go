package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
)

// SnapshotEntityViews is the bundle of three per-snapshot view-row sets
// computed at capture time and written together by ReplaceSnapshotEntityViews.
type SnapshotEntityViews struct {
	Nameservers []AnalysisSnapshotNameserverView
	Endpoints   []AnalysisSnapshotEndpointView
	ASNs        []AnalysisSnapshotASNView
}

// ComputeSnapshotEntityViews builds the three entity-view row sets for one
// (cohort, batch) snapshot from the underlying fact tables.
func (s *SQLJobStore) ComputeSnapshotEntityViews(cohortID int64, batchID string) (SnapshotEntityViews, error) {
	if batchID == "" {
		return SnapshotEntityViews{}, fmt.Errorf("compute snapshot entity views: batch_id is required")
	}
	endpoints, err := s.queryBatchEndpoints(cohortID, batchID)
	if err != nil {
		return SnapshotEntityViews{}, err
	}
	addrFacts, err := s.queryBatchAddressASNs(cohortID, batchID)
	if err != nil {
		return SnapshotEntityViews{}, err
	}
	domainASNs, err := s.queryBatchDomainASNs(cohortID, batchID)
	if err != nil {
		return SnapshotEntityViews{}, err
	}
	prefixByID, err := s.collectPrefixesByID(addrFacts)
	if err != nil {
		return SnapshotEntityViews{}, err
	}
	asnByID, err := s.collectASNsByID(addrFacts, domainASNs)
	if err != nil {
		return SnapshotEntityViews{}, err
	}
	domainNames := s.collectDomainNames(endpoints, addrFacts, domainASNs)
	return SnapshotEntityViews{
		Nameservers: buildNameserverViews(endpoints, addrFacts, asnByID, domainNames),
		Endpoints:   buildEndpointViews(endpoints, addrFacts, asnByID, prefixByID),
		ASNs:        buildASNViews(endpoints, addrFacts, domainASNs, asnByID),
	}, nil
}

// marshalStringList encodes nil as `[]` so the column never holds NULL.
func marshalStringList(in []string) (string, error) {
	if in == nil {
		return "[]", nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// marshalInt64List encodes nil as `[]` so the column never holds NULL.
func marshalInt64List(in []int64) (string, error) {
	if in == nil {
		return "[]", nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalStringList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalInt64List(raw string) []int64 {
	if raw == "" {
		return nil
	}
	var out []int64
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// collectDomainNames batches the domain id→name lookup for every
// distinct domain referenced by the snapshot's facts.
func (s *SQLJobStore) collectDomainNames(endpoints []batchEndpointRow, addrFacts []AnalysisRunAddressASN, domainASNs []AnalysisRunDomainASN) map[int64]string {
	idSet := map[int64]struct{}{}
	for _, ep := range endpoints {
		idSet[ep.DomainID] = struct{}{}
	}
	for _, f := range addrFacts {
		idSet[f.DomainID] = struct{}{}
	}
	for _, d := range domainASNs {
		idSet[d.DomainID] = struct{}{}
	}
	ids := make([]int64, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	return s.GetDomainNamesByIDs(ids)
}

type batchEndpointRow struct {
	NameserverID   int64
	AddressID      int64
	DomainID       int64
	NameserverName string
	Address        string
	Family         string
	QueryCount     int
}

func (s *SQLJobStore) queryBatchEndpoints(cohortID int64, batchID string) ([]batchEndpointRow, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT e.nameserver_id, e.address_id, e.domain_id,
				COALESCE(n.name, ''),
				COALESCE(a.address, ''),
				COALESCE(a.family, e.family),
				e.query_count
			FROM analysis_run_ns_endpoints e
			JOIN runs r ON r.id = e.run_id
			LEFT JOIN analysis_nameservers n ON n.id = e.nameserver_id
			LEFT JOIN analysis_addresses a ON a.id = e.address_id
			WHERE e.cohort_id = %s AND r.batch_id = %s AND e.role <> 'parent'`,
			s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("query batch endpoints: %w", err)
	}
	defer rows.Close()

	var out []batchEndpointRow
	for rows.Next() {
		var row batchEndpointRow
		if err := rows.Scan(
			&row.NameserverID, &row.AddressID, &row.DomainID,
			&row.NameserverName, &row.Address, &row.Family, &row.QueryCount,
		); err != nil {
			return nil, fmt.Errorf("scan batch endpoint: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *SQLJobStore) queryBatchAddressASNs(cohortID int64, batchID string) ([]AnalysisRunAddressASN, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT da.cohort_id, da.run_id, da.domain_id, da.address_id,
				da.prefix_id, da.asn, da.lookup_status, da.source
			FROM analysis_run_address_asns da
			JOIN runs r ON r.id = da.run_id
			WHERE da.cohort_id = %s AND r.batch_id = %s`,
			s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("query batch address asns: %w", err)
	}
	defer rows.Close()

	var out []AnalysisRunAddressASN
	for rows.Next() {
		var (
			fact     AnalysisRunAddressASN
			prefixID sql.NullInt64
			asn      sql.NullInt64
		)
		if err := rows.Scan(
			&fact.CohortID, &fact.RunID, &fact.DomainID, &fact.AddressID,
			&prefixID, &asn, &fact.LookupStatus, &fact.Source,
		); err != nil {
			return nil, fmt.Errorf("scan batch address asn: %w", err)
		}
		fact.PrefixID = nullInt64Ptr(prefixID)
		fact.ASN = nullInt64Ptr(asn)
		out = append(out, fact)
	}
	return out, rows.Err()
}

func (s *SQLJobStore) queryBatchDomainASNs(cohortID int64, batchID string) ([]AnalysisRunDomainASN, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT da.cohort_id, da.run_id, da.domain_id, da.asn, da.family, da.source
			FROM analysis_run_domain_asns da
			JOIN runs r ON r.id = da.run_id
			WHERE da.cohort_id = %s AND r.batch_id = %s`,
			s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("query batch domain asns: %w", err)
	}
	defer rows.Close()

	var out []AnalysisRunDomainASN
	for rows.Next() {
		var fact AnalysisRunDomainASN
		if err := rows.Scan(
			&fact.CohortID, &fact.RunID, &fact.DomainID,
			&fact.ASN, &fact.Family, &fact.Source,
		); err != nil {
			return nil, fmt.Errorf("scan batch domain asn: %w", err)
		}
		out = append(out, fact)
	}
	return out, rows.Err()
}

func (s *SQLJobStore) collectPrefixesByID(facts []AnalysisRunAddressASN) (map[int64]string, error) {
	idSet := map[int64]struct{}{}
	for _, f := range facts {
		if f.PrefixID != nil {
			idSet[*f.PrefixID] = struct{}{}
		}
	}
	out := make(map[int64]string, len(idSet))
	for id := range idSet {
		if p, ok := s.GetAnalysisPrefix(id); ok {
			out[id] = p.Prefix
		}
	}
	return out, nil
}

func (s *SQLJobStore) collectASNsByID(facts []AnalysisRunAddressASN, domainASNs []AnalysisRunDomainASN) (map[int64]string, error) {
	asnSet := map[int64]struct{}{}
	for _, f := range facts {
		if f.ASN != nil {
			asnSet[*f.ASN] = struct{}{}
		}
	}
	for _, d := range domainASNs {
		asnSet[d.ASN] = struct{}{}
	}
	out := make(map[int64]string, len(asnSet))
	for asn := range asnSet {
		if meta, ok := s.GetAnalysisASN(asn); ok {
			out[asn] = meta.Label
		}
	}
	return out, nil
}

func buildNameserverViews(endpoints []batchEndpointRow, addrFacts []AnalysisRunAddressASN, asnByID map[int64]string, domainNames map[int64]string) []AnalysisSnapshotNameserverView {
	addrASN := map[int64]int64{}
	for _, f := range addrFacts {
		if f.ASN != nil {
			addrASN[f.AddressID] = *f.ASN
		}
	}
	type bucket struct {
		name           string
		domains        map[int64]struct{}
		addresses      map[int64]struct{}
		ipv4           map[int64]struct{}
		ipv6           map[int64]struct{}
		asns           map[int64]struct{}
		addressLiteral map[int64]string
		queryCount     int
	}
	buckets := map[int64]*bucket{}
	for _, ep := range endpoints {
		b, ok := buckets[ep.NameserverID]
		if !ok {
			b = &bucket{
				name:           ep.NameserverName,
				domains:        map[int64]struct{}{},
				addresses:      map[int64]struct{}{},
				ipv4:           map[int64]struct{}{},
				ipv6:           map[int64]struct{}{},
				asns:           map[int64]struct{}{},
				addressLiteral: map[int64]string{},
			}
			buckets[ep.NameserverID] = b
		}
		b.domains[ep.DomainID] = struct{}{}
		if ep.AddressID == 0 {
			continue
		}
		b.addresses[ep.AddressID] = struct{}{}
		if ep.Address != "" {
			b.addressLiteral[ep.AddressID] = ep.Address
		}
		switch ep.Family {
		case "ipv4":
			b.ipv4[ep.AddressID] = struct{}{}
		case "ipv6":
			b.ipv6[ep.AddressID] = struct{}{}
		}
		if asn, has := addrASN[ep.AddressID]; has {
			b.asns[asn] = struct{}{}
		}
		b.queryCount += ep.QueryCount
	}
	out := make([]AnalysisSnapshotNameserverView, 0, len(buckets))
	for nsID, b := range buckets {
		view := AnalysisSnapshotNameserverView{
			NameserverID:   nsID,
			NameserverName: b.name,
			DomainCount:    len(b.domains),
			EndpointCount:  len(b.addresses),
			IPv4Count:      len(b.ipv4),
			IPv6Count:      len(b.ipv6),
			ASNCount:       len(b.asns),
			QueryCount:     b.queryCount,
		}
		switch len(b.asns) {
		case 0:
		case 1:
			for asn := range b.asns {
				asnCopy := asn
				view.OperatorASN = &asnCopy
				view.Operator = asnByID[asn]
			}
		default:
			view.Operator = "Multiple"
		}

		addresses := make([]string, 0, len(b.addressLiteral))
		for _, addr := range b.addressLiteral {
			addresses = append(addresses, addr)
		}
		sort.Strings(addresses)
		view.Addresses = addresses

		asns := make([]int64, 0, len(b.asns))
		for asn := range b.asns {
			asns = append(asns, asn)
		}
		sort.Slice(asns, func(i, j int) bool { return asns[i] < asns[j] })
		view.ASNs = asns

		domains := make([]string, 0, len(b.domains))
		for id := range b.domains {
			if name, ok := domainNames[id]; ok && name != "" {
				domains = append(domains, name)
			}
		}
		sort.Strings(domains)
		view.Domains = domains

		out = append(out, view)
	}
	return out
}

func buildEndpointViews(endpoints []batchEndpointRow, addrFacts []AnalysisRunAddressASN, asnByID map[int64]string, prefixByID map[int64]string) []AnalysisSnapshotEndpointView {
	factsByAddr := map[int64][]AnalysisRunAddressASN{}
	for _, f := range addrFacts {
		factsByAddr[f.AddressID] = append(factsByAddr[f.AddressID], f)
	}
	type key struct {
		nsID   int64
		addrID int64
	}
	type bucket struct {
		name    string
		address string
		family  string
		domains map[int64]struct{}
	}
	buckets := map[key]*bucket{}
	for _, ep := range endpoints {
		if ep.AddressID == 0 {
			continue
		}
		k := key{nsID: ep.NameserverID, addrID: ep.AddressID}
		b, ok := buckets[k]
		if !ok {
			b = &bucket{
				name:    ep.NameserverName,
				address: ep.Address,
				family:  ep.Family,
				domains: map[int64]struct{}{},
			}
			buckets[k] = b
		}
		b.domains[ep.DomainID] = struct{}{}
	}
	out := make([]AnalysisSnapshotEndpointView, 0, len(buckets))
	for k, b := range buckets {
		v := AnalysisSnapshotEndpointView{
			NameserverID:   k.nsID,
			AddressID:      k.addrID,
			NameserverName: b.name,
			Address:        b.address,
			Family:         b.family,
			DomainCount:    len(b.domains),
		}
		asnSet := map[int64]struct{}{}
		prefixSet := map[string]struct{}{}
		for _, f := range factsByAddr[k.addrID] {
			if _, has := b.domains[f.DomainID]; !has {
				continue
			}
			if f.ASN != nil {
				asnSet[*f.ASN] = struct{}{}
			}
			if f.PrefixID != nil {
				if p, ok := prefixByID[*f.PrefixID]; ok {
					prefixSet[p] = struct{}{}
				}
			}
		}
		if len(asnSet) == 1 {
			for asn := range asnSet {
				asnCopy := asn
				v.ASN = &asnCopy
				v.ASNLabel = asnByID[asn]
			}
		}
		if len(prefixSet) == 1 {
			for p := range prefixSet {
				v.Prefix = p
			}
		}
		out = append(out, v)
	}
	return out
}

func buildASNViews(endpoints []batchEndpointRow, addrFacts []AnalysisRunAddressASN, domainASNs []AnalysisRunDomainASN, asnByID map[int64]string) []AnalysisSnapshotASNView {
	addrFamily := map[int64]string{}
	for _, ep := range endpoints {
		if ep.AddressID != 0 && ep.Family != "" {
			addrFamily[ep.AddressID] = ep.Family
		}
	}
	addrToNS := map[int64]map[int64]struct{}{}
	for _, ep := range endpoints {
		if ep.AddressID == 0 {
			continue
		}
		set, ok := addrToNS[ep.AddressID]
		if !ok {
			set = map[int64]struct{}{}
			addrToNS[ep.AddressID] = set
		}
		set[ep.NameserverID] = struct{}{}
	}
	type bucket struct {
		domains     map[int64]struct{}
		addresses   map[int64]struct{}
		prefixes    map[int64]struct{}
		nameservers map[int64]struct{}
		ipv4        map[int64]struct{}
		ipv6        map[int64]struct{}
	}
	newBucket := func() *bucket {
		return &bucket{
			domains:     map[int64]struct{}{},
			addresses:   map[int64]struct{}{},
			prefixes:    map[int64]struct{}{},
			nameservers: map[int64]struct{}{},
			ipv4:        map[int64]struct{}{},
			ipv6:        map[int64]struct{}{},
		}
	}
	buckets := map[int64]*bucket{}
	for _, f := range addrFacts {
		if f.ASN == nil {
			continue
		}
		b, ok := buckets[*f.ASN]
		if !ok {
			b = newBucket()
			buckets[*f.ASN] = b
		}
		b.domains[f.DomainID] = struct{}{}
		b.addresses[f.AddressID] = struct{}{}
		if f.PrefixID != nil {
			b.prefixes[*f.PrefixID] = struct{}{}
		}
		switch addrFamily[f.AddressID] {
		case "ipv4":
			b.ipv4[f.AddressID] = struct{}{}
		case "ipv6":
			b.ipv6[f.AddressID] = struct{}{}
		}
	}
	for _, b := range buckets {
		for addrID := range b.addresses {
			for nsID := range addrToNS[addrID] {
				b.nameservers[nsID] = struct{}{}
			}
		}
	}
	for _, d := range domainASNs {
		b, ok := buckets[d.ASN]
		if !ok {
			b = newBucket()
			buckets[d.ASN] = b
		}
		b.domains[d.DomainID] = struct{}{}
	}
	out := make([]AnalysisSnapshotASNView, 0, len(buckets))
	for asn, b := range buckets {
		out = append(out, AnalysisSnapshotASNView{
			ASN:             asn,
			Label:           asnByID[asn],
			DomainCount:     len(b.domains),
			AddressCount:    len(b.addresses),
			NameserverCount: len(b.nameservers),
			PrefixCount:     len(b.prefixes),
			IPv4Count:       len(b.ipv4),
			IPv6Count:       len(b.ipv6),
		})
	}
	return out
}

// ReplaceSnapshotEntityViews atomically swaps the three view-row sets for
// one snapshot. All-or-nothing; idempotent on rerun.
func (s *SQLJobStore) ReplaceSnapshotEntityViews(snapshotID int64, views SnapshotEntityViews) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace snapshot views: %w", err)
	}
	for _, table := range []string{
		"analysis_snapshot_nameserver_view",
		"analysis_snapshot_endpoint_view",
		"analysis_snapshot_asn_view",
	} {
		if _, err := tx.Exec(
			fmt.Sprintf(`DELETE FROM %s WHERE snapshot_id = %s`, table, s.ph(1)),
			snapshotID,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("clear %s for snapshot %d: %w", table, snapshotID, err)
		}
	}
	for _, v := range views.Nameservers {
		addressesJSON, err := marshalStringList(v.Addresses)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal nameserver addresses ns=%d: %w", v.NameserverID, err)
		}
		asnsJSON, err := marshalInt64List(v.ASNs)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal nameserver asns ns=%d: %w", v.NameserverID, err)
		}
		domainsJSON, err := marshalStringList(v.Domains)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal nameserver domains ns=%d: %w", v.NameserverID, err)
		}
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_snapshot_nameserver_view
				(snapshot_id, nameserver_id, nameserver_name, domain_count, endpoint_count,
				 ipv4_count, ipv6_count, asn_count, operator, operator_asn, query_count,
				 addresses_json, asns_json, domains_json)
				VALUES (%s)`, s.phRange(1, 14)),
			snapshotID, v.NameserverID, v.NameserverName, v.DomainCount, v.EndpointCount,
			v.IPv4Count, v.IPv6Count, v.ASNCount, v.Operator, nullInt64Value(v.OperatorASN), v.QueryCount,
			addressesJSON, asnsJSON, domainsJSON,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert nameserver view ns=%d: %w", v.NameserverID, err)
		}
	}
	for _, v := range views.Endpoints {
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_snapshot_endpoint_view
				(snapshot_id, nameserver_id, address_id, nameserver_name, address, family,
				 domain_count, asn, asn_label, prefix)
				VALUES (%s)`, s.phRange(1, 10)),
			snapshotID, v.NameserverID, v.AddressID, v.NameserverName, v.Address, v.Family,
			v.DomainCount, nullInt64Value(v.ASN), v.ASNLabel, v.Prefix,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert endpoint view ns=%d addr=%d: %w", v.NameserverID, v.AddressID, err)
		}
	}
	for _, v := range views.ASNs {
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_snapshot_asn_view
				(snapshot_id, asn, label, domain_count, address_count, nameserver_count,
				 prefix_count, ipv4_count, ipv6_count)
				VALUES (%s)`, s.phRange(1, 9)),
			snapshotID, v.ASN, v.Label, v.DomainCount, v.AddressCount, v.NameserverCount,
			v.PrefixCount, v.IPv4Count, v.IPv6Count,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert asn view asn=%d: %w", v.ASN, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace snapshot views: %w", err)
	}
	return nil
}

const analysisSnapshotNameserverViewCols = `snapshot_id, nameserver_id, nameserver_name,
	domain_count, endpoint_count, ipv4_count, ipv6_count, asn_count,
	operator, operator_asn, query_count, addresses_json, asns_json, domains_json`

func scanSnapshotNameserverView(row rowScanner) (AnalysisSnapshotNameserverView, error) {
	var (
		v             AnalysisSnapshotNameserverView
		operatorASN   sql.NullInt64
		addressesJSON string
		asnsJSON      string
		domainsJSON   string
	)
	if err := row.Scan(
		&v.SnapshotID, &v.NameserverID, &v.NameserverName,
		&v.DomainCount, &v.EndpointCount, &v.IPv4Count, &v.IPv6Count, &v.ASNCount,
		&v.Operator, &operatorASN, &v.QueryCount,
		&addressesJSON, &asnsJSON, &domainsJSON,
	); err != nil {
		return AnalysisSnapshotNameserverView{}, err
	}
	v.OperatorASN = nullInt64Ptr(operatorASN)
	v.Addresses = unmarshalStringList(addressesJSON)
	v.ASNs = unmarshalInt64List(asnsJSON)
	v.Domains = unmarshalStringList(domainsJSON)
	return v, nil
}

// ListSnapshotNameserverViews returns every nameserver view row for one
// snapshot, ordered by (domain_count desc, name asc, id asc) for stability.
func (s *SQLJobStore) ListSnapshotNameserverViews(snapshotID int64) []AnalysisSnapshotNameserverView {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_nameserver_view
			WHERE snapshot_id = %s
			ORDER BY domain_count DESC, nameserver_name ASC, nameserver_id ASC`,
			analysisSnapshotNameserverViewCols, s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotNameserverView
	for rows.Next() {
		v, err := scanSnapshotNameserverView(rows)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// GetSnapshotNameserverViewByName looks up a single row by case-insensitive
// nameserver name. Used by the per-nameserver detail page so the handler
// is one indexed read instead of a cohort-wide fact load.
func (s *SQLJobStore) GetSnapshotNameserverViewByName(snapshotID int64, name string) (AnalysisSnapshotNameserverView, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_nameserver_view
			WHERE snapshot_id = %s AND LOWER(nameserver_name) = LOWER(%s)
			LIMIT 1`,
			analysisSnapshotNameserverViewCols, s.ph(1), s.ph(2)),
		snapshotID, name,
	)
	v, err := scanSnapshotNameserverView(row)
	if err != nil {
		return AnalysisSnapshotNameserverView{}, false
	}
	return v, true
}

// ListSnapshotEndpointViews returns every endpoint view row for one snapshot.
func (s *SQLJobStore) ListSnapshotEndpointViews(snapshotID int64) []AnalysisSnapshotEndpointView {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT snapshot_id, nameserver_id, address_id, nameserver_name,
				address, family, domain_count, asn, asn_label, prefix
			FROM analysis_snapshot_endpoint_view
			WHERE snapshot_id = %s
			ORDER BY domain_count DESC, nameserver_name ASC, address ASC`,
			s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotEndpointView
	for rows.Next() {
		var (
			v   AnalysisSnapshotEndpointView
			asn sql.NullInt64
		)
		if err := rows.Scan(
			&v.SnapshotID, &v.NameserverID, &v.AddressID, &v.NameserverName,
			&v.Address, &v.Family, &v.DomainCount, &asn, &v.ASNLabel, &v.Prefix,
		); err != nil {
			return nil
		}
		v.ASN = nullInt64Ptr(asn)
		out = append(out, v)
	}
	return out
}

// ListSnapshotASNViews returns every ASN view row for one snapshot.
func (s *SQLJobStore) ListSnapshotASNViews(snapshotID int64) []AnalysisSnapshotASNView {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT snapshot_id, asn, label, domain_count, address_count,
				nameserver_count, prefix_count, ipv4_count, ipv6_count
			FROM analysis_snapshot_asn_view
			WHERE snapshot_id = %s
			ORDER BY domain_count DESC, asn ASC`,
			s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotASNView
	for rows.Next() {
		var v AnalysisSnapshotASNView
		if err := rows.Scan(
			&v.SnapshotID, &v.ASN, &v.Label, &v.DomainCount, &v.AddressCount,
			&v.NameserverCount, &v.PrefixCount, &v.IPv4Count, &v.IPv6Count,
		); err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}
