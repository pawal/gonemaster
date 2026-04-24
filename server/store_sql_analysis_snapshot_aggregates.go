package server

import (
	"encoding/json"
	"fmt"
	"time"
)

// Aggregate category tokens written to analysis_cohort_snapshot_aggregates.
// Stable across releases — the UI keys on them when diffing snapshots.
const (
	SnapshotAggregateSeverityDistribution = "severity_distribution"
	SnapshotAggregateGradeDistribution    = "grade_distribution"
	SnapshotAggregateSigned               = "signed"
	SnapshotAggregateDNSKEYAlgo           = "dnskey_algo"
	SnapshotAggregateTopTags              = "top_tags"
	SnapshotAggregateTopNameservers       = "top_nameservers"
	SnapshotAggregateTopASNs              = "top_asns"
)

const snapshotTopN = 20

// TopTagEntry is one row of the top_tags aggregate payload.
type TopTagEntry struct {
	Tag         string `json:"tag"`
	Level       string `json:"level,omitempty"`
	DomainCount int    `json:"domain_count"`
}

// TopNameserverEntry is one row of the top_nameservers aggregate payload.
type TopNameserverEntry struct {
	Nameserver  string `json:"nameserver"`
	DomainCount int    `json:"domain_count"`
}

// TopASNEntry is one row of the top_asns aggregate payload.
type TopASNEntry struct {
	ASN         int64  `json:"asn"`
	Label       string `json:"label,omitempty"`
	DomainCount int    `json:"domain_count"`
}

// ComputeSnapshotAggregates renders every aggregate category for one
// (cohort, batch) snapshot into serializable rows ready to be passed to
// ReplaceSnapshotAggregates. Runs with an empty batch_id are excluded;
// counts collapse to distinct domains per bucket so a batch with
// multiple runs per domain is not double-counted.
func (s *SQLJobStore) ComputeSnapshotAggregates(cohortID int64, batchID string) ([]AnalysisCohortSnapshotAggregate, error) {
	if batchID == "" {
		return nil, fmt.Errorf("compute snapshot aggregates: batch_id is required")
	}
	now := time.Now().UTC()
	base := AnalysisCohortSnapshotAggregate{ComputedAt: now}

	addCategory := func(out []AnalysisCohortSnapshotAggregate, category string, payload any) ([]AnalysisCohortSnapshotAggregate, error) {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", category, err)
		}
		agg := base
		agg.Category = category
		agg.PayloadJSON = string(b)
		return append(out, agg), nil
	}

	var out []AnalysisCohortSnapshotAggregate

	severity, err := s.queryBatchSeverityDistribution(cohortID, batchID)
	if err != nil {
		return nil, err
	}
	if out, err = addCategory(out, SnapshotAggregateSeverityDistribution, severity); err != nil {
		return nil, err
	}

	grades, err := s.queryBatchGradeDistribution(cohortID, batchID)
	if err != nil {
		return nil, err
	}
	if out, err = addCategory(out, SnapshotAggregateGradeDistribution, grades); err != nil {
		return nil, err
	}

	signed, err := s.queryBatchFactDistribution(cohortID, batchID, FactCategorySigned)
	if err != nil {
		return nil, err
	}
	if out, err = addCategory(out, SnapshotAggregateSigned, signed); err != nil {
		return nil, err
	}

	dnskey, err := s.queryBatchFactDistribution(cohortID, batchID, FactCategoryDNSKEYAlgorithm)
	if err != nil {
		return nil, err
	}
	if out, err = addCategory(out, SnapshotAggregateDNSKEYAlgo, dnskey); err != nil {
		return nil, err
	}

	topTags, err := s.queryBatchTopTags(cohortID, batchID, snapshotTopN)
	if err != nil {
		return nil, err
	}
	if out, err = addCategory(out, SnapshotAggregateTopTags, topTags); err != nil {
		return nil, err
	}

	topNameservers, err := s.queryBatchTopNameservers(cohortID, batchID, snapshotTopN)
	if err != nil {
		return nil, err
	}
	if out, err = addCategory(out, SnapshotAggregateTopNameservers, topNameservers); err != nil {
		return nil, err
	}

	topASNs, err := s.queryBatchTopASNs(cohortID, batchID, snapshotTopN)
	if err != nil {
		return nil, err
	}
	if out, err = addCategory(out, SnapshotAggregateTopASNs, topASNs); err != nil {
		return nil, err
	}

	return out, nil
}

// queryBatchSeverityDistribution returns the per-severity domain counts for
// one (cohort, batch) snapshot. Domains with no severity (no findings at
// all) fall into the "OK" bucket so the distribution always covers the
// full cohort membership.
func (s *SQLJobStore) queryBatchSeverityDistribution(cohortID int64, batchID string) (map[string]int, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT ards.worst_level, COUNT(DISTINCT ards.domain_id)
			FROM analysis_run_domain_summary ards
			JOIN runs r ON r.id = ards.run_id
			WHERE ards.cohort_id = %s AND r.batch_id = %s
			GROUP BY ards.worst_level`, s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("severity distribution: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return nil, fmt.Errorf("severity distribution scan: %w", err)
		}
		if level == "" {
			level = "OK"
		}
		out[level] += count
	}
	return out, rows.Err()
}

// queryBatchGradeDistribution returns the per-grade domain counts. Runs
// whose grade has not been computed are excluded so the payload stays
// clean; the catch-all lives under severity_distribution instead.
func (s *SQLJobStore) queryBatchGradeDistribution(cohortID int64, batchID string) (map[string]int, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT ards.grade, COUNT(DISTINCT ards.domain_id)
			FROM analysis_run_domain_summary ards
			JOIN runs r ON r.id = ards.run_id
			WHERE ards.cohort_id = %s AND r.batch_id = %s AND ards.grade IS NOT NULL AND ards.grade <> ''
			GROUP BY ards.grade`, s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("grade distribution: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var grade string
		var count int
		if err := rows.Scan(&grade, &count); err != nil {
			return nil, fmt.Errorf("grade distribution scan: %w", err)
		}
		out[grade] += count
	}
	return out, rows.Err()
}

// queryBatchFactDistribution returns the per-key distinct-domain count for
// one fact category (e.g. "signed" or "dnskey_algo") scoped to a batch.
func (s *SQLJobStore) queryBatchFactDistribution(cohortID int64, batchID, category string) (map[string]int, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT f.fact_key, COUNT(DISTINCT f.domain_id)
			FROM analysis_run_domain_facts f
			JOIN runs r ON r.id = f.run_id
			WHERE f.cohort_id = %s AND r.batch_id = %s AND f.category = %s
			GROUP BY f.fact_key`, s.ph(1), s.ph(2), s.ph(3)),
		cohortID, batchID, category,
	)
	if err != nil {
		return nil, fmt.Errorf("fact distribution %s: %w", category, err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, fmt.Errorf("fact distribution scan: %w", err)
		}
		out[key] += count
	}
	return out, rows.Err()
}

// queryBatchTopTags returns the top-N tags by distinct domain count in one
// (cohort, batch) snapshot. The level column carries the worst level the
// projector observed for that tag so the UI can tone the row.
func (s *SQLJobStore) queryBatchTopTags(cohortID int64, batchID string, limit int) ([]TopTagEntry, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT t.tag, t.level, COUNT(DISTINCT t.domain_id) AS dc
			FROM analysis_run_tag_summary t
			JOIN runs r ON r.id = t.run_id
			WHERE t.cohort_id = %s AND r.batch_id = %s
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

// queryBatchTopNameservers returns the top-N authoritative nameservers by
// distinct domain count. Parent-role endpoints are excluded so root / TLD
// registry servers don't dominate the chart.
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
// resolving each ASN's label from analysis_asns so the UI has a display
// name without a second round-trip per row.
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

// CountBatchSnapshotRuns returns the number of distinct runs and domains
// materialized for a (cohort, batch) snapshot so the projector can keep
// the snapshot row's counters accurate without bookkeeping on every
// graduation. Timestamps cover the span of finished_at across those runs.
func (s *SQLJobStore) CountBatchSnapshotRuns(cohortID int64, batchID string) (runCount, domainCount int, firstFinished, lastFinished time.Time, err error) {
	if batchID == "" {
		return 0, 0, time.Time{}, time.Time{}, fmt.Errorf("count batch snapshot runs: batch_id is required")
	}
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT
			COUNT(DISTINCT ards.run_id),
			COUNT(DISTINCT ards.domain_id),
			COALESCE(MIN(r.finished_at), ''),
			COALESCE(MAX(r.finished_at), '')
			FROM analysis_run_domain_summary ards
			JOIN runs r ON r.id = ards.run_id
			WHERE ards.cohort_id = %s AND r.batch_id = %s`, s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	var minStr, maxStr string
	if err := row.Scan(&runCount, &domainCount, &minStr, &maxStr); err != nil {
		return 0, 0, time.Time{}, time.Time{}, fmt.Errorf("count batch snapshot runs: %w", err)
	}
	firstFinished = parseTimestampStr(minStr)
	lastFinished = parseTimestampStr(maxStr)
	return runCount, domainCount, firstFinished, lastFinished, nil
}

// CohortBatchFactStats is one (cohort, batch) pair that has materialized
// fact rows. The projector's first-boot backfill enumerates these to
// reconstruct snapshot rows for historical data.
type CohortBatchFactStats struct {
	CohortID      int64
	BatchID       string
	RunCount      int
	DomainCount   int
	FirstFinished time.Time
	LastFinished  time.Time
}

// ListCohortBatchesWithFacts returns every (cohort, batch) pair that has
// at least one materialized domain_summary row, along with run / domain
// counts and the finished_at span. Used by the Phase 7 first-boot
// snapshot backfill so historical batches get one captured snapshot
// each without reprojecting.
func (s *SQLJobStore) ListCohortBatchesWithFacts() ([]CohortBatchFactStats, error) {
	rows, err := s.db.Query(
		`SELECT ards.cohort_id, r.batch_id,
			COUNT(DISTINCT ards.run_id),
			COUNT(DISTINCT ards.domain_id),
			COALESCE(MIN(r.finished_at), ''),
			COALESCE(MAX(r.finished_at), '')
			FROM analysis_run_domain_summary ards
			JOIN runs r ON r.id = ards.run_id
			WHERE r.batch_id <> ''
			GROUP BY ards.cohort_id, r.batch_id
			ORDER BY ards.cohort_id, r.batch_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list cohort batches with facts: %w", err)
	}
	defer rows.Close()
	var out []CohortBatchFactStats
	for rows.Next() {
		var stats CohortBatchFactStats
		var minStr, maxStr string
		if err := rows.Scan(&stats.CohortID, &stats.BatchID, &stats.RunCount, &stats.DomainCount, &minStr, &maxStr); err != nil {
			return nil, fmt.Errorf("scan cohort batch stats: %w", err)
		}
		stats.FirstFinished = parseTimestampStr(minStr)
		stats.LastFinished = parseTimestampStr(maxStr)
		out = append(out, stats)
	}
	return out, rows.Err()
}

// CountOutstandingJobsForBatch returns the number of in-flight (queued /
// running / paused) jobs still attached to a batch. Callers use this to
// decide whether a pending snapshot is safe to promote to captured.
func (s *SQLJobStore) CountOutstandingJobsForBatch(batchID string) (int, error) {
	var count int
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM jobs WHERE batch_id = %s`, s.ph(1)),
		batchID,
	)
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count outstanding jobs for batch %s: %w", batchID, err)
	}
	return count, nil
}

// CountUnprojectedSnapshotRuns returns completed runs in a snapshot-intent
// batch that belong to the cohort's source tag but do not yet have a ready
// analysis projection row. Snapshot capture must wait for this to reach zero;
// otherwise aggregates can be computed from a partial materialization even
// after the queue itself has drained.
func (s *SQLJobStore) CountUnprojectedSnapshotRuns(cohortID int64, batchID string) (int, error) {
	if batchID == "" {
		return 0, fmt.Errorf("count unprojected snapshot runs: batch_id is required")
	}
	var count int
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*)
			FROM runs r
			JOIN analysis_cohort_catalog c ON c.id = %s
			JOIN domain_tags dt ON dt.domain_id = r.domain_id AND dt.tag = c.source_tag
			LEFT JOIN analysis_projection_state ps
				ON ps.cohort_id = c.id
				AND ps.run_id = r.id
				AND ps.status = %s
			WHERE r.batch_id = %s
			  AND c.source_type = 'tag'
			  AND ps.run_id IS NULL`,
			s.ph(1), s.ph(2), s.ph(3)),
		cohortID, AnalysisMaterializationReady, batchID,
	)
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count unprojected snapshot runs for cohort %d batch %s: %w", cohortID, batchID, err)
	}
	return count, nil
}
