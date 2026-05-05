package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// SnapshotEntityViews is the bundle of per-snapshot view-row sets computed
// at capture time and written together by ReplaceSnapshotEntityViews.
type SnapshotEntityViews struct {
	Nameservers []AnalysisSnapshotNameserverView
	Endpoints   []AnalysisSnapshotEndpointView
	ASNs        []AnalysisSnapshotASNView
	Tags        []AnalysisSnapshotTagView
	Domains     []AnalysisSnapshotDomainView
	Prefixes    []AnalysisSnapshotPrefixView
}

// ComputeSnapshotEntityViews builds the three entity-view row sets for one
// (cohort, batch) snapshot from the underlying fact tables. minLevel is the
// effective tag-view floor; an empty or invalid value falls back to the
// server-wide default.
func (s *SQLJobStore) ComputeSnapshotEntityViews(cohortID int64, batchID string, minLevel string) (SnapshotEntityViews, error) {
	if batchID == "" {
		return SnapshotEntityViews{}, fmt.Errorf("compute snapshot entity views: batch_id is required")
	}
	floor := s.tagViewMinLevel
	if IsValidTagViewMinLevel(minLevel) {
		floor = strings.ToUpper(strings.TrimSpace(minLevel))
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
	tagSummaries, err := s.queryBatchTagSummaries(cohortID, batchID)
	if err != nil {
		return SnapshotEntityViews{}, err
	}
	summaries, finishedAt, err := s.queryBatchDomainSummaries(cohortID, batchID)
	if err != nil {
		return SnapshotEntityViews{}, err
	}
	domainNames := s.collectDomainNames(endpoints, addrFacts, domainASNs, tagSummaries)
	for _, sm := range summaries {
		if _, ok := domainNames[sm.DomainID]; !ok {
			domainNames = mergeDomainNames(domainNames, s.GetDomainNamesByIDs([]int64{sm.DomainID}))
		}
	}
	nameserverNames := s.collectNameserverNames(endpoints)
	addressLiterals := s.collectAddressLiterals(endpoints, addrFacts)
	return SnapshotEntityViews{
		Nameservers: buildNameserverViews(endpoints, addrFacts, asnByID, domainNames),
		Endpoints:   buildEndpointViews(endpoints, addrFacts, asnByID, prefixByID, domainNames),
		ASNs:        buildASNViews(endpoints, addrFacts, domainASNs, asnByID, prefixByID, domainNames, nameserverNames),
		Tags:        buildTagViews(tagSummaries, domainNames, floor),
		Domains:     buildDomainViews(summaries, finishedAt, endpoints, addrFacts, asnByID, prefixByID, domainNames, tagSummaries, floor),
		Prefixes:    buildPrefixViews(addrFacts, prefixByID, asnByID, domainNames, addressLiterals),
	}, nil
}

func (s *SQLJobStore) collectNameserverNames(endpoints []batchEndpointRow) map[int64]string {
	out := map[int64]string{}
	for _, ep := range endpoints {
		if ep.NameserverName != "" {
			out[ep.NameserverID] = ep.NameserverName
		}
	}
	return out
}

func (s *SQLJobStore) collectAddressLiterals(endpoints []batchEndpointRow, addrFacts []AnalysisRunAddressASN) map[int64]string {
	out := map[int64]string{}
	for _, ep := range endpoints {
		if ep.AddressID != 0 && ep.Address != "" {
			out[ep.AddressID] = ep.Address
		}
	}
	for _, f := range addrFacts {
		if _, ok := out[f.AddressID]; ok {
			continue
		}
		if addr, found := s.GetAnalysisAddress(f.AddressID); found {
			out[f.AddressID] = addr.Address
		}
	}
	return out
}

func mergeDomainNames(base, extra map[int64]string) map[int64]string {
	for id, name := range extra {
		if _, ok := base[id]; !ok {
			base[id] = name
		}
	}
	return base
}

// queryBatchDomainSummaries returns the per-(run, domain) summary rows
// joined to runs in the snapshot's batch, plus a run_id -> finished_at
// map so the per-domain detail page can surface the run timestamp.
func (s *SQLJobStore) queryBatchDomainSummaries(cohortID int64, batchID string) ([]AnalysisRunDomainSummary, map[string]time.Time, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT s.cohort_id, s.run_id, s.domain_id, s.score, s.grade,
				s.nameserver_count, s.endpoint_count, s.asn_count, s.prefix_count,
				s.worst_level, r.finished_at
			FROM analysis_run_domain_summary s
			JOIN runs r ON r.id = s.run_id
			WHERE s.cohort_id = %s AND r.batch_id = %s`,
			s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query batch domain summaries: %w", err)
	}
	defer rows.Close()

	var (
		out      []AnalysisRunDomainSummary
		finished = map[string]time.Time{}
	)
	for rows.Next() {
		var (
			sm       AnalysisRunDomainSummary
			score    sql.NullInt64
			grade    sql.NullString
			finishTS sql.NullString
		)
		if err := rows.Scan(
			&sm.CohortID, &sm.RunID, &sm.DomainID, &score, &grade,
			&sm.NameserverCount, &sm.EndpointCount, &sm.ASNCount, &sm.PrefixCount,
			&sm.WorstLevel, &finishTS,
		); err != nil {
			return nil, nil, fmt.Errorf("scan batch domain summary: %w", err)
		}
		if score.Valid {
			v := int(score.Int64)
			sm.Score = &v
		}
		if grade.Valid {
			g := grade.String
			sm.Grade = &g
		}
		out = append(out, sm)
		if t := parseTimestampNullStr(finishTS); !t.IsZero() {
			finished[sm.RunID] = t
		}
	}
	return out, finished, rows.Err()
}

// queryBatchTagSummaries returns every analysis_run_tag_summary row joined
// to runs in the snapshot's batch.
func (s *SQLJobStore) queryBatchTagSummaries(cohortID int64, batchID string) ([]AnalysisRunTagSummary, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT t.cohort_id, t.run_id, t.domain_id, t.tag, t.module, t.testcase, t.level, t.occurrence_count
			FROM analysis_run_tag_summary t
			JOIN runs r ON r.id = t.run_id
			WHERE t.cohort_id = %s AND r.batch_id = %s`,
			s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("query batch tag summaries: %w", err)
	}
	defer rows.Close()

	var out []AnalysisRunTagSummary
	for rows.Next() {
		var t AnalysisRunTagSummary
		if err := rows.Scan(
			&t.CohortID, &t.RunID, &t.DomainID, &t.Tag, &t.Module, &t.Testcase, &t.Level, &t.OccurrenceCount,
		); err != nil {
			return nil, fmt.Errorf("scan batch tag summary: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// buildTagViews collapses per-(run, domain) tag summaries into one row per
// tag for the snapshot. Tags whose worst level falls below minLevel are
// dropped - INFO/NOTICE-only chatter never gets a detail page.
func buildTagViews(rows []AnalysisRunTagSummary, domainNames map[int64]string, minLevel string) []AnalysisSnapshotTagView {
	floor := severityRank(minLevel)
	type bucket struct {
		module      string
		testcase    string
		level       string
		domains     map[int64]struct{}
		occurrences int
	}
	buckets := map[string]*bucket{}
	for _, ts := range rows {
		tag := ts.Tag
		if tag == "" {
			continue
		}
		b, ok := buckets[tag]
		if !ok {
			b = &bucket{
				module:   ts.Module,
				testcase: ts.Testcase,
				level:    ts.Level,
				domains:  map[int64]struct{}{},
			}
			buckets[tag] = b
		}
		if severityRank(ts.Level) > severityRank(b.level) {
			b.level = ts.Level
		}
		if b.module == "" && ts.Module != "" {
			b.module = ts.Module
		}
		if b.testcase == "" && ts.Testcase != "" {
			b.testcase = ts.Testcase
		}
		b.domains[ts.DomainID] = struct{}{}
		b.occurrences += ts.OccurrenceCount
	}
	out := make([]AnalysisSnapshotTagView, 0, len(buckets))
	for tag, b := range buckets {
		if severityRank(b.level) < floor {
			continue
		}
		domains := make([]string, 0, len(b.domains))
		for id := range b.domains {
			if name, ok := domainNames[id]; ok && name != "" {
				domains = append(domains, name)
			}
		}
		sort.Strings(domains)
		out = append(out, AnalysisSnapshotTagView{
			Tag:             tag,
			Module:          b.module,
			Testcase:        b.testcase,
			Level:           b.level,
			DomainCount:     len(b.domains),
			OccurrenceCount: b.occurrences,
			Domains:         domains,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return out
}

// buildDomainViews builds one row per cohort domain in the snapshot's
// batch, baking the per-NS-address roster, flat addresses list, and
// per-domain tag list (filtered by minLevel) as JSON columns.
func buildDomainViews(
	summaries []AnalysisRunDomainSummary,
	finishedAt map[string]time.Time,
	endpoints []batchEndpointRow,
	addrFacts []AnalysisRunAddressASN,
	asnByID map[int64]string,
	prefixByID map[int64]string,
	domainNames map[int64]string,
	tagSummaries []AnalysisRunTagSummary,
	minLevel string,
) []AnalysisSnapshotDomainView {
	floor := severityRank(minLevel)

	endpointsByDomain := map[int64][]batchEndpointRow{}
	for _, ep := range endpoints {
		endpointsByDomain[ep.DomainID] = append(endpointsByDomain[ep.DomainID], ep)
	}
	addrFactsByDomain := map[int64][]AnalysisRunAddressASN{}
	for _, f := range addrFacts {
		addrFactsByDomain[f.DomainID] = append(addrFactsByDomain[f.DomainID], f)
	}
	tagsByDomain := map[int64][]AnalysisRunTagSummary{}
	for _, t := range tagSummaries {
		tagsByDomain[t.DomainID] = append(tagsByDomain[t.DomainID], t)
	}

	// Collapse to one summary per domain, preferring the run with the
	// freshest finished_at. A snapshot batch normally has one run per
	// domain, but historical data may have reprojected duplicates.
	latestByDomain := map[int64]AnalysisRunDomainSummary{}
	for _, sm := range summaries {
		existing, seen := latestByDomain[sm.DomainID]
		if !seen {
			latestByDomain[sm.DomainID] = sm
			continue
		}
		newAt := finishedAt[sm.RunID]
		oldAt := finishedAt[existing.RunID]
		if newAt.After(oldAt) {
			latestByDomain[sm.DomainID] = sm
		}
	}

	out := make([]AnalysisSnapshotDomainView, 0, len(latestByDomain))
	for _, sm := range latestByDomain {
		v := AnalysisSnapshotDomainView{
			DomainID:        sm.DomainID,
			DomainName:      domainNames[sm.DomainID],
			Score:           sm.Score,
			WorstLevel:      sm.WorstLevel,
			NameserverCount: sm.NameserverCount,
			EndpointCount:   sm.EndpointCount,
			ASNCount:        sm.ASNCount,
			PrefixCount:     sm.PrefixCount,
		}
		if sm.Grade != nil {
			v.Grade = *sm.Grade
		}
		if t, ok := finishedAt[sm.RunID]; ok && !t.IsZero() {
			ft := t
			v.FinishedAt = &ft
		}

		nsViews, addrViews := buildDomainNSAndAddresses(
			endpointsByDomain[sm.DomainID],
			addrFactsByDomain[sm.DomainID],
			asnByID, prefixByID,
		)
		v.Nameservers = nsViews
		v.Addresses = addrViews
		v.Tags = buildDomainTags(tagsByDomain[sm.DomainID], floor)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DomainName < out[j].DomainName })
	return out
}

// buildDomainNSAndAddresses returns the per-NS roster and the flat
// deduplicated address list for one domain's endpoints + address facts.
// Per-NS status is "unresolved" when the projector recorded the NS with
// AddressID=0 (no resolved address); per-address status is "unreachable"
// when no endpoint row for that address has QueryCount > 0.
func buildDomainNSAndAddresses(
	domainEPs []batchEndpointRow,
	domainAddrFacts []AnalysisRunAddressASN,
	asnByID, prefixByID map[int64]string,
) ([]DomainViewNameserver, []DomainViewAddress) {
	type nsAccum struct {
		name       string
		v4         map[int64]struct{}
		v6         map[int64]struct{}
		addrIDs    map[int64]struct{}
		unresolved bool
	}
	type addrAccum struct {
		family  string
		literal string
		hasOK   bool
		seen    bool
	}
	addrAccums := map[int64]*addrAccum{}
	nsBuckets := map[int64]*nsAccum{}
	for _, ep := range domainEPs {
		nb, ok := nsBuckets[ep.NameserverID]
		if !ok {
			nb = &nsAccum{
				name:    ep.NameserverName,
				v4:      map[int64]struct{}{},
				v6:      map[int64]struct{}{},
				addrIDs: map[int64]struct{}{},
			}
			nsBuckets[ep.NameserverID] = nb
		}
		if ep.AddressID == 0 {
			nb.unresolved = true
			continue
		}
		nb.addrIDs[ep.AddressID] = struct{}{}
		switch ep.Family {
		case "ipv4":
			nb.v4[ep.AddressID] = struct{}{}
		case "ipv6":
			nb.v6[ep.AddressID] = struct{}{}
		}
		aa, exists := addrAccums[ep.AddressID]
		if !exists {
			aa = &addrAccum{family: ep.Family, literal: ep.Address, seen: true}
			addrAccums[ep.AddressID] = aa
		}
		if ep.QueryCount > 0 {
			aa.hasOK = true
		}
	}

	addrFactByAddrID := map[int64]AnalysisRunAddressASN{}
	for _, f := range domainAddrFacts {
		addrFactByAddrID[f.AddressID] = f
	}

	addrViewByID := map[int64]DomainViewAddress{}
	for addrID, aa := range addrAccums {
		view := DomainViewAddress{Address: aa.literal, Family: aa.family}
		if !aa.hasOK {
			view.Status = "unreachable"
		}
		if fact, ok := addrFactByAddrID[addrID]; ok {
			if fact.ASN != nil {
				asn := *fact.ASN
				view.ASN = &asn
				view.ASNLabel = asnByID[asn]
			}
			if fact.PrefixID != nil {
				view.Prefix = prefixByID[*fact.PrefixID]
			}
		}
		addrViewByID[addrID] = view
	}

	nsViews := make([]DomainViewNameserver, 0, len(nsBuckets))
	for _, nb := range nsBuckets {
		nv := DomainViewNameserver{
			Name:      nb.name,
			IPv4Count: len(nb.v4),
			IPv6Count: len(nb.v6),
		}
		if nb.unresolved && len(nb.addrIDs) == 0 {
			nv.Status = "unresolved"
		}
		for addrID := range nb.addrIDs {
			if av, ok := addrViewByID[addrID]; ok {
				nv.Addresses = append(nv.Addresses, av)
			}
		}
		sort.Slice(nv.Addresses, func(i, j int) bool {
			if nv.Addresses[i].Family != nv.Addresses[j].Family {
				return nv.Addresses[i].Family < nv.Addresses[j].Family
			}
			return nv.Addresses[i].Address < nv.Addresses[j].Address
		})
		nsViews = append(nsViews, nv)
	}
	sort.Slice(nsViews, func(i, j int) bool { return nsViews[i].Name < nsViews[j].Name })

	addrViews := make([]DomainViewAddress, 0, len(addrViewByID))
	for _, av := range addrViewByID {
		addrViews = append(addrViews, av)
	}
	sort.Slice(addrViews, func(i, j int) bool { return addrViews[i].Address < addrViews[j].Address })

	return nsViews, addrViews
}

// buildDomainTags collapses per-(run, domain, tag) summaries into one
// tag per (tag) for one domain, keeping the worst-seen level. Tags
// whose worst level falls below floor are dropped.
func buildDomainTags(rows []AnalysisRunTagSummary, floor int) []DomainViewTag {
	type bucket struct {
		module, testcase, level string
	}
	buckets := map[string]*bucket{}
	for _, t := range rows {
		tag := strings.TrimSpace(t.Tag)
		if tag == "" {
			continue
		}
		b, ok := buckets[tag]
		if !ok {
			b = &bucket{module: t.Module, testcase: t.Testcase, level: t.Level}
			buckets[tag] = b
		}
		if severityRank(t.Level) > severityRank(b.level) {
			b.level = t.Level
		}
		if b.module == "" {
			b.module = t.Module
		}
		if b.testcase == "" {
			b.testcase = t.Testcase
		}
	}
	out := make([]DomainViewTag, 0, len(buckets))
	for tag, b := range buckets {
		if severityRank(b.level) < floor {
			continue
		}
		out = append(out, DomainViewTag{
			Tag:      tag,
			Module:   b.module,
			Testcase: b.testcase,
			Level:    b.level,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return out
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
func (s *SQLJobStore) collectDomainNames(endpoints []batchEndpointRow, addrFacts []AnalysisRunAddressASN, domainASNs []AnalysisRunDomainASN, tagSummaries []AnalysisRunTagSummary) map[int64]string {
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
	for _, t := range tagSummaries {
		idSet[t.DomainID] = struct{}{}
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
		slices.Sort(asns)
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

func buildEndpointViews(endpoints []batchEndpointRow, addrFacts []AnalysisRunAddressASN, asnByID map[int64]string, prefixByID map[int64]string, domainNames map[int64]string) []AnalysisSnapshotEndpointView {
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

		domains := make([]string, 0, len(b.domains))
		for id := range b.domains {
			if name, ok := domainNames[id]; ok && name != "" {
				domains = append(domains, name)
			}
		}
		sort.Strings(domains)
		v.Domains = domains

		out = append(out, v)
	}
	return out
}

func buildASNViews(
	endpoints []batchEndpointRow,
	addrFacts []AnalysisRunAddressASN,
	domainASNs []AnalysisRunDomainASN,
	asnByID, prefixByID, domainNames, nameserverNames map[int64]string,
) []AnalysisSnapshotASNView {
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
		v := AnalysisSnapshotASNView{
			ASN:             asn,
			Label:           asnByID[asn],
			DomainCount:     len(b.domains),
			AddressCount:    len(b.addresses),
			NameserverCount: len(b.nameservers),
			PrefixCount:     len(b.prefixes),
			IPv4Count:       len(b.ipv4),
			IPv6Count:       len(b.ipv6),
		}
		domains := make([]string, 0, len(b.domains))
		for id := range b.domains {
			if name, ok := domainNames[id]; ok && name != "" {
				domains = append(domains, name)
			}
		}
		sort.Strings(domains)
		v.Domains = domains

		nsNames := make([]string, 0, len(b.nameservers))
		for id := range b.nameservers {
			if name, ok := nameserverNames[id]; ok && name != "" {
				nsNames = append(nsNames, name)
			}
		}
		sort.Strings(nsNames)
		v.Nameservers = nsNames

		prefixes := make([]string, 0, len(b.prefixes))
		for id := range b.prefixes {
			if p, ok := prefixByID[id]; ok && p != "" {
				prefixes = append(prefixes, p)
			}
		}
		sort.Strings(prefixes)
		v.Prefixes = prefixes

		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ASN < out[j].ASN })
	return out
}

// buildPrefixViews aggregates one row per CIDR prefix observed in the
// snapshot's authoritative-address fact set, baking the ASN/domain/
// address rosters as JSON columns so the prefix detail and listing
// handlers serve from one indexed read.
func buildPrefixViews(
	addrFacts []AnalysisRunAddressASN,
	prefixByID map[int64]string,
	asnByID map[int64]string,
	domainNames map[int64]string,
	addressLiterals map[int64]string,
) []AnalysisSnapshotPrefixView {
	type bucket struct {
		prefix    string
		family    string
		domains   map[int64]struct{}
		addresses map[int64]struct{}
		asns      map[int64]struct{}
	}
	buckets := map[int64]*bucket{}
	prefixFamily := map[string]string{}
	for _, f := range addrFacts {
		if f.PrefixID == nil {
			continue
		}
		prefixID := *f.PrefixID
		prefix, ok := prefixByID[prefixID]
		if !ok || prefix == "" {
			continue
		}
		b, exists := buckets[prefixID]
		if !exists {
			family := "ipv4"
			if strings.Contains(prefix, ":") {
				family = "ipv6"
			}
			b = &bucket{
				prefix:    prefix,
				family:    family,
				domains:   map[int64]struct{}{},
				addresses: map[int64]struct{}{},
				asns:      map[int64]struct{}{},
			}
			buckets[prefixID] = b
			prefixFamily[prefix] = family
		}
		b.domains[f.DomainID] = struct{}{}
		b.addresses[f.AddressID] = struct{}{}
		if f.ASN != nil {
			b.asns[*f.ASN] = struct{}{}
		}
	}
	out := make([]AnalysisSnapshotPrefixView, 0, len(buckets))
	for _, b := range buckets {
		v := AnalysisSnapshotPrefixView{
			Prefix:       b.prefix,
			Family:       b.family,
			DomainCount:  len(b.domains),
			AddressCount: len(b.addresses),
		}
		if len(b.asns) == 1 {
			for asn := range b.asns {
				asnCopy := asn
				v.ASN = &asnCopy
				v.ASNLabel = asnByID[asn]
			}
		}
		asns := make([]int64, 0, len(b.asns))
		for asn := range b.asns {
			asns = append(asns, asn)
		}
		slices.Sort(asns)
		v.ASNs = asns

		domains := make([]string, 0, len(b.domains))
		for id := range b.domains {
			if name, ok := domainNames[id]; ok && name != "" {
				domains = append(domains, name)
			}
		}
		sort.Strings(domains)
		v.Domains = domains

		addrs := make([]string, 0, len(b.addresses))
		for id := range b.addresses {
			if lit, ok := addressLiterals[id]; ok && lit != "" {
				addrs = append(addrs, lit)
			}
		}
		sort.Strings(addrs)
		v.Addresses = addrs

		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Prefix < out[j].Prefix })
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
		"analysis_snapshot_tag_view",
		"analysis_snapshot_domain_view",
		"analysis_snapshot_prefix_view",
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
		domainsJSON, err := marshalStringList(v.Domains)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal endpoint domains ns=%d addr=%d: %w", v.NameserverID, v.AddressID, err)
		}
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_snapshot_endpoint_view
				(snapshot_id, nameserver_id, address_id, nameserver_name, address, family,
				 domain_count, asn, asn_label, prefix, domains_json)
				VALUES (%s)`, s.phRange(1, 11)),
			snapshotID, v.NameserverID, v.AddressID, v.NameserverName, v.Address, v.Family,
			v.DomainCount, nullInt64Value(v.ASN), v.ASNLabel, v.Prefix, domainsJSON,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert endpoint view ns=%d addr=%d: %w", v.NameserverID, v.AddressID, err)
		}
	}
	for _, v := range views.ASNs {
		domainsJSON, err := marshalStringList(v.Domains)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal asn domains asn=%d: %w", v.ASN, err)
		}
		nsJSON, err := marshalStringList(v.Nameservers)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal asn nameservers asn=%d: %w", v.ASN, err)
		}
		prefixesJSON, err := marshalStringList(v.Prefixes)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal asn prefixes asn=%d: %w", v.ASN, err)
		}
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_snapshot_asn_view
				(snapshot_id, asn, label, domain_count, address_count, nameserver_count,
				 prefix_count, ipv4_count, ipv6_count,
				 domains_json, nameservers_json, prefixes_json)
				VALUES (%s)`, s.phRange(1, 12)),
			snapshotID, v.ASN, v.Label, v.DomainCount, v.AddressCount, v.NameserverCount,
			v.PrefixCount, v.IPv4Count, v.IPv6Count,
			domainsJSON, nsJSON, prefixesJSON,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert asn view asn=%d: %w", v.ASN, err)
		}
	}
	for _, v := range views.Prefixes {
		asnsJSON, err := marshalInt64List(v.ASNs)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal prefix asns prefix=%s: %w", v.Prefix, err)
		}
		domainsJSON, err := marshalStringList(v.Domains)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal prefix domains prefix=%s: %w", v.Prefix, err)
		}
		addrsJSON, err := marshalStringList(v.Addresses)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal prefix addresses prefix=%s: %w", v.Prefix, err)
		}
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_snapshot_prefix_view
				(snapshot_id, prefix, family, domain_count, address_count,
				 asn, asn_label, asns_json, domains_json, addresses_json)
				VALUES (%s)`, s.phRange(1, 10)),
			snapshotID, v.Prefix, v.Family, v.DomainCount, v.AddressCount,
			nullInt64Value(v.ASN), v.ASNLabel, asnsJSON, domainsJSON, addrsJSON,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert prefix view prefix=%s: %w", v.Prefix, err)
		}
	}
	for _, v := range views.Tags {
		domainsJSON, err := marshalStringList(v.Domains)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal tag domains tag=%s: %w", v.Tag, err)
		}
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_snapshot_tag_view
				(snapshot_id, tag, module, testcase, level, domain_count, occurrence_count, domains_json)
				VALUES (%s)`, s.phRange(1, 8)),
			snapshotID, v.Tag, v.Module, v.Testcase, v.Level, v.DomainCount, v.OccurrenceCount, domainsJSON,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert tag view tag=%s: %w", v.Tag, err)
		}
	}
	for _, v := range views.Domains {
		nsJSON, err := marshalDomainNameservers(v.Nameservers)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal domain nameservers dom=%d: %w", v.DomainID, err)
		}
		addrJSON, err := marshalDomainAddresses(v.Addresses)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal domain addresses dom=%d: %w", v.DomainID, err)
		}
		tagsJSON, err := marshalDomainTags(v.Tags)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal domain tags dom=%d: %w", v.DomainID, err)
		}
		var (
			score      sql.NullInt64
			finishedAt sql.NullString
		)
		if v.Score != nil {
			score = sql.NullInt64{Int64: int64(*v.Score), Valid: true}
		}
		if v.FinishedAt != nil && !v.FinishedAt.IsZero() {
			finishedAt = sql.NullString{String: v.FinishedAt.UTC().Format(time.RFC3339Nano), Valid: true}
		}
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_snapshot_domain_view
				(snapshot_id, domain_id, domain_name, score, grade, worst_level, finished_at,
				 nameserver_count, endpoint_count, asn_count, prefix_count,
				 nameservers_json, addresses_json, tags_json)
				VALUES (%s)`, s.phRange(1, 14)),
			snapshotID, v.DomainID, v.DomainName, score, v.Grade, v.WorstLevel, finishedAt,
			v.NameserverCount, v.EndpointCount, v.ASNCount, v.PrefixCount,
			nsJSON, addrJSON, tagsJSON,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert domain view dom=%d: %w", v.DomainID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace snapshot views: %w", err)
	}
	return nil
}

func marshalDomainNameservers(in []DomainViewNameserver) (string, error) {
	if in == nil {
		return "[]", nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func marshalDomainAddresses(in []DomainViewAddress) (string, error) {
	if in == nil {
		return "[]", nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func marshalDomainTags(in []DomainViewTag) (string, error) {
	if in == nil {
		return "[]", nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalDomainNameservers(raw string) []DomainViewNameserver {
	if raw == "" {
		return nil
	}
	var out []DomainViewNameserver
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalDomainAddresses(raw string) []DomainViewAddress {
	if raw == "" {
		return nil
	}
	var out []DomainViewAddress
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalDomainTags(raw string) []DomainViewTag {
	if raw == "" {
		return nil
	}
	var out []DomainViewTag
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
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

const analysisSnapshotEndpointViewCols = `snapshot_id, nameserver_id, address_id, nameserver_name,
	address, family, domain_count, asn, asn_label, prefix, domains_json`

func scanSnapshotEndpointView(row rowScanner) (AnalysisSnapshotEndpointView, error) {
	var (
		v           AnalysisSnapshotEndpointView
		asn         sql.NullInt64
		domainsJSON string
	)
	if err := row.Scan(
		&v.SnapshotID, &v.NameserverID, &v.AddressID, &v.NameserverName,
		&v.Address, &v.Family, &v.DomainCount, &asn, &v.ASNLabel, &v.Prefix, &domainsJSON,
	); err != nil {
		return AnalysisSnapshotEndpointView{}, err
	}
	v.ASN = nullInt64Ptr(asn)
	v.Domains = unmarshalStringList(domainsJSON)
	return v, nil
}

// ListSnapshotEndpointViews returns every endpoint view row for one snapshot.
func (s *SQLJobStore) ListSnapshotEndpointViews(snapshotID int64) []AnalysisSnapshotEndpointView {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_endpoint_view
			WHERE snapshot_id = %s
			ORDER BY domain_count DESC, nameserver_name ASC, address ASC`,
			analysisSnapshotEndpointViewCols, s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotEndpointView
	for rows.Next() {
		v, err := scanSnapshotEndpointView(rows)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// ListSnapshotEndpointViewsByAddress returns every endpoint view row for
// one snapshot matching a case-insensitive IP literal. Optional nameserver
// (also case-insensitive) narrows to a single nameserver when the address
// is shared across more than one. Used by the per-IP detail page.
func (s *SQLJobStore) ListSnapshotEndpointViewsByAddress(snapshotID int64, address, nameserver string) []AnalysisSnapshotEndpointView {
	args := []any{snapshotID, address}
	query := fmt.Sprintf(`SELECT %s
		FROM analysis_snapshot_endpoint_view
		WHERE snapshot_id = %s AND LOWER(address) = LOWER(%s)`,
		analysisSnapshotEndpointViewCols, s.ph(1), s.ph(2))
	if nameserver != "" {
		query += " AND LOWER(nameserver_name) = LOWER(" + s.ph(3) + ")"
		args = append(args, nameserver)
	}
	query += " ORDER BY nameserver_name ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotEndpointView
	for rows.Next() {
		v, err := scanSnapshotEndpointView(rows)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

const analysisSnapshotASNViewCols = `snapshot_id, asn, label, domain_count, address_count,
	nameserver_count, prefix_count, ipv4_count, ipv6_count,
	domains_json, nameservers_json, prefixes_json`

func scanSnapshotASNView(row rowScanner) (AnalysisSnapshotASNView, error) {
	var (
		v           AnalysisSnapshotASNView
		domainsJSON string
		nsJSON      string
		prefixJSON  string
	)
	if err := row.Scan(
		&v.SnapshotID, &v.ASN, &v.Label, &v.DomainCount, &v.AddressCount,
		&v.NameserverCount, &v.PrefixCount, &v.IPv4Count, &v.IPv6Count,
		&domainsJSON, &nsJSON, &prefixJSON,
	); err != nil {
		return AnalysisSnapshotASNView{}, err
	}
	v.Domains = unmarshalStringList(domainsJSON)
	v.Nameservers = unmarshalStringList(nsJSON)
	v.Prefixes = unmarshalStringList(prefixJSON)
	return v, nil
}

// ListSnapshotASNViews returns every ASN view row for one snapshot.
func (s *SQLJobStore) ListSnapshotASNViews(snapshotID int64) []AnalysisSnapshotASNView {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_asn_view
			WHERE snapshot_id = %s
			ORDER BY domain_count DESC, asn ASC`,
			analysisSnapshotASNViewCols, s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotASNView
	for rows.Next() {
		v, err := scanSnapshotASNView(rows)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// GetSnapshotASNView looks up a single ASN view row by exact ASN match.
func (s *SQLJobStore) GetSnapshotASNView(snapshotID, asn int64) (AnalysisSnapshotASNView, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_asn_view
			WHERE snapshot_id = %s AND asn = %s
			LIMIT 1`,
			analysisSnapshotASNViewCols, s.ph(1), s.ph(2)),
		snapshotID, asn,
	)
	v, err := scanSnapshotASNView(row)
	if err != nil {
		return AnalysisSnapshotASNView{}, false
	}
	return v, true
}

const analysisSnapshotPrefixViewCols = `snapshot_id, prefix, family, domain_count, address_count,
	asn, asn_label, asns_json, domains_json, addresses_json`

func scanSnapshotPrefixView(row rowScanner) (AnalysisSnapshotPrefixView, error) {
	var (
		v           AnalysisSnapshotPrefixView
		asn         sql.NullInt64
		asnsJSON    string
		domainsJSON string
		addrsJSON   string
	)
	if err := row.Scan(
		&v.SnapshotID, &v.Prefix, &v.Family, &v.DomainCount, &v.AddressCount,
		&asn, &v.ASNLabel, &asnsJSON, &domainsJSON, &addrsJSON,
	); err != nil {
		return AnalysisSnapshotPrefixView{}, err
	}
	v.ASN = nullInt64Ptr(asn)
	v.ASNs = unmarshalInt64List(asnsJSON)
	v.Domains = unmarshalStringList(domainsJSON)
	v.Addresses = unmarshalStringList(addrsJSON)
	return v, nil
}

// ListSnapshotPrefixViews returns every prefix view row for one snapshot.
func (s *SQLJobStore) ListSnapshotPrefixViews(snapshotID int64) []AnalysisSnapshotPrefixView {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_prefix_view
			WHERE snapshot_id = %s
			ORDER BY domain_count DESC, prefix ASC`,
			analysisSnapshotPrefixViewCols, s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotPrefixView
	for rows.Next() {
		v, err := scanSnapshotPrefixView(rows)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// GetSnapshotPrefixView looks up a single prefix view row by exact match.
func (s *SQLJobStore) GetSnapshotPrefixView(snapshotID int64, prefix string) (AnalysisSnapshotPrefixView, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_prefix_view
			WHERE snapshot_id = %s AND prefix = %s
			LIMIT 1`,
			analysisSnapshotPrefixViewCols, s.ph(1), s.ph(2)),
		snapshotID, prefix,
	)
	v, err := scanSnapshotPrefixView(row)
	if err != nil {
		return AnalysisSnapshotPrefixView{}, false
	}
	return v, true
}

const analysisSnapshotTagViewCols = `snapshot_id, tag, module, testcase, level,
	domain_count, occurrence_count, domains_json`

func scanSnapshotTagView(row rowScanner) (AnalysisSnapshotTagView, error) {
	var (
		v           AnalysisSnapshotTagView
		domainsJSON string
	)
	if err := row.Scan(
		&v.SnapshotID, &v.Tag, &v.Module, &v.Testcase, &v.Level,
		&v.DomainCount, &v.OccurrenceCount, &domainsJSON,
	); err != nil {
		return AnalysisSnapshotTagView{}, err
	}
	v.Domains = unmarshalStringList(domainsJSON)
	return v, nil
}

// ListSnapshotTagViews returns every tag view row for one snapshot.
// Default order is domain_count desc with tag asc as a stable secondary.
func (s *SQLJobStore) ListSnapshotTagViews(snapshotID int64) []AnalysisSnapshotTagView {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_tag_view
			WHERE snapshot_id = %s
			ORDER BY domain_count DESC, tag ASC`,
			analysisSnapshotTagViewCols, s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotTagView
	for rows.Next() {
		v, err := scanSnapshotTagView(rows)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// GetSnapshotTagView looks up a single tag view row by exact tag match.
func (s *SQLJobStore) GetSnapshotTagView(snapshotID int64, tag string) (AnalysisSnapshotTagView, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_tag_view
			WHERE snapshot_id = %s AND tag = %s
			LIMIT 1`,
			analysisSnapshotTagViewCols, s.ph(1), s.ph(2)),
		snapshotID, tag,
	)
	v, err := scanSnapshotTagView(row)
	if err != nil {
		return AnalysisSnapshotTagView{}, false
	}
	return v, true
}

const analysisSnapshotDomainViewCols = `snapshot_id, domain_id, domain_name,
	score, grade, worst_level, finished_at,
	nameserver_count, endpoint_count, asn_count, prefix_count,
	nameservers_json, addresses_json, tags_json`

func scanSnapshotDomainView(row rowScanner) (AnalysisSnapshotDomainView, error) {
	var (
		v          AnalysisSnapshotDomainView
		score      sql.NullInt64
		finishedAt sql.NullString
		nsJSON     string
		addrJSON   string
		tagsJSON   string
	)
	if err := row.Scan(
		&v.SnapshotID, &v.DomainID, &v.DomainName,
		&score, &v.Grade, &v.WorstLevel, &finishedAt,
		&v.NameserverCount, &v.EndpointCount, &v.ASNCount, &v.PrefixCount,
		&nsJSON, &addrJSON, &tagsJSON,
	); err != nil {
		return AnalysisSnapshotDomainView{}, err
	}
	if score.Valid {
		s := int(score.Int64)
		v.Score = &s
	}
	if t := parseTimestampNullStr(finishedAt); !t.IsZero() {
		v.FinishedAt = &t
	}
	v.Nameservers = unmarshalDomainNameservers(nsJSON)
	v.Addresses = unmarshalDomainAddresses(addrJSON)
	v.Tags = unmarshalDomainTags(tagsJSON)
	return v, nil
}

// ListSnapshotDomainViews returns every domain view row for one snapshot,
// ordered by domain_name for stability.
func (s *SQLJobStore) ListSnapshotDomainViews(snapshotID int64) []AnalysisSnapshotDomainView {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_domain_view
			WHERE snapshot_id = %s
			ORDER BY domain_name ASC`,
			analysisSnapshotDomainViewCols, s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisSnapshotDomainView
	for rows.Next() {
		v, err := scanSnapshotDomainView(rows)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// GetSnapshotDomainViewByName looks up a single domain row by case-
// insensitive domain name. Used by the per-domain detail page so the
// handler is one indexed read instead of a cohort-wide fact load.
func (s *SQLJobStore) GetSnapshotDomainViewByName(snapshotID int64, name string) (AnalysisSnapshotDomainView, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_domain_view
			WHERE snapshot_id = %s AND LOWER(domain_name) = LOWER(%s)
			LIMIT 1`,
			analysisSnapshotDomainViewCols, s.ph(1), s.ph(2)),
		snapshotID, name,
	)
	v, err := scanSnapshotDomainView(row)
	if err != nil {
		return AnalysisSnapshotDomainView{}, false
	}
	return v, true
}
