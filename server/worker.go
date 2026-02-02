package server

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
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

	jobCtx, cancel := context.WithCancel(context.Background())
	s.registerCancel(jobID, cancel)
	defer func() {
		s.unregisterCancel(jobID)
		cancel()
	}()

	now := time.Now().UTC()
	job.Status = JobRunning
	job.StartedAt = now
	job.Progress = 0
	if err := s.store.Update(job); err != nil {
		return err
	}

	entries, runErr := s.runEngineForJob(job, jobCtx)
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

	if jobCtx.Err() != nil {
		job.Status = JobCanceled
		job.Error = "canceled"
		result.Status = JobCanceled
		result.Summary = map[string]any{
			"error": "canceled",
		}
	} else if runErr != nil {
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

	job.Progress = 100
	job.FinishedAt = finishedAt
	if err := s.store.Update(job); err != nil {
		return err
	}
	if err := s.store.SetResult(job.ID, result); err != nil {
		return err
	}

	return runErr
}

func (s *Server) runEngineForJob(job Job, ctx context.Context) ([]engine.LogEntry, error) {
	minLevel := s.cfg.MinLevel
	if job.MinLevel != "" {
		minLevel = job.MinLevel
	}
	req := engine.RunRequest{
		Domain:   job.Domain,
		MinLevel: minLevel,
		Context:  ctx,
	}

	cleanup, err := applyProfileOverrides(&req, job.Overrides, s.cfg.ProfilePath)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	if len(job.Tests) == 0 {
		if tracker := s.newProgressTracker(job.ID, req); tracker != nil {
			req.LogCallback = tracker.Callback
		}
	}

	if len(job.Tests) == 1 {
		req.Testcase = job.Tests[0]
		return s.runEngine(req)
	}
	if len(job.Tests) == 0 {
		return s.runEngine(req)
	}

	var all []engine.LogEntry
	total := len(job.Tests)
	for i, testcase := range job.Tests {
		runReq := req
		runReq.Testcase = testcase
		entries, err := s.runEngine(runReq)
		if total > 0 {
			done := i + 1
			progress := int(math.Round((float64(done) / float64(total)) * 100))
			s.updateJobProgress(job.ID, progress)
		}
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
	if s.engineRunner != nil {
		return s.engineRunner(req)
	}
	return engine.Run(req)
}

func (s *Server) updateJobProgress(jobID string, progress int) {
	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}
	job, ok := s.store.Get(jobID)
	if !ok {
		return
	}
	if job.Progress >= progress {
		return
	}
	job.Progress = progress
	_ = s.store.Update(job)
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

func applyProfileOverrides(req *engine.RunRequest, overrides map[string]any, baseProfile string) (func(), error) {
	if req == nil || len(overrides) == 0 {
		if req != nil && baseProfile != "" {
			req.Profile = baseProfile
		}
		return nil, nil
	}
	payload, err := json.Marshal(overrides)
	if err != nil {
		return nil, err
	}
	base := profile.New()
	if baseProfile != "" {
		data, err := os.ReadFile(baseProfile)
		if err != nil {
			return nil, err
		}
		base, err = profile.FromYAML(string(data))
		if err != nil {
			return nil, err
		}
	}
	overrideProfile, err := profile.FromJSON(string(payload))
	if err != nil {
		return nil, err
	}
	if err := base.Merge(overrideProfile); err != nil {
		return nil, err
	}
	merged, err := base.ToJSON()
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "gonemaster-profile-*.json")
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Write([]byte(merged)); err != nil {
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
