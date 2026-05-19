package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/methodsv2"
	serverpkg "codeberg.org/pawal/gonemaster/server"
	_ "modernc.org/sqlite"
)

type soakReport struct {
	GeneratedAtUTC             string         `json:"generated_at_utc"`
	Hostname                   string         `json:"hostname"`
	GoVersion                  string         `json:"go_version"`
	PID                        int            `json:"pid"`
	Backend                    string         `json:"backend"`
	SQLiteDSN                  string         `json:"sqlite_dsn,omitempty"`
	SQLiteBytes                int64          `json:"sqlite_bytes,omitempty"`
	Workers                    int            `json:"workers"`
	MaxConcurrentJobs          int            `json:"max_concurrent_jobs"`
	ProfilePath                string         `json:"profile_path,omitempty"`
	MinLevel                   string         `json:"min_level"`
	DomainsFile                string         `json:"domains_file"`
	DomainsCount               int            `json:"domains_count"`
	DomainsSHA256              string         `json:"domains_sha256"`
	Rounds                     int            `json:"rounds"`
	WarmupRounds               int            `json:"warmup_rounds"`
	SampleSeconds              int            `json:"sample_seconds"`
	PollSeconds                int            `json:"poll_seconds"`
	BatchTimeoutSeconds        int            `json:"batch_timeout_seconds"`
	Methodsv2CacheClearedStart bool           `json:"methodsv2_cache_cleared_start"`
	Methodsv2CacheEntriesStart int            `json:"methodsv2_cache_entries_start"`
	Methodsv2CacheEntriesEnd   int            `json:"methodsv2_cache_entries_end"`
	PeakRSSKB                  int64          `json:"peak_rss_kb"`
	Samples                    []rssSample    `json:"samples"`
	Checkpoints                []gcCheckpoint `json:"checkpoints"`
	Batches                    []batchRun     `json:"batches"`
	Metadata                   map[string]any `json:"metadata,omitempty"`
}

type rssSample struct {
	TimestampUTC   string  `json:"timestamp_utc"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	Round          int     `json:"round"`
	RSSKB          int64   `json:"rss_kb"`
}

type gcCheckpoint struct {
	Label                 string  `json:"label"`
	Round                 int     `json:"round"`
	TimestampUTC          string  `json:"timestamp_utc"`
	ElapsedSeconds        float64 `json:"elapsed_seconds"`
	RSSKB                 int64   `json:"rss_kb"`
	HeapAllocBytes        uint64  `json:"heap_alloc_bytes"`
	HeapInuseBytes        uint64  `json:"heap_inuse_bytes"`
	HeapIdleBytes         uint64  `json:"heap_idle_bytes"`
	HeapReleasedBytes     uint64  `json:"heap_released_bytes"`
	HeapObjects           uint64  `json:"heap_objects"`
	StackInuseBytes       uint64  `json:"stack_inuse_bytes"`
	SysBytes              uint64  `json:"sys_bytes"`
	NextGCBytes           uint64  `json:"next_gc_bytes"`
	NumGC                 uint32  `json:"num_gc"`
	NumGoroutine          int     `json:"num_goroutine"`
	Methodsv2CacheEntries int     `json:"methodsv2_cache_entries"`
}

type batchRun struct {
	Round             int            `json:"round"`
	BatchID           string         `json:"batch_id"`
	StartedAtUTC      string         `json:"started_at_utc"`
	FinishedAtUTC     string         `json:"finished_at_utc"`
	WallSeconds       float64        `json:"wall_seconds"`
	Total             int            `json:"total"`
	StatusCounts      map[string]int `json:"status_counts"`
	CompletedTerminal int            `json:"completed_terminal"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	var domainsPath string
	var outPath string
	var backend string
	var sqliteDSN string
	var workers int
	var maxConcurrentJobs int
	var rounds int
	var warmupRounds int
	var sampleSeconds int
	var pollSeconds int
	var batchTimeoutSeconds int
	var profilePath string
	var minLevel string

	fs := flag.NewFlagSet("memsoak", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&domainsPath, "domains", "", "Domain file (one per line)")
	fs.StringVar(&outPath, "out", "", "Write JSON report to file instead of stdout")
	fs.StringVar(&backend, "backend", "memory", "Storage backend: memory or sqlite")
	fs.StringVar(&sqliteDSN, "sqlite-dsn", "", "SQLite DB path (optional when backend=sqlite)")
	fs.IntVar(&workers, "workers", 8, "Server workers")
	fs.IntVar(&maxConcurrentJobs, "max-concurrent-jobs", 8, "Max concurrent engine jobs")
	fs.IntVar(&rounds, "rounds", 3, "Number of batch rounds to run")
	fs.IntVar(&warmupRounds, "warmup-rounds", 1, "Rounds to treat as warm-up before first forced-GC checkpoint")
	fs.IntVar(&sampleSeconds, "sample-seconds", 5, "RSS sampling interval in seconds")
	fs.IntVar(&pollSeconds, "poll-seconds", 2, "Batch status polling interval in seconds")
	fs.IntVar(&batchTimeoutSeconds, "batch-timeout-seconds", 5400, "Timeout per batch round in seconds")
	fs.StringVar(&profilePath, "profile", "", "Optional profile path")
	fs.StringVar(&minLevel, "min-level", "INFO", "Server min-level")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if strings.TrimSpace(domainsPath) == "" {
		fmt.Fprintln(os.Stderr, "--domains is required")
		return 2
	}
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend != "memory" && backend != "sqlite" {
		fmt.Fprintln(os.Stderr, "--backend must be memory or sqlite")
		return 2
	}
	if workers < 1 || maxConcurrentJobs < 0 || rounds < 1 || warmupRounds < 1 || warmupRounds > rounds {
		fmt.Fprintln(os.Stderr, "invalid numeric flag combination")
		return 2
	}
	if sampleSeconds < 1 || pollSeconds < 1 || batchTimeoutSeconds < 1 {
		fmt.Fprintln(os.Stderr, "sample, poll, and timeout seconds must be >= 1")
		return 2
	}

	domains, domainsSHA, err := readDomains(domainsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read domains: %v\n", err)
		return 1
	}
	if len(domains) == 0 {
		fmt.Fprintln(os.Stderr, "no domains after normalization")
		return 1
	}

	sqliteDir := ""
	if backend == "sqlite" && strings.TrimSpace(sqliteDSN) == "" {
		sqliteDir, err = os.MkdirTemp("", "gonemaster-memsoak-sqlite-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "create sqlite temp dir: %v\n", err)
			return 1
		}
		sqliteDSN = filepath.Join(sqliteDir, "jobs.db")
	}

	report, err := runSoak(domainsPath, domains, domainsSHA, backend, sqliteDSN, workers, maxConcurrentJobs, rounds, warmupRounds, sampleSeconds, pollSeconds, batchTimeoutSeconds, profilePath, minLevel)
	if sqliteDir != "" {
		defer os.RemoveAll(sqliteDir)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "memsoak failed: %v\n", err)
		return 1
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal report: %v\n", err)
		return 1
	}
	if outPath != "" {
		if err := os.WriteFile(outPath, encoded, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write report: %v\n", err)
			return 1
		}
		log.Printf("wrote report to %s", outPath)
	} else {
		if _, err := os.Stdout.Write(encoded); err != nil {
			fmt.Fprintf(os.Stderr, "write stdout: %v\n", err)
			return 1
		}
		if _, err := os.Stdout.Write([]byte{'\n'}); err != nil {
			fmt.Fprintf(os.Stderr, "write stdout newline: %v\n", err)
			return 1
		}
	}

	return 0
}

func runSoak(domainsPath string, domains []string, domainsSHA string, backend string, sqliteDSN string, workers int, maxConcurrentJobs int, rounds int, warmupRounds int, sampleSeconds int, pollSeconds int, batchTimeoutSeconds int, profilePath string, minLevel string) (soakReport, error) {
	report := soakReport{
		GeneratedAtUTC:      time.Now().UTC().Format(time.RFC3339Nano),
		GoVersion:           runtime.Version(),
		PID:                 os.Getpid(),
		Backend:             backend,
		SQLiteDSN:           sqliteDSN,
		Workers:             workers,
		MaxConcurrentJobs:   maxConcurrentJobs,
		ProfilePath:         profilePath,
		MinLevel:            minLevel,
		DomainsFile:         domainsPath,
		DomainsCount:        len(domains),
		DomainsSHA256:       domainsSHA,
		Rounds:              rounds,
		WarmupRounds:        warmupRounds,
		SampleSeconds:       sampleSeconds,
		PollSeconds:         pollSeconds,
		BatchTimeoutSeconds: batchTimeoutSeconds,
		Metadata: map[string]any{
			"engine_version": engine.VersionFull(),
		},
	}
	if host, err := os.Hostname(); err == nil {
		report.Hostname = host
	}

	cfg := serverpkg.DefaultConfig()
	cfg.WorkerCount = workers
	cfg.MaxConcurrentJobs = maxConcurrentJobs
	cfg.MinLevel = minLevel
	if profilePath != "" {
		cfg.ProfilePath = profilePath
	}
	if backend == "sqlite" {
		cfg.Database.Driver = "sqlite"
		cfg.Database.DSN = sqliteDSN
	}

	methodsv2.ClearParentNSCache()
	report.Methodsv2CacheClearedStart = true
	report.Methodsv2CacheEntriesStart = 0
	defer methodsv2.ClearParentNSCache()

	srv, err := serverpkg.NewWithOptions(cfg)
	if err != nil {
		return report, fmt.Errorf("new server: %w", err)
	}
	srv.Start()
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Stop(stopCtx); err != nil {
			log.Printf("server stop: %v", err)
		}
	}()

	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	// Expose pprof on a fixed localhost port so we can grab heap profiles.
	go func() {
		log.Printf("pprof listening on 127.0.0.1:6060 (curl http://127.0.0.1:6060/debug/pprof/heap)")
		_ = http.ListenAndServe("127.0.0.1:6060", nil)
	}()

	started := time.Now()
	sampleCtx, sampleCancel := context.WithCancel(context.Background())
	defer sampleCancel()

	var currentRound atomic.Int64
	var samplesMu sync.Mutex
	var peakMu sync.Mutex
	var peakRSSKB int64
	recordSample := func() {
		rssKB, err := readSelfRSSKB()
		if err != nil {
			log.Printf("rss sample error: %v", err)
			return
		}
		sample := rssSample{
			TimestampUTC:   time.Now().UTC().Format(time.RFC3339Nano),
			ElapsedSeconds: time.Since(started).Seconds(),
			Round:          int(currentRound.Load()),
			RSSKB:          rssKB,
		}
		samplesMu.Lock()
		report.Samples = append(report.Samples, sample)
		samplesMu.Unlock()
		peakMu.Lock()
		if rssKB > peakRSSKB {
			peakRSSKB = rssKB
		}
		peakMu.Unlock()
	}
	recordSample()

	var samplerWG sync.WaitGroup
	samplerWG.Go(func() {
		ticker := time.NewTicker(time.Duration(sampleSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-sampleCtx.Done():
				return
			case <-ticker.C:
				recordSample()
			}
		}
	})

	client := &http.Client{Timeout: 30 * time.Second}
	for round := 1; round <= rounds; round++ {
		currentRound.Store(int64(round))
		log.Printf("round %d/%d: submitting %d domains on backend=%s", round, rounds, len(domains), backend)
		runStart := time.Now()
		batchResp, err := createBatch(client, httpSrv.URL, domains)
		if err != nil {
			return report, fmt.Errorf("round %d create batch: %w", round, err)
		}
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Duration(batchTimeoutSeconds)*time.Second)
		summary, err := waitForBatch(waitCtx, client, httpSrv.URL, batchResp.BatchID, time.Duration(pollSeconds)*time.Second)
		cancel()
		if err != nil {
			return report, fmt.Errorf("round %d wait batch %s: %w", round, batchResp.BatchID, err)
		}
		completed := terminalCount(summary.StatusCounts)
		batch := batchRun{
			Round:             round,
			BatchID:           batchResp.BatchID,
			StartedAtUTC:      runStart.UTC().Format(time.RFC3339Nano),
			FinishedAtUTC:     time.Now().UTC().Format(time.RFC3339Nano),
			WallSeconds:       time.Since(runStart).Seconds(),
			Total:             summary.Total,
			StatusCounts:      cloneStatusCounts(summary.StatusCounts),
			CompletedTerminal: completed,
		}
		report.Batches = append(report.Batches, batch)
		log.Printf("round %d/%d complete: batch=%s wall=%.3fs terminal=%d/%d cache_entries=%d", round, rounds, batch.BatchID, batch.WallSeconds, completed, summary.Total, 0)

		// Take a checkpoint after every round so we can see growth over time.
		label := "round_end"
		if round == warmupRounds {
			label = "post_warmup"
		}
		if round == rounds {
			label = "late_run"
		}
		cp, err := takeGCCheckpoint(label, round, started)
		if err != nil {
			return report, fmt.Errorf("%s checkpoint: %w", label, err)
		}
		report.Checkpoints = append(report.Checkpoints, cp)
		log.Printf("[%s] round=%d rss=%dKB heap_alloc=%dKB heap_inuse=%dKB stack_inuse=%dKB goroutines=%d objects=%d",
			label, round, cp.RSSKB, cp.HeapAllocBytes/1024, cp.HeapInuseBytes/1024, cp.StackInuseBytes/1024, cp.NumGoroutine, cp.HeapObjects)
	}

	sampleCancel()
	samplerWG.Wait()
	recordSample()

	report.Methodsv2CacheEntriesEnd = 0
	peakMu.Lock()
	report.PeakRSSKB = peakRSSKB
	peakMu.Unlock()
	if backend == "sqlite" && sqliteDSN != "" {
		if st, err := os.Stat(sqliteDSN); err == nil {
			report.SQLiteBytes = st.Size()
		}
	}

	return report, nil
}

func createBatch(client *http.Client, baseURL string, domains []string) (serverpkg.JobBatchResponse, error) {
	payload, err := json.Marshal(serverpkg.JobBatchRequest{Domains: domains})
	if err != nil {
		return serverpkg.JobBatchResponse{}, err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/jobs/batch", bytes.NewReader(payload))
	if err != nil {
		return serverpkg.JobBatchResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return serverpkg.JobBatchResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		body, _ := ioReadAllLimit(resp.Body, 1<<20)
		return serverpkg.JobBatchResponse{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out serverpkg.JobBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return serverpkg.JobBatchResponse{}, err
	}
	if out.BatchID == "" {
		return serverpkg.JobBatchResponse{}, errors.New("empty batch_id")
	}
	return out, nil
}

func waitForBatch(ctx context.Context, client *http.Client, baseURL string, batchID string, pollInterval time.Duration) (serverpkg.BatchSummary, error) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return serverpkg.BatchSummary{}, ctx.Err()
		case <-timer.C:
		}

		summary, err := getBatchSummary(client, baseURL, batchID)
		if err != nil {
			return serverpkg.BatchSummary{}, err
		}
		if summary.Total > 0 && terminalCount(summary.StatusCounts) >= summary.Total {
			return summary, nil
		}

		timer.Reset(pollInterval)
	}
}

func getBatchSummary(client *http.Client, baseURL string, batchID string) (serverpkg.BatchSummary, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/batches/"+batchID, nil)
	if err != nil {
		return serverpkg.BatchSummary{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return serverpkg.BatchSummary{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioReadAllLimit(resp.Body, 1<<20)
		return serverpkg.BatchSummary{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var summary serverpkg.BatchSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return serverpkg.BatchSummary{}, err
	}
	return summary, nil
}

func takeGCCheckpoint(label string, round int, started time.Time) (gcCheckpoint, error) {
	runtime.GC()
	runtime.GC()

	rssKB, err := readSelfRSSKB()
	if err != nil {
		return gcCheckpoint{}, err
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return gcCheckpoint{
		Label:                 label,
		Round:                 round,
		TimestampUTC:          time.Now().UTC().Format(time.RFC3339Nano),
		ElapsedSeconds:        time.Since(started).Seconds(),
		RSSKB:                 rssKB,
		HeapAllocBytes:        ms.HeapAlloc,
		HeapInuseBytes:        ms.HeapInuse,
		HeapIdleBytes:         ms.HeapIdle,
		HeapReleasedBytes:     ms.HeapReleased,
		HeapObjects:           ms.HeapObjects,
		StackInuseBytes:       ms.StackInuse,
		SysBytes:              ms.Sys,
		NextGCBytes:           ms.NextGC,
		NumGC:                 ms.NumGC,
		NumGoroutine:          runtime.NumGoroutine(),
		Methodsv2CacheEntries: 0,
	}, nil
}

func readDomains(path string) ([]string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	seen := make(map[string]struct{})
	domains := make([]string, 0, 1024)
	var normalized bytes.Buffer
	for scanner.Scan() {
		line := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		domains = append(domains, line)
		normalized.WriteString(line)
		normalized.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(normalized.Bytes())
	return domains, hex.EncodeToString(sum[:]), nil
}

func readSelfRSSKB() (int64, error) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("unexpected VmRSS line: %q", line)
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse VmRSS %q: %w", fields[1], err)
		}
		return value, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("VmRSS not found in /proc/self/status")
}

func terminalCount(counts map[string]int) int {
	return counts[string(serverpkg.JobSucceeded)] +
		counts[string(serverpkg.JobFailed)] +
		counts[string(serverpkg.JobCanceled)] +
		counts[string(serverpkg.JobExpired)]
}

func cloneStatusCounts(src map[string]int) map[string]int {
	if len(src) == 0 {
		return map[string]int{}
	}
	keys := make([]string, 0, len(src))
	for key := range src {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]int, len(src))
	for _, key := range keys {
		out[key] = src[key]
	}
	return out
}

func ioReadAllLimit(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}
