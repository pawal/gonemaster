package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Trend payload keys for the non-fact-category aggregate slots. The fact
// category constants (FactCategorySeverity, FactCategoryDNSSECPosture, ...)
// double as trend keys for everything backed by the per-domain fact store,
// so they don't need their own SnapshotAggregate* alias.
const (
	SnapshotAggregateTopTags        = "top_tags"
	SnapshotAggregateTopNameservers = "top_nameservers"
	SnapshotAggregateTopASNs        = "top_asns"
	SnapshotAggregateOverviewV2     = "overview_v2"
)

const snapshotTopN = 20

// TopTagEntry is one row of the top_tags overview payload.
type TopTagEntry struct {
	Tag         string `json:"tag"`
	Level       string `json:"level,omitempty"`
	DomainCount int    `json:"domain_count"`
}

// TopNameserverEntry is one row of the top_nameservers overview payload.
type TopNameserverEntry struct {
	Nameserver  string `json:"nameserver"`
	DomainCount int    `json:"domain_count"`
}

// TopASNEntry is one row of the top_asns overview payload.
type TopASNEntry struct {
	ASN         int64  `json:"asn"`
	Label       string `json:"label,omitempty"`
	DomainCount int    `json:"domain_count"`
}

// SnapshotOverviewTotals carries the cohort-wide counts the overview tab's
// header cards display.
type SnapshotOverviewTotals struct {
	DomainCount     int `json:"domain_count"`
	NameserverCount int `json:"nameserver_count"`
	EndpointCount   int `json:"endpoint_count"`
	ASNCount        int `json:"asn_count"`
	PrefixCount     int `json:"prefix_count"`
}

// SnapshotOverviewV2 is the consolidated overview payload baked into one
// per-snapshot row at capture time. All distribution-shaped data (severity,
// DNSSEC posture, grade, DNSKEY algorithm, future categories) lives in
// FactDistributions; only top-N lists and totals get their own slots.
type SnapshotOverviewV2 struct {
	Totals            SnapshotOverviewTotals                    `json:"totals"`
	TopTags           []TopTagEntry                             `json:"top_tags"`
	TopNameservers    []TopNameserverEntry                      `json:"top_nameservers"`
	TopASNs           []TopASNEntry                             `json:"top_asns"`
	FactDistributions map[string]PublicAnalysisFactDistribution `json:"fact_distributions,omitempty"`
}

// AsCategoryPayloads returns the category->raw-JSON map shape that the
// trend handler and snapshot detail expose. Each fact-distribution
// category is emitted as a count map so the trend chart sees the same
// {key: count} shape regardless of which statistic it tracks.
func (o SnapshotOverviewV2) AsCategoryPayloads() (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, 4+len(o.FactDistributions))
	emit := func(category string, payload any) error {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", category, err)
		}
		out[category] = json.RawMessage(b)
		return nil
	}
	for category, dist := range o.FactDistributions {
		if err := emit(category, bucketsToCounts(dist.Buckets)); err != nil {
			return nil, err
		}
	}
	if err := emit(SnapshotAggregateTopTags, o.TopTags); err != nil {
		return nil, err
	}
	if err := emit(SnapshotAggregateTopNameservers, o.TopNameservers); err != nil {
		return nil, err
	}
	if err := emit(SnapshotAggregateTopASNs, o.TopASNs); err != nil {
		return nil, err
	}
	if err := emit(SnapshotAggregateOverviewV2, o); err != nil {
		return nil, err
	}
	return out, nil
}

func bucketsToCounts(buckets []PublicAnalysisFactBucket) map[string]int {
	out := make(map[string]int, len(buckets))
	for _, b := range buckets {
		out[b.Key] = b.Count
	}
	return out
}

// ComputeSnapshotOverview builds the per-snapshot overview payload from
// the (cohort, batch) fact rows. Caller persists it via
// ReplaceSnapshotOverview at capture time.
func (s *SQLJobStore) ComputeSnapshotOverview(cohortID int64, batchID string) (SnapshotOverviewV2, error) {
	if batchID == "" {
		return SnapshotOverviewV2{}, fmt.Errorf("compute snapshot overview: batch_id is required")
	}
	allFacts, err := s.queryBatchAllFactDistributions(cohortID, batchID)
	if err != nil {
		return SnapshotOverviewV2{}, err
	}
	topTags, err := s.queryBatchTopTags(cohortID, batchID, snapshotTopN)
	if err != nil {
		return SnapshotOverviewV2{}, err
	}
	topNameservers, err := s.queryBatchTopNameservers(cohortID, batchID, snapshotTopN)
	if err != nil {
		return SnapshotOverviewV2{}, err
	}
	topASNs, err := s.queryBatchTopASNs(cohortID, batchID, snapshotTopN)
	if err != nil {
		return SnapshotOverviewV2{}, err
	}
	totals, err := s.queryBatchTotals(cohortID, batchID)
	if err != nil {
		return SnapshotOverviewV2{}, err
	}
	factDistributions := make(map[string]PublicAnalysisFactDistribution, len(allFacts))
	for category, counts := range allFacts {
		if len(counts) == 0 {
			continue
		}
		factDistributions[category] = factDistributionFromCounts(category, counts)
	}
	return SnapshotOverviewV2{
		Totals:            totals,
		TopTags:           topTags,
		TopNameservers:    topNameservers,
		TopASNs:           topASNs,
		FactDistributions: factDistributions,
	}, nil
}

// ReplaceSnapshotOverview swaps the row for one snapshot. Idempotent on
// re-run via DELETE+INSERT.
func (s *SQLJobStore) ReplaceSnapshotOverview(snapshotID int64, overview SnapshotOverviewV2) error {
	tagsJSON, err := json.Marshal(overview.TopTags)
	if err != nil {
		return fmt.Errorf("marshal top_tags: %w", err)
	}
	if len(overview.TopTags) == 0 {
		tagsJSON = []byte("[]")
	}
	nsJSON, err := json.Marshal(overview.TopNameservers)
	if err != nil {
		return fmt.Errorf("marshal top_nameservers: %w", err)
	}
	if len(overview.TopNameservers) == 0 {
		nsJSON = []byte("[]")
	}
	asnsJSON, err := json.Marshal(overview.TopASNs)
	if err != nil {
		return fmt.Errorf("marshal top_asns: %w", err)
	}
	if len(overview.TopASNs) == 0 {
		asnsJSON = []byte("[]")
	}
	factJSON, err := marshalFactDistributions(overview.FactDistributions)
	if err != nil {
		return fmt.Errorf("marshal fact_distributions: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace snapshot overview: %w", err)
	}
	if _, err := tx.Exec(
		fmt.Sprintf(`DELETE FROM analysis_snapshot_overview_view WHERE snapshot_id = %s`, s.ph(1)),
		snapshotID,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("clear overview view for snapshot %d: %w", snapshotID, err)
	}
	if _, err := tx.Exec(
		fmt.Sprintf(`INSERT INTO analysis_snapshot_overview_view
			(snapshot_id, domain_count, nameserver_count, endpoint_count, asn_count, prefix_count,
			 top_tags_json, top_nameservers_json, top_asns_json, fact_distributions_json)
			VALUES (%s)`, s.phRange(1, 10)),
		snapshotID,
		overview.Totals.DomainCount, overview.Totals.NameserverCount, overview.Totals.EndpointCount,
		overview.Totals.ASNCount, overview.Totals.PrefixCount,
		string(tagsJSON), string(nsJSON), string(asnsJSON), string(factJSON),
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("insert overview view for snapshot %d: %w", snapshotID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace snapshot overview: %w", err)
	}
	return nil
}

func marshalFactDistributions(m map[string]PublicAnalysisFactDistribution) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

const analysisSnapshotOverviewViewCols = `snapshot_id, domain_count, nameserver_count,
	endpoint_count, asn_count, prefix_count,
	top_tags_json, top_nameservers_json, top_asns_json, fact_distributions_json`

func scanSnapshotOverviewView(row rowScanner) (int64, SnapshotOverviewV2, error) {
	var (
		snapshotID                                          int64
		domainCount, nsCount, epCount, asnCount, prefixCount int
		topTagsJSON, topNSJSON, topASNsJSON, factJSON       string
	)
	if err := row.Scan(
		&snapshotID, &domainCount, &nsCount, &epCount, &asnCount, &prefixCount,
		&topTagsJSON, &topNSJSON, &topASNsJSON, &factJSON,
	); err != nil {
		return 0, SnapshotOverviewV2{}, err
	}
	out := SnapshotOverviewV2{
		Totals: SnapshotOverviewTotals{
			DomainCount:     domainCount,
			NameserverCount: nsCount,
			EndpointCount:   epCount,
			ASNCount:        asnCount,
			PrefixCount:     prefixCount,
		},
		TopTags:           unmarshalTopTags(topTagsJSON),
		TopNameservers:    unmarshalTopNameservers(topNSJSON),
		TopASNs:           unmarshalTopASNs(topASNsJSON),
		FactDistributions: unmarshalFactDistributions(factJSON),
	}
	return snapshotID, out, nil
}

func unmarshalTopTags(raw string) []TopTagEntry {
	if raw == "" {
		return nil
	}
	var out []TopTagEntry
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalTopNameservers(raw string) []TopNameserverEntry {
	if raw == "" {
		return nil
	}
	var out []TopNameserverEntry
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalTopASNs(raw string) []TopASNEntry {
	if raw == "" {
		return nil
	}
	var out []TopASNEntry
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalFactDistributions(raw string) map[string]PublicAnalysisFactDistribution {
	if raw == "" {
		return nil
	}
	var out map[string]PublicAnalysisFactDistribution
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// GetSnapshotOverview returns the captured overview row for one snapshot.
func (s *SQLJobStore) GetSnapshotOverview(snapshotID int64) (SnapshotOverviewV2, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s
			FROM analysis_snapshot_overview_view
			WHERE snapshot_id = %s
			LIMIT 1`,
			analysisSnapshotOverviewViewCols, s.ph(1)),
		snapshotID,
	)
	_, overview, err := scanSnapshotOverviewView(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return SnapshotOverviewV2{}, false
		}
		return SnapshotOverviewV2{}, false
	}
	return overview, true
}

// ListSnapshotOverviewsByIDs returns the overview rows for a set of
// snapshots, keyed by snapshot id. Missing rows are simply absent from
// the map. Used by the trends endpoint to drive a category time series
// off one indexed scan.
func (s *SQLJobStore) ListSnapshotOverviewsByIDs(snapshotIDs []int64) map[int64]SnapshotOverviewV2 {
	out := map[int64]SnapshotOverviewV2{}
	if len(snapshotIDs) == 0 {
		return out
	}
	placeholders := make([]string, len(snapshotIDs))
	args := make([]any, len(snapshotIDs))
	for i, id := range snapshotIDs {
		placeholders[i] = s.ph(i + 1)
		args[i] = id
	}
	query := fmt.Sprintf(`SELECT %s
		FROM analysis_snapshot_overview_view
		WHERE snapshot_id IN (%s)`,
		analysisSnapshotOverviewViewCols, joinPlaceholders(placeholders))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		id, overview, err := scanSnapshotOverviewView(rows)
		if err != nil {
			return out
		}
		out[id] = overview
	}
	return out
}

func joinPlaceholders(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	out := items[0]
	for _, p := range items[1:] {
		out += ", " + p
	}
	return out
}

// queryBatchTotals returns the cohort-wide counts (distinct domains,
// nameservers, endpoints, ASNs, prefixes) for one batch. Parent-role
// endpoints are excluded so the counts match what the entity tabs show.
func (s *SQLJobStore) queryBatchTotals(cohortID int64, batchID string) (SnapshotOverviewTotals, error) {
	var totals SnapshotOverviewTotals

	domainRow := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(DISTINCT ards.domain_id)
			FROM analysis_run_domain_summary ards
			JOIN runs r ON r.id = ards.run_id
			WHERE ards.cohort_id = %s AND r.batch_id = %s`, s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err := domainRow.Scan(&totals.DomainCount); err != nil {
		return totals, fmt.Errorf("totals domain_count: %w", err)
	}

	nsRow := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(DISTINCT e.nameserver_id)
			FROM analysis_run_ns_endpoints e
			JOIN runs r ON r.id = e.run_id
			WHERE e.cohort_id = %s AND r.batch_id = %s AND e.role <> 'parent'`,
			s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err := nsRow.Scan(&totals.NameserverCount); err != nil {
		return totals, fmt.Errorf("totals nameserver_count: %w", err)
	}

	epRow := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM (
			SELECT e.nameserver_id, e.address_id
			FROM analysis_run_ns_endpoints e
			JOIN runs r ON r.id = e.run_id
			WHERE e.cohort_id = %s AND r.batch_id = %s AND e.role <> 'parent' AND e.address_id <> 0
			GROUP BY e.nameserver_id, e.address_id
		) AS pairs`, s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err := epRow.Scan(&totals.EndpointCount); err != nil {
		return totals, fmt.Errorf("totals endpoint_count: %w", err)
	}

	asnRow := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(DISTINCT asn) FROM (
			SELECT da.asn
			FROM analysis_run_address_asns da
			JOIN runs r ON r.id = da.run_id
			WHERE da.cohort_id = %s AND r.batch_id = %s AND da.asn IS NOT NULL
			UNION
			SELECT da.asn
			FROM analysis_run_domain_asns da
			JOIN runs r ON r.id = da.run_id
			WHERE da.cohort_id = %s AND r.batch_id = %s
		) AS combined`, s.ph(1), s.ph(2), s.ph(3), s.ph(4)),
		cohortID, batchID, cohortID, batchID,
	)
	if err := asnRow.Scan(&totals.ASNCount); err != nil {
		return totals, fmt.Errorf("totals asn_count: %w", err)
	}

	pfxRow := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(DISTINCT da.prefix_id)
			FROM analysis_run_address_asns da
			JOIN runs r ON r.id = da.run_id
			WHERE da.cohort_id = %s AND r.batch_id = %s AND da.prefix_id IS NOT NULL`,
			s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err := pfxRow.Scan(&totals.PrefixCount); err != nil {
		return totals, fmt.Errorf("totals prefix_count: %w", err)
	}
	return totals, nil
}

// queryBatchAllFactDistributions returns per-(category, key) distinct-domain
// counts for every fact category materialized for a batch in one indexed scan.
func (s *SQLJobStore) queryBatchAllFactDistributions(cohortID int64, batchID string) (map[string]map[string]int, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT f.category, f.fact_key, COUNT(DISTINCT f.domain_id)
			FROM analysis_run_domain_facts f
			JOIN runs r ON r.id = f.run_id
			WHERE f.cohort_id = %s AND r.batch_id = %s
			GROUP BY f.category, f.fact_key`, s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("fact distributions: %w", err)
	}
	defer rows.Close()

	out := map[string]map[string]int{}
	for rows.Next() {
		var category, key string
		var count int
		if err := rows.Scan(&category, &key, &count); err != nil {
			return nil, fmt.Errorf("fact distributions scan: %w", err)
		}
		bucket, ok := out[category]
		if !ok {
			bucket = map[string]int{}
			out[category] = bucket
		}
		bucket[key] = count
	}
	return out, rows.Err()
}

// queryBatchTopTags returns the top-N tags by distinct domain count.
// INFO/NOTICE chatter is excluded so actionable findings dominate.
func (s *SQLJobStore) queryBatchTopTags(cohortID int64, batchID string, limit int) ([]TopTagEntry, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT t.tag, t.level, COUNT(DISTINCT t.domain_id) AS dc
			FROM analysis_run_tag_summary t
			JOIN runs r ON r.id = t.run_id
			WHERE t.cohort_id = %s AND r.batch_id = %s
			AND t.level IN ('WARNING', 'ERROR', 'CRITICAL')
			GROUP BY t.tag, t.level
			ORDER BY dc DESC, t.tag ASC
			LIMIT %d`, s.ph(1), s.ph(2), limit),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("top tags: %w", err)
	}
	defer rows.Close()

	out := make([]TopTagEntry, 0, limit)
	for rows.Next() {
		var entry TopTagEntry
		if err := rows.Scan(&entry.Tag, &entry.Level, &entry.DomainCount); err != nil {
			return nil, fmt.Errorf("top tags scan: %w", err)
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// queryBatchTopNameservers returns the top-N authoritative nameservers
// by distinct domain count. Parent-role endpoints are excluded.
func (s *SQLJobStore) queryBatchTopNameservers(cohortID int64, batchID string, limit int) ([]TopNameserverEntry, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT n.name, COUNT(DISTINCT e.domain_id) AS dc
			FROM analysis_run_ns_endpoints e
			JOIN analysis_nameservers n ON n.id = e.nameserver_id
			JOIN runs r ON r.id = e.run_id
			WHERE e.cohort_id = %s AND r.batch_id = %s AND e.role <> 'parent'
			GROUP BY n.name
			ORDER BY dc DESC, n.name ASC
			LIMIT %d`, s.ph(1), s.ph(2), limit),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("top nameservers: %w", err)
	}
	defer rows.Close()

	out := make([]TopNameserverEntry, 0, limit)
	for rows.Next() {
		var entry TopNameserverEntry
		if err := rows.Scan(&entry.Nameserver, &entry.DomainCount); err != nil {
			return nil, fmt.Errorf("top nameservers scan: %w", err)
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// queryBatchTopASNs returns the top-N ASNs by distinct domain count,
// resolving each ASN's display label from analysis_asns.
func (s *SQLJobStore) queryBatchTopASNs(cohortID int64, batchID string, limit int) ([]TopASNEntry, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT da.asn, COALESCE(a.label, ''), COUNT(DISTINCT da.domain_id) AS dc
			FROM analysis_run_domain_asns da
			LEFT JOIN analysis_asns a ON a.asn = da.asn
			JOIN runs r ON r.id = da.run_id
			WHERE da.cohort_id = %s AND r.batch_id = %s
			GROUP BY da.asn, a.label
			ORDER BY dc DESC, da.asn ASC
			LIMIT %d`, s.ph(1), s.ph(2), limit),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("top asns: %w", err)
	}
	defer rows.Close()

	out := make([]TopASNEntry, 0, limit)
	for rows.Next() {
		var entry TopASNEntry
		if err := rows.Scan(&entry.ASN, &entry.Label, &entry.DomainCount); err != nil {
			return nil, fmt.Errorf("top asns scan: %w", err)
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}
