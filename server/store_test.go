package server

import (
	"fmt"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// graduateTestJob is a test helper that graduates a job with optional entries.
func graduateTestJob(t *testing.T, store *InMemoryJobStore, job Job, entries []engine.LogEntry) {
	t.Helper()
	graduate(t, store, job, entries)
}

func TestInMemoryJobStoreCRUD(t *testing.T) {
	store := NewInMemoryJobStore()
	job := Job{
		ID:        "job1",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}
	created, err := store.Create(job)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID != job.ID {
		t.Fatalf("expected id %s, got %s", job.ID, created.ID)
	}

	got, ok := store.Get(job.ID)
	if !ok {
		t.Fatalf("expected job in store")
	}
	if got.Domain != job.Domain {
		t.Fatalf("expected domain %s, got %s", job.Domain, got.Domain)
	}

	got.Status = JobRunning
	if err := store.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}

	list := store.List(JobFilter{Status: JobRunning, Limit: 10})
	if list.Total != 1 {
		t.Fatalf("expected total 1, got %d", list.Total)
	}
	if len(list.Items) != 1 || list.Items[0].ID != job.ID {
		t.Fatalf("expected job in list")
	}

	// Graduate the job and verify result is accessible.
	got.Status = JobSucceeded
	got.FinishedAt = time.Now().UTC()
	graduateTestJob(t, store, got, nil)

	// After graduation job is in runs, not in-flight.
	list = store.List(JobFilter{Limit: 10})
	if list.Total != 0 {
		t.Fatalf("expected 0 in-flight jobs after graduation, got %d", list.Total)
	}

	// Get still works via runs lookup.
	graduated, ok := store.Get(job.ID)
	if !ok {
		t.Fatalf("expected job to be accessible after graduation")
	}
	if graduated.Status != JobSucceeded {
		t.Fatalf("expected status succeeded, got %s", graduated.Status)
	}

	// GetResult works.
	result, ok := store.GetResult(job.ID)
	if !ok {
		t.Fatalf("expected result in store")
	}
	if result.Status != JobSucceeded {
		t.Fatalf("expected result status succeeded, got %s", result.Status)
	}
}

func TestInMemoryJobStorePreservesNameserverTimingsInResult(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	job := Job{
		ID:        "job-ns-timings",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: now,
	}
	created, err := store.Create(job)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	created.Status = JobSucceeded
	created.StartedAt = now
	created.FinishedAt = now.Add(2 * time.Second)
	created.NameserverTimings = []NameserverTiming{
		{
			Nameserver: "ns1.example.com",
			Address:    "192.0.2.10",
			AvgMS:      24,
			MinMS:      20,
			MaxMS:      30,
			MedianMS:   22,
			StddevMS:   4,
			Count:      3,
		},
	}
	graduateTestJob(t, store, created, nil)

	result, ok := store.GetResult(created.ID)
	if !ok {
		t.Fatal("expected result")
	}
	if len(result.NameserverTimings) != 1 {
		t.Fatalf("nameserver timings len = %d, want 1", len(result.NameserverTimings))
	}
	if result.NameserverTimings[0].Nameserver != "ns1.example.com" {
		t.Fatalf("nameserver = %q", result.NameserverTimings[0].Nameserver)
	}
}

func TestInMemoryJobStoreFilters(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC().Add(-time.Minute)
	_ = seedJob(store, "job1", "batch1", base, JobQueued)
	_ = seedJob(store, "job2", "batch2", base.Add(time.Second), JobRunning)
	_, _ = store.Create(Job{
		ID:        "job3",
		BatchID:   "batch3",
		Domain:    "alpha.example.org",
		Status:    JobQueued,
		CreatedAt: base.Add(2 * time.Second),
	})

	list := store.List(JobFilter{BatchID: "batch2", Limit: 10})
	if list.Total != 1 || list.Items[0].ID != "job2" {
		t.Fatalf("expected batch filter to return job2")
	}

	list = store.List(JobFilter{CreatedAfter: base.Add(500 * time.Millisecond), Limit: 10})
	if list.Total != 2 || list.Items[0].ID != "job3" || list.Items[1].ID != "job2" {
		t.Fatalf("expected created_after filter to return job3 and job2")
	}

	list = store.List(JobFilter{CreatedBefore: base.Add(500 * time.Millisecond), Limit: 10})
	if list.Total != 1 || list.Items[0].ID != "job1" {
		t.Fatalf("expected created_before filter to return job1")
	}

	list = store.List(JobFilter{Domain: "alpha", Limit: 10})
	if list.Total != 1 || list.Items[0].ID != "job3" {
		t.Fatalf("expected domain filter to return job3")
	}
}

func TestInMemoryJobStoreSorting(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC().Add(-time.Minute)

	_, _ = store.Create(Job{
		ID:        "job1",
		BatchID:   "batch_b",
		Domain:    "zeta.example",
		Status:    JobQueued,
		CreatedAt: base,
		StartedAt: base.Add(2 * time.Second),
	})
	_, _ = store.Create(Job{
		ID:        "job2",
		BatchID:   "batch_a",
		Domain:    "alpha.example",
		Status:    JobQueued,
		CreatedAt: base.Add(time.Second),
		StartedAt: base.Add(3 * time.Second),
	})
	_, _ = store.Create(Job{
		ID:        "job3",
		BatchID:   "batch_c",
		Domain:    "beta.example",
		Status:    JobQueued,
		CreatedAt: base.Add(4 * time.Second),
	})

	defaultList := store.List(JobFilter{Limit: 10})
	if len(defaultList.Items) != 3 || defaultList.Items[0].ID != "job3" {
		t.Fatalf("expected default started_at_desc sorting with created_at fallback")
	}

	domainAsc := store.List(JobFilter{Limit: 10, Sort: JobSortDomainAsc})
	if len(domainAsc.Items) != 3 || domainAsc.Items[0].ID != "job2" {
		t.Fatalf("expected domain_asc sorting to return alpha first")
	}

	batchIDAsc := store.List(JobFilter{Limit: 10, Sort: JobSortBatchIDAsc})
	if len(batchIDAsc.Items) != 3 || batchIDAsc.Items[0].ID != "job2" {
		t.Fatalf("expected batch_id_asc sorting to return batch_a first")
	}

	batchIDDesc := store.List(JobFilter{Limit: 10, Sort: JobSortBatchIDDesc})
	if len(batchIDDesc.Items) != 3 || batchIDDesc.Items[0].ID != "job3" {
		t.Fatalf("expected batch_id_desc sorting to return batch_c first")
	}

	startedAsc := store.List(JobFilter{Limit: 10, Sort: JobSortStartedAtAsc})
	if len(startedAsc.Items) != 3 || startedAsc.Items[0].ID != "job1" {
		t.Fatalf("expected started_at_asc sorting to return earliest effective start first")
	}
}

func TestInMemoryJobStorePaginationMetadata(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC().Add(-time.Minute)
	_ = seedJob(store, "job1", "batch1", base, JobQueued)
	_ = seedJob(store, "job2", "batch1", base.Add(time.Second), JobQueued)
	_ = seedJob(store, "job3", "batch1", base.Add(2*time.Second), JobQueued)

	first := store.List(JobFilter{Limit: 1, Sort: JobSortCreatedAtAsc})
	if first.Total != 3 {
		t.Fatalf("expected total 3, got %d", first.Total)
	}
	if len(first.Items) != 1 || first.Items[0].ID != "job1" {
		t.Fatalf("expected first page to include job1")
	}
	if first.NextCursor != "1" || first.PrevCursor != "" {
		t.Fatalf("expected next cursor 1 and no prev cursor, got next=%q prev=%q", first.NextCursor, first.PrevCursor)
	}

	second := store.List(JobFilter{Limit: 1, Sort: JobSortCreatedAtAsc, Offset: 1})
	if len(second.Items) != 1 || second.Items[0].ID != "job2" {
		t.Fatalf("expected second page to include job2")
	}
	if second.NextCursor != "2" || second.PrevCursor != "0" {
		t.Fatalf("expected next cursor 2 and prev cursor 0, got next=%q prev=%q", second.NextCursor, second.PrevCursor)
	}
}

func TestInMemoryJobStoreGraduateJobCreatesRun(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	job := Job{
		ID:        "j1",
		PublicID:  "pub12345",
		Domain:    "example.com",
		BatchID:   "batch1",
		Status:    JobRunning,
		CreatedAt: now.Add(-time.Minute),
		StartedAt: now.Add(-30 * time.Second),
		Progress:  100,
	}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}

	job.Status = JobSucceeded
	job.FinishedAt = now
	entries := []engine.LogEntry{
		{Module: "DNS", Testcase: "dns01", Tag: "DNS01", Level: "WARNING", Timestamp: 1.0},
		{Module: "DNS", Testcase: "dns01", Tag: "DNS02", Level: "ERROR", Timestamp: 2.0},
	}
	if err := store.GraduateJob(job, entries); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	// Job should be gone from in-flight list.
	if list := store.List(JobFilter{Limit: 10}); list.Total != 0 {
		t.Fatalf("expected 0 in-flight jobs after graduation")
	}

	// Run should be accessible.
	run, ok := store.GetRun("j1")
	if !ok {
		t.Fatal("expected run after graduation")
	}
	if run.Status != JobSucceeded {
		t.Fatalf("run status = %s, want succeeded", run.Status)
	}
	if run.SevWarning != 1 || run.SevError != 1 {
		t.Fatalf("unexpected severity: warning=%d error=%d", run.SevWarning, run.SevError)
	}
	if run.WorstLevel != "ERROR" {
		t.Fatalf("worst_level = %q, want ERROR", run.WorstLevel)
	}
	if run.EntryCount != 2 {
		t.Fatalf("entry_count = %d, want 2", run.EntryCount)
	}

	// GetByPublicID should work after graduation.
	gotJob, ok := store.GetByPublicID("pub12345")
	if !ok {
		t.Fatal("expected job by public ID after graduation")
	}
	if gotJob.Status != JobSucceeded {
		t.Fatalf("got status %s, want succeeded", gotJob.Status)
	}

	// GetResult should return result with entries.
	result, ok := store.GetResult("j1")
	if !ok {
		t.Fatal("expected result after graduation")
	}
	if result.Status != JobSucceeded {
		t.Fatalf("result status = %s, want succeeded", result.Status)
	}
	if result.Raw == nil || len(result.Raw.Entries) != 2 {
		t.Fatalf("expected 2 raw entries, got %v", result.Raw)
	}
	if result.Summary == nil {
		t.Fatal("expected summary")
	}
}

func TestInMemoryJobStoreGraduateCreatesOrUpdatesDomain(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	job := Job{
		ID:        "j1",
		Domain:    "example.com",
		Status:    JobRunning,
		CreatedAt: now.Add(-time.Minute),
	}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	job.Status = JobSucceeded
	job.FinishedAt = now
	if err := store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	// Domain should be created.
	d, ok := store.domains["example.com"]
	if !ok {
		t.Fatal("expected domain to be created")
	}
	if d.RunCount != 1 {
		t.Fatalf("run_count = %d, want 1", d.RunCount)
	}
	if d.LatestRunID != "j1" {
		t.Fatalf("latest_run_id = %q, want j1", d.LatestRunID)
	}
}

func TestInMemoryJobStoreListRunsFiltersAndSorts(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC()

	// Create and graduate several jobs.
	for i, tc := range []struct {
		id     string
		domain string
		batch  string
		status JobStatus
		warn   int
		err    int
		crit   int
	}{
		{"r1", "alpha.example", "b1", JobSucceeded, 0, 1, 0},
		{"r2", "beta.example", "b1", JobSucceeded, 2, 0, 0},
		{"r3", "gamma.example", "b2", JobFailed, 0, 0, 1},
	} {
		job := Job{
			ID:        tc.id,
			Domain:    tc.domain,
			BatchID:   tc.batch,
			Status:    tc.status,
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}
		if _, err := store.Create(job); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
		job.FinishedAt = base.Add(time.Duration(i)*time.Second + time.Minute)
		entries := make([]engine.LogEntry, 0, tc.warn+tc.err+tc.crit)
		for j := 0; j < tc.warn; j++ {
			entries = append(entries, engine.LogEntry{Level: "WARNING", Tag: "T", Module: "M"})
		}
		for j := 0; j < tc.err; j++ {
			entries = append(entries, engine.LogEntry{Level: "ERROR", Tag: "T", Module: "M"})
		}
		for j := 0; j < tc.crit; j++ {
			entries = append(entries, engine.LogEntry{Level: "CRITICAL", Tag: "T", Module: "M"})
		}
		if err := store.GraduateJob(job, entries); err != nil {
			t.Fatalf("graduate %s: %v", tc.id, err)
		}
	}

	// Filter by batch.
	list := store.ListRuns(RunFilter{BatchID: "b1", Limit: 10})
	if list.Total != 2 {
		t.Fatalf("batch filter: expected 2, got %d", list.Total)
	}

	// Filter by domain.
	list = store.ListRuns(RunFilter{Domain: "alpha", Limit: 10})
	if list.Total != 1 || list.Items[0].ID != "r1" {
		t.Fatalf("domain filter: expected r1, got %v", list.Items)
	}

	// Sort by error_desc.
	list = store.ListRuns(RunFilter{Limit: 10, Sort: JobSortErrorDesc})
	if list.Total != 3 || list.Items[0].ID != "r3" || list.Items[1].ID != "r1" {
		t.Fatalf("error_desc sort: expected r3, r1 first, got %v", list.Items)
	}
}

func TestInMemoryJobStoreGetOrCreateDomain(t *testing.T) {
	store := NewInMemoryJobStore()
	d1, err := store.GetOrCreateDomain("example.com")
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}
	if d1.ID == 0 || d1.Name != "example.com" {
		t.Fatalf("unexpected domain: %+v", d1)
	}

	d2, err := store.GetOrCreateDomain("example.com")
	if err != nil {
		t.Fatalf("GetOrCreateDomain second call: %v", err)
	}
	if d2.ID != d1.ID {
		t.Fatalf("expected same ID on second call, got %d vs %d", d1.ID, d2.ID)
	}
}

func TestInMemoryJobStoreGetOrCreateDomainNormalizesName(t *testing.T) {
	store := NewInMemoryJobStore()

	// Uppercase input should be stored as lowercase and deduplicated.
	d1, err := store.GetOrCreateDomain("EXAMPLE.COM")
	if err != nil {
		t.Fatalf("GetOrCreateDomain uppercase: %v", err)
	}
	if d1.Name != "example.com" {
		t.Fatalf("expected lowercase name, got %q", d1.Name)
	}

	d2, err := store.GetOrCreateDomain("example.com")
	if err != nil {
		t.Fatalf("GetOrCreateDomain lowercase: %v", err)
	}
	if d2.ID != d1.ID {
		t.Fatalf("uppercase and lowercase should resolve to the same domain ID")
	}

	// Unicode IDN label should be converted to ACE.
	d3, err := store.GetOrCreateDomain("münchen.de")
	if err != nil {
		t.Fatalf("GetOrCreateDomain IDN: %v", err)
	}
	if d3.Name != "xn--mnchen-3ya.de" {
		t.Fatalf("expected ACE form xn--mnchen-3ya.de, got %q", d3.Name)
	}
}

func TestInMemoryJobStoreUpdateDomainLatestMissingReturnsError(t *testing.T) {
	store := NewInMemoryJobStore()
	err := store.UpdateDomainLatest(9999, "run-x", time.Now().UTC(), "succeeded", "")
	if err == nil {
		t.Fatal("expected error for missing domain ID")
	}
}

func TestInMemoryJobStoreGetDomainIncludesTags(t *testing.T) {
	store := NewInMemoryJobStore()
	d, _ := store.GetOrCreateDomain("tagged.example")
	_ = store.CreateTag("mytag", "")
	_ = store.TagDomains("mytag", []int64{d.ID})

	got, ok := store.GetDomain(d.ID)
	if !ok {
		t.Fatal("GetDomain: not found")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "mytag" {
		t.Fatalf("expected Tags=[mytag], got %v", got.Tags)
	}

	byName, ok := store.GetDomainByName("tagged.example")
	if !ok {
		t.Fatal("GetDomainByName: not found")
	}
	if len(byName.Tags) != 1 || byName.Tags[0] != "mytag" {
		t.Fatalf("expected Tags=[mytag] from GetDomainByName, got %v", byName.Tags)
	}
}

func TestInMemoryJobStoreCreateBatchGetBatch(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	if err := store.CreateBatch(Batch{ID: "b1", Description: "test", DomainCount: 5, CreatedAt: now}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	got, ok := store.GetBatch("b1")
	if !ok {
		t.Fatal("GetBatch: not found")
	}
	if got.Description != "test" || got.DomainCount != 5 {
		t.Fatalf("unexpected batch: %+v", got)
	}
	_, ok = store.GetBatch("missing")
	if ok {
		t.Fatal("expected false for missing batch")
	}
}

func TestInMemoryJobStoreListBatchesByTag(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC()
	for i, entry := range []struct {
		id  string
		tag string
	}{
		{"b1", "tld"},
		{"b2", "tld"},
		{"b3", "muni"},
	} {
		if err := store.CreateBatch(Batch{
			ID:        entry.id,
			Tag:       entry.tag,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("CreateBatch %s: %v", entry.id, err)
		}
	}
	list := store.ListBatchesByTag("tld", 10, 0)
	if list.Total != 2 {
		t.Fatalf("Total = %d, want 2", list.Total)
	}
	if list.Items[0].ID != "b2" || list.Items[1].ID != "b1" {
		t.Fatalf("order = %v, want [b2 b1]", []string{list.Items[0].ID, list.Items[1].ID})
	}
}

func TestInMemoryJobStoreListBatches(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC()
	for i, entry := range []struct {
		id  string
		tag string
	}{
		{"b1", "tld-weekly"},
		{"b2", "se-batch"},
		{"b3", "tld-monthly"},
	} {
		if err := store.CreateBatch(Batch{
			ID:        entry.id,
			Tag:       entry.tag,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("CreateBatch %s: %v", entry.id, err)
		}
	}

	// All batches, newest first.
	all := store.ListBatches("", 10, 0)
	if all.Total != 3 {
		t.Fatalf("Total = %d, want 3", all.Total)
	}
	if all.Items[0].ID != "b3" || all.Items[2].ID != "b1" {
		t.Fatalf("order = %v, want newest first", []string{all.Items[0].ID, all.Items[1].ID, all.Items[2].ID})
	}

	// Substring tag filter is case-insensitive and matches both tld batches.
	tld := store.ListBatches("TLD", 10, 0)
	if tld.Total != 2 {
		t.Fatalf("label=TLD Total = %d, want 2", tld.Total)
	}
	if tld.Items[0].ID != "b3" || tld.Items[1].ID != "b1" {
		t.Fatalf("label=TLD order = %v, want [b3 b1]", []string{tld.Items[0].ID, tld.Items[1].ID})
	}

	// Pagination.
	page := store.ListBatches("", 1, 1)
	if page.Total != 3 || len(page.Items) != 1 || page.Items[0].ID != "b2" {
		t.Fatalf("limit=1 offset=1 = %+v, want only b2", page)
	}
}

func TestInMemoryJobStoreDeleteBatchRemovesEverything(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	if err := store.CreateBatch(Batch{ID: "b1", Tag: "tld", CreatedAt: now}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if err := seedJob(store, "j1", "b1", now, JobSucceeded); err != nil {
		t.Fatalf("seedJob: %v", err)
	}
	job, _ := store.Get("j1")
	job.FinishedAt = now
	if err := store.GraduateJob(job, nil); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	if _, err := store.DeleteBatch("b1"); err != nil {
		t.Fatalf("DeleteBatch: %v", err)
	}
	if _, ok := store.GetBatch("b1"); ok {
		t.Fatal("batch still present")
	}
	if _, ok := store.GetRun("j1"); ok {
		t.Fatal("run still present")
	}
}

func TestInMemoryJobStoreBatchDeletePreviewStats(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	if err := store.CreateBatch(Batch{ID: "b1", Tag: "tld", CreatedAt: now, SnapshotIntent: true}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if err := seedJob(store, "j1", "b1", now, JobQueued); err != nil {
		t.Fatalf("seedJob queued: %v", err)
	}
	if err := seedJob(store, "j2", "b1", now, JobRunning); err != nil {
		t.Fatalf("seedJob running: %v", err)
	}

	preview, err := store.BatchDeletePreviewStats("b1")
	if err != nil {
		t.Fatalf("BatchDeletePreviewStats: %v", err)
	}
	if !preview.Exists {
		t.Fatal("Exists should be true")
	}
	if preview.QueuedJobs != 1 || preview.RunningJobs != 1 {
		t.Fatalf("queued=%d running=%d", preview.QueuedJobs, preview.RunningJobs)
	}
	if !preview.SnapshotIntent {
		t.Fatal("SnapshotIntent should round-trip")
	}
}

func seedJob(store *InMemoryJobStore, id, batch string, created time.Time, status JobStatus) error {
	job := Job{
		ID:        id,
		BatchID:   batch,
		Domain:    "example.com",
		Status:    status,
		CreatedAt: created,
	}
	_, err := store.Create(job)
	return err
}

func TestInMemoryJobStoreConcurrentBasic(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC().Add(-time.Minute)
	const totalJobs = 50
	for i := range totalJobs {
		id := fmt.Sprintf("job-%03d", i)
		if _, err := store.Create(Job{
			ID:        id,
			Domain:    fmt.Sprintf("bench-%03d.example", i),
			Status:    JobQueued,
			CreatedAt: base.Add(time.Duration(i) * time.Millisecond),
		}); err != nil {
			t.Fatalf("seed create %s: %v", id, err)
		}
	}

	done := make(chan struct{})
	errors := make(chan error, 100)

	// Concurrent updates.
	for w := range 4 {
		go func(w int) {
			for {
				select {
				case <-done:
					return
				default:
				}
				id := fmt.Sprintf("job-%03d", w%totalJobs)
				if job, ok := store.Get(id); ok && job.Status == JobQueued {
					job.Status = JobRunning
					if err := store.Update(job); err != nil && err.Error() != "job not found" {
						errors <- err
					}
				}
			}
		}(w)
	}

	// Concurrent reads.
	for range 4 {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				list := store.List(JobFilter{Limit: 20, Sort: JobSortCreatedAtAsc})
				if list.Total < 0 {
					errors <- fmt.Errorf("negative total")
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(done)

	select {
	case err := <-errors:
		t.Fatalf("concurrent error: %v", err)
	default:
	}
}
