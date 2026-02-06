package server

// updateJobWithMetrics persists a job update and records the status transition.
func (s *Server) updateJobWithMetrics(job Job) error {
	previous, ok := s.store.Get(job.ID)
	if err := s.store.Update(job); err != nil {
		return err
	}
	fromStatus := JobStatus("")
	if ok {
		fromStatus = previous.Status
	}
	s.metrics.ObserveJobStatusTransition(fromStatus, job.Status)
	return nil
}
