package server

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
