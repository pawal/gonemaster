package server

import (
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

type progressTracker struct {
	server       *Server
	jobID        string
	total        int
	completed    int
	started      map[string]bool
	completedSet map[string]bool
	planned      map[string]bool
	mu           sync.Mutex
}

func (s *Server) newProgressTracker(jobID string, req engine.RunRequest) *progressTracker {
	planned, err := engine.PlannedTestcases(req)
	if err != nil {
		return nil
	}
	plannedSet := make(map[string]bool, len(planned))
	for _, name := range planned {
		key := normalizeTestcase(name)
		if key != "" {
			plannedSet[key] = true
		}
	}
	if len(plannedSet) == 0 {
		return nil
	}
	return &progressTracker{
		server:       s,
		jobID:        jobID,
		total:        len(plannedSet),
		started:      map[string]bool{},
		completedSet: map[string]bool{},
		planned:      plannedSet,
	}
}

func (p *progressTracker) Callback(entry *logger.Entry) error {
	if p == nil || entry == nil || p.total == 0 {
		return nil
	}

	testcaseKey := normalizeTestcase(entry.Testcase)
	if testcaseKey == "" {
		return nil
	}
	if p.planned != nil && !p.planned[testcaseKey] {
		return nil
	}

	switch entry.Tag {
	case "TEST_CASE_START":
		p.onStart(testcaseKey)
	case "TEST_CASE_END":
		p.onEnd(testcaseKey)
	default:
		p.onNonMarker(testcaseKey)
	}
	return nil
}

func (p *progressTracker) onStart(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started[key] {
		return
	}
	p.started[key] = true
	current := p.completed + 1
	if current > p.total {
		current = p.total
	}
	p.update(current)
}

func (p *progressTracker) onEnd(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.completedSet[key] {
		return
	}
	p.completedSet[key] = true
	if !p.started[key] {
		p.started[key] = true
	}
	p.completed++
	current := p.completed
	if current > p.total {
		current = p.total
	}
	p.update(current)
}

func (p *progressTracker) onNonMarker(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started[key] || p.completedSet[key] {
		return
	}
	p.started[key] = true
	p.completedSet[key] = true
	p.completed++
	current := p.completed
	if current > p.total {
		current = p.total
	}
	p.update(current)
}

func (p *progressTracker) update(current int) {
	percent := current * 100 / p.total
	p.server.updateJobProgress(p.jobID, percent)
}

func normalizeTestcase(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
