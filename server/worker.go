package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/dnssecchain"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

type workerPool struct {
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	cancels []context.CancelFunc // one per live worker goroutine
}

// Start launches background workers that consume queued jobs. The purge loop
// is also started and checks cfg.Database.RetentionDays each tick, so changes
// via the settings API take effect at the next purge cycle.
func (s *Server) Start() {
	if s.workers.ctx != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.workers.ctx = ctx
	s.workers.cancel = cancel

	workerCount := max(s.cfg.WorkerCount, 1)
	for range workerCount {
		s.startWorker()
	}

	// Always start the purge goroutine; it reads RetentionDays and the purge
	// interval dynamically so changes via the settings API take effect without
	// a restart.
	startPurgeLoop(ctx, s.store, &s.retentionDays, &s.purgeIntervalSec, s.metrics, func(format string, args ...any) {
		log.Printf(format, args...)
	})

	if s.rateLimiter != nil {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					s.rateLimiter.Cleanup()
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if s.analysis != nil {
		go func() {
			if err := s.analysis.RepairAllCohorts(ctx); err != nil && ctx.Err() == nil {
				log.Printf("analysis: startup repair failed: %v", err)
			}
		}()
		startSnapshotCaptureLoop(ctx, s.analysis)
	}
}

// startSnapshotCaptureLoop runs one immediate capture pass plus a ticker
// that re-scans pending snapshots every 30 seconds. Pulled out of Start()
// so analysis_runtime_test.go can exercise the loop without standing up a
// full server.
func startSnapshotCaptureLoop(ctx context.Context, ctrl AnalysisController) {
	go func() {
		if err := ctrl.CaptureCompletedSnapshots(ctx); err != nil && ctx.Err() == nil {
			log.Printf("analysis: snapshot capture (initial): %v", err)
		}
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := ctrl.CaptureCompletedSnapshots(ctx); err != nil && ctx.Err() == nil {
					log.Printf("analysis: snapshot capture: %v", err)
				}
			}
		}
	}()
}

// startWorker starts one worker goroutine with its own cancellable context
// derived from the pool context. It must be called with the pool context
// already initialised (i.e. after Start has set s.workers.ctx).
func (s *Server) startWorker() {
	workerCtx, workerCancel := context.WithCancel(s.workers.ctx)
	s.workers.mu.Lock()
	s.workers.cancels = append(s.workers.cancels, workerCancel)
	s.workers.mu.Unlock()
	s.workers.wg.Add(1)
	go s.workerLoop(workerCtx)
}

// resizeWorkerPool adjusts the number of live workers to match targetCount.
// Scaling up starts new goroutines immediately. Scaling down cancels the
// excess workers' contexts; they exit cleanly after their current job (if any)
// finishes.
func (s *Server) resizeWorkerPool(targetCount int) {
	if s.workers.ctx == nil {
		return
	}
	if targetCount < 1 {
		targetCount = 1
	}
	s.workers.mu.Lock()
	current := len(s.workers.cancels)
	s.workers.mu.Unlock()

	if targetCount == current {
		return
	}
	if targetCount > current {
		for i := current; i < targetCount; i++ {
			s.startWorker()
		}
		return
	}
	// Scale down: cancel the last (current - targetCount) workers and remove
	// them from the slice.
	s.workers.mu.Lock()
	toCancel := make([]context.CancelFunc, current-targetCount)
	copy(toCancel, s.workers.cancels[targetCount:])
	s.workers.cancels = s.workers.cancels[:targetCount]
	s.workers.mu.Unlock()
	for _, cancel := range toCancel {
		cancel()
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

func (s *Server) workerLoop(ctx context.Context) {
	defer s.workers.wg.Done()
	for {
		jobID, err := s.queue.Dequeue(ctx)
		if err != nil {
			return
		}
		if err := s.runJob(jobID); err != nil && s.cfg.Debug {
			log.Printf("worker: job %s error: %v", jobID, err)
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

	art, runErr := s.runEngineForJob(job, jobCtx)
	s.metrics.ObserveDNSQueries(art.stats.ipv4, art.stats.ipv6)
	s.metrics.ObserveCacheMetrics(art.stats.cacheHits, art.stats.cacheMisses, art.stats.cacheEvictions)
	finishedAt := time.Now().UTC()

	if jobCtx.Err() != nil {
		job.Status = JobCanceled
		job.Error = "canceled"
	} else if runErr != nil {
		job.Status = JobFailed
		job.Error = runErr.Error()
	} else {
		job.Status = JobSucceeded
	}

	job.Progress = 100
	job.FinishedAt = finishedAt
	job.EffectiveProfile = art.effectiveProfile
	job.NameserverTimings = art.nsTimings
	job.DNSSECChainJSON = art.dnssecChainJSON

	// Get previous status for metrics before graduation removes the job.
	previous, prevOK := s.store.Get(job.ID)

	if err := s.store.GraduateJob(job, art.entries); err != nil {
		log.Printf("CRITICAL: job %s: failed to graduate: %v", job.ID, err)
		return err
	}

	fromStatus := JobStatus("")
	if prevOK {
		fromStatus = previous.Status
	}
	s.metrics.ObserveJobStatusTransition(fromStatus, job.Status)
	becameTerminal := !isTerminalMetricsStatus(fromStatus) && isTerminalMetricsStatus(job.Status)
	if becameTerminal {
		duration := time.Duration(-1)
		if !job.StartedAt.IsZero() {
			duration = finishedAt.Sub(job.StartedAt)
		}
		s.metrics.ObserveJobCompletionWithContext(job.BatchID, job.Domain, job.Status, duration, severityTotalsFromEntries(art.entries))
	}

	if s.analysis != nil {
		if err := s.analysis.ProjectRun(job.ID); err != nil {
			log.Printf("analysis: project run %s: %v", job.ID, err)
		}
	}

	return runErr
}

type jobQueryStats struct {
	ipv4           int64
	ipv6           int64
	cacheHits      int64
	cacheMisses    int64
	cacheEvictions int64
}

// jobArtifacts bundles everything a run produces for graduation.
type jobArtifacts struct {
	entries          []engine.LogEntry
	stats            jobQueryStats
	nsTimings        []NameserverTiming
	effectiveProfile string
	dnssecChainJSON  string
}

// maxDNSSECChainBytes bounds the stored chain blob well under the MariaDB
// TEXT limit; oversized summaries are dropped rather than truncated.
const maxDNSSECChainBytes = 60 * 1024

func (s *Server) runEngineForJob(job Job, ctx context.Context) (jobArtifacts, error) {
	if s.engineLimiter != nil {
		if err := s.engineLimiter.Acquire(ctx); err != nil {
			return jobArtifacts{}, err
		}
		defer s.engineLimiter.Release()
	}

	minLevel := s.cfg.MinLevel
	if job.MinLevel != "" {
		minLevel = job.MinLevel
	}
	req := engine.RunRequest{
		Domain:                 job.Domain,
		UndelegatedNameservers: job.UndelegatedNS,
		UndelegatedDSInfo:      job.UndelegatedDS,
		MinLevel:               minLevel,
		Context:                ctx,
	}
	if job.IPv4Disabled {
		disabled := false
		req.IPv4 = &disabled
	}
	if job.IPv6Disabled {
		disabled := false
		req.IPv6 = &disabled
	}
	// Clamp the guard on unless the instance permits non-global targets.
	if !s.cfg.PublicAPI.AllowNonGlobalTargets {
		block := false
		req.AllowNonGlobalTargets = &block
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
	hotCacheKey := nameserverHotCacheKey(req)
	cacheStore, releaseHotCache := s.hotCache.Lease(hotCacheKey)
	defer releaseHotCache()
	req.NameserverCache = cacheStore

	queryCounter := &dnsQueryCounter{}
	callbacks := []func(*logger.Entry) error{
		queryCounter.Callback,
	}
	if err := applyProfileOverrides(&req, s.store, job.ProfileID, job.Overrides, s.cfg.ProfilePath); err != nil {
		return jobArtifacts{}, err
	}
	effectiveProfile, err := engine.EffectiveProfile(req)
	if err != nil {
		return jobArtifacts{}, err
	}
	effectiveProfileJSON, err := effectiveProfile.ToJSON()
	if err != nil {
		return jobArtifacts{}, err
	}

	if len(job.Tests) == 0 {
		if tracker := s.newProgressTracker(job.ID, req); tracker != nil {
			callbacks = append(callbacks, tracker.Callback)
		}
	}
	req.LogCallback = chainLogCallbacks(callbacks...)

	// Collect the per-run DNSSEC chain only for public jobs when the flag is on.
	var chainSummary *dnssecchain.Summary
	if job.Origin == JobOriginPublic && s.cfg.ShowDNSSECChainPublic {
		req.DNSSECChainSink = func(sm *dnssecchain.Summary) { chainSummary = sm }
	}

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

	var entries []engine.LogEntry
	var runErr error
	switch {
	case len(job.Tests) == 1:
		req.Testcases = []string{job.Tests[0]}
		entries, runErr = s.runEngine(req)
	case len(job.Tests) == 0:
		entries, runErr = s.runEngine(req)
	default:
		total := len(job.Tests)
		for i, testcase := range job.Tests {
			runReq := req
			runReq.Testcases = []string{testcase}
			part, err := s.runEngine(runReq)
			if total > 0 {
				progress := int(math.Round((float64(i+1) / float64(total)) * 100))
				s.updateJobProgress(job.ID, progress)
			}
			if err != nil {
				runErr = err
				break
			}
			entries = append(entries, part...)
		}
	}

	art := jobArtifacts{
		entries:          entries,
		stats:            collectStats(),
		nsTimings:        s.collectNameserverTimings(job, cacheStore.QueryTimings(), cacheStore.QueryTimeouts(), entries),
		effectiveProfile: effectiveProfileJSON,
		dnssecChainJSON:  s.marshalDNSSECChain(chainSummary),
	}
	return art, runErr
}

// marshalDNSSECChain serializes the chain summary, dropping it when nil or
// larger than the storage cap.
func (s *Server) marshalDNSSECChain(sm *dnssecchain.Summary) string {
	if sm == nil {
		return ""
	}
	blob, err := json.Marshal(sm)
	if err != nil {
		return ""
	}
	if len(blob) > maxDNSSECChainBytes {
		if s.cfg.Debug {
			log.Printf("dnssec chain: %d bytes exceeds %d cap, skipping", len(blob), maxDNSSECChainBytes)
		}
		return ""
	}
	return string(blob)
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

func applyProfileOverrides(req *engine.RunRequest, store JobStore, profileID *int64, overrides map[string]any, baseProfile string) error {
	if req == nil {
		return nil
	}
	if profileID == nil && len(overrides) == 0 {
		if baseProfile != "" {
			req.Profile = baseProfile
		}
		return nil
	}
	base := profile.New()
	if baseProfile != "" {
		data, err := os.ReadFile(baseProfile)
		if err != nil {
			return err
		}
		base, err = profile.FromYAML(string(data))
		if err != nil {
			return err
		}
	}
	if profileID != nil {
		if store == nil {
			return fmt.Errorf("profile %d not found", *profileID)
		}
		stored, ok := store.GetProfile(*profileID)
		if !ok {
			return fmt.Errorf("profile %d not found", *profileID)
		}
		storedProfile, err := profile.FromJSON(stored.Config)
		if err != nil {
			return err
		}
		if err := base.Merge(storedProfile); err != nil {
			return err
		}
	}
	if len(overrides) > 0 {
		payload, err := json.Marshal(overrides)
		if err != nil {
			return err
		}
		overrideProfile, err := profile.FromJSON(string(payload))
		if err != nil {
			return err
		}
		if err := base.Merge(overrideProfile); err != nil {
			return err
		}
	}
	merged, err := base.ToJSON()
	if err != nil {
		return err
	}
	req.ProfileData = merged
	return nil
}
