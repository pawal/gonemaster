package server

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const analysisCohortCols = `id, source_type, source_tag, label, description,
	analysis_enabled, public_enabled, is_default, sort_order, materialization_status,
	materialization_done, materialization_total,
	last_materialized_at, last_materialization_error, created_at, updated_at`

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intToBool(v int) bool { return v != 0 }

func (s *SQLJobStore) scanAnalysisCohort(row rowScanner) (AnalysisCohort, error) {
	var (
		cohort                 AnalysisCohort
		analysisEnabled        int
		publicEnabled          int
		isDefault              int
		lastMaterializedAt     sql.NullString
		lastMaterializationErr string
		createdAt              string
		updatedAt              string
	)
	if err := row.Scan(
		&cohort.ID,
		&cohort.SourceType,
		&cohort.SourceTag,
		&cohort.Label,
		&cohort.Description,
		&analysisEnabled,
		&publicEnabled,
		&isDefault,
		&cohort.SortOrder,
		&cohort.MaterializationStatus,
		&cohort.MaterializationDone,
		&cohort.MaterializationTotal,
		&lastMaterializedAt,
		&lastMaterializationErr,
		&createdAt,
		&updatedAt,
	); err != nil {
		return AnalysisCohort{}, err
	}
	cohort.AnalysisEnabled = intToBool(analysisEnabled)
	cohort.PublicEnabled = intToBool(publicEnabled)
	cohort.IsDefault = intToBool(isDefault)
	cohort.LastMaterializedAt = parseTimestampNullStr(lastMaterializedAt)
	cohort.LastMaterializationError = lastMaterializationErr
	cohort.CreatedAt = parseTimestampStr(createdAt)
	cohort.UpdatedAt = parseTimestampStr(updatedAt)
	return cohort, nil
}

// GetAnalysisCohort returns one analysis cohort catalog entry by numeric id.
func (s *SQLJobStore) GetAnalysisCohort(id int64) (AnalysisCohort, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_catalog WHERE id = %s`, analysisCohortCols, s.ph(1)),
		id,
	)
	cohort, err := s.scanAnalysisCohort(row)
	if err == nil {
		return cohort, true
	}
	return AnalysisCohort{}, false
}

// GetAnalysisCohortBySource returns one analysis cohort catalog entry by source key.
func (s *SQLJobStore) GetAnalysisCohortBySource(sourceType, sourceTag string) (AnalysisCohort, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_catalog
			WHERE source_type = %s AND source_tag = %s`,
			analysisCohortCols, s.ph(1), s.ph(2)),
		sourceType, sourceTag,
	)
	cohort, err := s.scanAnalysisCohort(row)
	if err == nil {
		return cohort, true
	}
	return AnalysisCohort{}, false
}

// ListAnalysisCohorts returns all analysis cohort catalog entries ordered for UI use.
func (s *SQLJobStore) ListAnalysisCohorts() []AnalysisCohort {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_cohort_catalog
			ORDER BY sort_order ASC, label ASC, source_tag ASC`, analysisCohortCols),
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisCohort
	for rows.Next() {
		cohort, err := s.scanAnalysisCohort(rows)
		if err != nil {
			return nil
		}
		out = append(out, cohort)
	}
	return out
}

// UpsertAnalysisCohort inserts a new cohort catalog row or updates the existing
// row for the same source_type/source_tag pair.
func (s *SQLJobStore) UpsertAnalysisCohort(cohort AnalysisCohort) (AnalysisCohort, error) {
	if cohort.SourceType == "" {
		return AnalysisCohort{}, errors.New("source_type is required")
	}
	if cohort.SourceTag == "" {
		return AnalysisCohort{}, errors.New("source_tag is required")
	}
	if cohort.Label == "" {
		cohort.Label = cohort.SourceTag
	}
	if cohort.MaterializationStatus == "" {
		cohort.MaterializationStatus = AnalysisMaterializationPending
	}

	now := time.Now().UTC()
	existing, found := s.GetAnalysisCohortBySource(cohort.SourceType, cohort.SourceTag)
	if found {
		if cohort.CreatedAt.IsZero() {
			cohort.CreatedAt = existing.CreatedAt
		}
		if cohort.UpdatedAt.IsZero() {
			cohort.UpdatedAt = now
		}
		_, err := s.db.Exec(
			fmt.Sprintf(`UPDATE analysis_cohort_catalog SET
				label = %s,
				description = %s,
				analysis_enabled = %s,
				public_enabled = %s,
				is_default = %s,
				sort_order = %s,
				materialization_status = %s,
				materialization_done = %s,
				materialization_total = %s,
				last_materialized_at = %s,
				last_materialization_error = %s,
				updated_at = %s
			WHERE id = %s`,
				s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
				s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10), s.ph(11), s.ph(12), s.ph(13),
			),
			cohort.Label,
			cohort.Description,
			boolToInt(cohort.AnalysisEnabled),
			boolToInt(cohort.PublicEnabled),
			boolToInt(cohort.IsDefault),
			cohort.SortOrder,
			cohort.MaterializationStatus,
			cohort.MaterializationDone,
			cohort.MaterializationTotal,
			s.ts(cohort.LastMaterializedAt),
			cohort.LastMaterializationError,
			s.ts(cohort.UpdatedAt),
			existing.ID,
		)
		if err != nil {
			return AnalysisCohort{}, fmt.Errorf("update analysis cohort: %w", err)
		}
		updated, ok := s.GetAnalysisCohort(existing.ID)
		if !ok {
			return AnalysisCohort{}, errors.New("analysis cohort updated but not readable")
		}
		return updated, nil
	}

	if cohort.CreatedAt.IsZero() {
		cohort.CreatedAt = now
	}
	if cohort.UpdatedAt.IsZero() {
		cohort.UpdatedAt = cohort.CreatedAt
	}
	_, err := s.db.Exec(
		fmt.Sprintf(`INSERT INTO analysis_cohort_catalog (
			source_type, source_tag, label, description, analysis_enabled,
			public_enabled, is_default, sort_order, materialization_status,
			materialization_done, materialization_total,
			last_materialized_at, last_materialization_error, created_at, updated_at
		) VALUES (%s)`, s.phRange(1, 15)),
		cohort.SourceType,
		cohort.SourceTag,
		cohort.Label,
		cohort.Description,
		boolToInt(cohort.AnalysisEnabled),
		boolToInt(cohort.PublicEnabled),
		boolToInt(cohort.IsDefault),
		cohort.SortOrder,
		cohort.MaterializationStatus,
		cohort.MaterializationDone,
		cohort.MaterializationTotal,
		s.ts(cohort.LastMaterializedAt),
		cohort.LastMaterializationError,
		s.ts(cohort.CreatedAt),
		s.ts(cohort.UpdatedAt),
	)
	if err != nil {
		return AnalysisCohort{}, fmt.Errorf("insert analysis cohort: %w", err)
	}

	created, ok := s.GetAnalysisCohortBySource(cohort.SourceType, cohort.SourceTag)
	if !ok {
		return AnalysisCohort{}, errors.New("analysis cohort inserted but not readable")
	}
	return created, nil
}

// SetAnalysisCohortProgress writes just the materialization_done /
// materialization_total counters for one cohort. Called often during a
// rebuild so the admin UI can poll and render a progress bar; kept as a
// narrow UPDATE so each progress bump is a single column write, not a
// full cohort row rewrite.
func (s *SQLJobStore) SetAnalysisCohortProgress(id int64, done, total int) error {
	_, err := s.db.Exec(
		fmt.Sprintf(`UPDATE analysis_cohort_catalog SET
			materialization_done = %s,
			materialization_total = %s
		WHERE id = %s`, s.ph(1), s.ph(2), s.ph(3)),
		done, total, id,
	)
	if err != nil {
		return fmt.Errorf("update analysis cohort progress: %w", err)
	}
	return nil
}

// DeleteAnalysisCohort removes one cohort catalog row and any materialized data
// attached to it.
func (s *SQLJobStore) DeleteAnalysisCohort(id int64) error {
	if err := s.ClearAnalysisCohortMaterialization(id); err != nil {
		return err
	}
	_, err := s.db.Exec(
		fmt.Sprintf(`DELETE FROM analysis_cohort_catalog WHERE id = %s`, s.ph(1)),
		id,
	)
	if err != nil {
		return fmt.Errorf("delete analysis cohort: %w", err)
	}
	return nil
}
