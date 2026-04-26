package server

import (
	"fmt"
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
			return fmt.Errorf("graduate orphan job %s: %w", job.ID, err)
		}
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
