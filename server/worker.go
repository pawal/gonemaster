package server

import (
	"context"
	"log"
	"sync"
	"time"
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

	result := JobResult{
		JobID:   job.ID,
		BatchID: job.BatchID,
		Status:  JobSucceeded,
		Summary: map[string]any{
			"note": "job execution not implemented",
		},
	}

	job.Status = JobSucceeded
	job.Progress = 1
	job.FinishedAt = time.Now().UTC()
	if err := s.store.Update(job); err != nil {
		return err
	}
	if err := s.store.SetResult(job.ID, result); err != nil {
		return err
	}

	return nil
}
