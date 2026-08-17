package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// runJobWithFailingGraduation drives one job through runJob with a store
// whose GraduateJob always fails, which is what a MariaDB contention abort
// looks like from the worker's side.
func runJobWithFailingGraduation(t *testing.T, cause error) (*Server, *spyJobStore, Job) {
	t.Helper()
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	spy.graduateErr = cause
	srv.store = spy
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	job := Job{
		ID:        "job-wedge",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := srv.runJob(job.ID); err == nil {
		t.Fatal("runJob should surface the graduation failure")
	}
	return srv, spy, job
}

func TestRunJobFailedGraduationLeavesTerminalStatus(t *testing.T) {
	// The wedge: before this, a graduation that would not commit left the
	// job at "running" forever, so callers polled until their own timeout
	// and batch snapshot capture never completed.
	cause := errors.New("Error 1020: Record has changed since last read in table 'domains'")
	srv, _, job := runJobWithFailingGraduation(t, cause)

	stored, ok := srv.store.Get(job.ID)
	if !ok {
		t.Fatal("job row should still exist after a failed graduation")
	}
	if stored.Status != JobFailed {
		t.Fatalf("status = %q, want %q", stored.Status, JobFailed)
	}
	if !strings.Contains(stored.Error, "graduation failed") {
		t.Fatalf("error = %q, want it to name the graduation failure", stored.Error)
	}
	if !strings.Contains(stored.Error, "1020") {
		t.Fatalf("error = %q, want it to carry the underlying cause", stored.Error)
	}
	if stored.FinishedAt.IsZero() {
		t.Fatal("a terminal job needs a finished timestamp")
	}
}

func TestRunJobFailedGraduationDrainsInFlightGauge(t *testing.T) {
	// The queued-to-running transition raises the in-flight gauge. Without
	// a matching terminal transition it climbs forever, which is what made
	// the reporter's dashboard show workers that were never actually busy.
	srv, _, _ := runJobWithFailingGraduation(t, errors.New("commit refused"))

	snapshot := srv.metrics.Snapshot()
	if snapshot.Health.InFlightJobs != 0 {
		t.Fatalf("InFlightJobs = %d, want 0 after the job reached a terminal status", snapshot.Health.InFlightJobs)
	}
	if got := snapshot.Jobs.StatusCounts[string(JobRunning)]; got != 0 {
		t.Fatalf("running status count = %d, want 0", got)
	}
}

func TestRunJobReEnqueuesWhenStartUpdateFails(t *testing.T) {
	// The job has already been dequeued when the queued-to-running write
	// happens. If that write fails the job is still "queued" in the store,
	// so it has to go back on the queue or nothing will ever pick it up.
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy

	job := Job{
		ID:        "job-start-fails",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	// Fail only the transition into "running".
	spy.updateHook = func(update Job) error {
		if update.Status == JobRunning {
			return errors.New("store unavailable")
		}
		return nil
	}

	if err := srv.runJob(job.ID); err == nil {
		t.Fatal("runJob should surface the failed start write")
	}

	queued, err := srv.queue.(*InMemoryQueue).Dequeue(t.Context())
	if err != nil {
		t.Fatalf("the job should be back on the queue: %v", err)
	}
	if queued != job.ID {
		t.Fatalf("dequeued %q, want %q", queued, job.ID)
	}
}
