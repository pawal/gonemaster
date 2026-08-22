package server

import (
	"errors"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func queuedJob(id string) Job {
	return Job{ID: id, Domain: "example.com", Status: JobQueued, CreatedAt: time.Now().UTC()}
}

func TestFakeJobStoreDelegatesByDefault(t *testing.T) {
	fake := newFakeJobStore()

	created, err := fake.Create(queuedJob("job-1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// A method the fake does not override must still reach the real store.
	got, ok := fake.Get(created.ID)
	if !ok || got.Domain != "example.com" {
		t.Fatalf("Get after Create returned %+v, ok=%v", got, ok)
	}
}

func TestFakeJobStoreCreateErr(t *testing.T) {
	fake := newFakeJobStore()
	wantErr := errors.New("duplicate key")
	fake.createErr = wantErr

	if _, err := fake.Create(queuedJob("job-1")); !errors.Is(err, wantErr) {
		t.Fatalf("Create err = %v, want %v", err, wantErr)
	}
	if _, ok := fake.Get("job-1"); ok {
		t.Fatal("expected a failed Create not to reach the store")
	}
}

func TestFakeJobStoreGraduateErr(t *testing.T) {
	fake := newFakeJobStore()
	job, err := fake.Create(queuedJob("job-1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	wantErr := errors.New("commit refused")
	fake.graduateErr = wantErr

	if err := fake.GraduateJob(job, nil); !errors.Is(err, wantErr) {
		t.Fatalf("GraduateJob err = %v, want %v", err, wantErr)
	}
	if _, ok := fake.GetRun(job.ID); ok {
		t.Fatal("expected a failed graduation not to produce a run")
	}
}

func TestFakeJobStoreUpdateHookRunsBeforeTheStore(t *testing.T) {
	fake := newProgressSpy()
	job, err := fake.Create(queuedJob("job-1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	wantErr := errors.New("store unavailable")
	fake.updateHook = func(update Job) error {
		if update.Status == JobRunning {
			return wantErr
		}
		return nil
	}

	job.Status = JobRunning
	job.Progress = 40
	if err := fake.Update(job); !errors.Is(err, wantErr) {
		t.Fatalf("Update err = %v, want %v", err, wantErr)
	}
	// The write is recorded even when the hook fails it, so a test can see
	// what the server attempted.
	if got := fake.Progresses(); len(got) != 1 || got[0] != 40 {
		t.Fatalf("Progresses = %v, want [40]", got)
	}
	stored, _ := fake.Get(job.ID)
	if stored.Status == JobRunning {
		t.Fatal("expected the failed Update not to reach the store")
	}
}

func TestFakeJobStoreProgressRecordingIsOptIn(t *testing.T) {
	plain := newFakeJobStore()
	job, err := plain.Create(queuedJob("job-1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	job.Progress = 10
	if err := plain.Update(job); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Recording is off by default so the contention benchmark's Update stays
	// lock-free, but the count is always kept.
	if got := plain.Progresses(); len(got) != 0 {
		t.Fatalf("Progresses = %v, want none without newProgressSpy", got)
	}
	if got := plain.UpdateCount(); got != 1 {
		t.Fatalf("UpdateCount = %d, want 1", got)
	}

	spy := newProgressSpy()
	if _, err := spy.Create(queuedJob("job-2")); err != nil {
		t.Fatalf("create: %v", err)
	}
	job2, _ := spy.Get("job-2")
	job2.Progress = 70
	if err := spy.Update(job2); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := spy.Progresses(); len(got) != 1 || got[0] != 70 {
		t.Fatalf("Progresses = %v, want [70]", got)
	}
}

func TestFakeJobStoreChainErr(t *testing.T) {
	fake := newFakeJobStore()
	wantErr := errors.New("db down")
	fake.chainErr = wantErr

	if _, _, err := fake.GetRunDNSSECChain("run-1"); !errors.Is(err, wantErr) {
		t.Fatalf("GetRunDNSSECChain err = %v, want %v", err, wantErr)
	}
}

func TestWrapStoreKeepsWhatTheServerSeeded(t *testing.T) {
	srv := newTestServer(t)
	job, err := srv.store.Create(queuedJob("job-1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := srv.store.GraduateJob(job, []engine.LogEntry{
		{Timestamp: 1, Module: "System", Tag: "MODULE_START", Level: "INFO"},
	}); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	fake := wrapStore(t, srv)

	if srv.store != JobStore(fake) {
		t.Fatal("expected the server to use the fake")
	}
	// Wrapping must not lose data: the tests that inject a failure seed first.
	if _, ok := srv.store.GetRun(job.ID); !ok {
		t.Fatal("expected the seeded run to survive wrapping")
	}
}

func TestSeedGraduatedRunDefaults(t *testing.T) {
	store := NewInMemoryJobStore()
	before := time.Now().UTC()

	job := seedGraduatedRun(t, store, runSpec{})

	if job.ID == "" || job.Domain != "example.com" {
		t.Fatalf("expected a generated id for example.com, got %+v", job)
	}
	if job.Status != JobSucceeded {
		t.Fatalf("Status = %q, want %q", job.Status, JobSucceeded)
	}
	if job.FinishedAt.Before(before) {
		t.Fatalf("FinishedAt = %v, want now or later", job.FinishedAt)
	}
	// Create assigns the public id, so the caller can look the run up by it.
	if job.PublicID == "" {
		t.Fatal("expected Create to have assigned a public id")
	}
	run, ok := store.GetRun(job.ID)
	if !ok {
		t.Fatal("expected the run to be graduated")
	}
	if run.Domain != "example.com" {
		t.Fatalf("run domain = %q", run.Domain)
	}
}

func TestSeedGraduatedRunSpecFields(t *testing.T) {
	store := NewInMemoryJobStore()
	at := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	job := seedGraduatedRun(t, store, runSpec{
		ID:            "job-1",
		Domain:        "a.example",
		BatchID:       "batch-1",
		At:            at,
		Duration:      time.Minute,
		Entries:       systemStartEntry(),
		Timings:       []NameserverTiming{{Nameserver: "ns1.a.example"}},
		Progress:      100,
		Origin:        JobOriginPublic,
		ResolveDomain: true,
	})

	if job.ID != "job-1" || job.BatchID != "batch-1" {
		t.Fatalf("id/batch = %q/%q", job.ID, job.BatchID)
	}
	if !job.FinishedAt.Equal(at) {
		t.Fatalf("FinishedAt = %v, want %v", job.FinishedAt, at)
	}
	if want := at.Add(-time.Minute); !job.StartedAt.Equal(want) {
		t.Fatalf("StartedAt = %v, want %v", job.StartedAt, want)
	}
	// ResolveDomain must set DomainID before graduation, which the batch and
	// analysis handlers rely on.
	if job.DomainID == 0 {
		t.Fatal("expected ResolveDomain to set DomainID")
	}
	if job.Origin != JobOriginPublic || job.Progress != 100 {
		t.Fatalf("origin/progress = %q/%d", job.Origin, job.Progress)
	}
	run, ok := store.GetRun("job-1")
	if !ok {
		t.Fatal("expected the run graduated")
	}
	if len(run.NameserverTimings) != 1 {
		t.Fatalf("expected the timings carried through, got %+v", run.NameserverTimings)
	}
}

func TestSeedGraduatedRunStoresTheChain(t *testing.T) {
	store := NewInMemoryJobStore()
	const chain = `{"links":[]}`

	job := seedGraduatedRun(t, store, runSpec{ChainJSON: chain})

	// The chain can only be set after Create, which is what assigns the
	// public id it is looked up by.
	got, ok, err := store.GetRunDNSSECChain(job.ID)
	if err != nil || !ok {
		t.Fatalf("GetRunDNSSECChain: ok=%v err=%v", ok, err)
	}
	if got != chain {
		t.Fatalf("chain = %q, want %q", got, chain)
	}
}

func TestGraduateFillsInStatusAndFinishTime(t *testing.T) {
	store := NewInMemoryJobStore()
	created := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	job, err := store.Create(Job{ID: "job-1", Domain: "a.example", Status: JobQueued, CreatedAt: created})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	graduate(t, store, job, nil)

	run, ok := store.GetRun("job-1")
	if !ok {
		t.Fatal("expected the run graduated")
	}
	// A queued job graduates as succeeded, and the finish time follows the
	// creation time rather than wall clock.
	if run.Status != JobSucceeded {
		t.Fatalf("Status = %q, want %q", run.Status, JobSucceeded)
	}
	if want := created.Add(time.Second); !run.FinishedAt.Equal(want) {
		t.Fatalf("FinishedAt = %v, want %v", run.FinishedAt, want)
	}
}

func TestCreateAndGraduateKeepsCallerFields(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	got := createAndGraduate(t, store, Job{
		ID:         "job-1",
		Domain:     "a.example",
		Status:     JobSucceeded,
		CreatedAt:  now,
		FinishedAt: now,
		Priority:   PriorityBatch,
	}, nil)

	// Fields runSpec does not model must survive, which is the whole reason
	// this rung exists.
	if got.Priority != PriorityBatch {
		t.Fatalf("Priority = %v, want %v", got.Priority, PriorityBatch)
	}
	run, ok := store.GetRun("job-1")
	if !ok {
		t.Fatal("expected the run graduated")
	}
	if run.Domain != "a.example" {
		t.Fatalf("run domain = %q", run.Domain)
	}
}
