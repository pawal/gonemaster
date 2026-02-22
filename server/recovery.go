package server

import "fmt"

// RecoverJobs is called at startup to handle an unclean previous shutdown.
// Jobs left in "running" state are transitioned to "failed". Jobs still
// "queued" are re-enqueued so workers can pick them up.
//
// For in-memory stores this is a no-op because data does not survive restarts.
func RecoverJobs(store JobStore, queue Queue) error {
	s, ok := store.(*SQLJobStore)
	if !ok {
		return nil
	}

	// Mark any jobs that were running as failed.
	if _, err := s.db.Exec(
		`UPDATE jobs SET status = 'failed', error = 'server restarted during job'
		 WHERE status = 'running'`,
	); err != nil {
		return fmt.Errorf("recover running jobs: %w", err)
	}

	// Re-enqueue jobs that were queued but never started.
	rows, err := s.db.Query(
		`SELECT id FROM jobs WHERE status = 'queued' ORDER BY created_at ASC`,
	)
	if err != nil {
		return fmt.Errorf("query queued jobs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan job id: %w", err)
		}
		if err := queue.Enqueue(id); err != nil {
			return fmt.Errorf("re-enqueue job %s: %w", id, err)
		}
	}
	return rows.Err()
}
