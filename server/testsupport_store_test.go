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
