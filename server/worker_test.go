package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
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

func TestRunEngineForJobParallel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxConcurrentJobs = 0
	srv := New(cfg)

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		started <- struct{}{}
		<-release
		return nil, nil
	}

	job1 := Job{ID: "job-par-1", Domain: "example.com", Tests: []string{"basic01"}}
	job2 := Job{ID: "job-par-2", Domain: "example.net", Tests: []string{"basic01"}}

	errs := make(chan error, 2)
	go func() {
		_, _, _, err := srv.runEngineForJob(job1, context.Background())
		errs <- err
	}()
	go func() {
		_, _, _, err := srv.runEngineForJob(job2, context.Background())
		errs <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(250 * time.Millisecond):
			t.Fatalf("expected both runs to start in parallel, got %d", i)
		}
	}

	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestRunEngineForJobLimiter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxConcurrentJobs = 1
	srv := New(cfg)

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		started <- struct{}{}
		<-release
		return nil, nil
	}

	job1 := Job{ID: "job-cap-1", Domain: "example.com", Tests: []string{"basic01"}}
	job2 := Job{ID: "job-cap-2", Domain: "example.net", Tests: []string{"basic01"}}

	errs := make(chan error, 2)
	go func() {
		_, _, _, err := srv.runEngineForJob(job1, context.Background())
		errs <- err
	}()
	go func() {
		_, _, _, err := srv.runEngineForJob(job2, context.Background())
		errs <- err
	}()

	select {
	case <-started:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("expected first run to start")
	}
	select {
	case <-started:
		t.Fatalf("expected second run to be blocked by limiter")
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestDNSQueryCounterCallback(t *testing.T) {
	counter := &dnsQueryCounter{}
	entries := []*logger.Entry{
		{Tag: "EXTERNAL_QUERY", Args: map[string]any{"ip": "192.0.2.10"}},
		{Tag: "external_query", Args: map[string]any{"ip": "2001:db8::10"}},
		{Tag: "EXTERNAL_QUERY", Args: map[string]any{"ip": "not-an-ip"}},
		{Tag: "EXTERNAL_RESPONSE", Args: map[string]any{"ip": "198.51.100.20"}},
	}
	for _, entry := range entries {
		if err := counter.Callback(entry); err != nil {
			t.Fatalf("callback error: %v", err)
		}
	}
	ipv4, ipv6 := counter.Totals()
	if ipv4 != 1 {
		t.Fatalf("ipv4 = %d, want 1", ipv4)
	}
	if ipv6 != 1 {
		t.Fatalf("ipv6 = %d, want 1", ipv6)
	}
}
