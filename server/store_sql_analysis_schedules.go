package server

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"codeberg.org/pawal/gonemaster/server/recurrence"
)

const analysisScheduleCols = `cohort_id, enabled, kind, interval_days, anchor_date, weekdays,
	days_of_month, last_day, time_of_day, timezone, profile_id, promote_default, catch_up,
	next_run_at, last_run_at, last_batch_id, last_outcome, last_error, created_at, updated_at`

func scanAnalysisCohortSchedule(row rowScanner) (AnalysisCohortSchedule, error) {
	var (
		sched                                      AnalysisCohortSchedule
		enabled, weekdays, days, lastDay           int
		promote, catchUp                           int
		kind, anchor, tod, nextRunAt, created, upd string
		profileID                                  sql.NullInt64
		lastRunAt                                  sql.NullString
	)
	err := row.Scan(&sched.CohortID, &enabled, &kind, &sched.Rule.IntervalDays, &anchor, &weekdays,
		&days, &lastDay, &tod, &sched.Rule.Zone, &profileID, &promote, &catchUp,
		&nextRunAt, &lastRunAt, &sched.LastBatchID, &sched.LastOutcome, &sched.LastError, &created, &upd)
	if err != nil {
		return AnalysisCohortSchedule{}, err
	}
	sched.Enabled = intToBool(enabled)
	sched.Rule.Kind = recurrence.Kind(kind)
	if anchor != "" {
		sched.Rule.AnchorDate, _ = recurrence.ParseDate(anchor)
	}
	sched.Rule.Weekdays = recurrence.WeekdaySet(weekdays)
	sched.Rule.DaysOfMonth = recurrence.DaySet(days)
	sched.Rule.LastDay = intToBool(lastDay)
	sched.Rule.TimeOfDay, _ = recurrence.ParseTimeOfDay(tod)
	sched.ProfileID = nullInt64Ptr(profileID)
	sched.PromoteDefault = intToBool(promote)
	sched.CatchUp = intToBool(catchUp)
	sched.NextRunAt = parseTimestampStr(nextRunAt)
	sched.LastRunAt = parseTimestampNullStr(lastRunAt)
	sched.CreatedAt = parseTimestampStr(created)
	sched.UpdatedAt = parseTimestampStr(upd)
	return sched, nil
}

func (s *SQLJobStore) queryAnalysisCohortSchedules(where string, args ...any) []AnalysisCohortSchedule {
	rows, err := s.db.Query(`SELECT `+analysisScheduleCols+` FROM analysis_cohort_schedules `+where, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AnalysisCohortSchedule
	for rows.Next() {
		sched, err := scanAnalysisCohortSchedule(rows)
		if err != nil {
			return out
		}
		out = append(out, sched)
	}
	return out
}

// GetAnalysisCohortSchedule returns the schedule of one cohort.
func (s *SQLJobStore) GetAnalysisCohortSchedule(cohortID int64) (AnalysisCohortSchedule, bool) {
	sched, err := scanAnalysisCohortSchedule(s.db.QueryRow(
		`SELECT `+analysisScheduleCols+` FROM analysis_cohort_schedules WHERE cohort_id = `+s.ph(1), cohortID))
	return sched, err == nil
}

// PutAnalysisCohortSchedule creates or replaces a schedule and keeps its last run.
func (s *SQLJobStore) PutAnalysisCohortSchedule(sched AnalysisCohortSchedule) (AnalysisCohortSchedule, error) {
	if sched.CohortID <= 0 || sched.NextRunAt.IsZero() {
		return AnalysisCohortSchedule{}, errors.New("cohort_id and next_run_at are required")
	}
	r := sched.Rule.Canonical()
	now := time.Now().UTC()
	args := []any{
		boolToInt(sched.Enabled), string(r.Kind), r.IntervalDays, r.AnchorDate.String(),
		int(r.Weekdays), int(r.DaysOfMonth), boolToInt(r.LastDay), r.TimeOfDay.String(), r.Zone,
		nullInt64Value(sched.ProfileID), boolToInt(sched.PromoteDefault), boolToInt(sched.CatchUp),
		s.ts(sched.NextRunAt), s.ts(now), sched.CohortID,
	}
	var err error
	if _, found := s.GetAnalysisCohortSchedule(sched.CohortID); found {
		_, err = s.db.Exec(fmt.Sprintf(`UPDATE analysis_cohort_schedules SET
			enabled = %s, kind = %s, interval_days = %s, anchor_date = %s, weekdays = %s,
			days_of_month = %s, last_day = %s, time_of_day = %s, timezone = %s, profile_id = %s,
			promote_default = %s, catch_up = %s, next_run_at = %s, updated_at = %s
			WHERE cohort_id = %s`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7), s.ph(8),
			s.ph(9), s.ph(10), s.ph(11), s.ph(12), s.ph(13), s.ph(14), s.ph(15)), args...)
	} else {
		_, err = s.db.Exec(fmt.Sprintf(`INSERT INTO analysis_cohort_schedules (
			enabled, kind, interval_days, anchor_date, weekdays, days_of_month, last_day,
			time_of_day, timezone, profile_id, promote_default, catch_up, next_run_at,
			updated_at, cohort_id, created_at, last_error)
			VALUES (%s, '')`, s.phRange(1, 16)), append(args, s.ts(now))...)
	}
	if err != nil {
		return AnalysisCohortSchedule{}, fmt.Errorf("put analysis cohort schedule: %w", err)
	}
	stored, ok := s.GetAnalysisCohortSchedule(sched.CohortID)
	if !ok {
		return AnalysisCohortSchedule{}, errors.New("analysis cohort schedule stored but not readable")
	}
	return stored, nil
}

// DeleteAnalysisCohortSchedule removes the schedule of one cohort.
func (s *SQLJobStore) DeleteAnalysisCohortSchedule(cohortID int64) error {
	if _, err := s.db.Exec(`DELETE FROM analysis_cohort_schedules WHERE cohort_id = `+s.ph(1), cohortID); err != nil {
		return fmt.Errorf("delete analysis cohort schedule: %w", err)
	}
	return nil
}

// ListAnalysisCohortSchedules returns every schedule by cohort id.
func (s *SQLJobStore) ListAnalysisCohortSchedules() []AnalysisCohortSchedule {
	return s.queryAnalysisCohortSchedules(`ORDER BY cohort_id`)
}

// ListDueAnalysisCohortSchedules returns enabled schedules due at now, oldest first.
func (s *SQLJobStore) ListDueAnalysisCohortSchedules(now time.Time) []AnalysisCohortSchedule {
	return s.queryAnalysisCohortSchedules(
		`WHERE enabled = 1 AND next_run_at <= `+s.ph(1)+` ORDER BY next_run_at, cohort_id`, s.ts(now))
}

// ClaimAnalysisCohortSchedule advances next_run_at only if it still equals expected.
func (s *SQLJobStore) ClaimAnalysisCohortSchedule(cohortID int64, expected, next time.Time) (bool, error) {
	res, err := s.db.Exec(fmt.Sprintf(`UPDATE analysis_cohort_schedules SET next_run_at = %s
		WHERE cohort_id = %s AND enabled = 1 AND next_run_at = %s`, s.ph(1), s.ph(2), s.ph(3)),
		s.ts(next), cohortID, s.ts(expected))
	if err != nil {
		return false, fmt.Errorf("claim analysis cohort schedule: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// RecordAnalysisCohortScheduleOutcome stores a firing; an empty batchID keeps the last one.
func (s *SQLJobStore) RecordAnalysisCohortScheduleOutcome(cohortID int64, ranAt time.Time, batchID, outcome, errText string) error {
	set := fmt.Sprintf(`last_run_at = %s, last_outcome = %s, last_error = %s`, s.ph(1), s.ph(2), s.ph(3))
	args := []any{s.ts(ranAt), outcome, errText}
	if batchID != "" {
		set += `, last_batch_id = ` + s.ph(len(args)+1)
		args = append(args, batchID)
	}
	args = append(args, cohortID)
	_, err := s.db.Exec(`UPDATE analysis_cohort_schedules SET `+set+` WHERE cohort_id = `+s.ph(len(args)), args...)
	if err != nil {
		return fmt.Errorf("record analysis cohort schedule outcome: %w", err)
	}
	return nil
}
