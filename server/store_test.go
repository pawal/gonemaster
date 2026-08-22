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

func TestInMemoryJobStorePurgeByTagDeletesTaggedRuns(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()

	// Two domains tagged "tld", one untagged domain.
	for _, name := range []string{"se", "dk", "other.example"} {
		id := name
		job := Job{ID: id, Domain: name, Status: JobSucceeded, CreatedAt: now, FinishedAt: now}
		if _, err := store.Create(job); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		if err := store.GraduateJob(job, []engine.LogEntry{
			{Module: "DNS", Tag: "TAG", Level: "NOTICE", Timestamp: 1.0},
		}); err != nil {
			t.Fatalf("graduate %s: %v", id, err)
		}
	}
	if err := store.CreateTag("tld", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	for _, name := range []string{"se", "dk"} {
		d, ok := store.GetDomainByName(name)
		if !ok {
			t.Fatalf("domain %s not found", name)
		}
		if err := store.TagDomains("tld", []int64{d.ID}); err != nil {
			t.Fatalf("TagDomains %s: %v", name, err)
		}
	}

	n, err := store.PurgeByTag("tld")
	if err != nil {
		t.Fatalf("purge by tag: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 purged, got %d", n)
	}
	// Tagged runs (and their entries) are gone.
	for _, id := range []string{"se", "dk"} {
		if _, ok := store.GetRun(id); ok {
			t.Fatalf("expected run %s to be deleted", id)
		}
		if _, ok := store.GetResult(id); ok {
			t.Fatalf("expected entries for %s to be deleted", id)
		}
	}
	// The untagged domain's run survives.
	if _, ok := store.GetRun("other.example"); !ok {
		t.Fatal("expected untagged run to survive purge by tag")
	}
}

func TestInMemoryJobStorePurgeByTagPreservesActiveJobs(t *testing.T) {
	store := NewInMemoryJobStore()

	d, err := store.GetOrCreateDomain("se")
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}
	job := Job{ID: "q1", Domain: "se", Status: JobQueued, CreatedAt: time.Now().UTC()}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.CreateTag("tld", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("TagDomains: %v", err)
	}

	n, err := store.PurgeByTag("tld")
	if err != nil {
		t.Fatalf("purge by tag: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 purged, got %d", n)
	}
	if list := store.List(JobFilter{Limit: 100}); list.Total != 1 {
		t.Fatalf("expected queued job preserved, got %d", list.Total)
	}
}

func TestInMemoryJobStorePurgeByTagReturnsZeroForUnknownTag(t *testing.T) {
	store := NewInMemoryJobStore()
	n, err := store.PurgeByTag("nope")
	if err != nil {
		t.Fatalf("purge by tag: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 for unknown tag, got %d", n)
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
		createAndGraduate(t, store, job, nil)
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

func TestInMemoryJobStoreSetTagDefaultProfile(t *testing.T) {
	store := NewInMemoryJobStore()
	if err := store.CreateTag("beta", "profiled tag"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	profile, err := store.CreateProfile(StoredProfile{Name: "default", Config: "{}"})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	if err := store.SetTagDefaultProfile("beta", &profile.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile: %v", err)
	}
	tag, ok := store.GetTag("beta")
	if !ok {
		t.Fatal("GetTag: not found")
	}
	if tag.DefaultProfileID == nil || *tag.DefaultProfileID != profile.ID {
		t.Fatalf("DefaultProfileID: got %v, want %d", tag.DefaultProfileID, profile.ID)
	}

	tags := store.ListTags(10, 0)
	if len(tags) != 1 {
		t.Fatalf("ListTags: got %d, want 1", len(tags))
	}
	if tags[0].DefaultProfileID == nil || *tags[0].DefaultProfileID != profile.ID {
		t.Fatalf("ListTags DefaultProfileID: got %v, want %d", tags[0].DefaultProfileID, profile.ID)
	}

	if err := store.SetTagDefaultProfile("beta", nil); err != nil {
		t.Fatalf("clear SetTagDefaultProfile: %v", err)
	}
	tag, _ = store.GetTag("beta")
	if tag.DefaultProfileID != nil {
		t.Fatalf("expected cleared DefaultProfileID, got %v", *tag.DefaultProfileID)
	}

	missingID := profile.ID + 1000
	if err := store.SetTagDefaultProfile("beta", &missingID); err == nil {
		t.Fatal("expected error for missing profile")
	}
	if err := store.SetTagDefaultProfile("missing-tag", &profile.ID); err == nil {
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
		createAndGraduate(t, store, job, nil)
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

func TestInMemoryJobStoreQueryEntries(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC()

	grad := func(id, domain, batchID string, entries []engine.LogEntry) {
		t.Helper()
		job := Job{
			ID:        id,
			Domain:    domain,
			BatchID:   batchID,
			Status:    JobSucceeded,
			CreatedAt: base,
		}
		createAndGraduate(t, store, job, entries)
	}

	grad("run1", "alpha.example", "batch1", []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "DNSSEC01", Tag: "DS01_ALGO_SHA1", Level: "WARNING"},
		{Module: "DNSSEC", Testcase: "DNSSEC02", Tag: "DS02_NO_DS", Level: "ERROR"},
	})
	grad("run2", "beta.example", "batch1", []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "DNSSEC01", Tag: "DS01_ALGO_SHA1", Level: "NOTICE"},
	})
	grad("run3", "gamma.example", "", []engine.LogEntry{
		{Module: "BASIC", Testcase: "BASIC01", Tag: "BASIC01_NO_GLUE", Level: "ERROR"},
	})

	// Tag beta and gamma.
	betaDomain, _ := store.GetDomainByName("beta.example")
	gammaDomain, _ := store.GetDomainByName("gamma.example")
	if err := store.CreateTag("tagged", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := store.TagDomains("tagged", []int64{betaDomain.ID, gammaDomain.ID}); err != nil {
		t.Fatalf("TagDomains: %v", err)
	}

	t.Run("no filter returns all", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{})
		if list.Total != 4 {
			t.Fatalf("expected 4 entries, got %d", list.Total)
		}
	})

	t.Run("filter by run_id", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{RunID: "run1"})
		if list.Total != 2 {
			t.Fatalf("expected 2, got %d", list.Total)
		}
	})

	t.Run("filter by module", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{Module: "DNSSEC"})
		if list.Total != 3 {
			t.Fatalf("expected 3, got %d", list.Total)
		}
	})

	t.Run("filter by testcase", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{Testcase: "DNSSEC01"})
		if list.Total != 2 {
			t.Fatalf("expected 2, got %d", list.Total)
		}
	})

	t.Run("filter by entry_tag", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{EntryTag: "DS02_NO_DS"})
		if list.Total != 1 {
			t.Fatalf("expected 1, got %d", list.Total)
		}
	})

	t.Run("filter by level", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{Level: "ERROR"})
		if list.Total != 2 {
			t.Fatalf("expected 2, got %d", list.Total)
		}
	})

	t.Run("filter by domain tag", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{Tag: "tagged"})
		// beta has 1, gamma has 1 = 2 total
		if list.Total != 2 {
			t.Fatalf("expected 2, got %d", list.Total)
		}
	})

	t.Run("filter by batch_id", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{BatchID: "batch1"})
		if list.Total != 3 {
			t.Fatalf("expected 3, got %d", list.Total)
		}
	})

	t.Run("latest only", func(t *testing.T) {
		// Graduate a second run for alpha.
		grad("run1b", "alpha.example", "", []engine.LogEntry{
			{Module: "BASIC", Testcase: "BASIC01", Tag: "B", Level: "NOTICE"},
		})
		list := store.QueryEntries(EntryFilter{LatestOnly: true, Module: "BASIC"})
		// Only run1b (latest for alpha) contributes a BASIC entry.
		// gamma also has a BASIC entry and is its own latest.
		if list.Total != 2 {
			t.Fatalf("expected 2 (latest-only BASIC entries), got %d", list.Total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		list := store.QueryEntries(EntryFilter{Limit: 2, Offset: 0})
		if len(list.Items) != 2 {
			t.Fatalf("expected 2 items on page 1, got %d", len(list.Items))
		}
		if list.NextCursor == "" {
			t.Fatal("expected NextCursor to be set")
		}
	})
}

func TestInMemoryStorePriorityPersistedOnJob(t *testing.T) {
	s := NewInMemoryJobStore()
	now := time.Now().UTC()
	job := Job{ID: "pj1", Domain: "example.com", Status: JobQueued, CreatedAt: now, Priority: PriorityBatch}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, ok := s.Get(job.ID)
	if !ok {
		t.Fatal("Get: not found")
	}
	if got.Priority != PriorityBatch {
		t.Fatalf("Priority: got %d, want %d", got.Priority, PriorityBatch)
	}
}

func TestInMemoryStorePriorityPersistedOnRun(t *testing.T) {
	s := NewInMemoryJobStore()
	now := time.Now().UTC()
	job := Job{
		ID: "pj2", Domain: "example.com", Status: JobSucceeded,
		CreatedAt: now, FinishedAt: now, Priority: PriorityBatch,
	}
	createAndGraduate(t, s, job, nil)
	run, ok := s.GetRun(job.ID)
	if !ok {
		t.Fatal("GetRun: not found")
	}
	if run.Priority != PriorityBatch {
		t.Fatalf("run Priority: got %d, want %d", run.Priority, PriorityBatch)
	}
	// Also verify jobFromRun round-trip.
	reconstructed, ok := s.Get(job.ID)
	if !ok {
		t.Fatal("Get after graduation: not found")
	}
	if reconstructed.Priority != PriorityBatch {
		t.Fatalf("reconstructed Priority: got %d, want %d", reconstructed.Priority, PriorityBatch)
	}
}

func TestInMemoryJobStoreProfileCRUD(t *testing.T) {
	store := NewInMemoryJobStore()

	// Create
	p, err := store.CreateProfile(StoredProfile{
		Name:        "strict-dnssec",
		Description: "Strict DNSSEC validation",
		Config:      `{"resolver.defaults.timeout": 10}`,
		Public:      true,
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be set")
	}

	// Get by ID
	got, ok := store.GetProfile(p.ID)
	if !ok {
		t.Fatal("GetProfile: not found")
	}
	if got.Name != "strict-dnssec" || got.Description != "Strict DNSSEC validation" || !got.Public {
		t.Fatalf("unexpected profile: %+v", got)
	}
	if got.Config != `{"resolver.defaults.timeout": 10}` {
		t.Fatalf("unexpected config: %s", got.Config)
	}

	// Get by name
	got, ok = store.GetProfileByName("strict-dnssec")
	if !ok {
		t.Fatal("GetProfileByName: not found")
	}
	if got.ID != p.ID {
		t.Fatalf("expected ID %d, got %d", p.ID, got.ID)
	}

	// Get missing
	_, ok = store.GetProfile(999)
	if ok {
		t.Fatal("expected false for missing profile")
	}
	_, ok = store.GetProfileByName("missing")
	if ok {
		t.Fatal("expected false for missing profile name")
	}

	// Update
	got.Description = "Updated description"
	got.Public = false
	if err := store.UpdateProfile(got); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	updated, _ := store.GetProfile(p.ID)
	if updated.Description != "Updated description" || updated.Public {
		t.Fatalf("update not reflected: %+v", updated)
	}

	// Duplicate name rejected on create
	_, err = store.CreateProfile(StoredProfile{Name: "strict-dnssec", Config: "{}"})
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}

	// Duplicate name rejected on update (rename collision)
	p2, _ := store.CreateProfile(StoredProfile{Name: "quick", Config: "{}"})
	p2.Name = "strict-dnssec"
	if err := store.UpdateProfile(p2); err == nil {
		t.Fatal("expected error for duplicate name on update")
	}

	// List returns sorted by name
	profiles := store.ListProfiles()
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0].Name != "quick" || profiles[1].Name != "strict-dnssec" {
		t.Fatalf("expected sorted order: quick, strict-dnssec; got %s, %s", profiles[0].Name, profiles[1].Name)
	}

	// Delete
	if err := store.DeleteProfile(p.ID); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	_, ok = store.GetProfile(p.ID)
	if ok {
		t.Fatal("expected profile to be deleted")
	}

	// Delete missing returns error
	if err := store.DeleteProfile(999); err == nil {
		t.Fatal("expected error for deleting missing profile")
	}

	// Update missing returns error
	if err := store.UpdateProfile(StoredProfile{ID: 999, Name: "x", Config: "{}"}); err == nil {
		t.Fatal("expected error for updating missing profile")
	}
}

func TestInMemoryJobStoreProfileReferencesPersist(t *testing.T) {
	store := NewInMemoryJobStore()

	profile, err := store.CreateProfile(StoredProfile{
		Name:   "strict",
		Config: `{"resolver.defaults.timeout":10}`,
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if err := store.CreateTag("ops", "operations"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := store.SetTagDefaultProfile("ops", &profile.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile: %v", err)
	}

	now := time.Now().UTC()
	queued := Job{
		ID:               "job-profile-queued",
		Domain:           "queued.example",
		Status:           JobQueued,
		CreatedAt:        now,
		ProfileID:        &profile.ID,
		ProfileName:      profile.Name,
		EffectiveProfile: `{"resolver.defaults.timeout":10}`,
	}
	if _, err := store.Create(queued); err != nil {
		t.Fatalf("Create queued: %v", err)
	}
	gotQueued, ok := store.Get(queued.ID)
	if !ok {
		t.Fatal("Get queued: not found")
	}
	if gotQueued.ProfileID == nil || *gotQueued.ProfileID != profile.ID {
		t.Fatalf("queued ProfileID: got %v, want %d", gotQueued.ProfileID, profile.ID)
	}
	if gotQueued.ProfileName != profile.Name {
		t.Fatalf("queued ProfileName: got %q, want %q", gotQueued.ProfileName, profile.Name)
	}

	runJob := Job{
		ID:               "job-profile-run",
		Domain:           "run.example",
		Status:           JobSucceeded,
		CreatedAt:        now,
		StartedAt:        now,
		FinishedAt:       now.Add(2 * time.Second),
		ProfileID:        &profile.ID,
		ProfileName:      profile.Name,
		EffectiveProfile: `{"resolver.defaults.timeout":15}`,
	}
	createAndGraduate(t, store, runJob, nil)
	run, ok := store.GetRun(runJob.ID)
	if !ok {
		t.Fatal("GetRun: not found")
	}
	if run.ProfileID == nil || *run.ProfileID != profile.ID {
		t.Fatalf("run ProfileID: got %v, want %d", run.ProfileID, profile.ID)
	}
	if run.ProfileName != profile.Name {
		t.Fatalf("run ProfileName: got %q, want %q", run.ProfileName, profile.Name)
	}
	if run.EffectiveProfile != `{"resolver.defaults.timeout":15}` {
		t.Fatalf("run EffectiveProfile: got %q", run.EffectiveProfile)
	}

	if err := store.DeleteProfile(profile.ID); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}

	tag, ok := store.GetTag("ops")
	if !ok {
		t.Fatal("GetTag after delete: not found")
	}
	if tag.DefaultProfileID != nil {
		t.Fatalf("expected cleared tag DefaultProfileID, got %v", *tag.DefaultProfileID)
	}

	gotQueued, ok = store.Get(queued.ID)
	if !ok {
		t.Fatal("Get queued after delete: not found")
	}
	if gotQueued.ProfileID != nil {
		t.Fatalf("expected queued ProfileID cleared, got %v", *gotQueued.ProfileID)
	}
	if gotQueued.ProfileName != profile.Name {
		t.Fatalf("queued ProfileName after delete: got %q, want %q", gotQueued.ProfileName, profile.Name)
	}

	run, ok = store.GetRun(runJob.ID)
	if !ok {
		t.Fatal("GetRun after delete: not found")
	}
	if run.ProfileID != nil {
		t.Fatalf("expected run ProfileID cleared, got %v", *run.ProfileID)
	}
	if run.ProfileName != profile.Name {
		t.Fatalf("run ProfileName after delete: got %q, want %q", run.ProfileName, profile.Name)
	}
	if run.EffectiveProfile != `{"resolver.defaults.timeout":15}` {
		t.Fatalf("run EffectiveProfile after delete: got %q", run.EffectiveProfile)
	}
}

func TestInMemoryJobStoreSettingsCRUD(t *testing.T) {
	store := NewInMemoryJobStore()

	// Get missing
	_, ok := store.GetSetting("worker_count")
	if ok {
		t.Fatal("expected ok=false for missing setting")
	}

	// Set and get
	if err := store.SetSetting("worker_count", "8"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	v, ok := store.GetSetting("worker_count")
	if !ok {
		t.Fatal("expected setting to exist")
	}
	if v != "8" {
		t.Fatalf("got %q, want %q", v, "8")
	}

	// Overwrite
	if err := store.SetSetting("worker_count", "12"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	v, _ = store.GetSetting("worker_count")
	if v != "12" {
		t.Fatalf("got %q after overwrite, want %q", v, "12")
	}

	// Set another
	if err := store.SetSetting("min_level", "WARNING"); err != nil {
		t.Fatalf("SetSetting min_level: %v", err)
	}

	// List
	all := store.ListSettings()
	if len(all) != 2 {
		t.Fatalf("ListSettings: got %d, want 2", len(all))
	}
	if all["worker_count"] != "12" || all["min_level"] != "WARNING" {
		t.Fatalf("ListSettings: unexpected values: %v", all)
	}

	// Delete
	if err := store.DeleteSetting("worker_count"); err != nil {
		t.Fatalf("DeleteSetting: %v", err)
	}
	_, ok = store.GetSetting("worker_count")
	if ok {
		t.Fatal("expected setting deleted")
	}

	// Delete missing
	if err := store.DeleteSetting("nonexistent"); err == nil {
		t.Fatal("expected error on deleting missing setting")
	}
}
