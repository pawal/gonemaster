package server

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

type workerPool struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (s *Server) Start() {
	if s.workers.ctx != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.workers.ctx = ctx
	s.workers.cancel = cancel

	workerCount := s.cfg.WorkerCount
	if workerCount < 1 {
		workerCount = 1
	}
	for i := 0; i < workerCount; i++ {
		s.workers.wg.Add(1)
		go s.workerLoop(i)
	}
}

func (s *Server) Stop(ctx context.Context) error {
	if s.workers.cancel == nil {
		return nil
	}
	s.workers.cancel()
	_ = s.queue.Close()

	done := make(chan struct{})
	go func() {
		s.workers.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) workerLoop(id int) {
	defer s.workers.wg.Done()
	for {
		jobID, err := s.queue.Dequeue(s.workers.ctx)
		if err != nil {
			return
		}
		if err := s.runJob(jobID); err != nil && s.cfg.Debug {
			log.Printf("worker %d: job %s error: %v", id, jobID, err)
		}
	}
}

func (s *Server) runJob(jobID string) error {
	job, ok := s.store.Get(jobID)
	if !ok {
		return nil
	}
	if job.Status == JobCanceled || job.Status == JobPaused {
		return nil
	}

	now := time.Now().UTC()
	job.Status = JobRunning
	job.StartedAt = now
	job.Progress = 0
	if err := s.store.Update(job); err != nil {
		return err
	}

	entries, runErr := s.runEngineForJob(job)
	finishedAt := time.Now().UTC()

	result := JobResult{
		JobID:   job.ID,
		BatchID: job.BatchID,
		Status:  JobSucceeded,
		Summary: summarizeEntries(entries),
		Raw: map[string]any{
			"entries": entries,
		},
	}

	if runErr != nil {
		job.Status = JobFailed
		job.Error = runErr.Error()
		result.Status = JobFailed
		result.Summary = map[string]any{
			"error": runErr.Error(),
		}
		if len(entries) > 0 {
			result.Raw = map[string]any{
				"entries": entries,
			}
		}
	} else {
		job.Status = JobSucceeded
	}

	job.Progress = 1
	job.FinishedAt = finishedAt
	if err := s.store.Update(job); err != nil {
		return err
	}
	if err := s.store.SetResult(job.ID, result); err != nil {
		return err
	}

	return runErr
}

func (s *Server) runEngineForJob(job Job) ([]engine.LogEntry, error) {
	req := engine.RunRequest{
		Domain:   job.Domain,
		MinLevel: s.cfg.MinLevel,
	}

	cleanup, err := applyProfileOverrides(&req, job.Overrides)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	if len(job.Tests) == 1 {
		req.Testcase = job.Tests[0]
		return s.runEngine(req)
	}
	if len(job.Tests) == 0 {
		return s.runEngine(req)
	}

	var all []engine.LogEntry
	for _, testcase := range job.Tests {
		runReq := req
		runReq.Testcase = testcase
		entries, err := s.runEngine(runReq)
		if err != nil {
			return all, err
		}
		all = append(all, entries...)
	}
	return all, nil
}

func (s *Server) runEngine(req engine.RunRequest) ([]engine.LogEntry, error) {
	s.engineMu.Lock()
	defer s.engineMu.Unlock()
	return engine.Run(req)
}

func summarizeEntries(entries []engine.LogEntry) map[string]any {
	if len(entries) == 0 {
		return map[string]any{
			"total":  0,
			"levels": map[string]int{},
		}
	}
	levels := map[string]int{}
	for _, entry := range entries {
		levels[entry.Level]++
	}
	return map[string]any{
		"total":  len(entries),
		"levels": levels,
	}
}

func applyProfileOverrides(req *engine.RunRequest, overrides map[string]any) (func(), error) {
	if req == nil || len(overrides) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(overrides)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "gonemaster-profile-*.json")
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	req.Profile = tmp.Name()
	return func() { _ = os.Remove(tmp.Name()) }, nil
}
