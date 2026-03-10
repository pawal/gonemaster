package server

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	ns "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
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

	workerCount := s.cfg.WorkerCount
	if workerCount < 1 {
		workerCount = 1
	}
	for i := 0; i < workerCount; i++ {
		s.workers.wg.Add(1)
		go s.workerLoop(i)
	}
}

// Stop requests worker shutdown and waits for completion.
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
	if _, _, _, err := s.updateJobWithMetricsTransition(job); err != nil {
		return err
	}
	s.initProgressWriteState(job.ID, 0, now)
	defer s.clearProgressWriteState(job.ID)

	entries, qStats, runErr := s.runEngineForJob(job, jobCtx)
	s.metrics.ObserveDNSQueries(qStats.ipv4, qStats.ipv6)
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

type jobQueryStats struct {
	ipv4       int64
	ipv6       int64
	cacheHits  int64
	cacheMisses int64
	cacheEvictions int64
}

func (s *Server) runEngineForJob(job Job, ctx context.Context) ([]engine.LogEntry, jobQueryStats, error) {
	if s.engineLimiter != nil {
		if err := s.engineLimiter.Acquire(ctx); err != nil {
			return nil, jobQueryStats{}, err
		}
		defer s.engineLimiter.Release()
	}

	minLevel := s.cfg.MinLevel
	if job.MinLevel != "" {
		minLevel = job.MinLevel
	}
	cacheStore := ns.NewCacheStore()
	req := engine.RunRequest{
		Domain:                 job.Domain,
		UndelegatedNameservers: job.UndelegatedNS,
		UndelegatedDSInfo:      job.UndelegatedDS,
		MinLevel:               minLevel,
		NameserverCache:        cacheStore,
		Context:                ctx,
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
	if s.cfg.SourceAddr4 != nil {
		req.SourceAddr4 = s.cfg.SourceAddr4
	}
	if s.cfg.SourceAddr6 != nil {
		req.SourceAddr6 = s.cfg.SourceAddr6
	}

	queryCounter := &dnsQueryCounter{}
	callbacks := []func(*logger.Entry) error{
		queryCounter.Callback,
	}
	cleanup, err := applyProfileOverrides(&req, job.Overrides, s.cfg.ProfilePath)
	if err != nil {
		return nil, jobQueryStats{}, err
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

	collectStats := func() jobQueryStats {
		ipv4, ipv6 := queryCounter.Totals()
		cm := cacheStore.QueryMetrics()
		return jobQueryStats{
			ipv4:           ipv4,
			ipv6:           ipv6,
			cacheHits:      int64(cm.Hits),
			cacheMisses:    int64(cm.Misses),
			cacheEvictions: int64(cm.Evictions),
		}
	}

	if len(job.Tests) == 1 {
		req.Testcase = job.Tests[0]
		entries, err := s.runEngine(req)
		return entries, collectStats(), err
	}
	if len(job.Tests) == 0 {
		entries, err := s.runEngine(req)
		return entries, collectStats(), err
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
			return all, collectStats(), err
		}
		all = append(all, entries...)
	}
	return all, collectStats(), nil
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
	ipv4 int64
	ipv6 int64
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
		c.ipv4++
		return nil
	}
	if addr.Is6() {
		c.ipv6++
	}
	return nil
}

// Totals returns accumulated IPv4 and IPv6 query counts.
func (c *dnsQueryCounter) Totals() (int64, int64) {
	if c == nil {
		return 0, 0
	}
	return c.ipv4, c.ipv6
}

func dnsQueryAddrFromArgs(args map[string]any) (netip.Addr, bool) {
	if len(args) == 0 {
		return netip.Addr{}, false
	}
	value, ok := args[logargs.KeyAddress]
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
