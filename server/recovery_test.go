package server

import (
	"fmt"
	"testing"
	"time"
)

func TestRecoverJobsContinuesPastUngraduatableOrphan(t *testing.T) {
	// Restarting the server is the operator's fix for a stuck job, so one
	// row that will not graduate must not take startup down with it. The
	// remaining orphans still have to be recovered and queued jobs still
	// have to be re-enqueued.
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			store := testStoreForBackend(t, b)
			base := time.Now().UTC()

			for _, job := range []Job{
				{ID: "orphan-bad", Domain: "bad.test", Status: JobRunning, CreatedAt: base},
				{ID: "orphan-good", Domain: "good.test", Status: JobRunning, CreatedAt: base.Add(time.Second)},
				{ID: "still-queued", Domain: "queued.test", Status: JobQueued, CreatedAt: base.Add(2 * time.Second)},
			} {
				if _, err := store.Create(job); err != nil {
					t.Fatalf("Create %q: %v", job.ID, err)
				}
			}

			// Break graduation for one row by removing the runs table's
			// counterpart target: a duplicate run id makes the insert fail
			// permanently, which is not retryable.
			if err := blockGraduation(store, "orphan-bad", "bad.test"); err != nil {
				t.Fatalf("blockGraduation: %v", err)
			}

			queue := NewInMemoryQueue()
			if err := RecoverJobs(store, queue); err != nil {
				t.Fatalf("RecoverJobs must not fail on one bad orphan: %v", err)
			}

			// The healthy orphan graduated normally.
			good, ok := store.Get("orphan-good")
			if !ok {
				t.Fatal("orphan-good not found after recovery")
			}
			if good.Status != JobFailed {
				t.Fatalf("orphan-good status = %q, want failed", good.Status)
			}

			// The bad one is at least parked at a terminal status so it no
			// longer counts as outstanding work.
			bad, ok := store.Get("orphan-bad")
			if !ok {
				t.Fatal("orphan-bad should still be readable")
			}
			if bad.Status == JobRunning {
				t.Fatal("orphan-bad must not be left at running")
			}

			// The queued job still made it back onto the queue.
			id, err := queue.Dequeue(t.Context())
			if err != nil {
				t.Fatalf("Dequeue: %v", err)
			}
			if id != "still-queued" {
				t.Fatalf("dequeued %q, want still-queued", id)
			}
		})
	}
}

// blockGraduation makes GraduateJob fail permanently for one job by
// pre-inserting a run row whose id collides with the one it would insert.
func blockGraduation(store *SQLJobStore, jobID, domain string) error {
	d, err := store.GetOrCreateDomain(domain)
	if err != nil {
		return fmt.Errorf("create domain: %w", err)
	}
	if _, err := store.db.Exec(
		fmt.Sprintf(`INSERT INTO runs (id, domain_id, domain, status, created_at) VALUES (%s)`, store.phRange(1, 5)),
		jobID, d.ID, domain, string(JobFailed), store.ts(time.Now().UTC()),
	); err != nil {
		return fmt.Errorf("seed colliding run: %w", err)
	}
	return nil
}
