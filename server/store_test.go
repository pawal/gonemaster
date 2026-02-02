package server

import (
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
}

func TestInMemoryJobStoreFilters(t *testing.T) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC().Add(-time.Minute)
	_ = seedJob(store, "job1", "batch1", base, JobQueued)
	_ = seedJob(store, "job2", "batch2", base.Add(time.Second), JobRunning)

	list := store.List(JobFilter{BatchID: "batch2", Limit: 10})
	if list.Total != 1 || list.Items[0].ID != "job2" {
		t.Fatalf("expected batch filter to return job2")
	}

	list = store.List(JobFilter{CreatedAfter: base.Add(500 * time.Millisecond), Limit: 10})
	if list.Total != 1 || list.Items[0].ID != "job2" {
		t.Fatalf("expected created_after filter to return job2")
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
