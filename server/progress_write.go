package server

import (
	"time"
)

const defaultProgressWriteMinStep = 5
const defaultProgressWriteMinInterval = 200 * time.Millisecond

type progressWriteState struct {
	lastPersisted   int
	lastPersistedAt time.Time
	maxSeen         int
}

func (s *Server) effectiveProgressWriteMinStep() int {
	if s.progressWriteMinStep < 1 {
		return 1
	}
	return s.progressWriteMinStep
}

func (s *Server) effectiveProgressWriteMinInterval() time.Duration {
	if s.progressWriteMinInterval < 0 {
		return 0
	}
	return s.progressWriteMinInterval
}

func (s *Server) initProgressWriteState(jobID string, progress int, now time.Time) {
	if s == nil || jobID == "" {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.progressWriteMu.Lock()
	s.progressWrites[jobID] = progressWriteState{
		lastPersisted:   progress,
		lastPersistedAt: now,
		maxSeen:         progress,
	}
	s.progressWriteMu.Unlock()
}

func (s *Server) clearProgressWriteState(jobID string) {
	if s == nil || jobID == "" {
		return
	}
	s.progressWriteMu.Lock()
	delete(s.progressWrites, jobID)
	s.progressWriteMu.Unlock()
}

func (s *Server) prepareProgressPersist(jobID string, progress int, now time.Time) bool {
	if s == nil || jobID == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	minStep := s.effectiveProgressWriteMinStep()
	minInterval := s.effectiveProgressWriteMinInterval()

	s.progressWriteMu.Lock()
	state := s.progressWrites[jobID]
	if progress <= state.maxSeen {
		s.progressWriteMu.Unlock()
		return false
	}
	state.maxSeen = progress

	shouldPersist := false
	if progress >= 100 {
		shouldPersist = true
	} else if progress-state.lastPersisted >= minStep {
		shouldPersist = true
	} else if minInterval == 0 {
		shouldPersist = true
	} else if !state.lastPersistedAt.IsZero() && now.Sub(state.lastPersistedAt) >= minInterval {
		shouldPersist = true
	}

	if shouldPersist {
		state.lastPersisted = progress
		state.lastPersistedAt = now
	}
	s.progressWrites[jobID] = state
	s.progressWriteMu.Unlock()

	return shouldPersist
}
