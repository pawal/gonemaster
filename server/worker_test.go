package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logargs"
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

func (s *spyJobStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	return s.inner.PurgeOlderThan(cutoff)
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

func TestUpdateJobProgressCoalescesSmallIncrements(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy
	srv.progressWriteMinStep = 10
	srv.progressWriteMinInterval = time.Hour

	job := Job{
		ID:        "job-progress-coalesce",
		Domain:    "example.com",
		Status:    JobRunning,
		CreatedAt: time.Now().UTC(),
		Progress:  0,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	srv.initProgressWriteState(job.ID, 0, time.Now().UTC())
	defer srv.clearProgressWriteState(job.ID)

	for progress := 1; progress < 10; progress++ {
		srv.updateJobProgress(job.ID, progress)
	}
	if got := spy.Progresses(); len(got) != 0 {
		t.Fatalf("expected no persisted progress before threshold, got %v", got)
	}

	srv.updateJobProgress(job.ID, 10)
	progresses := spy.Progresses()
	if len(progresses) != 1 || progresses[0] != 10 {
		t.Fatalf("expected one persisted progress update [10], got %v", progresses)
	}
}

func TestUpdateJobProgressAlwaysPersistsTerminal100(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy
	srv.progressWriteMinStep = 200
	srv.progressWriteMinInterval = time.Hour

	job := Job{
		ID:        "job-progress-terminal",
		Domain:    "example.com",
		Status:    JobRunning,
		CreatedAt: time.Now().UTC(),
		Progress:  0,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	srv.initProgressWriteState(job.ID, 0, time.Now().UTC())
	defer srv.clearProgressWriteState(job.ID)

	srv.updateJobProgress(job.ID, 99)
	if got := spy.Progresses(); len(got) != 0 {
		t.Fatalf("expected no persisted progress before terminal update, got %v", got)
	}

	srv.updateJobProgress(job.ID, 100)
	progresses := spy.Progresses()
	if len(progresses) != 1 || progresses[0] != 100 {
		t.Fatalf("expected terminal progress update [100], got %v", progresses)
	}
	jobAfter, ok := srv.store.Get(job.ID)
	if !ok {
		t.Fatalf("expected stored job")
	}
	if jobAfter.Progress != 100 {
		t.Fatalf("expected stored progress 100, got %d", jobAfter.Progress)
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
		_, _, err := srv.runEngineForJob(job1, context.Background())
		errs <- err
	}()
	go func() {
		_, _, err := srv.runEngineForJob(job2, context.Background())
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
		_, _, err := srv.runEngineForJob(job1, context.Background())
		errs <- err
	}()
	go func() {
		_, _, err := srv.runEngineForJob(job2, context.Background())
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

func TestRunEngineForJobPassesUndelegatedInputs(t *testing.T) {
	cfg := DefaultConfig()
	srv := New(cfg)

	var captured engine.RunRequest
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}

	job := Job{
		ID:        "job-undel",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
		UndelegatedNS: []engine.UndelegatedNameserver{
			{Name: "ns1.example.com", IP: "192.0.2.1"},
			{Name: "ns1.example.com", IP: "2001:db8::1"},
		},
		UndelegatedDS: []engine.UndelegatedDSInfo{
			{KeyTag: 12345, Algorithm: 13, DigestType: 2, Digest: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		},
	}

	_, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if len(captured.UndelegatedNameservers) != 2 {
		t.Fatalf("expected 2 undelegated nameservers, got %d", len(captured.UndelegatedNameservers))
	}
	if captured.UndelegatedNameservers[0].Name != "ns1.example.com" || captured.UndelegatedNameservers[0].IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver: %+v", captured.UndelegatedNameservers[0])
	}
	if captured.UndelegatedNameservers[1].Name != "ns1.example.com" || captured.UndelegatedNameservers[1].IP != "2001:db8::1" {
		t.Fatalf("unexpected second nameserver: %+v", captured.UndelegatedNameservers[1])
	}
	if len(captured.UndelegatedDSInfo) != 1 {
		t.Fatalf("expected 1 undelegated DS record, got %d", len(captured.UndelegatedDSInfo))
	}
	if captured.UndelegatedDSInfo[0].KeyTag != 12345 {
		t.Fatalf("unexpected DS key tag: %d", captured.UndelegatedDSInfo[0].KeyTag)
	}
}

func TestRunEngineForJobPassesSourceAddrOverrides(t *testing.T) {
	cfg := DefaultConfig()
	source4 := "192.0.2.70"
	source6 := "2001:db8::70"
	cfg.SourceAddr4 = &source4
	cfg.SourceAddr6 = &source6
	srv := New(cfg)

	var captured engine.RunRequest
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}

	job := Job{
		ID:        "job-sourceaddr",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}

	_, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if captured.SourceAddr4 == nil || *captured.SourceAddr4 != source4 {
		t.Fatalf("expected SourceAddr4 %q, got %#v", source4, captured.SourceAddr4)
	}
	if captured.SourceAddr6 == nil || *captured.SourceAddr6 != source6 {
		t.Fatalf("expected SourceAddr6 %q, got %#v", source6, captured.SourceAddr6)
	}
}

func TestDNSQueryCounterCallback(t *testing.T) {
	counter := &dnsQueryCounter{}
	entries := []*logger.Entry{
		{Tag: "EXTERNAL_QUERY", Args: map[string]any{logargs.KeyAddress: "192.0.2.10"}},
		{Tag: "external_query", Args: map[string]any{logargs.KeyAddress: "2001:db8::10"}},
		{Tag: "EXTERNAL_QUERY", Args: map[string]any{logargs.KeyAddress: "not-an-ip"}},
		{Tag: "EXTERNAL_RESPONSE", Args: map[string]any{logargs.KeyAddress: "198.51.100.20"}},
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

func TestRunEngineForJobPassesCacheStore(t *testing.T) {
	cfg := DefaultConfig()
	srv := New(cfg)

	var captured engine.RunRequest
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}

	job := Job{
		ID:        "job-cache",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}

	_, stats, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if captured.NameserverCache == nil {
		t.Fatalf("expected NameserverCache to be set on RunRequest")
	}
	// With a stub engine that does no real queries, cache stats should be zero.
	if stats.cacheHits != 0 || stats.cacheMisses != 0 || stats.cacheEvictions != 0 {
		t.Fatalf("expected zero cache stats from stub engine, got hits=%d misses=%d evictions=%d",
			stats.cacheHits, stats.cacheMisses, stats.cacheEvictions)
	}
}
