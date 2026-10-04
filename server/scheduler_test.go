package server

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/server/recurrence"
)

// scheduleFixture is a server with an analysis-enabled cohort over tag tld.
type scheduleFixture struct {
	srv    *Server
	cohort AnalysisCohort
}

func newScheduleFixture(t *testing.T, opts ...srvOpt) scheduleFixture {
	t.Helper()
	srv := newTestServer(t, opts...)
	if err := srv.store.CreateTag("tld", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	seedTaggedDomains(t, srv, "tld", "se", "nu")
	return scheduleFixture{srv: srv, cohort: upsertCohort(t, srv.store, "tld")}
}

func (f scheduleFixture) schedule(t *testing.T) AnalysisCohortSchedule {
	t.Helper()
	sched, ok := f.srv.store.GetAnalysisCohortSchedule(f.cohort.ID)
	if !ok {
		t.Fatal("schedule missing")
	}
	return sched
}

func (f scheduleFixture) batches() []Batch {
	return f.srv.store.ListBatches("", 100, 0).Items
}

func TestRunDueSchedulesOutcomes(t *testing.T) {
	now := time.Date(2026, 10, 1, 2, 0, 30, 0, time.UTC)
	missing := int64(999)
	for _, tc := range []struct {
		name    string
		edit    func(sched *AnalysisCohortSchedule)
		setup   func(t *testing.T, f scheduleFixture)
		want    string
		wantErr string
		batches int
	}{
		{name: "submitted", want: ScheduleOutcomeSubmitted, batches: 1},
		{
			name: "missed without catch-up",
			edit: func(sched *AnalysisCohortSchedule) {
				sched.CatchUp = false
				sched.NextRunAt = now.Add(-2 * time.Hour)
			},
			want: ScheduleOutcomeSkippedMissed,
		},
		{
			name: "late within grace without catch-up",
			edit: func(sched *AnalysisCohortSchedule) {
				sched.CatchUp = false
				sched.NextRunAt = now.Add(-30 * time.Minute)
			},
			want: ScheduleOutcomeSubmitted, batches: 1,
		},
		{
			name: "cohort analysis disabled",
			setup: func(t *testing.T, f scheduleFixture) {
				f.cohort.AnalysisEnabled = false
				if _, err := f.srv.store.UpsertAnalysisCohort(f.cohort); err != nil {
					t.Fatalf("UpsertAnalysisCohort: %v", err)
				}
			},
			want: ScheduleOutcomeSkippedDisabled,
		},
		{
			name: "previous scheduled batch active",
			setup: func(t *testing.T, f scheduleFixture) {
				if _, err := f.srv.store.Create(Job{ID: "job-prev", BatchID: "batch-prev", Domain: "se", Status: JobRunning, CreatedAt: now}); err != nil {
					t.Fatalf("Create: %v", err)
				}
				if err := f.srv.store.RecordAnalysisCohortScheduleOutcome(f.cohort.ID, now.Add(-24*time.Hour), "batch-prev", ScheduleOutcomeSubmitted, ""); err != nil {
					t.Fatalf("RecordAnalysisCohortScheduleOutcome: %v", err)
				}
			},
			want: ScheduleOutcomeSkippedActive,
		},
		{
			name: "empty source tag",
			setup: func(t *testing.T, f scheduleFixture) {
				for _, name := range []string{"se", "nu"} {
					d, _ := f.srv.store.GetDomainByName(name)
					if err := f.srv.store.UntagDomains("tld", []int64{d.ID}); err != nil {
						t.Fatalf("UntagDomains: %v", err)
					}
				}
			},
			want: ScheduleOutcomeSkippedEmpty,
		},
		{
			name:    "unknown profile",
			edit:    func(sched *AnalysisCohortSchedule) { sched.ProfileID = &missing },
			want:    ScheduleOutcomeError,
			wantErr: "profile not found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newScheduleFixture(t)
			sched := newSchedule(f.cohort.ID, now.Add(-30*time.Second))
			if tc.edit != nil {
				tc.edit(&sched)
			}
			putSchedule(t, f.srv.store, sched)
			if tc.setup != nil {
				tc.setup(t, f)
			}

			f.srv.runDueSchedules(t.Context(), now)

			got := f.schedule(t)
			if got.LastOutcome != tc.want || got.LastError != tc.wantErr {
				t.Errorf("outcome = %q %q, want %q %q", got.LastOutcome, got.LastError, tc.want, tc.wantErr)
			}
			if !got.LastRunAt.Equal(now) {
				t.Errorf("last_run_at = %s, want %s", got.LastRunAt, now)
			}
			if want := time.Date(2026, 10, 2, 2, 0, 0, 0, time.UTC); !got.NextRunAt.Equal(want) {
				t.Errorf("next_run_at = %s, want %s", got.NextRunAt, want)
			}
			if n := len(f.batches()); n != tc.batches {
				t.Errorf("batches = %d, want %d", n, tc.batches)
			}
			if n := f.srv.metrics.Snapshot().Jobs.ScheduledRuns[tc.want]; n != 1 {
				t.Errorf("scheduled_runs[%s] = %d, want 1", tc.want, n)
			}
		})
	}
}

func TestScheduledBatchMatchesSnapshotRun(t *testing.T) {
	f := newScheduleFixture(t)
	profile, err := f.srv.store.CreateProfile(StoredProfile{Name: "strict", Config: "{}"})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	now := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
	sched := newSchedule(f.cohort.ID, now)
	sched.ProfileID = &profile.ID
	sched.PromoteDefault = true
	putSchedule(t, f.srv.store, sched)

	f.srv.runDueSchedules(t.Context(), now)

	batches := f.batches()
	if len(batches) != 1 {
		t.Fatalf("batches = %d, want 1", len(batches))
	}
	b := batches[0]
	if b.Tag != "tld" || !b.SnapshotIntent || b.Origin != BatchOriginSchedule || b.DomainCount != 2 {
		t.Errorf("batch = tag %q intent %v origin %q domains %d, want tld true schedule 2", b.Tag, b.SnapshotIntent, b.Origin, b.DomainCount)
	}
	if got := f.schedule(t).LastBatchID; got != b.ID {
		t.Errorf("last_batch_id = %q, want %q", got, b.ID)
	}
	if v, ok := f.srv.store.GetSetting(PromoteDefaultSettingKey(b.ID)); !ok || v != "1" {
		t.Errorf("promote intent = %q %v, want 1", v, ok)
	}
	jobs := f.srv.store.List(JobFilter{BatchID: b.ID, Limit: 10}).Items
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	for _, job := range jobs {
		if job.ProfileID == nil || *job.ProfileID != profile.ID || job.ProfileName != "strict" {
			t.Errorf("job %s profile = %v %q, want %d strict", job.Domain, job.ProfileID, job.ProfileName, profile.ID)
		}
	}
}

func TestRunDueSchedulesFiresOnceAfterDowntime(t *testing.T) {
	f := newScheduleFixture(t)
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	putSchedule(t, f.srv.store, newSchedule(f.cohort.ID, time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)))

	f.srv.runDueSchedules(t.Context(), now)
	f.srv.runDueSchedules(t.Context(), now.Add(time.Minute))

	if n := len(f.batches()); n != 1 {
		t.Errorf("batches = %d, want 1", n)
	}
	if want := time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC); !f.schedule(t).NextRunAt.Equal(want) {
		t.Errorf("next_run_at = %s, want %s", f.schedule(t).NextRunAt, want)
	}
}

func TestRunDueSchedulesSkipsWhenSchedulerDisabled(t *testing.T) {
	f := newScheduleFixture(t, withConfig(func(c *Config) { c.SchedulerEnabled = false }))
	due := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
	putSchedule(t, f.srv.store, newSchedule(f.cohort.ID, due))

	f.srv.runDueSchedules(t.Context(), due.Add(time.Hour))

	got := f.schedule(t)
	if !got.NextRunAt.Equal(due) || got.LastOutcome != "" || !got.LastRunAt.IsZero() {
		t.Errorf("schedule touched: next %s outcome %q last %s", got.NextRunAt, got.LastOutcome, got.LastRunAt)
	}
	if n := len(f.batches()); n != 0 {
		t.Errorf("batches = %d, want 0", n)
	}
}

func TestRunDueSchedulesDisablesBrokenRule(t *testing.T) {
	f := newScheduleFixture(t)
	due := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
	sched := newSchedule(f.cohort.ID, due)
	sched.Rule = recurrence.Rule{Kind: recurrence.KindWeekly, TimeOfDay: recurrence.TimeOfDay{Hour: 2}}
	putSchedule(t, f.srv.store, sched)

	f.srv.runDueSchedules(t.Context(), due)

	got := f.schedule(t)
	if got.Enabled || got.LastOutcome != ScheduleOutcomeError || got.LastError != recurrence.ErrWeekdays.Error() {
		t.Errorf("schedule = enabled %v outcome %q error %q, want false error %q", got.Enabled, got.LastOutcome, got.LastError, recurrence.ErrWeekdays)
	}
}

func TestSchedulerConcurrentClaims(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		f := newScheduleFixture(t, withDB(s))
		now := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
		putSchedule(t, s, newSchedule(f.cohort.ID, now))

		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { f.srv.runDueSchedules(t.Context(), now) })
		}
		wg.Wait()

		if n := len(f.batches()); n != 1 {
			t.Errorf("batches = %d, want 1", n)
		}
	})
}

func TestScheduleLoopFiresAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newScheduleFixture(t)
		putSchedule(t, f.srv.store, newSchedule(f.cohort.ID, time.Now().Add(90*time.Second)))
		ctx, cancel := context.WithCancel(t.Context())

		f.srv.runScheduleLoop(ctx, time.Minute)
		synctest.Wait()
		if n := len(f.batches()); n != 0 {
			t.Fatalf("batches before due = %d, want 0", n)
		}

		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if n := len(f.batches()); n != 1 {
			t.Errorf("batches after due = %d, want 1", n)
		}
		cancel()
		synctest.Wait()
	})
}

func TestStartFiresDueScheduleWithAnalysis(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newScheduleFixture(t,
			withWorkers(1, 0),
			withAnalysisController(newSpyAnalysisController()),
			withEngineRunner(func(engine.RunRequest) ([]engine.LogEntry, error) { return nil, nil }),
		)
		putSchedule(t, f.srv.store, newSchedule(f.cohort.ID, time.Now().Add(-time.Minute)))

		f.srv.Start()
		synctest.Wait()
		if err := f.srv.Stop(t.Context()); err != nil {
			t.Fatalf("Stop: %v", err)
		}

		if got := f.schedule(t).LastOutcome; got != ScheduleOutcomeSubmitted {
			t.Errorf("last_outcome = %q, want submitted", got)
		}
	})
}
