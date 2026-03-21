package server

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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

	result := JobResult{JobID: job.ID, Status: JobSucceeded}
	if err := store.SetResult(job.ID, result); err != nil {
		t.Fatalf("set result: %v", err)
	}
	stored, ok := store.GetResult(job.ID)
	if !ok {
		t.Fatalf("expected result in store")
	}
	if stored.Status != JobSucceeded {
		t.Fatalf("expected status %s, got %s", JobSucceeded, stored.Status)
	}

	list = store.List(JobFilter{Limit: 10})
	if len(list.Items) != 1 {
		t.Fatalf("expected one listed item")
	}
	for _, level := range []string{"NOTICE", "WARNING", "ERROR", "CRITICAL"} {
		if _, ok := list.Items[0].SeverityTotals[level]; !ok {
			t.Fatalf("expected severity_totals to include %s", level)
		}
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

	_ = store.SetResult("job1", JobResult{
		JobID:  "job1",
		Status: JobSucceeded,
		Summary: map[string]any{
			"levels": map[string]int{
				"ERROR": 1,
			},
		},
	})
	_ = store.SetResult("job2", JobResult{
		JobID:  "job2",
		Status: JobFailed,
		Summary: map[string]any{
			"levels": map[string]int{
				"CRITICAL": 2,
			},
		},
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

	errorDesc := store.List(JobFilter{Limit: 10, Sort: JobSortErrorDesc})
	if len(errorDesc.Items) != 3 || errorDesc.Items[0].ID != "job2" || errorDesc.Items[1].ID != "job1" {
		t.Fatalf("expected error_desc sorting to prioritize CRITICAL+ERROR totals")
	}

	criticalDesc := store.List(JobFilter{Limit: 10, Sort: JobSortCriticalDesc})
	if len(criticalDesc.Items) != 3 || criticalDesc.Items[0].ID != "job2" {
		t.Fatalf("expected critical_desc sorting to prioritize CRITICAL totals")
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

func TestInMemoryJobStoreSeverityTotalsFromSummary(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC().Add(-time.Minute)
	job := Job{
		ID:        "job-sev",
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: base,
	}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetResult(job.ID, JobResult{
		JobID:  job.ID,
		Status: JobSucceeded,
		Summary: map[string]any{
			"levels": map[string]int{
				"NOTICE": 1,
				"ERROR":  2,
			},
		},
	}); err != nil {
		t.Fatalf("set result: %v", err)
	}

	list := store.List(JobFilter{Limit: 10})
	if len(list.Items) != 1 {
		t.Fatalf("expected one listed item")
	}
	totals := list.Items[0].SeverityTotals
	if totals["NOTICE"] != 1 || totals["ERROR"] != 2 || totals["WARNING"] != 0 || totals["CRITICAL"] != 0 {
		t.Fatalf("unexpected severity_totals: %+v", totals)
	}
}

func TestInMemoryJobStoreSetResultMissingJob(t *testing.T) {
	store := NewInMemoryJobStore()
	err := store.SetResult("missing", JobResult{JobID: "missing"})
	if err == nil {
		t.Fatalf("expected error when setting result for missing job")
	}
}

func TestInMemoryJobStoreConcurrentAccess(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC().Add(-time.Minute)
	const totalJobs = 120
	for i := 0; i < totalJobs; i++ {
		id := fmt.Sprintf("job-%03d", i)
		_, err := store.Create(Job{
			ID:        id,
			BatchID:   "batch-a",
			Domain:    fmt.Sprintf("example-%03d.test", i),
			Status:    JobQueued,
			CreatedAt: base.Add(time.Duration(i) * time.Millisecond),
		})
		if err != nil {
			t.Fatalf("seed create %s: %v", id, err)
		}
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	var setResultErrors atomic.Int32

	// Writers update job metadata.
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			idx := worker
			for {
				select {
				case <-done:
					return
				default:
				}
				id := fmt.Sprintf("job-%03d", idx%totalJobs)
				job, ok := store.Get(id)
				if ok {
					job.Status = JobRunning
					job.StartedAt = time.Now().UTC()
					if err := store.Update(job); err != nil {
						t.Errorf("update %s: %v", id, err)
						return
					}
				}
				idx += 7
			}
		}(worker)
	}

	// Result writers continuously set synthetic summaries.
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			idx := worker
			for {
				select {
				case <-done:
					return
				default:
				}
				id := fmt.Sprintf("job-%03d", idx%totalJobs)
				err := store.SetResult(id, JobResult{
					JobID:  id,
					Status: JobSucceeded,
					Summary: map[string]any{
						"levels": map[string]int{
							"WARNING":  idx % 3,
							"ERROR":    idx % 2,
							"CRITICAL": (idx / 2) % 2,
						},
					},
				})
				if err != nil {
					setResultErrors.Add(1)
					t.Errorf("set result %s: %v", id, err)
					return
				}
				idx += 5
			}
		}(worker)
	}

	// Readers stress list paths that require severity totals and sorting.
	for worker := 0; worker < 6; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			sorts := []JobSort{JobSortStartedAtDesc, JobSortErrorDesc, JobSortCriticalDesc, JobSortDomainAsc}
			for iteration := 0; ; iteration++ {
				select {
				case <-done:
					return
				default:
				}
				list := store.List(JobFilter{
					Limit:    40,
					Sort:     sorts[(worker+iteration)%len(sorts)],
					Severity: JobSeverityWarningsPlus,
				})
				if list.Total < len(list.Items) {
					t.Errorf("invalid list total/items: total=%d items=%d", list.Total, len(list.Items))
					return
				}
				for _, job := range list.Items {
					totals := job.SeverityTotals
					for _, level := range []string{"NOTICE", "WARNING", "ERROR", "CRITICAL"} {
						if _, ok := totals[level]; !ok {
							t.Errorf("missing severity level %q for job %s", level, job.ID)
							return
						}
					}
				}
			}
		}(worker)
	}

	time.Sleep(500 * time.Millisecond)
	close(done)
	wg.Wait()

	if got := setResultErrors.Load(); got != 0 {
		t.Fatalf("set result errors = %d, want 0", got)
	}

	final := store.List(JobFilter{Limit: totalJobs, Sort: JobSortCreatedAtAsc})
	if final.Total != totalJobs {
		t.Fatalf("final total = %d, want %d", final.Total, totalJobs)
	}
}

func TestInMemoryJobStorePurgeOlderThanDeletesTerminalJobs(t *testing.T) {
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
	}

	n, err := store.PurgeOlderThan(cutoff)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 purged, got %d", n)
	}
	if list := store.List(JobFilter{Limit: 100}); list.Total != 0 {
		t.Fatalf("expected 0 jobs after purge, got %d", list.Total)
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

func TestInMemoryJobStorePurgeOlderThanPreservesNewJobs(t *testing.T) {
	store := NewInMemoryJobStore()
	cutoff := time.Now().UTC()
	recent := cutoff.Add(time.Hour) // finished after cutoff

	job := Job{ID: "s1", Domain: "example.com", Status: JobSucceeded, CreatedAt: recent, FinishedAt: recent}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}

	n, err := store.PurgeOlderThan(cutoff)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 purged, got %d", n)
	}
}

func TestInMemoryJobStorePurgeOlderThanDeletesResults(t *testing.T) {
	store := NewInMemoryJobStore()
	cutoff := time.Now().UTC()
	old := cutoff.Add(-24 * time.Hour)

	job := Job{ID: "s1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetResult("s1", JobResult{Summary: map[string]any{}}); err != nil {
		t.Fatalf("set result: %v", err)
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
	created, _ := store.Create(Job{
		ID:         "j1",
		Domain:     "example.com",
		Status:     JobSucceeded,
		FinishedAt: time.Now().UTC().Add(-48 * time.Hour),
	})
	pubID := created.PublicID
	store.PurgeOlderThan(time.Now().UTC())
	_, ok := store.GetByPublicID(pubID)
	if ok {
		t.Fatal("expected public ID index to be cleaned up after purge")
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
