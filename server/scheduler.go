package server

import (
	"context"
	"runtime/debug"
	"time"
)

// scheduleTickInterval is how often due schedules are checked.
const scheduleTickInterval = time.Minute

// missedRunGrace is how late a firing may run when catch-up is off.
const missedRunGrace = time.Hour

// startScheduleLoop fires due cohort schedules now and on every tick.
func (s *Server) startScheduleLoop(ctx context.Context) {
	s.runScheduleLoop(ctx, scheduleTickInterval)
}

// runScheduleLoop is the loop core with a test-settable interval.
func (s *Server) runScheduleLoop(ctx context.Context, interval time.Duration) {
	go func() {
		s.scheduleTick(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.scheduleTick(ctx)
			}
		}
	}()
}

// scheduleTick runs one pass and logs a panic instead of dying.
func (s *Server) scheduleTick(ctx context.Context) {
	defer func() {
		if p := recover(); p != nil {
			s.logger.Error("scheduler tick panicked", "panic", p, "stack", string(debug.Stack()))
		}
	}()
	s.runDueSchedules(ctx, time.Now().UTC())
}

// runDueSchedules fires every schedule due at now.
func (s *Server) runDueSchedules(ctx context.Context, now time.Time) {
	if !s.schedulerEnabled.Load() {
		return
	}
	for _, sched := range s.store.ListDueAnalysisCohortSchedules(now) {
		if ctx.Err() != nil {
			return
		}
		s.fireSchedule(ctx, sched, now)
	}
}

// fireSchedule claims one due occurrence and records what came of it.
func (s *Server) fireSchedule(ctx context.Context, sched AnalysisCohortSchedule, now time.Time) {
	// From now, not from the stale next_run_at, so downtime yields one run.
	next, err := sched.Rule.Next(now)
	if err != nil {
		s.disableBrokenSchedule(sched, now, err)
		return
	}
	claimed, err := s.store.ClaimAnalysisCohortSchedule(sched.CohortID, sched.NextRunAt, next)
	if err != nil {
		s.logger.Warn("scheduled run claim failed", "cohort_id", sched.CohortID, "err", err)
		return
	}
	if !claimed {
		return
	}
	cohort, _ := s.store.GetAnalysisCohort(sched.CohortID)
	outcome, batchID, errText := s.scheduledRun(ctx, sched, cohort, now)
	if err := s.store.RecordAnalysisCohortScheduleOutcome(sched.CohortID, now, batchID, outcome, errText); err != nil {
		s.logger.Warn("scheduled run outcome not stored", "cohort_id", sched.CohortID, "err", err)
	}
	s.metrics.ObserveScheduledRun(outcome)
	attrs := []any{"cohort_id", sched.CohortID, "tag", cohort.SourceTag, "outcome", outcome,
		"batch_id", batchID, "next_run_at", next.Format(time.RFC3339)}
	if outcome == ScheduleOutcomeError {
		s.logger.Warn("scheduled run", append(attrs, "err", errText)...)
		return
	}
	s.logger.Info("scheduled run", attrs...)
}

// scheduledRun decides the outcome of a claimed firing and submits the batch.
func (s *Server) scheduledRun(ctx context.Context, sched AnalysisCohortSchedule, cohort AnalysisCohort, now time.Time) (outcome, batchID, errText string) {
	if !sched.CatchUp && now.Sub(sched.NextRunAt) > missedRunGrace {
		return ScheduleOutcomeSkippedMissed, "", ""
	}
	if cohort.ID == 0 || !cohort.AnalysisEnabled {
		return ScheduleOutcomeSkippedDisabled, "", ""
	}
	if sched.LastBatchID != "" {
		n, err := s.store.CountOutstandingJobsForBatch(sched.LastBatchID)
		if err != nil {
			return ScheduleOutcomeError, "", err.Error()
		}
		if n > 0 {
			return ScheduleOutcomeSkippedActive, "", ""
		}
	}
	if ctx.Err() != nil {
		return ScheduleOutcomeError, "", "server stopping"
	}
	resp, rerr := s.submitBatch(JobBatchRequest{
		FromTag:                cohort.SourceTag,
		ProfileID:              sched.ProfileID,
		SnapshotIntent:         true,
		PromoteSnapshotDefault: sched.PromoteDefault,
	}, BatchOriginSchedule)
	switch {
	case rerr == nil:
		return ScheduleOutcomeSubmitted, resp.BatchID, ""
	case rerr.code == "missing_domain":
		return ScheduleOutcomeSkippedEmpty, "", ""
	default:
		return ScheduleOutcomeError, "", rerr.message
	}
}

// disableBrokenSchedule turns off a stored rule that no longer computes.
func (s *Server) disableBrokenSchedule(sched AnalysisCohortSchedule, now time.Time, cause error) {
	sched.Enabled = false
	if _, err := s.store.PutAnalysisCohortSchedule(sched); err != nil {
		s.logger.Warn("broken schedule not disabled", "cohort_id", sched.CohortID, "err", err)
		return
	}
	_ = s.store.RecordAnalysisCohortScheduleOutcome(sched.CohortID, now, "", ScheduleOutcomeError, cause.Error())
	s.metrics.ObserveScheduledRun(ScheduleOutcomeError)
	s.logger.Warn("schedule disabled", "cohort_id", sched.CohortID, "err", cause)
}
