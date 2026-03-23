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
	if job.Status == JobQueued || job.Status == "" {
		job.Status = JobSucceeded
	}
	if job.FinishedAt.IsZero() {
		job.FinishedAt = job.CreatedAt.Add(time.Second)
	}
	if err := store.GraduateJob(job, entries); err != nil {
		t.Fatalf("graduateTestJob %s: %v", job.ID, err)
	}
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

func TestInMemoryJobStoreGraduateMissingJobReturnsError(t *testing.T) {
	store := NewInMemoryJobStore()
	err := store.GraduateJob(Job{ID: "ghost", Domain: "example.com", Status: JobSucceeded}, nil)
	if err == nil {
		t.Fatal("expected error for missing job")
	}
}

func TestInMemoryJobStorePurgeOlderThanDeletesTerminalRuns(t *testing.T) {
	store := NewInMemoryJobStore()
	cutoff := time.Now().UTC()
	old := cutoff.Add(-24 * time.Hour)

	for _, tc := range []struct {
		id     string
		status JobStatus
	}{
		{"s1", JobSucceeded},
		{"f1", JobFailed},
		{"c1", JobCanceled},
		{"e1", JobExpired},
	} {
		job := Job{ID: tc.id, Domain: "example.com", Status: tc.status, CreatedAt: old, FinishedAt: old}
		if _, err := store.Create(job); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
		job.FinishedAt = old
		if err := store.GraduateJob(job, nil); err != nil {
			t.Fatalf("graduate %s: %v", tc.id, err)
		}
	}

	n, err := store.PurgeOlderThan(cutoff)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 purged, got %d", n)
	}
	// Runs should be gone.
	for _, id := range []string{"s1", "f1", "c1", "e1"} {
		if _, ok := store.GetRun(id); ok {
			t.Fatalf("expected run %s to be deleted after purge", id)
		}
	}
}

func TestInMemoryJobStorePurgeOlderThanPreservesActiveJobs(t *testing.T) {
	store := NewInMemoryJobStore()
	cutoff := time.Now().UTC()
	old := cutoff.Add(-24 * time.Hour)

	for _, tc := range []struct {
		id     string
		status JobStatus
	}{
		{"q1", JobQueued},
		{"r1", JobRunning},
		{"p1", JobPaused},
	} {
		job := Job{ID: tc.id, Domain: "example.com", Status: tc.status, CreatedAt: old}
		if _, err := store.Create(job); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
	}

	n, err := store.PurgeOlderThan(cutoff)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 purged, got %d", n)
	}
	if list := store.List(JobFilter{Limit: 100}); list.Total != 3 {
		t.Fatalf("expected 3 jobs preserved, got %d", list.Total)
	}
}

func TestInMemoryJobStorePurgeOlderThanPreservesNewRuns(t *testing.T) {
	store := NewInMemoryJobStore()
	cutoff := time.Now().UTC()
	recent := cutoff.Add(time.Hour)

	job := Job{ID: "s1", Domain: "example.com", Status: JobSucceeded,
		CreatedAt: recent, FinishedAt: recent}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Re-create since Create won't succeed with a graduated status...
	// Actually we need to graduate it first.
	// Since job is in jobs table, graduate it with a future finishedAt.
	if err := store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	n, err := store.PurgeOlderThan(cutoff)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 purged, got %d", n)
	}
}

func TestInMemoryJobStorePurgeDeletesEntries(t *testing.T) {
	store := NewInMemoryJobStore()
	cutoff := time.Now().UTC()
	old := cutoff.Add(-24 * time.Hour)

	job := Job{ID: "s1", Domain: "example.com", Status: JobSucceeded,
		CreatedAt: old, FinishedAt: old}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.GraduateJob(job, []engine.LogEntry{
		{Module: "DNS", Tag: "TAG", Level: "NOTICE", Timestamp: 1.0},
	}); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	if _, err := store.PurgeOlderThan(cutoff); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, ok := store.GetResult("s1"); ok {
		t.Fatal("expected result to be deleted after purge")
	}
}

func TestInMemoryJobStorePurgeOlderThanReturnsZeroWhenEmpty(t *testing.T) {
	store := NewInMemoryJobStore()
	n, err := store.PurgeOlderThan(time.Now().UTC())
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 on empty store, got %d", n)
	}
}

func TestInMemoryJobStoreCreateSetsPublicID(t *testing.T) {
	store := NewInMemoryJobStore()
	created, err := store.Create(Job{ID: "j1", Domain: "example.com", Status: JobQueued})
	if err != nil {
		t.Fatal(err)
	}
	if created.PublicID == "" {
		t.Fatal("expected PublicID to be set after Create")
	}
}

func TestInMemoryJobStoreGetByPublicIDReturnsJob(t *testing.T) {
	store := NewInMemoryJobStore()
	created, _ := store.Create(Job{ID: "j1", Domain: "example.com", Status: JobQueued})
	got, ok := store.GetByPublicID(created.PublicID)
	if !ok {
		t.Fatal("expected job to be found by public ID")
	}
	if got.ID != "j1" {
		t.Fatalf("got ID %q, want %q", got.ID, "j1")
	}
}

func TestInMemoryJobStoreGetByPublicIDMissingReturnsFalse(t *testing.T) {
	store := NewInMemoryJobStore()
	_, ok := store.GetByPublicID("notexist")
	if ok {
		t.Fatal("expected false for unknown public ID")
	}
}

func TestInMemoryJobStoreCreatePreservesExplicitPublicID(t *testing.T) {
	store := NewInMemoryJobStore()
	created, _ := store.Create(Job{ID: "j1", PublicID: "myid1234", Domain: "example.com", Status: JobQueued})
	if created.PublicID != "myid1234" {
		t.Fatalf("got %q, want %q", created.PublicID, "myid1234")
	}
	got, ok := store.GetByPublicID("myid1234")
	if !ok || got.ID != "j1" {
		t.Fatal("job not reachable by explicit public ID")
	}
}

func TestInMemoryJobStorePurgeRemovesPublicIDIndex(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	created, _ := store.Create(Job{
		ID:     "j1",
		Domain: "example.com",
		Status: JobSucceeded,
	})
	pubID := created.PublicID

	// Graduate it with an old finishedAt.
	created.FinishedAt = now.Add(-48 * time.Hour)
	if err := store.GraduateJob(created, nil); err != nil {
		t.Fatalf("graduate: %v", err)
	}
	store.PurgeOlderThan(now)

	_, ok := store.GetByPublicID(pubID)
	if ok {
		t.Fatal("expected public ID index to be cleaned up after purge")
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

func TestInMemoryJobStoreListRunsByDomain(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC()

	graduateJob := func(id, domain string, i int) {
		job := Job{
			ID:         id,
			Domain:     domain,
			Status:     JobSucceeded,
			CreatedAt:  base.Add(time.Duration(i) * time.Second),
			FinishedAt: base.Add(time.Duration(i)*time.Second + time.Minute),
		}
		if _, err := store.Create(job); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		if err := store.GraduateJob(job, nil); err != nil {
			t.Fatalf("graduate %s: %v", id, err)
		}
	}

	graduateJob("r1", "alpha.example", 0)
	graduateJob("r2", "alpha.example", 1)
	graduateJob("r3", "beta.example", 2)

	d, _ := store.GetDomainByName("alpha.example")

	list := store.ListRunsByDomain(d.ID, 10, 0)
	if list.Total != 2 {
		t.Fatalf("expected 2 runs for alpha.example, got %d", list.Total)
	}
	for _, r := range list.Items {
		if r.Domain != "alpha.example" {
			t.Fatalf("expected only alpha.example runs, got %q", r.Domain)
		}
	}

	// Pagination: limit 1.
	page := store.ListRunsByDomain(d.ID, 1, 0)
	if page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("pagination: total=%d items=%d", page.Total, len(page.Items))
	}

	// Missing domain ID returns empty.
	empty := store.ListRunsByDomain(9999, 10, 0)
	if empty.Total != 0 {
		t.Fatalf("expected 0 for unknown domain, got %d", empty.Total)
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

func TestInMemoryJobStoreGetDomain(t *testing.T) {
	store := NewInMemoryJobStore()
	d, err := store.GetOrCreateDomain("example.com")
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}

	got, ok := store.GetDomain(d.ID)
	if !ok {
		t.Fatal("GetDomain: not found")
	}
	if got.ID != d.ID || got.Name != "example.com" {
		t.Fatalf("unexpected domain: %+v", got)
	}

	_, ok = store.GetDomain(9999)
	if ok {
		t.Fatal("expected false for missing id")
	}
}

func TestInMemoryJobStoreGetDomainByName(t *testing.T) {
	store := NewInMemoryJobStore()
	_, err := store.GetOrCreateDomain("alpha.example")
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}

	got, ok := store.GetDomainByName("alpha.example")
	if !ok {
		t.Fatal("GetDomainByName: not found")
	}
	if got.Name != "alpha.example" {
		t.Fatalf("unexpected name: %q", got.Name)
	}

	_, ok = store.GetDomainByName("notexist.example")
	if ok {
		t.Fatal("expected false for missing name")
	}
}

func TestInMemoryJobStoreUpdateDomainLatest(t *testing.T) {
	store := NewInMemoryJobStore()
	d, err := store.GetOrCreateDomain("example.com")
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}
	if d.RunCount != 0 {
		t.Fatalf("expected RunCount=0, got %d", d.RunCount)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateDomainLatest(d.ID, "run-1", now, "succeeded", "ERROR"); err != nil {
		t.Fatalf("UpdateDomainLatest: %v", err)
	}

	got, ok := store.GetDomain(d.ID)
	if !ok {
		t.Fatal("GetDomain after update: not found")
	}
	if got.LatestRunID != "run-1" {
		t.Fatalf("LatestRunID: got %q, want %q", got.LatestRunID, "run-1")
	}
	if got.LatestStatus != "succeeded" {
		t.Fatalf("LatestStatus: got %q, want %q", got.LatestStatus, "succeeded")
	}
	if got.LatestLevel != "ERROR" {
		t.Fatalf("LatestLevel: got %q, want %q", got.LatestLevel, "ERROR")
	}
	if got.RunCount != 1 {
		t.Fatalf("RunCount: got %d, want 1", got.RunCount)
	}
	if !got.LatestRunAt.Equal(now) {
		t.Fatalf("LatestRunAt: got %v, want %v", got.LatestRunAt, now)
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

func TestInMemoryJobStoreGetTag(t *testing.T) {
	store := NewInMemoryJobStore()
	if err := store.CreateTag("alpha", "first tag"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	d, _ := store.GetOrCreateDomain("example.com")
	_ = store.TagDomains("alpha", []int64{d.ID})

	got, ok := store.GetTag("alpha")
	if !ok {
		t.Fatal("GetTag: not found")
	}
	if got.Name != "alpha" || got.Description != "first tag" || got.DomainCount != 1 {
		t.Fatalf("unexpected tag: %+v", got)
	}

	_, ok = store.GetTag("notexist")
	if ok {
		t.Fatal("expected false for missing tag")
	}
}

func TestInMemoryJobStoreUpdateTag(t *testing.T) {
	store := NewInMemoryJobStore()
	_ = store.CreateTag("beta", "old description")

	if err := store.UpdateTag("beta", "new description"); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}
	got, _ := store.GetTag("beta")
	if got.Description != "new description" {
		t.Fatalf("expected updated description, got %q", got.Description)
	}

	if err := store.UpdateTag("notexist", "x"); err == nil {
		t.Fatal("expected error for missing tag")
	}
}

func TestInMemoryJobStoreDeleteTag(t *testing.T) {
	store := NewInMemoryJobStore()
	d, _ := store.GetOrCreateDomain("example.com")
	_ = store.CreateTag("gamma", "")
	_ = store.TagDomains("gamma", []int64{d.ID})

	if err := store.DeleteTag("gamma"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, ok := store.GetTag("gamma"); ok {
		t.Fatal("expected tag to be deleted")
	}
	// Domain tag association should also be gone.
	tags := store.GetDomainTags(d.ID)
	for _, tag := range tags {
		if tag == "gamma" {
			t.Fatal("expected domain_tag association to be removed")
		}
	}

	if err := store.DeleteTag("notexist"); err == nil {
		t.Fatal("expected error for missing tag")
	}
}

func TestInMemoryJobStoreUntagDomains(t *testing.T) {
	store := NewInMemoryJobStore()
	d1, _ := store.GetOrCreateDomain("a.example")
	d2, _ := store.GetOrCreateDomain("b.example")
	_ = store.TagDomains("delta", []int64{d1.ID, d2.ID})

	if err := store.UntagDomains("delta", []int64{d1.ID}); err != nil {
		t.Fatalf("UntagDomains: %v", err)
	}

	got, _ := store.GetTag("delta")
	if got.DomainCount != 1 {
		t.Fatalf("expected 1 domain after untag, got %d", got.DomainCount)
	}
	tags := store.GetDomainTags(d1.ID)
	for _, tag := range tags {
		if tag == "delta" {
			t.Fatal("expected d1 to be untagged")
		}
	}
	tags2 := store.GetDomainTags(d2.ID)
	found := false
	for _, tag := range tags2 {
		if tag == "delta" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected d2 to still be tagged")
	}
}

func TestInMemoryJobStoreGetDomainTags(t *testing.T) {
	store := NewInMemoryJobStore()
	d, _ := store.GetOrCreateDomain("example.com")

	if tags := store.GetDomainTags(d.ID); len(tags) != 0 {
		t.Fatalf("expected no tags initially, got %v", tags)
	}

	_ = store.TagDomains("x", []int64{d.ID})
	_ = store.TagDomains("y", []int64{d.ID})

	tags := store.GetDomainTags(d.ID)
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %v", tags)
	}
}

func TestInMemoryJobStoreListDomainsByTag(t *testing.T) {
	store := NewInMemoryJobStore()
	d1, _ := store.GetOrCreateDomain("a.example")
	d2, _ := store.GetOrCreateDomain("b.example")
	_, _ = store.GetOrCreateDomain("c.example") // untagged
	_ = store.TagDomains("group", []int64{d1.ID, d2.ID})

	result := store.ListDomainsByTag("group", DomainFilter{Limit: 10})
	if result.Total != 2 {
		t.Fatalf("expected 2 domains in tag, got %d", result.Total)
	}
}

func TestInMemoryJobStoreGetTagSummary(t *testing.T) {
	store := NewInMemoryJobStore()
	d1, _ := store.GetOrCreateDomain("a.example")
	d2, _ := store.GetOrCreateDomain("b.example")
	d3, _ := store.GetOrCreateDomain("c.example")
	_ = store.TagDomains("summary-tag", []int64{d1.ID, d2.ID, d3.ID})

	// Graduate jobs to set latest_level.
	for _, tc := range []struct {
		domain string
		id     string
		level  string
	}{
		{"a.example", "run-a", "ERROR"},
		{"b.example", "run-b", "WARNING"},
		{"c.example", "run-c", ""},
	} {
		job := Job{
			ID:         tc.id,
			Domain:     tc.domain,
			Status:     JobSucceeded,
			CreatedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
		}
		if _, err := store.Create(job); err != nil {
			t.Fatalf("Create %s: %v", tc.id, err)
		}
		if err := store.GraduateJob(job, nil); err != nil {
			t.Fatalf("GraduateJob %s: %v", tc.id, err)
		}
		d, _ := store.GetDomainByName(tc.domain)
		_ = store.UpdateDomainLatest(d.ID, tc.id, time.Now().UTC(), "succeeded", tc.level)
	}

	summary, ok := store.GetTagSummary("summary-tag")
	if !ok {
		t.Fatal("GetTagSummary: not found")
	}
	if summary.DomainCount != 3 {
		t.Fatalf("DomainCount: got %d, want 3", summary.DomainCount)
	}
	if summary.Error != 1 {
		t.Fatalf("Error: got %d, want 1", summary.Error)
	}
	if summary.Warning != 1 {
		t.Fatalf("Warning: got %d, want 1", summary.Warning)
	}
	if summary.OK != 1 {
		t.Fatalf("OK: got %d, want 1", summary.OK)
	}

	_, ok = store.GetTagSummary("notexist")
	if ok {
		t.Fatal("expected false for missing tag")
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
	for i := 0; i < totalJobs; i++ {
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
	for w := 0; w < 4; w++ {
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
	for w := 0; w < 4; w++ {
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
