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
	tag_view_min_level,
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
		&snap.TagViewMinLevel,
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

// GetAnalysisCohortSnapshotByBatch looks up the (cohort, batch) natural-key
// row so UpsertAnalysisCohortSnapshot is idempotent per rebuild and the
// projector can check whether a snapshot for one batch already exists.
func (s *SQLJobStore) GetAnalysisCohortSnapshotByBatch(cohortID int64, batchID string) (AnalysisCohortSnapshot, bool) {
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

// GetDefaultSnapshotForCohort resolves the cohort's default snapshot using
// the catalog's policy. `pinned` honours default_snapshot_id and falls back
// to auto-latest when the pin is dangling or resolves to a non-public
// snapshot, so a stale pointer left behind by a wipe does not hide every
// remaining captured snapshot from the public path. `auto_latest` (and the
// fallback) picks the most recent captured public snapshot.
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
		if snap, found := s.GetAnalysisCohortSnapshot(*cohort.DefaultSnapshotID); found && isPublicSnapshot(snap) {
			return snap, true
		}
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
	existing, found := s.GetAnalysisCohortSnapshotByBatch(snap.CohortID, snap.BatchID)
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
				tag_view_min_level = %s,
				updated_at = %s
			WHERE id = %s`,
				s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
				s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10),
				s.ph(11), s.ph(12), s.ph(13), s.ph(14), s.ph(15),
				s.ph(16),
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
			snap.TagViewMinLevel,
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
			tag_view_min_level,
			created_at, updated_at
		) VALUES (%s)`, s.phRange(1, 18)),
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
		snap.TagViewMinLevel,
		s.ts(snap.CreatedAt),
		s.ts(snap.UpdatedAt),
	)
	if err != nil {
		return AnalysisCohortSnapshot{}, fmt.Errorf("insert cohort snapshot: %w", err)
	}

	created, ok := s.GetAnalysisCohortSnapshotByBatch(snap.CohortID, snap.BatchID)
	if !ok {
		return AnalysisCohortSnapshot{}, errors.New("cohort snapshot inserted but not readable")
	}
	return created, nil
}

// ListPendingAnalysisCohortSnapshots returns every snapshot still in pending
// state across all cohorts. Used by the capture poller to check whether each
// such snapshot's batch has finished so the snapshot can be promoted to
// captured.
func (s *SQLJobStore) ListPendingAnalysisCohortSnapshots() []AnalysisCohortSnapshot {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_snapshots
			WHERE status = %s
			ORDER BY cohort_id, created_at, id`,
			analysisCohortSnapshotCols, s.ph(1)),
		AnalysisSnapshotStatusPending,
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

// DeleteAnalysisCohortSnapshot hard-deletes one snapshot row and its
// view rows by id. Used by the admin purge action; a regular retire
// goes through UpsertAnalysisCohortSnapshot with status=retired and
// leaves the fact rows untouched for undo.
func (s *SQLJobStore) DeleteAnalysisCohortSnapshot(id int64) error {
	for _, table := range []string{
		"analysis_snapshot_overview_view",
		"analysis_snapshot_nameserver_view",
		"analysis_snapshot_endpoint_view",
		"analysis_snapshot_asn_view",
		"analysis_snapshot_tag_view",
		"analysis_snapshot_domain_view",
		"analysis_snapshot_prefix_view",
	} {
		if _, err := s.db.Exec(
			fmt.Sprintf(`DELETE FROM %s WHERE snapshot_id = %s`, table, s.ph(1)),
			id,
		); err != nil {
			return fmt.Errorf("delete %s for snapshot %d: %w", table, id, err)
		}
	}
	if _, err := s.db.Exec(
		fmt.Sprintf(`DELETE FROM analysis_cohort_snapshots WHERE id = %s`, s.ph(1)),
		id,
	); err != nil {
		return fmt.Errorf("delete snapshot %d: %w", id, err)
	}
	return nil
}

// ClearAnalysisCohortSnapshots removes all snapshot rows and their view
// rows for one cohort. Paired with ClearAnalysisCohortMaterialization so
// a cohort rebuild starts with no stale snapshots pointing at facts that
// were just deleted.
func (s *SQLJobStore) ClearAnalysisCohortSnapshots(cohortID int64) error {
	for _, table := range []string{
		"analysis_snapshot_overview_view",
		"analysis_snapshot_nameserver_view",
		"analysis_snapshot_endpoint_view",
		"analysis_snapshot_asn_view",
		"analysis_snapshot_tag_view",
		"analysis_snapshot_domain_view",
		"analysis_snapshot_prefix_view",
	} {
		if _, err := s.db.Exec(
			fmt.Sprintf(`DELETE FROM %s
				WHERE snapshot_id IN (
					SELECT id FROM analysis_cohort_snapshots WHERE cohort_id = %s
				)`, table, s.ph(1)),
			cohortID,
		); err != nil {
			return fmt.Errorf("clear %s for cohort %d: %w", table, cohortID, err)
		}
	}
	if _, err := s.db.Exec(
		fmt.Sprintf(`DELETE FROM analysis_cohort_snapshots WHERE cohort_id = %s`, s.ph(1)),
		cohortID,
	); err != nil {
		return fmt.Errorf("clear snapshots for cohort %d: %w", cohortID, err)
	}
	// A pinned default_snapshot_id now points at a row that no longer
	// exists, which would make GetDefaultSnapshotForCohort return false
	// even after the cohort captures a new snapshot. Revert to
	// auto_latest so the next captured snapshot resolves automatically.
	if _, err := s.db.Exec(
		fmt.Sprintf(`UPDATE analysis_cohort_catalog
			SET default_snapshot_policy = %s, default_snapshot_id = NULL
			WHERE id = %s`, s.ph(1), s.ph(2)),
		DefaultSnapshotPolicyAutoLatest, cohortID,
	); err != nil {
		return fmt.Errorf("reset default snapshot pin for cohort %d: %w", cohortID, err)
	}
	return nil
}

// CountBatchSnapshotRuns returns run/domain counts and the finished_at
// span for one (cohort, batch) snapshot.
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

// CountOutstandingJobsForBatch returns in-flight (queued/running/paused)
// jobs still attached to a batch. Terminal statuses left in the jobs
// table (e.g. an orphaned succeeded/failed row that never made it
// through GraduateJob cleanly) must not block snapshot capture.
func (s *SQLJobStore) CountOutstandingJobsForBatch(batchID string) (int, error) {
	var count int
	row := s.db.QueryRow(
		fmt.Sprintf(
			`SELECT COUNT(*) FROM jobs WHERE batch_id = %s AND status IN (%s, %s, %s)`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4),
		),
		batchID, string(JobQueued), string(JobRunning), string(JobPaused),
	)
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count outstanding jobs for batch %s: %w", batchID, err)
	}
	return count, nil
}

// CountUnprojectedSnapshotRuns returns completed runs in a
// snapshot-intent batch with no ready projection. Capture must wait for
// this to reach zero or aggregates would come from a partial fact set.
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
