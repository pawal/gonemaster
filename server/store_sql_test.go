package server

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// testSQLiteStore opens an in-memory SQLite store with migrations applied.
func testSQLiteStore(t *testing.T) *SQLJobStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if err := runMigrations(db); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	return NewSQLJobStore(db, sqliteDialect{})
}

// ids extracts job IDs from a slice for readable error messages.
func ids(jobs []Job) []string {
	out := make([]string, len(jobs))
	for i, j := range jobs {
		out[i] = j.ID
	}
	return out
}

// ---- Migration tests -------------------------------------------------------

func TestRunMigrationsFresh(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Idempotent: second run must not error.
	if err := runMigrations(db); err != nil {
		t.Fatalf("second run (idempotent): %v", err)
	}

	for _, tbl := range []string{"jobs", "results", "schema_migrations"} {
		var name string
		if err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name); err != nil || name != tbl {
			t.Errorf("table %q not found after migration", tbl)
		}
	}

	var version int
	if err := db.QueryRow(`SELECT version FROM schema_migrations WHERE version=1`).Scan(&version); err != nil {
		t.Fatalf("migration version 1 not recorded: %v", err)
	}
}

// ---- Create / Get ----------------------------------------------------------

func TestSQLJobStoreCreateGet(t *testing.T) {
	s := testSQLiteStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	job := Job{
		ID:        "j1",
		BatchID:   "b1",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: now,
		MinLevel:  "WARNING",
	}
	created, err := s.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != job.ID {
		t.Fatalf("Create returned wrong id: %q", created.ID)
	}

	got, ok := s.Get(job.ID)
	if !ok {
		t.Fatal("Get: job not found")
	}
	if got.Domain != "example.com" {
		t.Fatalf("Domain: got %q", got.Domain)
	}
	if got.BatchID != "b1" {
		t.Fatalf("BatchID: got %q", got.BatchID)
	}
	if got.Status != JobQueued {
		t.Fatalf("Status: got %q", got.Status)
	}
	if got.MinLevel != "WARNING" {
		t.Fatalf("MinLevel: got %q", got.MinLevel)
	}
	if !got.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt: got %v, want %v", got.CreatedAt, now)
	}
	// SeverityTotals must be zero before SetResult.
	for _, lv := range []string{"NOTICE", "WARNING", "ERROR", "CRITICAL"} {
		if got.SeverityTotals[lv] != 0 {
			t.Errorf("SeverityTotals[%q] = %d, want 0", lv, got.SeverityTotals[lv])
		}
	}
}

func TestSQLJobStoreCreateDuplicate(t *testing.T) {
	s := testSQLiteStore(t)
	job := Job{ID: "dup", Domain: "x.com", Status: JobQueued, CreatedAt: time.Now().UTC()}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := s.Create(job); err == nil {
		t.Fatal("expected error on duplicate Create")
	}
}

func TestSQLJobStoreGetMissing(t *testing.T) {
	s := testSQLiteStore(t)
	_, ok := s.Get("does-not-exist")
	if ok {
		t.Fatal("expected ok=false for missing job")
	}
}

// ---- Update ----------------------------------------------------------------

func TestSQLJobStoreUpdate(t *testing.T) {
	s := testSQLiteStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	job := Job{ID: "j2", Domain: "update.test", Status: JobQueued, CreatedAt: now}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	started := now.Add(time.Second)
	job.Status = JobRunning
	job.StartedAt = started
	job.Progress = 50
	if err := s.Update(job); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, ok := s.Get(job.ID)
	if !ok {
		t.Fatal("Get after Update: not found")
	}
	if got.Status != JobRunning {
		t.Fatalf("Status: got %q, want running", got.Status)
	}
	if got.Progress != 50 {
		t.Fatalf("Progress: got %d", got.Progress)
	}
	if !got.StartedAt.Equal(started) {
		t.Fatalf("StartedAt: got %v, want %v", got.StartedAt, started)
	}
}

func TestSQLJobStoreUpdateMissing(t *testing.T) {
	s := testSQLiteStore(t)
	err := s.Update(Job{ID: "ghost", Status: JobRunning, CreatedAt: time.Now().UTC()})
	if err == nil {
		t.Fatal("expected error updating missing job")
	}
}

// ---- SetResult / GetResult -------------------------------------------------

func TestSQLJobStoreSetGetResult(t *testing.T) {
	s := testSQLiteStore(t)
	job := Job{ID: "j3", Domain: "result.test", Status: JobSucceeded, CreatedAt: time.Now().UTC()}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	result := JobResult{
		JobID:   "j3",
		BatchID: "b2",
		Status:  JobSucceeded,
		Summary: map[string]any{
			"levels": map[string]int{"NOTICE": 1, "WARNING": 2, "ERROR": 3},
		},
	}
	if err := s.SetResult("j3", result); err != nil {
		t.Fatalf("SetResult: %v", err)
	}

	got, ok := s.GetResult("j3")
	if !ok {
		t.Fatal("GetResult: not found")
	}
	if got.JobID != "j3" || got.BatchID != "b2" || got.Status != JobSucceeded {
		t.Fatalf("GetResult fields: %+v", got)
	}
}

func TestSQLJobStoreSetResultUpdatesSeverityTotals(t *testing.T) {
	s := testSQLiteStore(t)
	job := Job{ID: "j4", Domain: "sev.test", Status: JobSucceeded, CreatedAt: time.Now().UTC()}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.SetResult("j4", JobResult{
		JobID:  "j4",
		Status: JobSucceeded,
		Summary: map[string]any{
			"levels": map[string]int{"NOTICE": 1, "WARNING": 2, "ERROR": 3, "CRITICAL": 4},
		},
	}); err != nil {
		t.Fatalf("SetResult: %v", err)
	}

	list := s.List(JobFilter{Limit: 10})
	if len(list.Items) != 1 {
		t.Fatalf("List: expected 1 item, got %d", len(list.Items))
	}
	tot := list.Items[0].SeverityTotals
	if tot["NOTICE"] != 1 || tot["WARNING"] != 2 || tot["ERROR"] != 3 || tot["CRITICAL"] != 4 {
		t.Fatalf("unexpected severity_totals: %+v", tot)
	}
}

func TestSQLJobStoreSetResultWithRaw(t *testing.T) {
	s := testSQLiteStore(t)
	job := Job{ID: "j5", Domain: "raw.test", Status: JobSucceeded, CreatedAt: time.Now().UTC()}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	raw := &JobResultRaw{
		Locale: "en",
		Entries: []JobResultEntry{
			{Module: "DNSSEC", Testcase: "DNSSEC01", Tag: "NO_NSEC", Level: "WARNING"},
		},
	}
	if err := s.SetResult("j5", JobResult{JobID: "j5", Status: JobSucceeded, Raw: raw}); err != nil {
		t.Fatalf("SetResult with raw: %v", err)
	}

	got, ok := s.GetResult("j5")
	if !ok {
		t.Fatal("GetResult: not found")
	}
	if got.Raw == nil {
		t.Fatal("Raw is nil")
	}
	if got.Raw.Locale != "en" {
		t.Fatalf("Locale: got %q", got.Raw.Locale)
	}
	if len(got.Raw.Entries) != 1 || got.Raw.Entries[0].Module != "DNSSEC" {
		t.Fatalf("Entries: %+v", got.Raw.Entries)
	}
}

func TestSQLJobStoreSetResultMissingJob(t *testing.T) {
	s := testSQLiteStore(t)
	err := s.SetResult("ghost", JobResult{JobID: "ghost", Status: JobFailed})
	if err == nil {
		t.Fatal("expected error for missing job")
	}
}

func TestSQLJobStoreGetResultMissing(t *testing.T) {
	s := testSQLiteStore(t)
	_, ok := s.GetResult("no-result")
	if ok {
		t.Fatal("expected ok=false for missing result")
	}
}

// ---- List filters ----------------------------------------------------------

func TestSQLJobStoreListFilters(t *testing.T) {
	s := testSQLiteStore(t)
	base := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)

	for _, j := range []Job{
		{ID: "f1", BatchID: "batch1", Domain: "alpha.example.com", Status: JobQueued, CreatedAt: base},
		{ID: "f2", BatchID: "batch2", Domain: "beta.example.com", Status: JobRunning, CreatedAt: base.Add(time.Second)},
		{ID: "f3", BatchID: "batch2", Domain: "gamma.example.org", Status: JobSucceeded, CreatedAt: base.Add(2 * time.Second)},
	} {
		if _, err := s.Create(j); err != nil {
			t.Fatalf("Create %q: %v", j.ID, err)
		}
	}

	t.Run("status", func(t *testing.T) {
		list := s.List(JobFilter{Status: JobRunning, Limit: 10})
		if list.Total != 1 || list.Items[0].ID != "f2" {
			t.Fatalf("status filter: total=%d items=%v", list.Total, ids(list.Items))
		}
	})

	t.Run("batch_id", func(t *testing.T) {
		list := s.List(JobFilter{BatchID: "batch2", Limit: 10})
		if list.Total != 2 {
			t.Fatalf("batch_id filter: total=%d", list.Total)
		}
	})

	t.Run("domain substring", func(t *testing.T) {
		list := s.List(JobFilter{Domain: "alpha", Limit: 10})
		if list.Total != 1 || list.Items[0].ID != "f1" {
			t.Fatalf("domain filter: total=%d items=%v", list.Total, ids(list.Items))
		}
	})

	t.Run("domain case insensitive", func(t *testing.T) {
		list := s.List(JobFilter{Domain: "ALPHA", Limit: 10})
		if list.Total != 1 {
			t.Fatalf("domain case-insensitive filter: total=%d", list.Total)
		}
	})

	t.Run("created_after", func(t *testing.T) {
		list := s.List(JobFilter{CreatedAfter: base.Add(500 * time.Millisecond), Limit: 10})
		if list.Total != 2 {
			t.Fatalf("created_after: total=%d", list.Total)
		}
	})

	t.Run("created_before", func(t *testing.T) {
		list := s.List(JobFilter{CreatedBefore: base.Add(500 * time.Millisecond), Limit: 10})
		if list.Total != 1 || list.Items[0].ID != "f1" {
			t.Fatalf("created_before: total=%d items=%v", list.Total, ids(list.Items))
		}
	})
}

// ---- List severity filters -------------------------------------------------

func TestSQLJobStoreListSeverityFilter(t *testing.T) {
	s := testSQLiteStore(t)
	base := time.Now().UTC().Truncate(time.Millisecond)

	for _, id := range []string{"s1", "s2", "s3"} {
		if _, err := s.Create(Job{ID: id, Domain: id + ".test", Status: JobSucceeded, CreatedAt: base}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	_ = s.SetResult("s1", JobResult{
		JobID: "s1", Status: JobSucceeded,
		Summary: map[string]any{"levels": map[string]int{"WARNING": 1}},
	})
	_ = s.SetResult("s2", JobResult{
		JobID: "s2", Status: JobSucceeded,
		Summary: map[string]any{"levels": map[string]int{"CRITICAL": 1}},
	})
	// s3 has no result — sev_* all zero.

	t.Run("warnings_plus", func(t *testing.T) {
		list := s.List(JobFilter{Severity: JobSeverityWarningsPlus, Limit: 10})
		if list.Total != 2 {
			t.Fatalf("warnings_plus: total=%d", list.Total)
		}
	})

	t.Run("errors_only", func(t *testing.T) {
		list := s.List(JobFilter{Severity: JobSeverityErrorsOnly, Limit: 10})
		if list.Total != 1 || list.Items[0].ID != "s2" {
			t.Fatalf("errors_only: total=%d items=%v", list.Total, ids(list.Items))
		}
	})
}

// ---- List sorting ----------------------------------------------------------

func TestSQLJobStoreListSorting(t *testing.T) {
	s := testSQLiteStore(t)
	base := time.Now().UTC().Truncate(time.Millisecond)

	for _, j := range []Job{
		{ID: "sort1", BatchID: "batch_b", Domain: "zeta.example", Status: JobQueued,
			CreatedAt: base, StartedAt: base.Add(2 * time.Second)},
		{ID: "sort2", BatchID: "batch_a", Domain: "alpha.example", Status: JobQueued,
			CreatedAt: base.Add(time.Second), StartedAt: base.Add(3 * time.Second)},
		{ID: "sort3", BatchID: "batch_c", Domain: "beta.example", Status: JobQueued,
			CreatedAt: base.Add(4 * time.Second)},
	} {
		if _, err := s.Create(j); err != nil {
			t.Fatalf("Create %q: %v", j.ID, err)
		}
	}
	_ = s.SetResult("sort1", JobResult{
		JobID: "sort1", Status: JobSucceeded,
		Summary: map[string]any{"levels": map[string]int{"ERROR": 1}},
	})
	_ = s.SetResult("sort2", JobResult{
		JobID: "sort2", Status: JobFailed,
		Summary: map[string]any{"levels": map[string]int{"CRITICAL": 2}},
	})

	t.Run("default started_at_desc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10})
		// sort3 has no started_at so COALESCE gives created_at = base+4s, which is latest.
		if list.Items[0].ID != "sort3" {
			t.Fatalf("default sort: first=%q, want sort3", list.Items[0].ID)
		}
	})

	t.Run("created_at_asc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortCreatedAtAsc})
		if list.Items[0].ID != "sort1" {
			t.Fatalf("created_at_asc: first=%q", list.Items[0].ID)
		}
	})

	t.Run("created_at_desc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortCreatedAtDesc})
		if list.Items[0].ID != "sort3" {
			t.Fatalf("created_at_desc: first=%q", list.Items[0].ID)
		}
	})

	t.Run("domain_asc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortDomainAsc})
		if list.Items[0].ID != "sort2" {
			t.Fatalf("domain_asc: first=%q (want sort2/alpha)", list.Items[0].ID)
		}
	})

	t.Run("domain_desc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortDomainDesc})
		if list.Items[0].ID != "sort1" {
			t.Fatalf("domain_desc: first=%q (want sort1/zeta)", list.Items[0].ID)
		}
	})

	t.Run("batch_id_asc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortBatchIDAsc})
		if list.Items[0].ID != "sort2" {
			t.Fatalf("batch_id_asc: first=%q (want sort2/batch_a)", list.Items[0].ID)
		}
	})

	t.Run("batch_id_desc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortBatchIDDesc})
		if list.Items[0].ID != "sort3" {
			t.Fatalf("batch_id_desc: first=%q (want sort3/batch_c)", list.Items[0].ID)
		}
	})

	t.Run("started_at_asc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortStartedAtAsc})
		// sort1 has the earliest effective start time.
		if list.Items[0].ID != "sort1" {
			t.Fatalf("started_at_asc: first=%q (want sort1)", list.Items[0].ID)
		}
	})

	t.Run("error_desc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortErrorDesc})
		// sort2 has CRITICAL=2 (total err+crit=2), sort1 has ERROR=1 (total=1).
		if list.Items[0].ID != "sort2" || list.Items[1].ID != "sort1" {
			t.Fatalf("error_desc: got %v (want sort2, sort1)", ids(list.Items))
		}
	})

	t.Run("critical_desc", func(t *testing.T) {
		list := s.List(JobFilter{Limit: 10, Sort: JobSortCriticalDesc})
		if list.Items[0].ID != "sort2" {
			t.Fatalf("critical_desc: first=%q (want sort2/critical=2)", list.Items[0].ID)
		}
	})
}

// ---- Pagination ------------------------------------------------------------

func TestSQLJobStoreListPagination(t *testing.T) {
	s := testSQLiteStore(t)
	base := time.Now().UTC().Truncate(time.Millisecond)

	for i := 0; i < 5; i++ {
		id := string(rune('1'+i)) // "1".."5"
		_, err := s.Create(Job{
			ID: "p" + id, Domain: "p" + id + ".test", Status: JobQueued,
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("Create p%s: %v", id, err)
		}
	}

	first := s.List(JobFilter{Limit: 2, Sort: JobSortCreatedAtAsc})
	if first.Total != 5 {
		t.Fatalf("total: got %d, want 5", first.Total)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "p1" {
		t.Fatalf("first page: got %v", ids(first.Items))
	}
	if first.NextCursor != "2" || first.PrevCursor != "" {
		t.Fatalf("first page cursors: next=%q prev=%q", first.NextCursor, first.PrevCursor)
	}

	second := s.List(JobFilter{Limit: 2, Sort: JobSortCreatedAtAsc, Offset: 2})
	if len(second.Items) != 2 || second.Items[0].ID != "p3" {
		t.Fatalf("second page: got %v", ids(second.Items))
	}
	if second.PrevCursor != "0" || second.NextCursor != "4" {
		t.Fatalf("second page cursors: next=%q prev=%q", second.NextCursor, second.PrevCursor)
	}

	last := s.List(JobFilter{Limit: 2, Sort: JobSortCreatedAtAsc, Offset: 4})
	if len(last.Items) != 1 || last.Items[0].ID != "p5" {
		t.Fatalf("last page: got %v", ids(last.Items))
	}
	if last.NextCursor != "" {
		t.Fatalf("last page: unexpected NextCursor %q", last.NextCursor)
	}
}

func TestSQLJobStoreListDefaultLimit(t *testing.T) {
	s := testSQLiteStore(t)
	list := s.List(JobFilter{})
	if list.Limit != 100 {
		t.Fatalf("default limit: got %d, want 100", list.Limit)
	}
}

// ---- Nullable timestamps & complex fields ----------------------------------

func TestSQLJobStoreNullTimestamps(t *testing.T) {
	s := testSQLiteStore(t)
	job := Job{ID: "ts-null", Domain: "ts.test", Status: JobQueued, CreatedAt: time.Now().UTC()}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, ok := s.Get("ts-null")
	if !ok {
		t.Fatal("Get: not found")
	}
	if !got.StartedAt.IsZero() {
		t.Fatalf("StartedAt should be zero, got %v", got.StartedAt)
	}
	if !got.FinishedAt.IsZero() {
		t.Fatalf("FinishedAt should be zero, got %v", got.FinishedAt)
	}
}

func TestSQLJobStoreRoundTripComplexFields(t *testing.T) {
	s := testSQLiteStore(t)
	job := Job{
		ID:        "complex",
		Domain:    "delegated.test",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
		Tests:     []string{"DNSSEC", "Zone"},
		Overrides: map[string]any{"key": "value", "num": float64(42)},
		MinLevel:  "NOTICE",
	}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, ok := s.Get("complex")
	if !ok {
		t.Fatal("Get: not found")
	}
	if len(got.Tests) != 2 || got.Tests[0] != "DNSSEC" {
		t.Fatalf("Tests: got %v", got.Tests)
	}
	if got.MinLevel != "NOTICE" {
		t.Fatalf("MinLevel: got %q", got.MinLevel)
	}
	if got.Overrides["key"] != "value" {
		t.Fatalf("Overrides: got %v", got.Overrides)
	}
}

// ---- Recovery --------------------------------------------------------------

func TestRecoverJobsRunningToFailed(t *testing.T) {
	s := testSQLiteStore(t)
	base := time.Now().UTC()

	for _, job := range []Job{
		{ID: "r1", Domain: "r1.test", Status: JobRunning, CreatedAt: base},
		{ID: "q1", Domain: "q1.test", Status: JobQueued, CreatedAt: base.Add(time.Second)},
	} {
		if _, err := s.Create(job); err != nil {
			t.Fatalf("Create %q: %v", job.ID, err)
		}
	}

	q := NewInMemoryQueue()
	if err := RecoverJobs(s, q); err != nil {
		t.Fatalf("RecoverJobs: %v", err)
	}

	got, ok := s.Get("r1")
	if !ok {
		t.Fatal("r1 not found after recovery")
	}
	if got.Status != JobFailed {
		t.Fatalf("r1 status: got %q, want failed", got.Status)
	}
	if !strings.Contains(got.Error, "restarted") {
		t.Fatalf("r1 error should mention restart, got %q", got.Error)
	}
}

func TestRecoverJobsQueuedReenqueued(t *testing.T) {
	s := testSQLiteStore(t)
	base := time.Now().UTC()

	for _, job := range []Job{
		{ID: "q1", Domain: "q1.test", Status: JobQueued, CreatedAt: base},
		{ID: "q2", Domain: "q2.test", Status: JobQueued, CreatedAt: base.Add(time.Second)},
		{ID: "done", Domain: "done.test", Status: JobSucceeded, CreatedAt: base},
	} {
		if _, err := s.Create(job); err != nil {
			t.Fatalf("Create %q: %v", job.ID, err)
		}
	}

	q := NewInMemoryQueue()
	if err := RecoverJobs(s, q); err != nil {
		t.Fatalf("RecoverJobs: %v", err)
	}

	// Queued jobs must have been enqueued in created_at ASC order.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	id1, err := q.Dequeue(ctx)
	if err != nil || id1 != "q1" {
		t.Fatalf("dequeue 1: id=%q err=%v (want q1)", id1, err)
	}
	id2, err := q.Dequeue(ctx)
	if err != nil || id2 != "q2" {
		t.Fatalf("dequeue 2: id=%q err=%v (want q2)", id2, err)
	}

	// Only two items must have been enqueued (not "done").
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	extra, err := q.Dequeue(ctx2)
	if err == nil {
		t.Fatalf("unexpected third item in queue: %q", extra)
	}
}

func TestRecoverJobsNoopInMemory(t *testing.T) {
	store := NewInMemoryJobStore()
	queue := NewInMemoryQueue()
	if err := RecoverJobs(store, queue); err != nil {
		t.Fatalf("RecoverJobs on in-memory store: %v", err)
	}
}

// ---- NewWithOptions --------------------------------------------------------

func TestNewWithOptionsInMemory(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewWithOptions(cfg)
	if err != nil {
		t.Fatalf("NewWithOptions (in-memory): %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNewWithOptionsInvalidDriver(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.Driver = "notadriver"
	cfg.Database.DSN = "whatever"
	_, err := NewWithOptions(cfg)
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}
