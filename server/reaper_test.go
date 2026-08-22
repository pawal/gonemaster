package server

import (
	"context"
	"strings"
	"testing"
	"time"
)

// newReaperServer builds a server with the reaper timeout set to minutes,
// backed by the in-memory store the sweep will read and write.
func newReaperServer(t *testing.T, minutes int) *Server {
	t.Helper()
	// A store of its own, untouched by the scoring and analysis config New applies.
	return newTestServer(t,
		withConfig(func(cfg *Config) { cfg.StuckJobTimeoutMinutes = minutes }),
		withStore(NewInMemoryJobStore()))
}

// seedRunningJob stores a job already at "running", started age ago.
func seedRunningJob(t *testing.T, srv *Server, id string, age time.Duration) Job {
	t.Helper()
	now := time.Now().UTC()
	job := Job{
		ID:        id,
		Domain:    id + ".example",
		Status:    JobRunning,
		CreatedAt: now.Add(-age),
		StartedAt: now.Add(-age),
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	if err := srv.store.Update(job); err != nil {
		t.Fatalf("update %s: %v", id, err)
	}
	return job
}

func TestReapStuckJobsFailsAbandonedJob(t *testing.T) {
	// A job older than the timeout with no worker on it is abandoned:
	// nothing will ever move it off "running" on its own.
	srv := newReaperServer(t, 20)
	seedRunningJob(t, srv, "abandoned", 25*time.Minute)

	if got := srv.reapStuckJobs(time.Now().UTC()); got != 1 {
		t.Fatalf("reaped %d, want 1", got)
	}

	job, ok := srv.store.Get("abandoned")
	if !ok {
		t.Fatal("job should still be readable after reaping")
	}
	if job.Status != JobFailed {
		t.Fatalf("status = %q, want %q", job.Status, JobFailed)
	}
	if !strings.Contains(job.Error, "abandoned") {
		t.Fatalf("error = %q, want it to say the job was abandoned", job.Error)
	}
}

func TestReapStuckJobsSkipsLiveJob(t *testing.T) {
	// A job registered in the cancel map is being run by a worker right
	// now. Age alone must never reap it, or a slow run would be killed
	// while it is still making progress.
	srv := newReaperServer(t, 20)
	seedRunningJob(t, srv, "still-working", 90*time.Minute)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.registerCancel("still-working", cancel)

	if got := srv.reapStuckJobs(time.Now().UTC()); got != 0 {
		t.Fatalf("reaped %d, want 0 for a job a worker is running", got)
	}
	job, _ := srv.store.Get("still-working")
	if job.Status != JobRunning {
		t.Fatalf("status = %q, want it left at %q", job.Status, JobRunning)
	}
}

func TestReapStuckJobsSkipsYoungJob(t *testing.T) {
	// Below the timeout the job gets the benefit of the doubt, even with
	// no cancel entry: a worker may be between registering and starting.
	srv := newReaperServer(t, 20)
	seedRunningJob(t, srv, "recent", 5*time.Minute)

	if got := srv.reapStuckJobs(time.Now().UTC()); got != 0 {
		t.Fatalf("reaped %d, want 0 for a job younger than the timeout", got)
	}
	job, _ := srv.store.Get("recent")
	if job.Status != JobRunning {
		t.Fatalf("status = %q, want it left at %q", job.Status, JobRunning)
	}
}

func TestReapStuckJobsDisabledByZeroTimeout(t *testing.T) {
	// Zero is the operator's off switch, matching retention_days.
	srv := newReaperServer(t, 0)
	seedRunningJob(t, srv, "ancient", 48*time.Hour)

	if got := srv.reapStuckJobs(time.Now().UTC()); got != 0 {
		t.Fatalf("reaped %d, want 0 when the reaper is disabled", got)
	}
	job, _ := srv.store.Get("ancient")
	if job.Status != JobRunning {
		t.Fatalf("status = %q, want it untouched", job.Status)
	}
}

func TestReapStuckJobsDrainsInFlightGauge(t *testing.T) {
	// Reaping is the recovery path for jobs that wedged before the worker
	// learned to park them, so it has to settle the gauge they left high.
	srv := newReaperServer(t, 20)
	seedRunningJob(t, srv, "wedged", 30*time.Minute)
	srv.metrics.ObserveJobStatusTransition(JobQueued, JobRunning)

	if before := srv.metrics.Snapshot().Health.InFlightJobs; before != 1 {
		t.Fatalf("InFlightJobs = %d before reaping, want 1", before)
	}
	if got := srv.reapStuckJobs(time.Now().UTC()); got != 1 {
		t.Fatalf("reaped %d, want 1", got)
	}
	if after := srv.metrics.Snapshot().Health.InFlightJobs; after != 0 {
		t.Fatalf("InFlightJobs = %d after reaping, want 0", after)
	}
}

func TestReapStuckJobsHonoursSettingsChange(t *testing.T) {
	// The timeout is read per sweep, so a change through the settings API
	// takes effect on the next tick without a restart.
	srv := newReaperServer(t, 60)
	seedRunningJob(t, srv, "borderline", 30*time.Minute)

	if got := srv.reapStuckJobs(time.Now().UTC()); got != 0 {
		t.Fatalf("reaped %d at a 60 minute timeout, want 0", got)
	}

	srv.applySetting("stuck_job_timeout_minutes", "20")
	if got := srv.reapStuckJobs(time.Now().UTC()); got != 1 {
		t.Fatalf("reaped %d after lowering the timeout to 20, want 1", got)
	}
}

func TestGraduateJobKeepsErrorInMemory(t *testing.T) {
	// The SQL store carries job.Error onto the run row, and the in-memory
	// store must match it, or the reason a job failed is lost on the
	// backend that tests and the memory driver use.
	store := NewInMemoryJobStore()
	job := Job{ID: "with-error", Domain: "err.example", Status: JobQueued}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	job.Status = JobFailed
	job.Error = "graduation failed: commit refused"
	job.FinishedAt = time.Now().UTC()
	if err := store.GraduateJob(job, nil); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	run, ok := store.GetRun("with-error")
	if !ok {
		t.Fatal("run not found")
	}
	if run.Error != job.Error {
		t.Fatalf("run error = %q, want %q", run.Error, job.Error)
	}
}

func TestEffectiveStuckJobTimeout(t *testing.T) {
	cases := []struct {
		minutes int
		want    time.Duration
	}{
		{20, 20 * time.Minute},
		{1, time.Minute},
		{0, 0},
		{-5, 0},
	}
	for _, tc := range cases {
		cfg := Config{StuckJobTimeoutMinutes: tc.minutes}
		if got := cfg.EffectiveStuckJobTimeout(); got != tc.want {
			t.Fatalf("EffectiveStuckJobTimeout(%d) = %v, want %v", tc.minutes, got, tc.want)
		}
	}
}

func TestDefaultStuckJobTimeoutIsTwentyMinutes(t *testing.T) {
	if got := DefaultConfig().StuckJobTimeoutMinutes; got != 20 {
		t.Fatalf("default stuck job timeout = %d minutes, want 20", got)
	}
}
