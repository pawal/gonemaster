package server

import "time"

func (s *Server) updateJobWithMetricsTransition(job Job) (JobStatus, JobStatus, bool, error) {
	previous, ok := s.store.Get(job.ID)
	if err := s.store.Update(job); err != nil {
		return "", "", false, err
	}
	fromStatus := JobStatus("")
	if ok {
		fromStatus = previous.Status
	}
	toStatus := job.Status
	s.metrics.ObserveJobStatusTransition(fromStatus, toStatus)
	becameTerminal := !isTerminalMetricsStatus(fromStatus) && isTerminalMetricsStatus(toStatus)
	return fromStatus, toStatus, becameTerminal, nil
}

// observeCancelGraduation reports the metrics for a job graduated
// straight to a terminal status without passing through a worker.
func (s *Server) observeCancelGraduation(job Job, fromStatus JobStatus) {
	s.metrics.ObserveJobStatusTransition(fromStatus, job.Status)
	if isTerminalMetricsStatus(fromStatus) || !isTerminalMetricsStatus(job.Status) {
		return
	}
	duration := time.Duration(-1)
	if !job.StartedAt.IsZero() && !job.FinishedAt.IsZero() {
		duration = job.FinishedAt.Sub(job.StartedAt)
	}
	s.metrics.ObserveJobCompletionWithContext(job.BatchID, job.Domain, job.Status, duration, zeroMetricsSeverityTotals())
}
