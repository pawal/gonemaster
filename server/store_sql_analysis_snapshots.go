package server

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const analysisCohortSnapshotCols = `id, cohort_id, batch_id, slug, label, description,
	profile_id, profile_name, captured_at, first_run_at, last_run_at,
	run_count, domain_count, status, is_default, is_public,
	created_at, updated_at`

func (s *SQLJobStore) scanAnalysisCohortSnapshot(row rowScanner) (AnalysisCohortSnapshot, error) {
	var (
		snap        AnalysisCohortSnapshot
		profileID   sql.NullInt64
		capturedAt  string
		firstRunAt  string
		lastRunAt   string
		isDefault   int
		isPublic    int
		createdAt   string
		updatedAt   string
		description string
		label       string
		profileName string
	)
	if err := row.Scan(
		&snap.ID,
		&snap.CohortID,
		&snap.BatchID,
		&snap.Slug,
		&label,
		&description,
		&profileID,
		&profileName,
		&capturedAt,
		&firstRunAt,
		&lastRunAt,
		&snap.RunCount,
		&snap.DomainCount,
		&snap.Status,
		&isDefault,
		&isPublic,
		&createdAt,
		&updatedAt,
	); err != nil {
		return AnalysisCohortSnapshot{}, err
	}
	snap.Label = label
	snap.Description = description
	snap.ProfileID = nullInt64Ptr(profileID)
	snap.ProfileName = profileName
	snap.CapturedAt = parseTimestampStr(capturedAt)
	snap.FirstRunAt = parseTimestampStr(firstRunAt)
	snap.LastRunAt = parseTimestampStr(lastRunAt)
	snap.IsDefault = intToBool(isDefault)
	snap.IsPublic = intToBool(isPublic)
	snap.CreatedAt = parseTimestampStr(createdAt)
	snap.UpdatedAt = parseTimestampStr(updatedAt)
	return snap, nil
}

// snapshotStoredTimestamp returns the text value written to a NOT NULL TEXT
// timestamp column; the snapshot table's captured_at / first_run_at /
// last_run_at default to empty string when the snapshot is pending.
func snapshotStoredTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatSortableTimestamp(t)
}

// GetAnalysisCohortSnapshot returns one snapshot by numeric id.
func (s *SQLJobStore) GetAnalysisCohortSnapshot(id int64) (AnalysisCohortSnapshot, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_snapshots WHERE id = %s`,
			analysisCohortSnapshotCols, s.ph(1)),
		id,
	)
	snap, err := s.scanAnalysisCohortSnapshot(row)
	if err == nil {
		return snap, true
	}
	return AnalysisCohortSnapshot{}, false
}

// GetAnalysisCohortSnapshotBySlug returns one snapshot addressed by its
// human-readable slug within a cohort.
func (s *SQLJobStore) GetAnalysisCohortSnapshotBySlug(cohortID int64, slug string) (AnalysisCohortSnapshot, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_snapshots
			WHERE cohort_id = %s AND slug = %s`,
			analysisCohortSnapshotCols, s.ph(1), s.ph(2)),
		cohortID, slug,
	)
	snap, err := s.scanAnalysisCohortSnapshot(row)
	if err == nil {
		return snap, true
	}
	return AnalysisCohortSnapshot{}, false
}

// getAnalysisCohortSnapshotByBatch looks up the (cohort, batch) natural-key
// row so UpsertAnalysisCohortSnapshot is idempotent per rebuild.
func (s *SQLJobStore) getAnalysisCohortSnapshotByBatch(cohortID int64, batchID string) (AnalysisCohortSnapshot, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_snapshots
			WHERE cohort_id = %s AND batch_id = %s`,
			analysisCohortSnapshotCols, s.ph(1), s.ph(2)),
		cohortID, batchID,
	)
	snap, err := s.scanAnalysisCohortSnapshot(row)
	if err == nil {
		return snap, true
	}
	return AnalysisCohortSnapshot{}, false
}

// ListAnalysisCohortSnapshots returns all snapshots for one cohort ordered
// newest-captured-first, with pending snapshots surfaced after captured ones
// so UIs default to the most recent addressable snapshot.
func (s *SQLJobStore) ListAnalysisCohortSnapshots(cohortID int64) []AnalysisCohortSnapshot {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_snapshots
			WHERE cohort_id = %s
			ORDER BY captured_at DESC, created_at DESC, id DESC`,
			analysisCohortSnapshotCols, s.ph(1)),
		cohortID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisCohortSnapshot
	for rows.Next() {
		snap, err := s.scanAnalysisCohortSnapshot(rows)
		if err != nil {
			return nil
		}
		out = append(out, snap)
	}
	return out
}

// GetDefaultSnapshotForCohort resolves the cohort's default snapshot using the
// catalog's policy: `pinned` honours default_snapshot_id, `auto_latest` picks
// the most recent captured public snapshot.
func (s *SQLJobStore) GetDefaultSnapshotForCohort(cohortID int64) (AnalysisCohortSnapshot, bool) {
	cohort, ok := s.GetAnalysisCohort(cohortID)
	if !ok {
		return AnalysisCohortSnapshot{}, false
	}
	policy := cohort.DefaultSnapshotPolicy
	if policy == "" {
		policy = DefaultSnapshotPolicyAutoLatest
	}
	if policy == DefaultSnapshotPolicyPinned && cohort.DefaultSnapshotID != nil {
		return s.GetAnalysisCohortSnapshot(*cohort.DefaultSnapshotID)
	}
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_snapshots
			WHERE cohort_id = %s AND status = %s AND is_public = 1
			ORDER BY captured_at DESC, id DESC
			LIMIT 1`,
			analysisCohortSnapshotCols, s.ph(1), s.ph(2)),
		cohortID, AnalysisSnapshotStatusCaptured,
	)
	snap, err := s.scanAnalysisCohortSnapshot(row)
	if err == nil {
		return snap, true
	}
	return AnalysisCohortSnapshot{}, false
}

// UpsertAnalysisCohortSnapshot inserts a new snapshot row or updates the
// existing row for the same (cohort_id, batch_id) pair. The natural key is
// the batch so projector rebuilds are idempotent.
func (s *SQLJobStore) UpsertAnalysisCohortSnapshot(snap AnalysisCohortSnapshot) (AnalysisCohortSnapshot, error) {
	if snap.CohortID == 0 {
		return AnalysisCohortSnapshot{}, errors.New("cohort_id is required")
	}
	if snap.BatchID == "" {
		return AnalysisCohortSnapshot{}, errors.New("batch_id is required")
	}
	if snap.Slug == "" {
		return AnalysisCohortSnapshot{}, errors.New("slug is required")
	}
	if snap.Status == "" {
		snap.Status = AnalysisSnapshotStatusPending
	}

	now := time.Now().UTC()
	existing, found := s.getAnalysisCohortSnapshotByBatch(snap.CohortID, snap.BatchID)
	if found {
		if snap.CreatedAt.IsZero() {
			snap.CreatedAt = existing.CreatedAt
		}
		if snap.UpdatedAt.IsZero() {
			snap.UpdatedAt = now
		}
		_, err := s.db.Exec(
			fmt.Sprintf(`UPDATE analysis_cohort_snapshots SET
				slug = %s,
				label = %s,
				description = %s,
				profile_id = %s,
				profile_name = %s,
				captured_at = %s,
				first_run_at = %s,
				last_run_at = %s,
				run_count = %s,
				domain_count = %s,
				status = %s,
				is_default = %s,
				is_public = %s,
				updated_at = %s
			WHERE id = %s`,
				s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
				s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10),
				s.ph(11), s.ph(12), s.ph(13), s.ph(14), s.ph(15),
			),
			snap.Slug,
			snap.Label,
			snap.Description,
			nullInt64Value(snap.ProfileID),
			snap.ProfileName,
			snapshotStoredTimestamp(snap.CapturedAt),
			snapshotStoredTimestamp(snap.FirstRunAt),
			snapshotStoredTimestamp(snap.LastRunAt),
			snap.RunCount,
			snap.DomainCount,
			snap.Status,
			boolToInt(snap.IsDefault),
			boolToInt(snap.IsPublic),
			s.ts(snap.UpdatedAt),
			existing.ID,
		)
		if err != nil {
			return AnalysisCohortSnapshot{}, fmt.Errorf("update cohort snapshot: %w", err)
		}
		updated, ok := s.GetAnalysisCohortSnapshot(existing.ID)
		if !ok {
			return AnalysisCohortSnapshot{}, errors.New("cohort snapshot updated but not readable")
		}
		return updated, nil
	}

	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = now
	}
	if snap.UpdatedAt.IsZero() {
		snap.UpdatedAt = snap.CreatedAt
	}
	_, err := s.db.Exec(
		fmt.Sprintf(`INSERT INTO analysis_cohort_snapshots (
			cohort_id, batch_id, slug, label, description,
			profile_id, profile_name, captured_at, first_run_at, last_run_at,
			run_count, domain_count, status, is_default, is_public,
			created_at, updated_at
		) VALUES (%s)`, s.phRange(1, 17)),
		snap.CohortID,
		snap.BatchID,
		snap.Slug,
		snap.Label,
		snap.Description,
		nullInt64Value(snap.ProfileID),
		snap.ProfileName,
		snapshotStoredTimestamp(snap.CapturedAt),
		snapshotStoredTimestamp(snap.FirstRunAt),
		snapshotStoredTimestamp(snap.LastRunAt),
		snap.RunCount,
		snap.DomainCount,
		snap.Status,
		boolToInt(snap.IsDefault),
		boolToInt(snap.IsPublic),
		s.ts(snap.CreatedAt),
		s.ts(snap.UpdatedAt),
	)
	if err != nil {
		return AnalysisCohortSnapshot{}, fmt.Errorf("insert cohort snapshot: %w", err)
	}

	created, ok := s.getAnalysisCohortSnapshotByBatch(snap.CohortID, snap.BatchID)
	if !ok {
		return AnalysisCohortSnapshot{}, errors.New("cohort snapshot inserted but not readable")
	}
	return created, nil
}

// ListSnapshotAggregates returns every pre-computed aggregate row attached to
// one snapshot, ordered by category for stable API payloads.
func (s *SQLJobStore) ListSnapshotAggregates(snapshotID int64) []AnalysisCohortSnapshotAggregate {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT snapshot_id, category, payload_json, computed_at
			FROM analysis_cohort_snapshot_aggregates
			WHERE snapshot_id = %s
			ORDER BY category ASC`, s.ph(1)),
		snapshotID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisCohortSnapshotAggregate
	for rows.Next() {
		var (
			agg        AnalysisCohortSnapshotAggregate
			computedAt string
		)
		if err := rows.Scan(&agg.SnapshotID, &agg.Category, &agg.PayloadJSON, &computedAt); err != nil {
			return nil
		}
		agg.ComputedAt = parseTimestampStr(computedAt)
		out = append(out, agg)
	}
	return out
}

// ReplaceSnapshotAggregates atomically swaps the aggregate rows for one
// snapshot. Writing is all-or-nothing so partial failures leave the prior set
// intact.
func (s *SQLJobStore) ReplaceSnapshotAggregates(snapshotID int64, aggs []AnalysisCohortSnapshotAggregate) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace aggregates: %w", err)
	}
	if _, err := tx.Exec(
		fmt.Sprintf(`DELETE FROM analysis_cohort_snapshot_aggregates WHERE snapshot_id = %s`, s.ph(1)),
		snapshotID,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete aggregates for snapshot %d: %w", snapshotID, err)
	}
	now := time.Now().UTC()
	for _, agg := range aggs {
		computedAt := agg.ComputedAt
		if computedAt.IsZero() {
			computedAt = now
		}
		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO analysis_cohort_snapshot_aggregates
				(snapshot_id, category, payload_json, computed_at)
				VALUES (%s)`, s.phRange(1, 4)),
			snapshotID, agg.Category, agg.PayloadJSON, s.ts(computedAt),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert aggregate %s for snapshot %d: %w", agg.Category, snapshotID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace aggregates: %w", err)
	}
	return nil
}
