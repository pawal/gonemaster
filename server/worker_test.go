package server

import (
	"sync"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

type spyJobStore struct {
	mu         sync.Mutex
	inner      *InMemoryJobStore
	progresses []int
}

func newSpyJobStore() *spyJobStore {
	return &spyJobStore{inner: NewInMemoryJobStore()}
}

func (s *spyJobStore) Create(job Job) (Job, error) {
	return s.inner.Create(job)
}

func (s *spyJobStore) Get(id string) (Job, bool) {
	return s.inner.Get(id)
}

func (s *spyJobStore) Update(job Job) error {
	s.mu.Lock()
	s.progresses = append(s.progresses, job.Progress)
	s.mu.Unlock()
	return s.inner.Update(job)
}

func (s *spyJobStore) List(filter JobFilter) JobList {
	return s.inner.List(filter)
}

func (s *spyJobStore) SetResult(jobID string, result JobResult) error {
	return s.inner.SetResult(jobID, result)
}

func (s *spyJobStore) GetResult(jobID string) (JobResult, bool) {
	return s.inner.GetResult(jobID)
}

func (s *spyJobStore) Progresses() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int, len(s.progresses))
	copy(out, s.progresses)
	return out
}

func TestProgressUpdatesForMultipleTests(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy

	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	job := Job{
		ID:        "job-progress",
		Domain:    "example.com",
		Tests:     []string{"t1", "t2", "t3"},
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
		Progress:  0,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := srv.runJob(job.ID); err != nil {
		t.Fatalf("run job: %v", err)
	}

	progresses := spy.Progresses()
	seen := map[int]bool{}
	for _, value := range progresses {
		seen[value] = true
	}
	if !seen[33] || !seen[67] || !seen[100] {
		t.Fatalf("expected progress updates including 33, 67, 100, got %v", progresses)
	}
}
