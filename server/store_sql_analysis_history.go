package server

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// AnalysisEntityHistory returns one entity's metric series across a cohort's
// captured public snapshots (oldest first) for entity-detail sparklines.
// Supported entities: nameserver, asn, tag, domain. Snapshots where the
// entity is absent still appear, with Present=false.
func (s *SQLJobStore) AnalysisEntityHistory(cohortID int64, entity, key string) ([]AnalysisEntityHistoryPoint, error) {
	entity = strings.ToLower(strings.TrimSpace(entity))

	var (
		table   string
		onCond  string
		keyArg  any
		metrics string // domain_count, latency_p50_ms, score, grade
	)
	switch entity {
	case "nameserver":
		table = "analysis_snapshot_nameserver_view"
		onCond = "LOWER(v.nameserver_name) = LOWER(" + s.ph(1) + ")"
		keyArg = key
		metrics = "v.domain_count, v.latency_p50_ms, NULL, NULL"
	case "asn":
		asn, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(key)), "as"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid asn %q", key)
		}
		table = "analysis_snapshot_asn_view"
		onCond = "v.asn = " + s.ph(1)
		keyArg = asn
		metrics = "v.domain_count, v.latency_p50_ms, NULL, NULL"
	case "tag":
		table = "analysis_snapshot_tag_view"
		onCond = "v.tag = " + s.ph(1)
		keyArg = key
		metrics = "v.domain_count, NULL, NULL, NULL"
	case "domain":
		table = "analysis_snapshot_domain_view"
		onCond = "LOWER(v.domain_name) = LOWER(" + s.ph(1) + ")"
		keyArg = key
		metrics = "NULL, NULL, v.score, v.grade"
	default:
		return nil, fmt.Errorf("unsupported history entity %q", entity)
	}

	query := fmt.Sprintf(`SELECT s.slug, s.captured_at,
			CASE WHEN v.snapshot_id IS NULL THEN 0 ELSE 1 END,
			%s
		FROM analysis_cohort_snapshots s
		LEFT JOIN %s v ON v.snapshot_id = s.id AND %s
		WHERE s.cohort_id = %s AND s.status = %s AND s.is_public = 1
		ORDER BY s.captured_at ASC, s.id ASC`,
		metrics, table, onCond, s.ph(2), s.ph(3))

	rows, err := s.db.Query(query, keyArg, cohortID, AnalysisSnapshotStatusCaptured)
	if err != nil {
		return nil, fmt.Errorf("query entity history: %w", err)
	}
	defer rows.Close()

	var out []AnalysisEntityHistoryPoint
	for rows.Next() {
		var (
			pt          AnalysisEntityHistoryPoint
			capturedAt  string
			present     int
			domainCount sql.NullInt64
			latencyP50  sql.NullFloat64
			score       sql.NullInt64
			grade       sql.NullString
		)
		if err := rows.Scan(&pt.Slug, &capturedAt, &present, &domainCount, &latencyP50, &score, &grade); err != nil {
			return nil, fmt.Errorf("scan entity history: %w", err)
		}
		pt.CapturedAt = parseTimestampStr(capturedAt)
		pt.Present = present == 1
		pt.DomainCount = int(domainCount.Int64)
		pt.LatencyP50MS = nullFloat64Ptr(latencyP50)
		if score.Valid {
			v := int(score.Int64)
			pt.Score = &v
		}
		pt.Grade = grade.String
		out = append(out, pt)
	}
	return out, rows.Err()
}
