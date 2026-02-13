package server

import (
	"context"
	"errors"
	"log"
	"math"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

type workerPool struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Start launches background workers that consume queued jobs.
func (s *Server) Start() {
	if s.workers.ctx != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.workers.ctx = ctx
	s.workers.cancel = cancel

	workerCount := s.cfg.EffectiveWorkerCount()
	for i := 0; i < workerCount; i++ {
		s.workers.wg.Add(1)
		go s.workerLoop(i)
	}
}

// Stop requests worker shutdown and waits for completion.
func (s *Server) Stop(ctx context.Context) error {
	if s.workers.cancel == nil {
		if s.profileOverrideCache != nil {
			s.profileOverrideCache.Close()
		}
		if s.nameserverHotCache != nil {
			s.nameserverHotCache.Close()
		}
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
		if s.profileOverrideCache != nil {
			s.profileOverrideCache.Close()
		}
		if s.nameserverHotCache != nil {
			s.nameserverHotCache.Close()
		}
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
	if _, _, _, err := s.updateJobWithMetricsTransition(job); err != nil {
		return err
	}
	s.initProgressWriteState(job.ID, 0, now)
	defer s.clearProgressWriteState(job.ID)

	entries, ipv4Queries, ipv6Queries, runErr := s.runEngineForJob(job, jobCtx)
	s.metrics.ObserveDNSQueries(ipv4Queries, ipv6Queries)
	finishedAt := time.Now().UTC()

	result := JobResult{
		JobID:   job.ID,
		BatchID: job.BatchID,
		Status:  JobSucceeded,
		Summary: summarizeEntries(entries),
		Raw: &JobResultRaw{
			Entries: buildResultEntries(entries),
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
			result.Raw = &JobResultRaw{
				Entries: buildResultEntries(entries),
			}
		}
	} else {
		job.Status = JobSucceeded
	}

	job.Progress = 100
	job.FinishedAt = finishedAt
	_, _, becameTerminal, err := s.updateJobWithMetricsTransition(job)
	if err != nil {
		return err
	}
	if err := s.store.SetResult(job.ID, result); err != nil {
		return err
	}
	if becameTerminal {
		duration := time.Duration(-1)
		if !job.StartedAt.IsZero() {
			duration = finishedAt.Sub(job.StartedAt)
		}
		s.metrics.ObserveJobCompletionWithContext(job.BatchID, job.Domain, job.Status, duration, severityTotalsFromEntries(entries))
	}

	return runErr
}

func (s *Server) runEngineForJob(job Job, ctx context.Context) ([]engine.LogEntry, int64, int64, error) {
	if s.engineLimiter != nil {
		if err := s.engineLimiter.Acquire(ctx); err != nil {
			return nil, 0, 0, err
		}
		defer s.engineLimiter.Release()
	}

	minLevel := s.cfg.MinLevel
	if job.MinLevel != "" {
		minLevel = job.MinLevel
	}
	req := engine.RunRequest{
		Domain:   job.Domain,
		MinLevel: minLevel,
		Context:  ctx,
	}
	if s.cfg.PositiveCacheTTL != nil {
		req.PositiveCacheTTL = s.cfg.PositiveCacheTTL
	}
	if s.cfg.NegativeCacheTTL != nil {
		req.NegativeCacheTTL = s.cfg.NegativeCacheTTL
	}
	if s.cfg.Timeout != nil {
		req.Timeout = s.cfg.Timeout
	}
	if s.cfg.Retry != nil {
		req.Retry = s.cfg.Retry
	}
	if s.cfg.Retrans != nil {
		req.Retrans = s.cfg.Retrans
	}
	if s.cfg.Fallback != nil {
		req.Fallback = s.cfg.Fallback
	}

	queryCounter := &dnsQueryCounter{}
	callbacks := []func(*logger.Entry) error{
		queryCounter.Callback,
	}
	cleanup, err := applyProfileOverridesWithCache(&req, job.Overrides, s.cfg.ProfilePath, s.profileOverrideCache)
	if err != nil {
		return nil, 0, 0, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	if len(job.Tests) == 0 {
		if tracker := s.newProgressTracker(job.ID, req); tracker != nil {
			callbacks = append(callbacks, tracker.Callback)
		}
	}
	req.LogCallback = chainLogCallbacks(callbacks...)
	useSharedRunner := len(job.Tests) > 1 || s.nameserverHotCache != nil
	if useSharedRunner {
		runner, err := engine.BuildRunner(req)
		if err != nil {
			return nil, 0, 0, err
		}
		if s.nameserverHotCache != nil {
			cache, release := s.nameserverHotCache.Lease(nameserverHotCacheKey(req))
			runner.NameserverCache = cache
			if release != nil {
				defer release()
			}
		}
		req.Runner = runner
	}

	if len(job.Tests) == 1 {
		req.Testcase = job.Tests[0]
		entries, err := s.runEngine(req)
		ipv4, ipv6 := queryCounter.Totals()
		return entries, ipv4, ipv6, err
	}
	if len(job.Tests) == 0 {
		entries, err := s.runEngine(req)
		ipv4, ipv6 := queryCounter.Totals()
		return entries, ipv4, ipv6, err
	}
	if s.effectiveJobTestParallelism() <= 1 {
		return s.runJobTestcasesSequential(job.ID, req, job.Tests, queryCounter)
	}
	return s.runJobTestcasesParallel(job.ID, req, job.Tests, s.effectiveJobTestParallelism(), queryCounter)
}

func (s *Server) effectiveJobTestParallelism() int {
	return s.cfg.EffectiveJobTestParallelism()
}

func (s *Server) runJobTestcasesSequential(jobID string, req engine.RunRequest, testcases []string, queryCounter *dnsQueryCounter) ([]engine.LogEntry, int64, int64, error) {
	var all []engine.LogEntry
	total := len(testcases)
	for i, testcase := range testcases {
		runReq := req
		runReq.Testcase = testcase
		entries, err := s.runEngine(runReq)
		if total > 0 {
			done := i + 1
			progress := int(math.Round((float64(done) / float64(total)) * 100))
			s.updateJobProgress(jobID, progress)
		}
		if err != nil {
			ipv4, ipv6 := queryCounter.Totals()
			return all, ipv4, ipv6, err
		}
		all = append(all, entries...)
	}
	ipv4, ipv6 := queryCounter.Totals()
	return all, ipv4, ipv6, nil
}

type testcaseWorkItem struct {
	index    int
	testcase string
}

type testcaseRunResult struct {
	entries []engine.LogEntry
	err     error
}

func (s *Server) runJobTestcasesParallel(jobID string, req engine.RunRequest, testcases []string, parallelism int, queryCounter *dnsQueryCounter) ([]engine.LogEntry, int64, int64, error) {
	if len(testcases) == 0 {
		ipv4, ipv6 := queryCounter.Totals()
		return nil, ipv4, ipv6, nil
	}
	if parallelism < 1 {
		parallelism = 1
	}
	if parallelism > len(testcases) {
		parallelism = len(testcases)
	}

	parentCtx := req.Context
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	runCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	results := make([]testcaseRunResult, len(testcases))
	workCh := make(chan testcaseWorkItem)
	var completed atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workCh {
				runReq := req
				runReq.Context = runCtx
				runReq.Testcase = item.testcase
				entries, err := s.runEngine(runReq)
				results[item.index] = testcaseRunResult{
					entries: entries,
					err:     err,
				}
				done := int(completed.Add(1))
				progress := int(math.Round((float64(done) / float64(len(testcases))) * 100))
				s.updateJobProgress(jobID, progress)
				if err != nil {
					cancel()
				}
			}
		}()
	}

enqueueLoop:
	for idx, testcase := range testcases {
		select {
		case <-runCtx.Done():
			break enqueueLoop
		case workCh <- testcaseWorkItem{index: idx, testcase: testcase}:
		}
	}
	close(workCh)
	wg.Wait()

	failIdx, failErr := firstTestcaseError(results, parentCtx.Err())
	if failIdx >= 0 {
		all := mergeOrderedResults(results, failIdx)
		ipv4, ipv6 := queryCounter.Totals()
		return all, ipv4, ipv6, failErr
	}

	all := mergeOrderedResults(results, len(results))
	ipv4, ipv6 := queryCounter.Totals()
	return all, ipv4, ipv6, nil
}

func firstTestcaseError(results []testcaseRunResult, parentCtxErr error) (int, error) {
	cancelIdx := -1
	for idx, result := range results {
		if result.err == nil {
			continue
		}
		if !errors.Is(result.err, context.Canceled) {
			return idx, result.err
		}
		if cancelIdx < 0 {
			cancelIdx = idx
		}
	}
	if cancelIdx < 0 {
		return -1, nil
	}
	if parentCtxErr != nil {
		return cancelIdx, parentCtxErr
	}
	return cancelIdx, results[cancelIdx].err
}

func mergeOrderedResults(results []testcaseRunResult, limit int) []engine.LogEntry {
	if limit < 0 {
		limit = 0
	}
	if limit > len(results) {
		limit = len(results)
	}
	totalEntries := 0
	for idx := 0; idx < limit; idx++ {
		if results[idx].err == nil {
			totalEntries += len(results[idx].entries)
		}
	}
	all := make([]engine.LogEntry, 0, totalEntries)
	for idx := 0; idx < limit; idx++ {
		if results[idx].err != nil {
			continue
		}
		all = append(all, results[idx].entries...)
	}
	return all
}

func (s *Server) runEngine(req engine.RunRequest) ([]engine.LogEntry, error) {
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
	now := time.Now().UTC()
	if !s.prepareProgressPersist(jobID, progress, now) {
		return
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

func severityTotalsFromEntries(entries []engine.LogEntry) map[string]int64 {
	totals := zeroMetricsSeverityTotals()
	for _, entry := range entries {
		level := strings.ToUpper(strings.TrimSpace(entry.Level))
		if _, ok := totals[level]; !ok {
			continue
		}
		totals[level]++
	}
	return totals
}

type dnsQueryCounter struct {
	ipv4 atomic.Int64
	ipv6 atomic.Int64
}

// Callback counts EXTERNAL_QUERY events split by IP family.
func (c *dnsQueryCounter) Callback(entry *logger.Entry) error {
	if c == nil || entry == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(entry.Tag), "EXTERNAL_QUERY") {
		return nil
	}
	addr, ok := dnsQueryAddrFromArgs(entry.Args)
	if !ok {
		return nil
	}
	if addr.Is4() {
		c.ipv4.Add(1)
		return nil
	}
	if addr.Is6() {
		c.ipv6.Add(1)
	}
	return nil
}

// Totals returns accumulated IPv4 and IPv6 query counts.
func (c *dnsQueryCounter) Totals() (int64, int64) {
	if c == nil {
		return 0, 0
	}
	return c.ipv4.Load(), c.ipv6.Load()
}

func dnsQueryAddrFromArgs(args map[string]any) (netip.Addr, bool) {
	if len(args) == 0 {
		return netip.Addr{}, false
	}
	value, ok := args["ip"]
	if !ok {
		return netip.Addr{}, false
	}
	switch ipValue := value.(type) {
	case string:
		addr, err := netip.ParseAddr(strings.TrimSpace(ipValue))
		if err != nil {
			return netip.Addr{}, false
		}
		return addr, true
	case netip.Addr:
		return ipValue, true
	default:
		return netip.Addr{}, false
	}
}

func chainLogCallbacks(callbacks ...func(*logger.Entry) error) func(*logger.Entry) error {
	active := make([]func(*logger.Entry) error, 0, len(callbacks))
	for _, callback := range callbacks {
		if callback != nil {
			active = append(active, callback)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return func(entry *logger.Entry) error {
		for _, callback := range active {
			if err := callback(entry); err != nil {
				return err
			}
		}
		return nil
	}
}
