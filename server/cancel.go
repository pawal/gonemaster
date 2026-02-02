package server

import "context"

func (s *Server) registerCancel(jobID string, cancel context.CancelFunc) {
	if jobID == "" || cancel == nil {
		return
	}
	s.cancelMu.Lock()
	if s.cancels == nil {
		s.cancels = map[string]context.CancelFunc{}
	}
	s.cancels[jobID] = cancel
	s.cancelMu.Unlock()
}

func (s *Server) unregisterCancel(jobID string) {
	if jobID == "" {
		return
	}
	s.cancelMu.Lock()
	delete(s.cancels, jobID)
	s.cancelMu.Unlock()
}

func (s *Server) cancelJob(jobID string) bool {
	if jobID == "" {
		return false
	}
	s.cancelMu.Lock()
	cancel := s.cancels[jobID]
	s.cancelMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}
