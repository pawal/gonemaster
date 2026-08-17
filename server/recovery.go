package server

import (
	"fmt"
	"log/slog"
	"time"
)

// RecoverJobs is called at startup to handle an unclean previous shutdown.
// Running jobs and any orphan terminal-status rows in the jobs table are
// graduated as failed runs so the jobs table doesn't accumulate garbage
// that blocks snapshot capture; queued jobs are re-enqueued.
// In-memory stores are a no-op because their data does not survive restarts.
func RecoverJobs(store JobStore, queue Queue) error {
	s, ok := store.(*SQLJobStore)
	if !ok {
		return nil
	}

	now := time.Now().UTC()

	orphans := s.List(JobFilter{Limit: 100000})
	failed := 0
	for _, job := range orphans.Items {
		if job.Status == JobQueued || job.Status == JobPaused {
			continue
		}
		job.Status = JobFailed
		if job.Error == "" {
			job.Error = "server restarted during job"
		}
		if job.FinishedAt.IsZero() {
			job.FinishedAt = now
		}
		if err := s.GraduateJob(job, nil); err != nil {
			// Restarting is the fix for a stuck job, so one bad row must
			// not block startup.
			slog.Warn("graduate orphan job failed", "job_id", job.ID, "err", err)
			if updateErr := s.Update(job); updateErr != nil {
				slog.Error("orphan job left unrecovered", "job_id", job.ID, "err", updateErr)
			}
			failed++
		}
	}
	if failed > 0 {
		slog.Warn("startup recovery left orphan jobs ungraduated", "count", failed)
	}

	rows, err := s.db.Query(
		`SELECT id, priority FROM jobs WHERE status = 'queued' ORDER BY created_at ASC`,
	)
	if err != nil {
		return fmt.Errorf("query queued jobs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var priority int
		if err := rows.Scan(&id, &priority); err != nil {
			return fmt.Errorf("scan job id: %w", err)
		}
		if err := queue.Enqueue(id, JobPriority(priority)); err != nil {
			return fmt.Errorf("re-enqueue job %s: %w", id, err)
		}
	}
	return rows.Err()
}
