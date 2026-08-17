package server

import (
	"context"
	"fmt"
	"time"
)

// startStuckJobReaper sweeps for abandoned jobs on a ticker. The timeout is
// read from an atomic each tick, so a change via the settings API applies
// without a restart, and zero disables the sweep.
func (s *Server) startStuckJobReaper(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(stuckJobSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reapStuckJobs(time.Now().UTC())
			}
		}
	}()
}

// reapStuckJobs fails jobs left at "running" that no worker in this process
// is executing. It returns the number reaped.
//
// A job is only a candidate when it is absent from the cancel registry,
// which holds exactly the jobs this process is running. That makes the
// sweep safe for slow jobs and unsafe for two servers sharing one database:
// each would reap the other's live work.
func (s *Server) reapStuckJobs(now time.Time) int {
	timeout := s.cfg.EffectiveStuckJobTimeout()
	if timeout <= 0 {
		return 0
	}
	cutoff := now.Add(-timeout)

	running := s.store.List(JobFilter{Status: JobRunning, Limit: 10000})
	reaped := 0
	for _, job := range running.Items {
		if job.StartedAt.IsZero() || job.StartedAt.After(cutoff) {
			continue
		}
		if s.isJobLive(job.ID) {
			continue
		}
		job.Status = JobFailed
		job.Error = fmt.Sprintf("abandoned at running for over %s", timeout)
		job.FinishedAt = now
		if err := s.store.GraduateJob(job, nil); err != nil {
			s.logger.Error("reaping stuck job failed", "job_id", job.ID, "err", err)
			continue
		}
		s.metrics.ObserveJobStatusTransition(JobRunning, JobFailed)
		s.logger.Warn("reaped stuck job",
			"job_id", job.ID, "domain", job.Domain, "started_at", job.StartedAt)
		reaped++
	}
	return reaped
}

// isJobLive reports whether a worker in this process is running the job.
func (s *Server) isJobLive(jobID string) bool {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	_, ok := s.cancels[jobID]
	return ok
}
