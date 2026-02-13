package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
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

func TestRunEngineForJobTestcaseParallelismOrderedMerge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JobTestParallelism = 3
	srv := New(cfg)

	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		switch req.Testcase {
		case "t1":
			time.Sleep(30 * time.Millisecond)
		case "t2":
			time.Sleep(10 * time.Millisecond)
		}
		return []engine.LogEntry{
			{Testcase: req.Testcase, Level: "NOTICE"},
		}, nil
	}

	job := Job{
		ID:     "job-ordered",
		Domain: "example.com",
		Tests:  []string{"t1", "t2", "t3"},
	}
	entries, _, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("run job: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Testcase != "t1" || entries[1].Testcase != "t2" || entries[2].Testcase != "t3" {
		t.Fatalf("expected ordered testcase merge [t1 t2 t3], got [%s %s %s]", entries[0].Testcase, entries[1].Testcase, entries[2].Testcase)
	}
}

func TestRunEngineForJobSingleTestNoRunnerWithoutHotCache(t *testing.T) {
	cfg := DefaultConfig()
	srv := New(cfg)

	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.Runner != nil {
			t.Fatalf("expected nil runner when hot cache is disabled and single testcase run is used")
		}
		return nil, nil
	}

	job := Job{
		ID:     "job-single-no-hot-cache",
		Domain: "example.com",
		Tests:  []string{"t1"},
	}
	if _, _, _, err := srv.runEngineForJob(job, context.Background()); err != nil {
		t.Fatalf("run job: %v", err)
	}
}

func TestRunEngineForJobCrossJobHotCacheReuse(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CrossJobHotCache = true
	cfg.CrossJobHotCacheTTLSeconds = 60
	srv := New(cfg)

	var (
		mu         sync.Mutex
		runIndex   int
		preWarmCnt []int
	)
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.Runner == nil || req.Runner.NameserverCache == nil {
			return nil, errors.New("expected runner with nameserver cache")
		}
		mu.Lock()
		runIndex++
		current := runIndex
		preWarmCnt = append(preWarmCnt, req.Runner.NameserverCache.AddressCacheCount())
		mu.Unlock()

		if current == 1 {
			if _, err := nameserver.NewWithCache(req.Runner.NameserverCache, "ns1.example", "192.0.2.90", nil); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}

	job1 := Job{ID: "job-hot-1", Domain: "example.com", Tests: []string{"t1"}}
	job2 := Job{ID: "job-hot-2", Domain: "example.net", Tests: []string{"t1"}}

	if _, _, _, err := srv.runEngineForJob(job1, context.Background()); err != nil {
		t.Fatalf("run job1: %v", err)
	}
	if _, _, _, err := srv.runEngineForJob(job2, context.Background()); err != nil {
		t.Fatalf("run job2: %v", err)
	}

	if len(preWarmCnt) != 2 {
		t.Fatalf("expected two runs, got %d", len(preWarmCnt))
	}
	if preWarmCnt[0] != 0 {
		t.Fatalf("first run should be cold, got %d", preWarmCnt[0])
	}
	if preWarmCnt[1] < 1 {
		t.Fatalf("second run should see warmed cache, got %d", preWarmCnt[1])
	}
}

func TestRunEngineForJobCrossJobHotCacheKeyIsolation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CrossJobHotCache = true
	cfg.CrossJobHotCacheTTLSeconds = 60
	srv := New(cfg)

	var preWarmCnt []int
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.Runner == nil || req.Runner.NameserverCache == nil {
			return nil, errors.New("expected runner with nameserver cache")
		}
		preWarmCnt = append(preWarmCnt, req.Runner.NameserverCache.AddressCacheCount())
		if len(preWarmCnt) == 1 {
			if _, err := nameserver.NewWithCache(req.Runner.NameserverCache, "ns1.example", "192.0.2.91", nil); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}

	job1 := Job{
		ID:        "job-hot-key-1",
		Domain:    "example.com",
		Tests:     []string{"t1"},
		Overrides: map[string]any{"resolver": map[string]any{"defaults": map[string]any{"retry": 1}}},
	}
	job2 := Job{
		ID:        "job-hot-key-2",
		Domain:    "example.net",
		Tests:     []string{"t1"},
		Overrides: map[string]any{"resolver": map[string]any{"defaults": map[string]any{"retry": 3}}},
	}

	if _, _, _, err := srv.runEngineForJob(job1, context.Background()); err != nil {
		t.Fatalf("run job1: %v", err)
	}
	if _, _, _, err := srv.runEngineForJob(job2, context.Background()); err != nil {
		t.Fatalf("run job2: %v", err)
	}

	if len(preWarmCnt) != 2 {
		t.Fatalf("expected two runs, got %d", len(preWarmCnt))
	}
	if preWarmCnt[0] != 0 {
		t.Fatalf("first run should be cold, got %d", preWarmCnt[0])
	}
	if preWarmCnt[1] != 0 {
		t.Fatalf("second run should be cold due to different hot-cache key, got %d", preWarmCnt[1])
	}
}

func TestRunEngineForJobTestcaseSequentialReusesRunner(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JobTestParallelism = 1
	srv := New(cfg)

	var runners []*engine.Runner
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.Runner == nil {
			return nil, errors.New("missing shared runner")
		}
		runners = append(runners, req.Runner)
		return []engine.LogEntry{{Testcase: req.Testcase, Level: "NOTICE"}}, nil
	}

	job := Job{
		ID:     "job-runner-seq",
		Domain: "example.com",
		Tests:  []string{"t1", "t2", "t3"},
	}
	_, _, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("run job: %v", err)
	}
	if len(runners) != len(job.Tests) {
		t.Fatalf("expected %d runner captures, got %d", len(job.Tests), len(runners))
	}
	first := runners[0]
	if first.Logger == nil || first.Profile == nil || first.NameserverCache == nil {
		t.Fatalf("shared runner should include logger, profile, and nameserver cache")
	}
	for idx, runner := range runners[1:] {
		if runner != first {
			t.Fatalf("expected runner reuse at call %d", idx+2)
		}
	}
}

func TestRunEngineForJobTestcaseParallelReusesRunner(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JobTestParallelism = 3
	srv := New(cfg)

	var (
		mu      sync.Mutex
		runners []*engine.Runner
	)
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.Runner == nil {
			return nil, errors.New("missing shared runner")
		}
		mu.Lock()
		runners = append(runners, req.Runner)
		mu.Unlock()
		return []engine.LogEntry{{Testcase: req.Testcase, Level: "NOTICE"}}, nil
	}

	job := Job{
		ID:     "job-runner-par",
		Domain: "example.com",
		Tests:  []string{"t1", "t2", "t3", "t4"},
	}
	_, _, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("run job: %v", err)
	}
	if len(runners) != len(job.Tests) {
		t.Fatalf("expected %d runner captures, got %d", len(job.Tests), len(runners))
	}
	first := runners[0]
	if first.Logger == nil || first.Profile == nil || first.NameserverCache == nil {
		t.Fatalf("shared runner should include logger, profile, and nameserver cache")
	}
	for idx, runner := range runners[1:] {
		if runner != first {
			t.Fatalf("expected runner reuse at call %d", idx+2)
		}
	}
}

func TestRunEngineForJobTestcaseParallelismCap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JobTestParallelism = 2
	srv := New(cfg)

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		current := inFlight.Add(1)
		for {
			previous := maxInFlight.Load()
			if current <= previous {
				break
			}
			if maxInFlight.CompareAndSwap(previous, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		inFlight.Add(-1)
		return []engine.LogEntry{{Testcase: req.Testcase, Level: "NOTICE"}}, nil
	}

	job := Job{
		ID:     "job-cap",
		Domain: "example.com",
		Tests:  []string{"t1", "t2", "t3", "t4"},
	}
	errs := make(chan error, 1)
	go func() {
		_, _, _, err := srv.runEngineForJob(job, context.Background())
		errs <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(250 * time.Millisecond):
			t.Fatalf("expected testcase run %d to start", i+1)
		}
	}
	select {
	case <-started:
		t.Fatalf("expected no third concurrent testcase run with parallelism cap=2")
	case <-time.After(150 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("run job: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for parallel run completion")
	}

	if got := maxInFlight.Load(); got > 2 {
		t.Fatalf("max in-flight testcase runs = %d, want <= 2", got)
	}
}

func TestRunEngineForJobTestcaseParallelismFirstError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JobTestParallelism = 3
	srv := New(cfg)

	boom := errors.New("testcase boom")
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		switch req.Testcase {
		case "t1":
			return []engine.LogEntry{{Testcase: "t1", Level: "NOTICE"}}, nil
		case "t2":
			return nil, boom
		case "t3":
			<-req.Context.Done()
			return nil, req.Context.Err()
		default:
			return nil, nil
		}
	}

	job := Job{
		ID:     "job-error",
		Domain: "example.com",
		Tests:  []string{"t1", "t2", "t3"},
	}
	entries, _, _, err := srv.runEngineForJob(job, context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}
	if len(entries) != 1 || entries[0].Testcase != "t1" {
		t.Fatalf("expected only successful entries before first failing testcase, got %+v", entries)
	}
}

func TestProgressUpdatesForMultipleTestsParallel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JobTestParallelism = 3
	srv := New(cfg)
	spy := newSpyJobStore()
	srv.store = spy

	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		switch req.Testcase {
		case "t1":
			time.Sleep(20 * time.Millisecond)
		case "t2":
			time.Sleep(5 * time.Millisecond)
		}
		return nil, nil
	}

	job := Job{
		ID:        "job-progress-par",
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
	if len(progresses) == 0 {
		t.Fatalf("expected progress updates")
	}
	last := -1
	for _, value := range progresses {
		if value < last {
			t.Fatalf("expected monotonic progress updates, got %v", progresses)
		}
		last = value
	}
	if last != 100 {
		t.Fatalf("expected final progress 100, got %d (all: %v)", last, progresses)
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
